package provisioner

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProvisioner_Bootstrap(t *testing.T) {
	t.Run("NilBlueprintReturnsError", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		if _, err := provisioner.Bootstrap(context.Background(), nil); err == nil {
			t.Fatal("Expected error for nil blueprint, got nil")
		}
	})

	t.Run("KubernetesBackendWithoutBlueprintBackendCollapsesToUp", func(t *testing.T) {
		// terraform.backend.type=kubernetes with no Blueprint.Backend and a present
		// kubeconfig (setupProvisionerMocks seeds one by default) is the "cluster
		// already provisioned" case — len(backendComponents) == 0 short-circuits straight to a
		// plain apply, no local-pin/migrate dance and no fail-fast guard.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "workloads/argocd"},
			},
		}
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		var ops []string
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			ops = append(ops, "up")
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if _, err := provisioner.Bootstrap(context.Background(), bp); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		// Must be a plain Up — no backend pinning, no migrate dance.
		if len(ops) != 1 || ops[0] != "up" {
			t.Errorf("Expected ops=[up], got %v", ops)
		}
	})

	t.Run("RefusesKubernetesBackendWithNoBackendWhenClusterNotYetCreated", func(t *testing.T) {
		// terraform.backend.type=kubernetes, no Blueprint.Backend, and no kubeconfig for
		// this context is the from-zero case: the blueprint gives no backend
		// boundary to pivot on, so applying anything would dial a cluster that cannot
		// exist yet. Refuse up front with an actionable error instead of letting
		// terraform's raw connection-refused surface.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/talos"},
			},
		}
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockStack := terraforminfra.NewMockStack()
		upCalled := false
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			upCalled = true
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})
		provisioner.configRoot = t.TempDir() // no .kube/config — cluster not yet created

		if _, err := provisioner.Bootstrap(context.Background(), bp); err == nil {
			t.Fatal("Expected error, got nil")
		} else if !strings.Contains(err.Error(), "no kubeconfig") || !strings.Contains(err.Error(), "backend:") {
			t.Errorf("Expected error to explain the missing backend declaration, got: %v", err)
		}
		if upCalled {
			t.Error("TerraformStack.Up must not run when the guard refuses")
		}
	})

	t.Run("EmptyBlueprintOnKubernetesBackendDoesNotTripTheGuard", func(t *testing.T) {
		// No terraform components at all means there is nothing to apply against the
		// cluster in the first place — the from-zero guard must not refuse a legitimate
		// no-op just because the backend happens to be kubernetes.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{}
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockStack := terraforminfra.NewMockStack()
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})
		provisioner.configRoot = t.TempDir() // no kubeconfig

		if _, err := provisioner.Bootstrap(context.Background(), bp); err != nil {
			t.Fatalf("Expected no error for an empty blueprint, got %v", err)
		}
	})

	t.Run("LocalBackendCollapsesToUp", func(t *testing.T) {
		// When terraform.backend.type is "local", Bootstrap forwards to Up
		// without any pivot — there is no remote backend to migrate state to.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "backend",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		var ops []string
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			ops = append(ops, "up")
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Bootstrap(context.Background(), bp)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expected := []string{"up"}
		if len(ops) != len(expected) || ops[0] != expected[0] {
			t.Errorf("Expected %v, got %v", expected, ops)
		}
	})

	t.Run("NoBackendFieldCollapsesToUp", func(t *testing.T) {
		// Without Blueprint.Backend set, the blueprint declares no backend components.
		// Bootstrap forwards to Up — every component uses the configured backend.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "cluster"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "s3"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		var ops []string
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			ops = append(ops, "up")
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Bootstrap(context.Background(), bp)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expected := []string{"up"}
		if len(ops) != len(expected) || ops[0] != expected[0] {
			t.Errorf("Expected %v, got %v", expected, ops)
		}
	})

	t.Run("SingleBackendComponentRunsExpectedOpSequence", func(t *testing.T) {
		// Backend names a single component → backend components = [backend].
		// Stage 1: set:local → migrate(backend) → up(backend) → set:configured.
		// Stage 2: migrate(backend) — push state up.
		// Stage 3: up(non-backend).
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "backend",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
				{Path: "gitops"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "azurerm"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		var ops []string
		var upBlueprints []*blueprintv1alpha1.Blueprint
		var migrateBlueprints []*blueprintv1alpha1.Blueprint
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(b *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			ops = append(ops, "up")
			upBlueprints = append(upBlueprints, b)
			return false, nil
		}
		mockStack.MigrateStateFunc = func(b *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			migrateBlueprints = append(migrateBlueprints, b)
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Bootstrap(context.Background(), bp)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := []string{"set:local", "migrate", "up", "set:azurerm", "migrate", "up"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected %d ops %v, got %d %v", len(expected), expected, len(ops), ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}

		if len(upBlueprints) != 2 {
			t.Fatalf("Expected 2 Up calls, got %d", len(upBlueprints))
		}
		stage1Up := upBlueprints[0].TerraformComponents
		if len(stage1Up) != 1 || stage1Up[0].Path != "backend" {
			t.Errorf("Stage 1 Up should run the backend [backend], got %#v", stage1Up)
		}
		stage3Up := upBlueprints[1].TerraformComponents
		if len(stage3Up) != 2 || stage3Up[0].Path != "cluster" || stage3Up[1].Path != "gitops" {
			t.Errorf("Stage 3 Up should run non-backend [cluster, gitops], got %#v", stage3Up)
		}

		if len(migrateBlueprints) != 2 {
			t.Fatalf("Expected 2 MigrateState calls (Stage 1 pull + Stage 2 push), got %d", len(migrateBlueprints))
		}
		for i, m := range migrateBlueprints {
			if len(m.TerraformComponents) != 1 || m.TerraformComponents[0].Path != "backend" {
				t.Errorf("MigrateState call %d should target the backend components only, got %#v", i, m.TerraformComponents)
			}
		}
	})

	t.Run("MultiComponentBackendAllAppliedTogether", func(t *testing.T) {
		// Backend names the last component of a multi-component backend (vpc, iam, cluster).
		// Stage 1 Up receives the whole backend component set [vpc, iam, cluster]; Stage 3 Up receives only
		// workloads. Both MigrateState calls operate on the full backend component set.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "cluster",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "networking/vpc"},
				{Path: "iam"},
				{Name: "cluster", Path: "cluster/eks"},
				{Path: "workloads/argocd"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "s3"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		var ops []string
		var upBlueprints []*blueprintv1alpha1.Blueprint
		var migrateBlueprints []*blueprintv1alpha1.Blueprint
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(b *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			ops = append(ops, "up")
			upBlueprints = append(upBlueprints, b)
			return false, nil
		}
		mockStack.MigrateStateFunc = func(b *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			migrateBlueprints = append(migrateBlueprints, b)
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Bootstrap(context.Background(), bp)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := []string{"set:local", "migrate", "up", "set:s3", "migrate", "up"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}

		stage1Up := upBlueprints[0].TerraformComponents
		if len(stage1Up) != 3 {
			t.Fatalf("Stage 1 Up should run 3 backend components, got %d: %#v", len(stage1Up), stage1Up)
		}
		expectedBackendComponents := []string{"networking/vpc", "iam", "cluster"}
		for i, want := range expectedBackendComponents {
			if stage1Up[i].GetID() != want {
				t.Errorf("Stage 1 backend[%d]: got %q, want %q", i, stage1Up[i].GetID(), want)
			}
		}
		stage3Up := upBlueprints[1].TerraformComponents
		if len(stage3Up) != 1 || stage3Up[0].Path != "workloads/argocd" {
			t.Errorf("Stage 3 Up should run [workloads/argocd], got %#v", stage3Up)
		}

		for i, m := range migrateBlueprints {
			if len(m.TerraformComponents) != 3 {
				t.Errorf("MigrateState call %d should target the 3-component backend, got %d components", i, len(m.TerraformComponents))
			}
		}
	})

	t.Run("BackendOnlyBlueprintSkipsStage3", func(t *testing.T) {
		// When every component is part of the backend, Stage 3 has nothing to apply.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "backend",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "s3"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		var ops []string
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			ops = append(ops, "up")
			return false, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Bootstrap(context.Background(), bp)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := []string{"set:local", "migrate", "up", "set:s3", "migrate"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
	})

	t.Run("UpFailureInStage1RestoresBackend", func(t *testing.T) {
		// If Stage 1 Up fails, the deferred backend restore in withBackendOverride
		// must still fire so the configured backend is reset before Bootstrap returns.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "backend",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "azurerm"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		var ops []string
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			ops = append(ops, "up-fail")
			return false, fmt.Errorf("apply blew up")
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Bootstrap(context.Background(), bp)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		// Restore (set:azurerm) must appear after the failure.
		var sawRestore bool
		for _, op := range ops {
			if op == "set:azurerm" {
				sawRestore = true
			}
		}
		if !sawRestore {
			t.Errorf("Expected backend restore (set:azurerm) after Stage 1 failure, got %v", ops)
		}
	})

	t.Run("Stage2MigrateStateErrorSurfaces", func(t *testing.T) {
		// Stage 2's push-up MigrateState error propagates as Bootstrap's return error.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "backend",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "azurerm"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			return false, nil
		}
		migrateCalls := 0
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			migrateCalls++
			// First call is Stage 1 pull (inside override) — succeeds.
			// Second call is Stage 2 push (after override exit) — fails.
			if migrateCalls == 2 {
				return nil, fmt.Errorf("push failed")
			}
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Bootstrap(context.Background(), bp)
		if err == nil {
			t.Fatal("Expected error from Stage 2 migrate failure, got nil")
		}
		if !strings.Contains(err.Error(), "push failed") {
			t.Errorf("Expected error to wrap Stage 2 failure, got: %v", err)
		}
	})

	t.Run("SkippedBackendComponentAfterApplyIsAnError", func(t *testing.T) {
		// If Stage 2's MigrateState reports a backend component skipped (its directory
		// disappeared between Up and migrate), Bootstrap surfaces the anomaly.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "backend",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "azurerm"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			return false, nil
		}
		calls := 0
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			calls++
			if calls == 2 {
				return []string{"backend"}, nil
			}
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Bootstrap(context.Background(), bp)
		if err == nil {
			t.Fatal("Expected error for skipped-after-apply, got nil")
		}
		if !strings.Contains(err.Error(), "skipped") || !strings.Contains(err.Error(), "backend") {
			t.Errorf("Expected error to name skipped component, got: %v", err)
		}
	})

	t.Run("LocalStateRemovalFailureWarnsButDoesNotAbort", func(t *testing.T) {
		// RemoveLocalState failure after a successful Stage 2 push is non-fatal —
		// the orphaned local file is a cleanup nit, not a state-integrity problem.
		// Bootstrap warns to stderr and proceeds to Stage 3.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "backend",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "azurerm"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		var ops []string
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			ops = append(ops, "up")
			return false, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		mockStack.RemoveLocalStateFunc = func(componentID string) error {
			ops = append(ops, fmt.Sprintf("remove-local:%s", componentID))
			return fmt.Errorf("permission denied")
		}

		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})
		var err error
		stderrOutput := captureStderr(t, func() {
			_, err = provisioner.Bootstrap(context.Background(), bp)
		})

		if err != nil {
			t.Fatalf("Expected no error (cleanup failure is warning-only), got %v", err)
		}
		if !strings.Contains(stderrOutput, "permission denied") {
			t.Errorf("Expected stderr warning to include underlying cause, got: %q", stderrOutput)
		}
		// Stage 3's Up must still run after the cleanup warning.
		var sawSecondUp bool
		upCount := 0
		for _, op := range ops {
			if op == "up" {
				upCount++
			}
		}
		if upCount >= 2 {
			sawSecondUp = true
		}
		if !sawSecondUp {
			t.Errorf("Expected Stage 3 Up after cleanup warning, got %v", ops)
		}
	})
}

func TestProvisioner_Up_BackendPivot(t *testing.T) {
	t.Run("PivotsDeclaredBackendWithoutBootstrap", func(t *testing.T) {
		// windsor up, called directly on a from-zero context whose blueprint declares
		// a declared backend, must run the same local-pin/migrate/apply sequence Bootstrap
		// runs — up no longer needs bootstrap called first.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "cluster",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Path: "cluster/talos"},
				{Path: "gitops/flux"},
			},
		}

		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		var ops []string
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				ops = append(ops, fmt.Sprintf("set:%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			ops = append(ops, "up")
			return false, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})
		provisioner.configRoot = t.TempDir() // no kubeconfig — genuinely from zero

		if _, err := provisioner.Up(context.Background(), bp); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := []string{"set:local", "migrate", "up", "set:kubernetes", "migrate", "up"}
		if len(ops) != len(expected) {
			t.Fatalf("Expected %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
	})

	t.Run("NoBackendDeclaredStillAppliesDirectlyOnceClusterExists", func(t *testing.T) {
		// Steady state: kubeconfig present (already bootstrapped), no declared backend —
		// Up applies directly against the configured backend, same as before this fix.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "gitops/flux"},
			},
		}
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockStack := terraforminfra.NewMockStack()
		upCalled := false
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			upCalled = true
			return false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})
		// setupProvisionerMocks seeds a kubeconfig by default — cluster already exists.

		if _, err := provisioner.Up(context.Background(), bp); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !upCalled {
			t.Error("Expected TerraformStack.Up to run directly")
		}
	})

	t.Run("DetectsOrphanedBackendStateDespiteTheLocalPivot", func(t *testing.T) {
		// windsorcli/cli#3367: a backend component (e.g. "backend") round-trips
		// through a local-backend override on every apply. Before this fix, the only
		// orphan check ran inside recoverHalfMigratedComponents. That check bails out
		// whenever terraform.backend.type is "local", which is true for the whole
		// pivot. So a renamed backend component's stranded local state was
		// invisible on exactly the runs most likely to produce it. The check now runs
		// once, up front, against the real configured backend. The pivot's temporary
		// override no longer affects it.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "backend",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "backend", Path: "backend"},
			},
		}
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "s3"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.ListLocalStateComponentIDsFunc = func() ([]string, error) {
			return []string{"backend-old"}, nil
		}
		mockStack.HasLocalStateWithResourcesFunc = func(componentID string) (bool, error) {
			return componentID == "backend-old", nil
		}
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			return false, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})
		provisioner.configRoot = t.TempDir()

		var err error
		stderrOutput := captureStderr(t, func() {
			_, err = provisioner.Up(context.Background(), bp)
		})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !strings.Contains(stderrOutput, "backend-old") {
			t.Errorf("Expected a warning naming the orphaned backend componentID, got: %q", stderrOutput)
		}
	})
}

func TestBuildBootstrapSummary(t *testing.T) {
	t.Run("PopulatesContextAndBackend", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{}
		s := BuildBootstrapSummary(bp, "staging", "s3")
		if s.ContextName != "staging" {
			t.Errorf("Expected ContextName=staging, got %q", s.ContextName)
		}
		if s.BackendType != "s3" {
			t.Errorf("Expected BackendType=s3, got %q", s.BackendType)
		}
	})

	t.Run("EmptyBackendDefaultsToLocal", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{}
		s := BuildBootstrapSummary(bp, "local", "")
		if s.BackendType != "local" {
			t.Errorf("Expected BackendType=local when input empty, got %q", s.BackendType)
		}
	})

	t.Run("ListsEnabledTerraformComponentsAndKustomizations", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "argocd"},
				{Name: "monitoring"},
			},
		}
		s := BuildBootstrapSummary(bp, "local", "s3")
		if len(s.Terraform) != 2 || s.Terraform[0].ComponentID != "backend" || s.Terraform[1].ComponentID != "cluster" {
			t.Errorf("Expected terraform [backend, cluster], got %#v", s.Terraform)
		}
		if len(s.Kustomize) != 2 || s.Kustomize[0] != "argocd" || s.Kustomize[1] != "monitoring" {
			t.Errorf("Expected kustomize [argocd, monitoring], got %#v", s.Kustomize)
		}
	})

	t.Run("IncludesSynthesizedCrdLayers", func(t *testing.T) {
		// Given a blueprint with local CRDs and an install source that vendors CRDs — the same layers
		// `windsor plan` materializes via withCrdLayer
		installed := true
		bp := &blueprintv1alpha1.Blueprint{
			Crds: []string{"cert-manager-1.16.2"},
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Install: &blueprintv1alpha1.BoolExpression{Value: &installed}, Crds: []string{"gateway-api-1.5.1"}},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "argocd"}},
		}

		s := BuildBootstrapSummary(bp, "local", "s3")

		// Then the bootstrap plan lists the synthesized CRD kustomizations ahead of the stack, matching
		// what `windsor plan` shows — operators see every CRD layer before confirming
		if !slices.Contains(s.Kustomize, "crds") {
			t.Errorf("expected synthesized 'crds' layer in bootstrap summary, got %#v", s.Kustomize)
		}
		if !slices.Contains(s.Kustomize, "crds-core") {
			t.Errorf("expected synthesized 'crds-core' layer in bootstrap summary, got %#v", s.Kustomize)
		}
		if !slices.Contains(s.Kustomize, "argocd") {
			t.Errorf("expected the regular kustomization retained, got %#v", s.Kustomize)
		}
	})

	t.Run("OmitsDisabledTerraformComponents", func(t *testing.T) {
		falseValue := false
		disabled := &blueprintv1alpha1.BoolExpression{Value: &falseValue}
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "skipped", Enabled: disabled},
				{Path: "cluster"},
			},
		}
		s := BuildBootstrapSummary(bp, "local", "local")
		if len(s.Terraform) != 2 {
			t.Fatalf("Expected 2 enabled components, got %d (%#v)", len(s.Terraform), s.Terraform)
		}
		for _, e := range s.Terraform {
			if e.ComponentID == "skipped" {
				t.Errorf("Disabled component leaked into summary: %#v", s.Terraform)
			}
		}
	})
}
