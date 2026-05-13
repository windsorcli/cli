package provisioner

import (
	"fmt"
	"io"
	"os"
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

		if _, err := provisioner.Bootstrap(nil, nil); err == nil {
			t.Fatal("Expected error for nil blueprint, got nil")
		}
	})

	t.Run("KubernetesWithoutBackendFieldErrorsBeforeConfirmAndAnyMutation", func(t *testing.T) {
		// A kubernetes-configured backend with no Blueprint.Backend would silently
		// fall through to plain Up against a cluster that does not yet exist. Refuse
		// before the confirmation prompt fires — an operator should not see a full
		// bootstrap summary, confirm intent, and then receive the hard error.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/talos"},
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
		setCalled := false
		mockCH.SetFunc = func(_ string, _ any) error {
			setCalled = true
			return nil
		}
		upCalled := false
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			upCalled = true
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		confirmCalled := false
		confirm := func(_ *BootstrapSummary) bool {
			confirmCalled = true
			return true
		}

		_, err := provisioner.Bootstrap(bp, confirm)
		if err == nil {
			t.Fatal("Expected error for kubernetes backend without Blueprint.Backend, got nil")
		}
		if !strings.Contains(err.Error(), "Blueprint.Backend") {
			t.Errorf("Expected error to name the missing field, got: %v", err)
		}
		if !strings.Contains(err.Error(), "kubernetes") {
			t.Errorf("Expected error to name the backend type, got: %v", err)
		}
		if confirmCalled {
			t.Error("Confirm callback must not run when refusing — fail-fast before the prompt")
		}
		if setCalled {
			t.Error("No backend override may engage when refusing")
		}
		if upCalled {
			t.Error("No apply may run when refusing")
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
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		applied, err := provisioner.Bootstrap(bp, nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !applied {
			t.Fatal("Expected applied=true")
		}
		expected := []string{"up"}
		if len(ops) != len(expected) || ops[0] != expected[0] {
			t.Errorf("Expected %v, got %v", expected, ops)
		}
	})

	t.Run("NoBackendFieldCollapsesToUp", func(t *testing.T) {
		// Without Blueprint.Backend set, the blueprint has no in-blueprint backend
		// tier. Bootstrap forwards to Up — every component uses the configured backend.
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
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		applied, err := provisioner.Bootstrap(bp, nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !applied {
			t.Fatal("Expected applied=true")
		}
		expected := []string{"up"}
		if len(ops) != len(expected) || ops[0] != expected[0] {
			t.Errorf("Expected %v, got %v", expected, ops)
		}
	})

	t.Run("SingleTierComponentRunsExpectedOpSequence", func(t *testing.T) {
		// Backend names a single component → tier = [backend].
		// Stage 1: set:local → migrate(tier) → up(tier) → set:configured.
		// Stage 2: migrate(tier) — push state up.
		// Stage 3: up(non-tier).
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
		mockStack.UpFunc = func(b *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			upBlueprints = append(upBlueprints, b)
			return nil
		}
		mockStack.MigrateStateFunc = func(b *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			migrateBlueprints = append(migrateBlueprints, b)
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		applied, err := provisioner.Bootstrap(bp, nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !applied {
			t.Fatal("Expected applied=true with no confirm callback")
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
			t.Errorf("Stage 1 Up should run tier [backend], got %#v", stage1Up)
		}
		stage3Up := upBlueprints[1].TerraformComponents
		if len(stage3Up) != 2 || stage3Up[0].Path != "cluster" || stage3Up[1].Path != "gitops" {
			t.Errorf("Stage 3 Up should run non-tier [cluster, gitops], got %#v", stage3Up)
		}

		if len(migrateBlueprints) != 2 {
			t.Fatalf("Expected 2 MigrateState calls (Stage 1 pull + Stage 2 push), got %d", len(migrateBlueprints))
		}
		for i, m := range migrateBlueprints {
			if len(m.TerraformComponents) != 1 || m.TerraformComponents[0].Path != "backend" {
				t.Errorf("MigrateState call %d should target tier only, got %#v", i, m.TerraformComponents)
			}
		}
	})

	t.Run("MultiComponentTierAllAppliedTogether", func(t *testing.T) {
		// Backend names the last component of a multi-component tier (vpc, iam, cluster).
		// Stage 1 Up receives the whole tier [vpc, iam, cluster]; Stage 3 Up receives only
		// workloads. Both MigrateState calls operate on the full tier.
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
		mockStack.UpFunc = func(b *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			upBlueprints = append(upBlueprints, b)
			return nil
		}
		mockStack.MigrateStateFunc = func(b *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			migrateBlueprints = append(migrateBlueprints, b)
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		applied, err := provisioner.Bootstrap(bp, nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !applied {
			t.Fatal("Expected applied=true")
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
			t.Fatalf("Stage 1 Up should run 3 tier components, got %d: %#v", len(stage1Up), stage1Up)
		}
		expectedTier := []string{"networking/vpc", "iam", "cluster"}
		for i, want := range expectedTier {
			if stage1Up[i].GetID() != want {
				t.Errorf("Stage 1 tier[%d]: got %q, want %q", i, stage1Up[i].GetID(), want)
			}
		}
		stage3Up := upBlueprints[1].TerraformComponents
		if len(stage3Up) != 1 || stage3Up[0].Path != "workloads/argocd" {
			t.Errorf("Stage 3 Up should run [workloads/argocd], got %#v", stage3Up)
		}

		for i, m := range migrateBlueprints {
			if len(m.TerraformComponents) != 3 {
				t.Errorf("MigrateState call %d should target the 3-component tier, got %d components", i, len(m.TerraformComponents))
			}
		}
	})

	t.Run("TierOnlyBlueprintSkipsStage3", func(t *testing.T) {
		// When every component is part of the tier, Stage 3 has nothing to apply.
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
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			return nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		applied, err := provisioner.Bootstrap(bp, nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !applied {
			t.Fatal("Expected applied=true")
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
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up-fail")
			return fmt.Errorf("apply blew up")
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		_, err := provisioner.Bootstrap(bp, nil)
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
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			return nil
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

		_, err := provisioner.Bootstrap(bp, nil)
		if err == nil {
			t.Fatal("Expected error from Stage 2 migrate failure, got nil")
		}
		if !strings.Contains(err.Error(), "push failed") {
			t.Errorf("Expected error to wrap Stage 2 failure, got: %v", err)
		}
	})

	t.Run("SkippedTierComponentAfterApplyIsAnError", func(t *testing.T) {
		// If Stage 2's MigrateState reports a tier component skipped (its directory
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
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			return nil
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

		_, err := provisioner.Bootstrap(bp, nil)
		if err == nil {
			t.Fatal("Expected error for skipped-after-apply, got nil")
		}
		if !strings.Contains(err.Error(), "skipped") || !strings.Contains(err.Error(), "backend") {
			t.Errorf("Expected error to name skipped component, got: %v", err)
		}
	})

	t.Run("ConfirmReceivesSummaryBeforeAnyMutation", func(t *testing.T) {
		// confirm runs with the summary before any state-touching op fires.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "backend",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "argocd"},
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
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error { return nil }
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) { return nil, nil }
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		var received *BootstrapSummary
		confirm := func(s *BootstrapSummary) bool {
			received = s
			return true
		}

		applied, err := provisioner.Bootstrap(bp, confirm)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !applied {
			t.Fatal("Expected applied=true")
		}
		if received == nil {
			t.Fatal("Expected confirm callback to receive summary")
		}
		if received.BackendType != "s3" {
			t.Errorf("Expected BackendType=s3 in summary, got %q", received.BackendType)
		}
		if len(received.Terraform) != 1 || received.Terraform[0].ComponentID != "backend" {
			t.Errorf("Expected summary to list [backend], got %#v", received.Terraform)
		}
		if len(received.Kustomize) != 1 || received.Kustomize[0] != "argocd" {
			t.Errorf("Expected summary to list [argocd] kustomize, got %#v", received.Kustomize)
		}
	})

	t.Run("ConfirmDeclineSkipsAllWork", func(t *testing.T) {
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

		var setCalled, upCalled, migrateCalled bool
		mockCH.SetFunc = func(key string, _ any) error {
			if key == "terraform.backend.type" {
				setCalled = true
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			upCalled = true
			return nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			migrateCalled = true
			return nil, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		applied, err := provisioner.Bootstrap(bp, func(_ *BootstrapSummary) bool { return false })
		if err != nil {
			t.Fatalf("Expected no error on decline, got %v", err)
		}
		if applied {
			t.Error("Expected applied=false when confirm declines")
		}
		if setCalled || upCalled || migrateCalled {
			t.Errorf("Expected no state-touching ops on decline, got set=%v up=%v migrate=%v", setCalled, upCalled, migrateCalled)
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
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			return nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		mockStack.RemoveLocalStateFunc = func(componentID string) error {
			ops = append(ops, fmt.Sprintf("remove-local:%s", componentID))
			return fmt.Errorf("permission denied")
		}

		// Capture stderr to verify the warning surfaces without aborting Bootstrap.
		r, w, pipeErr := os.Pipe()
		if pipeErr != nil {
			t.Fatalf("Pipe failed: %v", pipeErr)
		}
		origStderr := os.Stderr
		os.Stderr = w
		defer func() { os.Stderr = origStderr }()

		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})
		applied, err := provisioner.Bootstrap(bp, nil)

		w.Close()
		stderrBytes, _ := io.ReadAll(r)
		stderrOutput := string(stderrBytes)

		if err != nil {
			t.Fatalf("Expected no error (cleanup failure is warning-only), got %v", err)
		}
		if !applied {
			t.Fatal("Expected applied=true")
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

func TestProvisioner_recoverHalfMigratedComponents(t *testing.T) {
	t.Run("UpRecoversLeftoverLocalState", func(t *testing.T) {
		// Provisioner.Up runs the recovery sweep before delegating to
		// TerraformStack.Up. For each component with local state but no remote
		// state, the sweep does the two-step reset-and-migrate: under
		// withBackendOverride("local") run InitComponent (writes local pointer
		// file), exit override, then MigrateComponentState (copies local →
		// configured remote). This protects the scenario where the pointer
		// records the configured backend even though state lives in the local
		// file — without the two-step reset, init -migrate-state would migrate
		// empty remote → local and destroy the local state.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "network"},
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
		mockStack.HasLocalStateWithResourcesFunc = func(componentID string) (bool, error) {
			ops = append(ops, fmt.Sprintf("probe-local:%s", componentID))
			return true, nil
		}
		mockStack.HasRemoteStateFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			ops = append(ops, fmt.Sprintf("probe-remote:%s", componentID))
			return false, nil
		}
		mockStack.InitComponentFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) error {
			ops = append(ops, fmt.Sprintf("init-component:%s", componentID))
			return nil
		}
		mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) error {
			ops = append(ops, fmt.Sprintf("migrate-component:%s", componentID))
			return nil
		}
		mockStack.RemoveLocalStateFunc = func(componentID string) error {
			ops = append(ops, fmt.Sprintf("remove-local:%s", componentID))
			return nil
		}
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			ops = append(ops, "up")
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if err := provisioner.Up(bp); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := []string{
			"probe-local:network",
			"probe-remote:network",
			"set:local",
			"init-component:network",
			"set:azurerm",
			"migrate-component:network",
			"remove-local:network",
			"probe-local:cluster",
			"probe-remote:cluster",
			"set:local",
			"init-component:cluster",
			"set:azurerm",
			"migrate-component:cluster",
			"remove-local:cluster",
			"up",
		}
		if len(ops) != len(expected) {
			t.Fatalf("Expected ops %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}
	})

	t.Run("ShortCircuitsForLocalBackend", func(t *testing.T) {
		// When the configured backend is "local", local IS the configured backend
		// — there is nothing to migrate to. The sweep must short-circuit before
		// iterating components so a local-backend windsor up does not spend a
		// per-component terraform init + show pair on every invocation.
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
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		probeLocalCalled := false
		probeRemoteCalled := false
		mockStack := terraforminfra.NewMockStack()
		mockStack.HasLocalStateWithResourcesFunc = func(_ string) (bool, error) {
			probeLocalCalled = true
			return true, nil
		}
		mockStack.HasRemoteStateFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) (bool, error) {
			probeRemoteCalled = true
			return true, nil
		}
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		if err := provisioner.Up(bp); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if probeLocalCalled {
			t.Error("HasLocalStateWithResources must not be called when backend is local — sweep should short-circuit")
		}
		if probeRemoteCalled {
			t.Error("HasRemoteState must not be called when backend is local")
		}
	})

	t.Run("AbortsOnProbeFailureToAvoidSilentStateOverwrite", func(t *testing.T) {
		// The reset-and-migrate uses terraform init -migrate-state -force-copy,
		// which unconditionally overwrites the destination. If HasRemoteState
		// fails transiently (auth, network, missing storage) and recovery
		// proceeds, the migrate would silently replace valid remote state with
		// the local file. Fail-fast: probe error is a hard abort with the
		// underlying error wrapped, no migrate runs, no state is touched.
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "network"},
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
		mockStack.HasLocalStateWithResourcesFunc = func(_ string) (bool, error) {
			return true, nil
		}
		mockStack.HasRemoteStateFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) (bool, error) {
			return false, fmt.Errorf("auth timeout")
		}
		migrateCalled := false
		mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
			migrateCalled = true
			return nil
		}
		initCalled := false
		mockStack.InitComponentFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
			initCalled = true
			return nil
		}
		removeCalled := false
		mockStack.RemoveLocalStateFunc = func(_ string) error {
			removeCalled = true
			return nil
		}
		upCalled := false
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(id string) error) error {
			upCalled = true
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{TerraformStack: mockStack})

		err := provisioner.Up(bp)
		if err == nil {
			t.Fatal("Expected Up to fail on recovery probe error, got nil")
		}
		if !strings.Contains(err.Error(), "auth timeout") {
			t.Errorf("Expected error to wrap underlying probe error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "recovery sweep aborted") {
			t.Errorf("Expected error to name the recovery sweep, got: %v", err)
		}
		if !strings.Contains(err.Error(), `"network"`) {
			t.Errorf("Expected error to name the affected component, got: %v", err)
		}
		if !strings.Contains(err.Error(), "force-copy") {
			t.Errorf("Expected error to explain the data-loss hazard, got: %v", err)
		}
		if initCalled {
			t.Error("InitComponent must not run when probe fails")
		}
		if migrateCalled {
			t.Error("MigrateComponentState must not run when probe fails")
		}
		if removeCalled {
			t.Error("RemoveLocalState must not run when probe fails")
		}
		if upCalled {
			t.Error("TerraformStack.Up must not run when recovery aborts")
		}
	})
}
