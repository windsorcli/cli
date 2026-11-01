package provisioner_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/provisioner"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProvisioner_Bootstrap(t *testing.T) {
	t.Run("NilBlueprintReturnsError", func(t *testing.T) {
		// Given a provisioner and a nil blueprint
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		// When Bootstrap runs
		// Then an error is returned
		if _, err := prov.Bootstrap(context.Background(), nil); err == nil {
			t.Fatal("Expected error for nil blueprint, got nil")
		}
	})

	t.Run("KubernetesBackendWithoutBlueprintBackendCollapsesToUp", func(t *testing.T) {
		// Given a kubernetes backend, no Blueprint.Backend, and an existing kubeconfig
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
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When Bootstrap runs
		// Then only a plain Up runs with no backend pinning
		if _, err := prov.Bootstrap(context.Background(), bp); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(ops) != 1 || ops[0] != "up" {
			t.Errorf("Expected ops=[up], got %v", ops)
		}
	})

	t.Run("RefusesKubernetesBackendWithNoBackendWhenClusterNotYetCreated", func(t *testing.T) {
		// Given a kubernetes backend, no Blueprint.Backend, and no kubeconfig
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
		mocks.Runtime.ConfigRoot = t.TempDir()
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When Bootstrap runs
		// Then Bootstrap refuses with a backend remediation and Up does not run
		if _, err := prov.Bootstrap(context.Background(), bp); err == nil {
			t.Fatal("Expected error, got nil")
		} else if we := werror.Find(err); !strings.Contains(err.Error(), "no kubeconfig") || we == nil || !strings.Contains(we.Remediation, "backend:") {
			t.Errorf("Expected error to explain the missing backend declaration, got: %v", err)
		}
		if upCalled {
			t.Error("TerraformStack.Up must not run when the guard refuses")
		}
	})

	t.Run("EmptyBlueprintOnKubernetesBackendDoesNotTripTheGuard", func(t *testing.T) {
		// Given an empty blueprint on a kubernetes backend with no kubeconfig
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
		mocks.Runtime.ConfigRoot = t.TempDir()
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When Bootstrap runs
		// Then no error is returned
		if _, err := prov.Bootstrap(context.Background(), bp); err != nil {
			t.Fatalf("Expected no error for an empty blueprint, got %v", err)
		}
	})

	t.Run("LocalBackendCollapsesToUp", func(t *testing.T) {
		// Given a local backend and a blueprint that declares a backend component
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
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When Bootstrap runs
		_, err := prov.Bootstrap(context.Background(), bp)
		// Then only a plain Up runs
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expected := []string{"up"}
		if len(ops) != len(expected) || ops[0] != expected[0] {
			t.Errorf("Expected %v, got %v", expected, ops)
		}
	})

	t.Run("NoBackendFieldCollapsesToUp", func(t *testing.T) {
		// Given an s3 backend and a blueprint with no Backend field
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
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When Bootstrap runs
		_, err := prov.Bootstrap(context.Background(), bp)
		// Then only a plain Up runs
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expected := []string{"up"}
		if len(ops) != len(expected) || ops[0] != expected[0] {
			t.Errorf("Expected %v, got %v", expected, ops)
		}
	})

	t.Run("SingleBackendComponentRunsExpectedOpSequence", func(t *testing.T) {
		// Given an azurerm backend and a blueprint with one backend component
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
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When Bootstrap runs
		_, err := prov.Bootstrap(context.Background(), bp)
		// Then the backend is pinned local, migrated, applied, pushed, then the rest applies
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
		// Given an s3 backend whose Backend names the last of three components
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
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When Bootstrap runs
		_, err := prov.Bootstrap(context.Background(), bp)
		// Then all three backend components apply together before the workloads
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
		// Given a blueprint where every component is part of the backend
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
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When Bootstrap runs
		_, err := prov.Bootstrap(context.Background(), bp)
		// Then no Stage 3 Up runs after the state push
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
		// Given an azurerm backend whose Stage 1 Up fails
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
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When Bootstrap runs
		_, err := prov.Bootstrap(context.Background(), bp)
		// Then an error is returned and the configured backend is restored
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

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
		// Given a MigrateState that fails on the Stage 2 push
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
			if migrateCalls == 2 {
				return nil, fmt.Errorf("push failed")
			}
			return nil, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When Bootstrap runs
		_, err := prov.Bootstrap(context.Background(), bp)
		// Then the push error is returned
		if err == nil {
			t.Fatal("Expected error from Stage 2 migrate failure, got nil")
		}
		if !strings.Contains(err.Error(), "push failed") {
			t.Errorf("Expected error to wrap Stage 2 failure, got: %v", err)
		}
	})

	t.Run("SkippedBackendComponentAfterApplyIsAnError", func(t *testing.T) {
		// Given a Stage 2 MigrateState that reports the backend component skipped
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
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When Bootstrap runs
		_, err := prov.Bootstrap(context.Background(), bp)
		// Then an error names the skipped component
		if err == nil {
			t.Fatal("Expected error for skipped-after-apply, got nil")
		}
		if !strings.Contains(err.Error(), "skipped") || !strings.Contains(err.Error(), "backend") {
			t.Errorf("Expected error to name skipped component, got: %v", err)
		}
	})

	t.Run("LocalStateRemovalFailureWarnsButDoesNotAbort", func(t *testing.T) {
		// Given a RemoveLocalState that fails after a successful Stage 2 push
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

		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})
		// When Bootstrap runs
		var err error
		warnings := captureWarnings(t, func(ctx context.Context) {
			_, err = prov.Bootstrap(ctx, bp)
		})

		// Then Bootstrap warns with the cause and still runs Stage 3 Up
		if err != nil {
			t.Fatalf("Expected no error (cleanup failure is warning-only), got %v", err)
		}
		if !strings.Contains(warnings, "permission denied") {
			t.Errorf("Expected warning to include underlying cause, got: %q", warnings)
		}
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
		// Given a kubernetes backend, a declared backend component, and no kubeconfig
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
		mocks.Runtime.ConfigRoot = t.TempDir()
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When Up runs without Bootstrap
		// Then Up runs the full local pin, migrate, and apply sequence
		if _, err := prov.Up(context.Background(), bp); err != nil {
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
		// Given a kubernetes backend, no declared backend, and an existing kubeconfig
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
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When Up runs
		// Then TerraformStack.Up runs directly
		if _, err := prov.Up(context.Background(), bp); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !upCalled {
			t.Error("Expected TerraformStack.Up to run directly")
		}
	})

	t.Run("DetectsOrphanedBackendStateDespiteTheLocalPivot", func(t *testing.T) {
		// Given an s3 backend and orphaned local state for a renamed backend component
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
		mocks.Runtime.ConfigRoot = t.TempDir()
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When Up runs through the local pivot
		var err error
		warnings := captureWarnings(t, func(ctx context.Context) {
			_, err = prov.Up(ctx, bp)
		})

		// Then a warning names the orphaned component
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !strings.Contains(warnings, "backend-old") {
			t.Errorf("Expected a warning naming the orphaned backend componentID, got: %q", warnings)
		}
	})
}

func TestBuildBootstrapSummary(t *testing.T) {
	t.Run("PopulatesContextAndBackend", func(t *testing.T) {
		// Given an empty blueprint
		bp := &blueprintv1alpha1.Blueprint{}
		// When BuildBootstrapSummary runs with a context and an s3 backend
		s := provisioner.BuildBootstrapSummary(bp, "staging", "s3")
		// Then the summary holds the context and backend
		if s.ContextName != "staging" {
			t.Errorf("Expected ContextName=staging, got %q", s.ContextName)
		}
		if s.BackendType != "s3" {
			t.Errorf("Expected BackendType=s3, got %q", s.BackendType)
		}
	})

	t.Run("EmptyBackendDefaultsToLocal", func(t *testing.T) {
		// Given an empty blueprint
		bp := &blueprintv1alpha1.Blueprint{}
		// When BuildBootstrapSummary runs with an empty backend
		s := provisioner.BuildBootstrapSummary(bp, "local", "")
		// Then the backend type defaults to local
		if s.BackendType != "local" {
			t.Errorf("Expected BackendType=local when input empty, got %q", s.BackendType)
		}
	})

	t.Run("ListsEnabledTerraformComponentsAndKustomizations", func(t *testing.T) {
		// Given a blueprint with two terraform components and two kustomizations
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
		// When BuildBootstrapSummary runs
		s := provisioner.BuildBootstrapSummary(bp, "local", "s3")
		// Then the summary lists all components and kustomizations in order
		if len(s.Terraform) != 2 || s.Terraform[0].ComponentID != "backend" || s.Terraform[1].ComponentID != "cluster" {
			t.Errorf("Expected terraform [backend, cluster], got %#v", s.Terraform)
		}
		if len(s.Kustomize) != 2 || s.Kustomize[0] != "argocd" || s.Kustomize[1] != "monitoring" {
			t.Errorf("Expected kustomize [argocd, monitoring], got %#v", s.Kustomize)
		}
	})

	t.Run("IncludesSynthesizedCrdLayers", func(t *testing.T) {
		// Given a blueprint with local CRDs and an install source that vendors CRDs
		installed := true
		bp := &blueprintv1alpha1.Blueprint{
			Crds: []string{"cert-manager-1.16.2"},
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Install: &blueprintv1alpha1.BoolExpression{Value: &installed}, Crds: []string{"gateway-api-1.5.1"}},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "argocd"}},
		}

		// When BuildBootstrapSummary runs
		s := provisioner.BuildBootstrapSummary(bp, "local", "s3")

		// Then the summary lists the synthesized CRD layers and the regular kustomization
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
		// Given a blueprint with one disabled terraform component
		falseValue := false
		disabled := &blueprintv1alpha1.BoolExpression{Value: &falseValue}
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "skipped", Enabled: disabled},
				{Path: "cluster"},
			},
		}
		// When BuildBootstrapSummary runs
		s := provisioner.BuildBootstrapSummary(bp, "local", "local")
		// Then the disabled component is not listed
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
