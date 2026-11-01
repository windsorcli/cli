package provisioner_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

func TestProvisioner_Down(t *testing.T) {
	// createWorkstationBlueprint returns a blueprint with a "workstation" terraform component.
	createWorkstationBlueprint := func() *blueprintv1alpha1.Blueprint {
		bp := createTestBlueprint()
		bp.TerraformComponents = append(bp.TerraformComponents, blueprintv1alpha1.TerraformComponent{
			Path: "workstation",
		})
		return bp
	}

	t.Run("SuccessNoopWhenNoWorkstationComponent", func(t *testing.T) {
		// Given a blueprint without a workstation component
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		// When Down is called
		err := prov.Down(context.Background(), createTestBlueprint())

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessDestroysWorkstationComponent", func(t *testing.T) {
		// Given a stack that records the destroyed component
		mocks := setupProvisionerMocks(t)
		destroyed := ""
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			destroyed = componentID
			return false, nil
		}
		opts := &provisioner.Provisioner{TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When Down is called with a workstation blueprint
		err := prov.Down(context.Background(), createWorkstationBlueprint())

		// Then the workstation component is destroyed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if destroyed != "workstation" {
			t.Errorf("Expected workstation component to be destroyed, got: %q", destroyed)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// Given a provisioner
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		// When Down is called with a nil blueprint
		err := prov.Down(context.Background(), nil)

		// Then a blueprint-not-provided error is returned
		if err == nil {
			t.Error("Expected error for nil blueprint")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("SuccessNoopWhenTerraformDisabled", func(t *testing.T) {
		// Given terraform is disabled in config
		mocks := setupProvisionerMocks(t)
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return false
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		// When Down is called
		err := prov.Down(context.Background(), createWorkstationBlueprint())

		// Then no error is returned and no stack is created
		if err != nil {
			t.Errorf("Expected no error when terraform is disabled, got: %v", err)
		}
		if prov.TerraformStack != nil {
			t.Error("Expected TerraformStack to remain nil when terraform is disabled")
		}
	})

	t.Run("SuccessNoopWhenWorkstationDisabled", func(t *testing.T) {
		// Given a workstation component that is explicitly disabled
		mocks := setupProvisionerMocks(t)
		destroyed := ""
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			destroyed = componentID
			return false, nil
		}
		opts := &provisioner.Provisioner{TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		bp := createWorkstationBlueprint()
		disabled := false
		bp.TerraformComponents[len(bp.TerraformComponents)-1].Enabled = &blueprintv1alpha1.BoolExpression{Value: &disabled}

		// When Down is called
		err := prov.Down(context.Background(), bp)

		// Then no error is returned and Destroy is not called
		if err != nil {
			t.Errorf("Expected no error for disabled workstation, got: %v", err)
		}
		if destroyed != "" {
			t.Errorf("Expected Destroy not to be called for disabled workstation, got: %q", destroyed)
		}
	})

	t.Run("ErrorWorkstationDestroyFails", func(t *testing.T) {
		// Given a stack whose Destroy fails
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			return false, fmt.Errorf("workstation destroy failed")
		}
		opts := &provisioner.Provisioner{TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When Down is called
		err := prov.Down(context.Background(), createWorkstationBlueprint())

		// Then a workstation destroy error is returned
		if err == nil {
			t.Error("Expected error for workstation destroy failure")
		}
		if !strings.Contains(err.Error(), "failed to destroy workstation terraform component") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestProvisioner_DestroyAllTerraform(t *testing.T) {
	t.Run("DestroysAllTerraformComponents", func(t *testing.T) {
		// Given a stack whose DestroyAll succeeds
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{}, nil
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When DestroyAllTerraform is called
		_, err := prov.DestroyAllTerraform(context.Background(), createTestBlueprint(), false)

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// Given a provisioner
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		// When DestroyAllTerraform is called with a nil blueprint
		_, err := prov.DestroyAllTerraform(context.Background(), nil, false)

		// Then a blueprint-not-provided error is returned
		if err == nil {
			t.Error("Expected error for nil blueprint")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorWhenTerraformDisabled", func(t *testing.T) {
		// Given terraform is disabled in config
		mocks := setupProvisionerMocks(t)
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return false
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		// When DestroyAllTerraform is called
		_, err := prov.DestroyAllTerraform(context.Background(), createTestBlueprint(), false)

		// Then a terraform-disabled error is returned
		if err == nil {
			t.Error("Expected error when terraform is disabled")
		}
		if !strings.Contains(err.Error(), "terraform is disabled") {
			t.Errorf("Expected 'terraform is disabled' error, got: %v", err)
		}
	})

	t.Run("ErrorDestroyAllFails", func(t *testing.T) {
		// Given a stack whose DestroyAll fails
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{}, fmt.Errorf("terraform destroy all failed")
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When DestroyAllTerraform is called
		_, err := prov.DestroyAllTerraform(context.Background(), createTestBlueprint(), false)

		// Then a terraform destroy error is returned
		if err == nil {
			t.Error("Expected error for terraform destroy all failure")
		}
		if !strings.Contains(err.Error(), "failed to run terraform destroy") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("DoesNotGateOnKubernetesReachability", func(t *testing.T) {
		// Given an unreachable cluster and a stack that records DestroyAll
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return fmt.Errorf("connection refused")
		}
		mockStack := terraforminfra.NewMockStack()
		destroyAllCalled := false
		mockStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalled = true
			return terraforminfra.DestroyOutcome{}, nil
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When DestroyAllTerraform is called
		// Then no error is returned and DestroyAll runs
		if _, err := prov.DestroyAllTerraform(context.Background(), createTestBlueprint(), false); err != nil {
			t.Fatalf("Expected no error: Provisioner must not gate on Kubernetes reachability, got: %v", err)
		}
		if !destroyAllCalled {
			t.Error("Expected terraform DestroyAll to run: Provisioner must not gate on Kubernetes reachability itself")
		}
	})
}

func TestProvisioner_Destroy(t *testing.T) {
	t.Run("DestroysRequestedComponent", func(t *testing.T) {
		// Given a stack whose Destroy succeeds
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) (bool, error) { return false, nil }
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When Destroy is called for a component
		_, err := prov.Destroy(context.Background(), createTestBlueprint(), "remote/path")

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// Given a provisioner
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		// When Destroy is called with a nil blueprint
		_, err := prov.Destroy(context.Background(), nil, "remote/path")

		// Then a blueprint-not-provided error is returned
		if err == nil {
			t.Error("Expected error for nil blueprint")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorWhenTerraformDisabled", func(t *testing.T) {
		// Given terraform is disabled in config
		mocks := setupProvisionerMocks(t)
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return false
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		// When Destroy is called
		_, err := prov.Destroy(context.Background(), createTestBlueprint(), "remote/path")

		// Then a terraform-disabled error is returned
		if err == nil {
			t.Error("Expected error when terraform is disabled")
		}
		if !strings.Contains(err.Error(), "terraform is disabled") {
			t.Errorf("Expected 'terraform is disabled' error, got: %v", err)
		}
	})

	t.Run("ErrorTerraformStackDestroy", func(t *testing.T) {
		// Given a stack whose Destroy fails
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			return false, fmt.Errorf("terraform stack destroy failed")
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When Destroy is called
		_, err := prov.Destroy(context.Background(), createTestBlueprint(), "remote/path")

		// Then a terraform destroy error is returned
		if err == nil {
			t.Error("Expected error for terraform stack destroy failure")
		}
		if !strings.Contains(err.Error(), "failed to run terraform destroy for") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ReturnsSkippedWhenStateIsEmpty", func(t *testing.T) {
		// Given a stack that reports empty state for the component
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			return true, nil
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When Destroy is called
		skipped, err := prov.Destroy(context.Background(), createTestBlueprint(), "remote/path")
		// Then skipped is true and no error is returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if !skipped {
			t.Error("Expected skipped=true when stack reports empty state")
		}
	})

	t.Run("DoesNotGateOnKubernetesReachability", func(t *testing.T) {
		// Given an unreachable cluster and a stack that records Destroy
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return fmt.Errorf("connection refused")
		}
		mockStack := terraforminfra.NewMockStack()
		destroyCalled := false
		mockStack.DestroyFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			destroyCalled = true
			return false, nil
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When Destroy is called
		// Then no error is returned and Destroy runs
		if _, err := prov.Destroy(context.Background(), createTestBlueprint(), "remote/path"); err != nil {
			t.Fatalf("Expected no error: Provisioner must not gate on Kubernetes reachability, got: %v", err)
		}
		if !destroyCalled {
			t.Error("Expected terraform Destroy to run: Provisioner must not gate on Kubernetes reachability itself")
		}
	})
}

func TestProvisioner_DestroyKustomize(t *testing.T) {
	t.Run("DeletesNamedKustomization", func(t *testing.T) {
		// Given a manager whose DeleteKustomization succeeds
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.DeleteKustomizationFunc = func(name, namespace string) error { return nil }
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When DestroyKustomize is called
		err := prov.DestroyKustomize(context.Background(), createTestBlueprint(), "test-kustomization")

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// Given a provisioner
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		// When DestroyKustomize is called with a nil blueprint
		err := prov.DestroyKustomize(context.Background(), nil, "test-kustomization")

		// Then a blueprint-not-provided error is returned
		if err == nil {
			t.Error("Expected error for nil blueprint")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorNilKubernetesManager", func(t *testing.T) {
		// Given a provisioner with no kubernetes manager
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)
		prov.KubernetesManager = nil

		// When DestroyKustomize is called
		err := prov.DestroyKustomize(context.Background(), createTestBlueprint(), "test-kustomization")

		// Then a manager-not-configured error is returned
		if err == nil {
			t.Error("Expected error for nil kubernetes manager")
		}
		if !strings.Contains(err.Error(), "kubernetes manager not configured") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorNotFound", func(t *testing.T) {
		// Given a provisioner
		mocks := setupProvisionerMocks(t)
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When DestroyKustomize is called with an unknown name
		err := prov.DestroyKustomize(context.Background(), createTestBlueprint(), "nonexistent")

		// Then a not-found error with a show remediation is returned
		if err == nil {
			t.Error("Expected error for kustomization not found")
		}
		if !strings.Contains(err.Error(), "not found in blueprint") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "windsor show kustomization") {
			t.Errorf("Expected a remediation that names windsor show kustomization, got: %v", err)
		}
	})

	t.Run("ErrorDeleteKustomization", func(t *testing.T) {
		// Given a manager whose DeleteKustomization fails
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.DeleteKustomizationFunc = func(name, namespace string) error {
			return fmt.Errorf("delete kustomization failed")
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When DestroyKustomize is called
		err := prov.DestroyKustomize(context.Background(), createTestBlueprint(), "test-kustomization")

		// Then a delete kustomization error is returned
		if err == nil {
			t.Error("Expected error for delete kustomization failure")
		}
		if !strings.Contains(err.Error(), "failed to delete kustomization") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("SkipsWhenKubeconfigMissing", func(t *testing.T) {
		// Given a config root with no kubeconfig, so the cluster is gone
		mocks := setupProvisionerMocks(t)
		mocks.Runtime.ConfigRoot = t.TempDir()

		var deleteKustomizationCalled bool
		mocks.KubernetesManager.DeleteKustomizationFunc = func(name, namespace string) error {
			deleteKustomizationCalled = true
			return nil
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When DestroyKustomize is called
		err := prov.DestroyKustomize(context.Background(), createTestBlueprint(), "test-kustomization")

		// Then no error is returned and DeleteKustomization does not run
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if deleteKustomizationCalled {
			t.Error("expected DeleteKustomization not to run when kubeconfig is missing")
		}
	})
}

func TestProvisioner_DestroyAll(t *testing.T) {
	t.Run("DeletesBlueprintAndDestroysTerraform", func(t *testing.T) {
		// Given a manager and a stack that both succeed
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.DeleteBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error { return nil }
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{}, nil
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When DestroyAll is called
		_, err := prov.DestroyAll(context.Background(), createTestBlueprint(), false)

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SuccessNoKubernetesManager", func(t *testing.T) {
		// Given a provisioner with no kubernetes manager
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{}, nil
		}
		opts := &provisioner.Provisioner{TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)
		prov.KubernetesManager = nil

		// When DestroyAll is called
		_, err := prov.DestroyAll(context.Background(), createTestBlueprint(), false)

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error when kubernetes manager is nil, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// Given a provisioner
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		// When DestroyAll is called with a nil blueprint
		_, err := prov.DestroyAll(context.Background(), nil, false)

		// Then a blueprint-not-provided error is returned
		if err == nil {
			t.Error("Expected error for nil blueprint")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorUninstall", func(t *testing.T) {
		// Given a manager whose DeleteBlueprint fails
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.DeleteBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("delete blueprint failed")
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When DestroyAll is called
		_, err := prov.DestroyAll(context.Background(), createTestBlueprint(), false)

		// Then a delete blueprint error is returned
		if err == nil {
			t.Error("Expected error when uninstall fails")
		}
		if !strings.Contains(err.Error(), "failed to delete blueprint") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorDown", func(t *testing.T) {
		// Given a stack whose DestroyAll fails
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.DeleteBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error { return nil }
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{}, fmt.Errorf("terraform down failed")
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When DestroyAll is called
		_, err := prov.DestroyAll(context.Background(), createTestBlueprint(), false)

		// Then the terraform error is returned
		if err == nil {
			t.Error("Expected error when terraform down fails")
		}
		if !strings.Contains(err.Error(), "terraform down failed") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("RunsUninstallWhenKubeconfigPresentEvenIfStale", func(t *testing.T) {
		// Given a present kubeconfig that may point at an unreachable cluster
		mocks := setupProvisionerMocks(t)

		var uninstallCalled bool
		mocks.KubernetesManager.DeleteBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			uninstallCalled = true
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{}, nil
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When DestroyAll is called
		// Then DeleteBlueprint still runs
		if _, err := prov.DestroyAll(context.Background(), createTestBlueprint(), false); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !uninstallCalled {
			t.Error("expected DeleteBlueprint to run")
		}
	})

	t.Run("SkipsUninstallWhenKubeconfigMissing", func(t *testing.T) {
		// Given a config root with no kubeconfig, so the cluster is gone
		mocks := setupProvisionerMocks(t)
		mocks.Runtime.ConfigRoot = t.TempDir()

		var uninstallCalled bool
		mocks.KubernetesManager.DeleteBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			uninstallCalled = true
			return nil
		}
		var terraformDestroyCalled bool
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			terraformDestroyCalled = true
			return terraforminfra.DestroyOutcome{}, nil
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When DestroyAll is called
		result, err := prov.DestroyAll(context.Background(), createTestBlueprint(), false)

		// Then kustomize is skipped as cluster-gone and terraform still runs
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if uninstallCalled {
			t.Error("expected DeleteBlueprint not to run when kubeconfig is missing")
		}
		if !terraformDestroyCalled {
			t.Error("expected terraform destroy to still run when kubeconfig is missing")
		}
		if result.TerraformDeferred {
			t.Error("expected TerraformDeferred=false when kustomize destroy was skipped, not failed")
		}
		if len(result.Destroyed) != 0 {
			t.Errorf("expected no kustomizations in Destroyed, got %v", result.Destroyed)
		}
		if len(result.Skipped) != 0 {
			t.Errorf("expected no terraform-style empty-state skips, got %v", result.Skipped)
		}
		if len(result.SkippedClusterGone) != 1 || result.SkippedClusterGone[0] != "test-kustomization" {
			t.Errorf("expected kustomization counted in SkippedClusterGone, got %v", result.SkippedClusterGone)
		}
	})

	t.Run("ContinueDefersTerraformWhenKustomizeFailsAndClusterReachable", func(t *testing.T) {
		// Given kustomize uninstall fails while the cluster stays reachable
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.DeleteBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("destroy aborted: timeout waiting for load balancer teardown")
		}
		var terraformDestroyCalled bool
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			terraformDestroyCalled = true
			return terraforminfra.DestroyOutcome{}, nil
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When DestroyAll runs with continueOnError
		result, err := prov.DestroyAll(context.Background(), createTestBlueprint(), true)

		// Then terraform is deferred and the kustomize failure is recorded
		if err != nil {
			t.Fatalf("expected continueOnError to absorb the kustomize failure, got: %v", err)
		}
		if terraformDestroyCalled {
			t.Error("expected terraform DestroyAll not to run when kustomize failed while the cluster is reachable")
		}
		if !result.TerraformDeferred {
			t.Error("expected TerraformDeferred=true")
		}
		var foundKustomize bool
		for _, f := range result.Failed {
			if f.ID == provisioner.KustomizeFailureID {
				foundKustomize = true
			}
		}
		if !foundKustomize {
			t.Errorf("expected kustomize failure in result.Failed, got %v", result.Failed)
		}
	})

	t.Run("ContinueDefersTerraformWhenKustomizeFailsAndClusterUnreachable", func(t *testing.T) {
		// Given kustomize uninstall fails and the cluster is unreachable
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.DeleteBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("delete blueprint failed")
		}
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(_ context.Context, _ string, _ func(string), _ ...string) error {
			return fmt.Errorf("cluster API unreachable")
		}
		var terraformDestroyCalled bool
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			terraformDestroyCalled = true
			return terraforminfra.DestroyOutcome{}, nil
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When DestroyAll runs with continueOnError
		result, err := prov.DestroyAll(context.Background(), createTestBlueprint(), true)

		// Then terraform is still deferred
		if err != nil {
			t.Fatalf("expected continueOnError to absorb the kustomize failure, got: %v", err)
		}
		if terraformDestroyCalled {
			t.Error("expected terraform DestroyAll not to run even when the cluster is unreachable")
		}
		if !result.TerraformDeferred {
			t.Error("expected TerraformDeferred=true even when the cluster is unreachable")
		}
	})

	t.Run("CountsEligibleKustomizationsInDestroyedOnSuccess", func(t *testing.T) {
		// Given a blueprint with three eligible kustomizations
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "dns"},
				{Name: "ingress"},
				{Name: "cert-manager"},
			},
		}
		mocks.KubernetesManager.DeleteBlueprintFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error { return nil }
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{}, nil
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When DestroyAll runs and DeleteBlueprint succeeds
		result, err := prov.DestroyAll(context.Background(), bp, false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then each eligible kustomization appears in result.Destroyed
		want := map[string]bool{"dns": false, "ingress": false, "cert-manager": false}
		for _, name := range result.Destroyed {
			if _, ok := want[name]; ok {
				want[name] = true
			}
		}
		for name, seen := range want {
			if !seen {
				t.Errorf("Expected kustomization %q in result.Destroyed, got %v", name, result.Destroyed)
			}
		}
	})

	t.Run("SkipsIneligibleKustomizationsFromDestroyedCount", func(t *testing.T) {
		// Given one eligible, one Destroy=false, and one DestroyOnly kustomization
		mocks := setupProvisionerMocks(t)
		destroyFalse := false
		destroyOnly := true
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "dns"},
				{Name: "keep-me", Destroy: &blueprintv1alpha1.BoolExpression{Value: &destroyFalse, IsExpr: false}},
				{Name: "hook-only", DestroyOnly: &destroyOnly},
			},
		}
		mocks.KubernetesManager.DeleteBlueprintFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error { return nil }
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{}, nil
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When DestroyAll runs
		result, err := prov.DestroyAll(context.Background(), bp, false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then only the eligible kustomization appears in result.Destroyed
		if len(result.Destroyed) != 1 || result.Destroyed[0] != "dns" {
			t.Errorf("Expected result.Destroyed=[dns], got %v", result.Destroyed)
		}
	})

	t.Run("DoesNotGateOnKubernetesReachability", func(t *testing.T) {
		// Given an unreachable cluster and a stack that records DestroyAll
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.DeleteBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error { return nil }
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return fmt.Errorf("connection refused")
		}
		mockStack := terraforminfra.NewMockStack()
		destroyAllCalled := false
		mockStack.DestroyAllFunc = func(bp *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalled = true
			return terraforminfra.DestroyOutcome{}, nil
		}
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When DestroyAll is called
		// Then no error is returned and DestroyAll runs
		if _, err := prov.DestroyAll(context.Background(), createTestBlueprint(), false); err != nil {
			t.Fatalf("Expected no error: Provisioner must not gate on Kubernetes reachability, got: %v", err)
		}
		if !destroyAllCalled {
			t.Error("Expected terraform DestroyAll to run: Provisioner must not gate on Kubernetes reachability itself")
		}
	})
}

func TestProvisioner_CheckComponentDestroyable(t *testing.T) {
	bp := &blueprintv1alpha1.Blueprint{
		Backend: "cluster",
		TerraformComponents: []blueprintv1alpha1.TerraformComponent{
			{Name: "compute", Path: "compute/hcloud"},
			{Name: "cluster", Path: "cluster/talos"},
			{Name: "dns", Path: "dns/zone/hetzner"},
		},
	}
	kubernetesBackend := func(mocks *ProvisionerTestMocks) {
		mocks.ConfigHandler.(*config.MockConfigHandler).GetStringFunc = func(key string, dv ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
			}
			if len(dv) > 0 {
				return dv[0]
			}
			return ""
		}
	}

	t.Run("RefusesBackendComponentOnKubernetesBackend", func(t *testing.T) {
		// Given a kubernetes backend
		mocks := setupProvisionerMocks(t)
		kubernetesBackend(mocks)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{})
		// When the backend member compute is checked
		err := prov.CheckComponentDestroyable(bp, "compute")
		// Then a refusal naming the backend is returned
		if err == nil || !strings.Contains(err.Error(), "backend") {
			t.Errorf("expected refusal naming backend, got %v", err)
		}
	})

	t.Run("AllowsNonBackendComponentOnKubernetesBackend", func(t *testing.T) {
		// Given a kubernetes backend
		mocks := setupProvisionerMocks(t)
		kubernetesBackend(mocks)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{})
		// When the non-backend component dns is checked
		// Then no error is returned
		if err := prov.CheckComponentDestroyable(bp, "dns"); err != nil {
			t.Errorf("expected non-backend component allowed, got %v", err)
		}
	})

	t.Run("AllowsBackendComponentOnLocalBackend", func(t *testing.T) {
		// Given the default local backend
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{})
		// When the backend member compute is checked
		// Then no error is returned
		if err := prov.CheckComponentDestroyable(bp, "compute"); err != nil {
			t.Errorf("expected local backend to allow any component, got %v", err)
		}
	})

	t.Run("RefusesWhenBackendFieldUnresolved", func(t *testing.T) {
		// Given a Backend field that names no component
		unresolved := &blueprintv1alpha1.Blueprint{
			Backend: "ghost",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "compute", Path: "compute/hcloud"},
			},
		}
		mocks := setupProvisionerMocks(t)
		kubernetesBackend(mocks)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{})
		// When compute is checked
		err := prov.CheckComponentDestroyable(unresolved, "compute")
		// Then a refusal naming the unresolved backend is returned
		if err == nil || !strings.Contains(err.Error(), `"ghost"`) {
			t.Errorf("expected refusal naming the unresolved backend, got %v", err)
		}
	})
}

func TestProvisioner_PivotToLocalIfClusterGone(t *testing.T) {
	kubernetesBackend := func(mocks *ProvisionerTestMocks, set *bool) {
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, dv ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
			}
			if len(dv) > 0 {
				return dv[0]
			}
			return ""
		}
		mockCH.SetFunc = func(key string, _ any) error {
			if key == "terraform.backend.type" {
				*set = true
			}
			return nil
		}
	}

	t.Run("PivotsWhenKubeconfigAbsent", func(t *testing.T) {
		// Given a kubernetes backend and no kubeconfig
		mocks := setupProvisionerMocks(t)
		set := false
		kubernetesBackend(mocks, &set)
		mocks.Runtime.ConfigRoot = t.TempDir()
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{})

		// When PivotToLocalIfClusterGone is called
		pivoted, err := prov.PivotToLocalIfClusterGone()
		// Then the backend pivots to local
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !pivoted || !set {
			t.Errorf("expected pivot to local when kubeconfig absent, pivoted=%v set=%v", pivoted, set)
		}
	})

	t.Run("DoesNotPivotWhenClusterReachable", func(t *testing.T) {
		// Given a kubernetes backend, a kubeconfig, and a reachable cluster
		mocks := setupProvisionerMocks(t)
		set := false
		kubernetesBackend(mocks, &set)
		dir := t.TempDir()
		_ = os.MkdirAll(filepath.Join(dir, ".kube"), 0o755)
		_ = os.WriteFile(filepath.Join(dir, ".kube", "config"), []byte("x"), 0o644)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, out func(string), nodes ...string) error {
			return nil
		}
		mocks.Runtime.ConfigRoot = dir
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When PivotToLocalIfClusterGone is called
		pivoted, err := prov.PivotToLocalIfClusterGone()
		// Then the backend does not pivot
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pivoted || set {
			t.Errorf("expected no pivot when cluster reachable, pivoted=%v set=%v", pivoted, set)
		}
	})

	t.Run("PivotsWhenClusterUnreachable", func(t *testing.T) {
		// Given a kubernetes backend, a present kubeconfig, and a cluster that fails the health check
		mocks := setupProvisionerMocks(t)
		set := false
		kubernetesBackend(mocks, &set)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, out func(string), nodes ...string) error {
			return fmt.Errorf("connection refused")
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When PivotToLocalIfClusterGone is called
		pivoted, err := prov.PivotToLocalIfClusterGone()

		// Then it pivots the backend to local
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !pivoted || !set {
			t.Errorf("expected pivot to local when cluster unreachable, pivoted=%v set=%v", pivoted, set)
		}
	})

	t.Run("NoOpOnLocalBackend", func(t *testing.T) {
		// Given the default local backend
		mocks := setupProvisionerMocks(t)
		set := false
		mocks.ConfigHandler.(*config.MockConfigHandler).SetFunc = func(_ string, _ any) error { set = true; return nil }
		mocks.Runtime.ConfigRoot = t.TempDir()
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{})

		// When PivotToLocalIfClusterGone is called
		pivoted, err := prov.PivotToLocalIfClusterGone()
		// Then the backend does not pivot
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pivoted || set {
			t.Errorf("expected no pivot on local backend, pivoted=%v set=%v", pivoted, set)
		}
	})
}

func TestProvisioner_PrepareLocalTeardown(t *testing.T) {
	bp := &blueprintv1alpha1.Blueprint{Backend: "cluster", TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Name: "cluster", Path: "cluster/talos"}}}
	kubernetesBackend := func(mocks *ProvisionerTestMocks, set *bool) {
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, dv ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
			}
			if len(dv) > 0 {
				return dv[0]
			}
			return ""
		}
		mockCH.SetFunc = func(key string, _ any) error {
			if key == "terraform.backend.type" {
				*set = true
			}
			return nil
		}
	}

	t.Run("PivotsToLocalAndMigratesUpFront", func(t *testing.T) {
		// Given a kubernetes backend and a migration that succeeds
		mocks := setupProvisionerMocks(t)
		set := false
		kubernetesBackend(mocks, &set)
		migrated := false
		mockStack := terraforminfra.NewMockStack()
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			migrated = true
			return nil, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When PrepareLocalTeardown is called
		pivoted, err := prov.PrepareLocalTeardown(context.Background(), bp)
		// Then the backend pivots to local and state migrates
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !pivoted || !set || !migrated {
			t.Errorf("expected pivot + migration, pivoted=%v set=%v migrated=%v", pivoted, set, migrated)
		}
	})

	t.Run("NoOpOnLocalBackend", func(t *testing.T) {
		// Given the default local backend
		mocks := setupProvisionerMocks(t)
		set := false
		mocks.ConfigHandler.(*config.MockConfigHandler).SetFunc = func(_ string, _ any) error { set = true; return nil }
		migrated := false
		mockStack := terraforminfra.NewMockStack()
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) { migrated = true; return nil, nil }
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When PrepareLocalTeardown is called
		pivoted, err := prov.PrepareLocalTeardown(context.Background(), bp)
		// Then nothing pivots or migrates
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pivoted || set || migrated {
			t.Errorf("expected no-op on local backend, pivoted=%v set=%v migrated=%v", pivoted, set, migrated)
		}
	})

	t.Run("NoOpOnNonKubernetesRemoteBackend", func(t *testing.T) {
		// Given a gcs backend, whose state store outlives the cluster
		mocks := setupProvisionerMocks(t)
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetTerraformBackendTypeFunc = func() string { return "gcs" }
		set := false
		mockCH.SetFunc = func(_ string, _ any) error { set = true; return nil }
		migrated := false
		mockStack := terraforminfra.NewMockStack()
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) { migrated = true; return nil, nil }
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When PrepareLocalTeardown is called
		pivoted, err := prov.PrepareLocalTeardown(context.Background(), bp)
		// Then nothing pivots or migrates
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pivoted || set || migrated {
			t.Errorf("expected no-op on gcs backend, pivoted=%v set=%v migrated=%v", pivoted, set, migrated)
		}
	})

	t.Run("AbortsAndRevertsPivotOnMigrationFailureWhileClusterReachable", func(t *testing.T) {
		// Given a reachable cluster and a migration that fails
		mocks := setupProvisionerMocks(t)
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, dv ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
			}
			if len(dv) > 0 {
				return dv[0]
			}
			return ""
		}
		var sets []string
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				sets = append(sets, fmt.Sprintf("%v", value))
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			return nil, fmt.Errorf("boom")
		}
		dir := t.TempDir()
		_ = os.MkdirAll(filepath.Join(dir, ".kube"), 0o755)
		_ = os.WriteFile(filepath.Join(dir, ".kube", "config"), []byte("x"), 0o644)
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, out func(string), nodes ...string) error {
			return nil
		}
		mocks.Runtime.ConfigRoot = dir
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack, KubernetesManager: mocks.KubernetesManager})

		// When PrepareLocalTeardown is called
		_, err := prov.PrepareLocalTeardown(context.Background(), bp)
		// Then it aborts and reverts the pivot to kubernetes
		if err == nil || !strings.Contains(err.Error(), "migrate terraform state") {
			t.Errorf("expected abort on migration failure with cluster up, got %v", err)
		}
		if len(sets) != 2 || sets[0] != "local" || sets[1] != "kubernetes" {
			t.Errorf("expected pivot to local then revert to kubernetes, got %v", sets)
		}
	})

	t.Run("ProceedsOnMigrationFailureWhenClusterGone", func(t *testing.T) {
		// Given no kubeconfig, so the cluster is gone, and a migration that fails
		mocks := setupProvisionerMocks(t)
		set := false
		kubernetesBackend(mocks, &set)
		mockStack := terraforminfra.NewMockStack()
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			return nil, fmt.Errorf("dial tcp: connection refused")
		}
		mocks.Runtime.ConfigRoot = t.TempDir()
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When PrepareLocalTeardown is called
		pivoted, err := prov.PrepareLocalTeardown(context.Background(), bp)
		// Then the backend pivots and teardown proceeds
		if err != nil {
			t.Fatalf("expected resume to proceed, got %v", err)
		}
		if !pivoted || !set {
			t.Errorf("expected pivot on resume, pivoted=%v set=%v", pivoted, set)
		}
	})
}

func TestProvisioner_Teardown(t *testing.T) {
	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// Given a provisioner
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		// When Teardown is called with a nil blueprint
		_, err := prov.Teardown(context.Background(), nil, false, false)

		// Then a blueprint-not-provided error is returned
		if err == nil {
			t.Fatal("Expected error for nil blueprint, got nil")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("KubernetesWithoutBackendFieldErrorsBeforeAnyDestroy", func(t *testing.T) {
		// Given a kubernetes backend and a blueprint with no Backend field
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
		destroyAllCalled := false
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalled = true
			return terraforminfra.DestroyOutcome{}, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When Teardown is called
		_, err := prov.Teardown(context.Background(), bp, true, false)
		// Then it refuses with a remediation and DestroyAll does not run
		if err == nil {
			t.Fatal("Expected error for kubernetes backend without Blueprint.Backend, got nil")
		}
		if !strings.Contains(err.Error(), "Blueprint.Backend") {
			t.Errorf("Expected error to name the missing field, got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "backend: <cluster-component-id>") {
			t.Errorf("Expected a remediation that names backend: <cluster-component-id>, got: %v", err)
		}
		if !strings.Contains(err.Error(), "kubernetes") {
			t.Errorf("Expected error to name the backend type, got: %v", err)
		}
		if destroyAllCalled {
			t.Error("DestroyAll must not run when refusing")
		}
	})

	t.Run("LocalBackendCollapsesToDestroyAllTerraform", func(t *testing.T) {
		// Given a local backend
		mocks := setupProvisionerMocks(t)
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
		setCalled := false
		mockCH.SetFunc = func(_ string, _ any) error {
			setCalled = true
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		destroyAllCalls := 0
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalls++
			return terraforminfra.DestroyOutcome{}, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		// When Teardown is called
		// Then DestroyAll runs once and the backend is not overridden
		if _, err := prov.Teardown(context.Background(), createTestBlueprint(), true, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if destroyAllCalls != 1 {
			t.Errorf("Expected single DestroyAll call for local backend, got %d", destroyAllCalls)
		}
		if setCalled {
			t.Error("Expected backend.type Set NOT to be called when backend is local")
		}
	})

	t.Run("NoBackendFieldCollapsesToDestroyAllTerraform", func(t *testing.T) {
		// Given an s3 backend and a blueprint with no Backend field
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
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
		setCalled := false
		mockCH.SetFunc = func(_ string, _ any) error {
			setCalled = true
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		var seenExclude []string
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			seenExclude = excludeIDs
			return terraforminfra.DestroyOutcome{}, nil
		}
		migrateCalled := false
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			migrateCalled = true
			return nil, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		// When Teardown is called
		// Then DestroyAll runs with no excludes, override, or migration
		if _, err := prov.Teardown(context.Background(), bp, true, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if setCalled {
			t.Error("Expected no backend override when blueprint has no declared backend")
		}
		if len(seenExclude) != 0 {
			t.Errorf("Expected no excludes when there is no declared backend, got %v", seenExclude)
		}
		if migrateCalled {
			t.Error("MigrateState must not run when there is no declared backend")
		}
	})

	t.Run("WarnsAboutOrphanedLocalStateBeforeDestroying", func(t *testing.T) {
		// Given local state with resources under a componentID not in the blueprint
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
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
			return []string{"vpc-old"}, nil
		}
		mockStack.HasLocalStateWithResourcesFunc = func(componentID string) (bool, error) {
			return componentID == "vpc-old", nil
		}
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{}, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		// When Teardown runs while warnings are recorded
		var err error
		warnings := captureWarnings(t, func(ctx context.Context) {
			_, err = prov.Teardown(ctx, bp, true, false)
		})

		// Then a warning names the orphaned componentID
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !strings.Contains(warnings, "vpc-old") {
			t.Errorf("Expected a warning naming the orphaned componentID, got: %q", warnings)
		}
	})

	t.Run("OrphanInspectionFailureWarnsButDoesNotBlockDestroy", func(t *testing.T) {
		// Given an orphaned state file that fails inspection
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
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
			return []string{"vpc-old"}, nil
		}
		mockStack.HasLocalStateWithResourcesFunc = func(_ string) (bool, error) {
			return false, fmt.Errorf("corrupted terraform.tfstate: unexpected end of JSON input")
		}
		destroyAllCalled := false
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalled = true
			return terraforminfra.DestroyOutcome{}, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		// When Teardown runs while warnings are recorded
		var err error
		warnings := captureWarnings(t, func(ctx context.Context) {
			_, err = prov.Teardown(ctx, bp, true, false)
		})

		// Then DestroyAll still runs and a warning names the componentID
		if err != nil {
			t.Fatalf("Expected no error — an orphan-inspection failure must only warn, got %v", err)
		}
		if !destroyAllCalled {
			t.Error("Expected DestroyAll to still run despite the orphan-inspection failure")
		}
		if !strings.Contains(warnings, "vpc-old") {
			t.Errorf("Expected a warning naming the componentID that failed inspection, got: %q", warnings)
		}
	})

	t.Run("RefusesUnresolvedBackendField", func(t *testing.T) {
		// Given a Backend field that names no real component
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "backend",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "network/gcp-vpc"},
				{Path: "cluster/gcp-gke"},
			},
		}
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "gcs"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockStack := terraforminfra.NewMockStack()
		destroyAllCalled := false
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalled = true
			return terraforminfra.DestroyOutcome{}, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		// When Teardown is called
		_, err := prov.Teardown(context.Background(), bp, true, false)
		// Then it refuses and DestroyAll does not run
		if err == nil {
			t.Fatal("Expected error for unresolved Backend field, got nil")
		}
		if !strings.Contains(err.Error(), `"backend"`) {
			t.Errorf("Expected error to name the unresolved backend, got: %v", err)
		}
		if destroyAllCalled {
			t.Error("DestroyAll must not run when the backend components cannot be resolved")
		}
	})

	t.Run("SingleBackendComponentDestroysNonBackendThenBackend", func(t *testing.T) {
		// Given an s3 backend and a blueprint with one backend component
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
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
		var destroyAllBlueprints []*blueprintv1alpha1.Blueprint
		var destroyAllExcludes [][]string
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(b *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			ops = append(ops, fmt.Sprintf("destroyAll:exclude=%v", excludeIDs))
			destroyAllBlueprints = append(destroyAllBlueprints, b)
			destroyAllExcludes = append(destroyAllExcludes, excludeIDs)
			return terraforminfra.DestroyOutcome{}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate")
			return nil, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		// When Teardown is called
		// Then non-backend destroys first, then the backend destroys against local
		if _, err := prov.Teardown(context.Background(), bp, true, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := []string{
			"destroyAll:exclude=[backend]",
			"set:local",
			"migrate",
			"destroyAll:exclude=[]",
			"set:s3",
		}
		if len(ops) != len(expected) {
			t.Fatalf("Expected %v, got %v", expected, ops)
		}
		for i, want := range expected {
			if ops[i] != want {
				t.Errorf("op %d: got %q, want %q (full: %v)", i, ops[i], want, ops)
			}
		}

		if len(destroyAllBlueprints) != 2 {
			t.Fatalf("Expected two DestroyAll calls, got %d", len(destroyAllBlueprints))
		}
		stage2Components := destroyAllBlueprints[1].TerraformComponents
		if len(stage2Components) != 1 || stage2Components[0].Path != "backend" {
			t.Errorf("Stage 2 DestroyAll should target the backend [backend], got %#v", stage2Components)
		}
	})

	t.Run("MultiComponentBackendDestroysMembersThenBackend", func(t *testing.T) {
		// Given an s3 backend whose backend set is vpc, iam, and cluster
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "cluster",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
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
		mockCH.SetFunc = func(_ string, _ any) error { return nil }

		var destroyAllBlueprints []*blueprintv1alpha1.Blueprint
		var destroyAllExcludes [][]string
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(b *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllBlueprints = append(destroyAllBlueprints, b)
			destroyAllExcludes = append(destroyAllExcludes, excludeIDs)
			return terraforminfra.DestroyOutcome{}, nil
		}
		var migrateBlueprints []*blueprintv1alpha1.Blueprint
		mockStack.MigrateStateFunc = func(b *blueprintv1alpha1.Blueprint) ([]string, error) {
			migrateBlueprints = append(migrateBlueprints, b)
			return nil, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		// When Teardown is called
		// Then members destroy before the cluster, after one migration
		if _, err := prov.Teardown(context.Background(), bp, true, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(destroyAllBlueprints) != 3 {
			t.Fatalf("Expected three DestroyAll calls (Stage 1, Stage 2a, Stage 2b), got %d", len(destroyAllBlueprints))
		}
		expectedStage1Excludes := []string{"networking/vpc", "iam", "cluster"}
		if len(destroyAllExcludes[0]) != 3 {
			t.Fatalf("Stage 1 excludes should list all 3 backend component IDs, got %v", destroyAllExcludes[0])
		}
		for i, want := range expectedStage1Excludes {
			if destroyAllExcludes[0][i] != want {
				t.Errorf("Stage 1 exclude[%d]: got %q, want %q", i, destroyAllExcludes[0][i], want)
			}
		}

		stage2aComponents := destroyAllBlueprints[1].TerraformComponents
		if len(stage2aComponents) != 3 {
			t.Fatalf("Stage 2a should still target the full 3-component backend set (member exclusion happens via excludeIDs), got %d", len(stage2aComponents))
		}
		if len(destroyAllExcludes[1]) != 1 || destroyAllExcludes[1][0] != "cluster" {
			t.Errorf("Stage 2a should exclude only the backend component %q, got %v", "cluster", destroyAllExcludes[1])
		}

		stage2bComponents := destroyAllBlueprints[2].TerraformComponents
		if len(stage2bComponents) != 1 || stage2bComponents[0].GetID() != "cluster" {
			t.Fatalf("Stage 2b should target only the backend component [cluster], got %#v", stage2bComponents)
		}
		if len(destroyAllExcludes[2]) != 0 {
			t.Errorf("Stage 2b should exclude nothing, got %v", destroyAllExcludes[2])
		}

		if len(migrateBlueprints) != 1 {
			t.Fatalf("Expected one MigrateState call, got %d", len(migrateBlueprints))
		}
		if len(migrateBlueprints[0].TerraformComponents) != 3 {
			t.Errorf("MigrateState should target the 3-component backend set, got %d", len(migrateBlueprints[0].TerraformComponents))
		}
	})

	t.Run("MultiComponentBackendMateFailureDefersBackend", func(t *testing.T) {
		// Given a backend set where the vpc member fails during Stage 2a
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "cluster",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "networking/vpc"},
				{Name: "cluster", Path: "cluster/eks"},
			},
		}
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "gcs"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockCH.SetFunc = func(_ string, _ any) error { return nil }

		destroyAllCalls := 0
		var stage2bCalled bool
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(b *blueprintv1alpha1.Blueprint, _ bool, excludeIDs ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalls++
			if destroyAllCalls == 1 {
				return terraforminfra.DestroyOutcome{}, nil
			}
			if len(excludeIDs) == 1 && excludeIDs[0] == "cluster" {
				return terraforminfra.DestroyOutcome{
					Failed: []terraforminfra.ComponentFailure{{ID: "networking/vpc", Err: fmt.Errorf("network still in use")}},
				}, nil
			}
			stage2bCalled = true
			return terraforminfra.DestroyOutcome{Destroyed: []string{"cluster"}}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) { return nil, nil }
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		// When Teardown runs with continueOnError
		result, err := prov.Teardown(context.Background(), bp, true, true)
		// Then the cluster is not destroyed and terraform is deferred
		if err != nil {
			t.Fatalf("Expected continueOnError to absorb the fellow-component failure, got %v", err)
		}
		if stage2bCalled {
			t.Error("Backend component must not be destroyed in the same pass a fellow backend component failed in")
		}
		if !result.TerraformDeferred {
			t.Error("Expected TerraformDeferred=true when a fellow backend component failed during Stage 2a")
		}
		if len(result.Failed) != 1 || result.Failed[0].ID != "networking/vpc" {
			t.Errorf("Expected the fellow-component failure to surface in result.Failed, got %v", result.Failed)
		}
	})

	t.Run("Stage1FailureAbortsBeforeStage2", func(t *testing.T) {
		// Given an s3 backend and a Stage 1 DestroyAll that fails
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
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
		setCalled := false
		mockCH.SetFunc = func(_ string, _ any) error {
			setCalled = true
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{}, fmt.Errorf("non-backend destroy failed")
		}
		migrateCalled := false
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			migrateCalled = true
			return nil, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		// When Teardown is called
		_, err := prov.Teardown(context.Background(), bp, true, false)
		// Then the error is returned and Stage 2 never starts
		if err == nil {
			t.Fatal("Expected error from Stage 1 failure, got nil")
		}
		if !strings.Contains(err.Error(), "non-backend destroy failed") {
			t.Errorf("Expected error to wrap Stage 1 failure, got %v", err)
		}
		if migrateCalled {
			t.Error("MigrateState must not run after Stage 1 fails")
		}
		if setCalled {
			t.Error("Backend override must not engage after Stage 1 fails")
		}
	})

	t.Run("Stage2MigrationFailureAbortsBackendDestroyAndRestoresBackend", func(t *testing.T) {
		// Given an s3 backend and a Stage 2 migration that fails
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
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
		destroyAllCalls := 0
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalls++
			ops = append(ops, "destroyAll")
			return terraforminfra.DestroyOutcome{}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			ops = append(ops, "migrate-fail")
			return nil, fmt.Errorf("configured backend unreachable")
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		// When Teardown is called
		_, err := prov.Teardown(context.Background(), bp, true, false)
		// Then no backend destroy runs and the backend is restored
		if err == nil {
			t.Fatal("Expected error from Stage 2 migration failure, got nil")
		}
		if !strings.Contains(err.Error(), "configured backend unreachable") {
			t.Errorf("Expected underlying migration cause in error, got %v", err)
		}
		if destroyAllCalls != 1 {
			t.Errorf("Stage 2 backend destroy must not run when migration fails (expected 1 DestroyAll from Stage 1, got %d)", destroyAllCalls)
		}
		var sawRestore bool
		for _, op := range ops {
			if op == "set:s3" {
				sawRestore = true
			}
		}
		if !sawRestore {
			t.Errorf("Expected backend restore via defer after Stage 2 failure, got %v", ops)
		}
	})

	t.Run("BackendRestoreFailureEmitsStderrWarning", func(t *testing.T) {
		// Given a backend restore that fails after a successful teardown
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
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
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" && value == "s3" {
				return fmt.Errorf("mock restore failure")
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) { return nil, nil }

		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})
		// When Teardown runs while warnings are recorded
		var err error
		warnings := captureWarnings(t, func(ctx context.Context) {
			_, err = prov.Teardown(ctx, bp, true, false)
		})

		// Then teardown succeeds and a warning reports about the restore failure
		if err != nil {
			t.Fatalf("Expected destroy to succeed despite restore failure, got %v", err)
		}
		if !strings.Contains(warnings, "failed to restore terraform.backend.type") {
			t.Errorf("Expected warning about restore failure, got: %q", warnings)
		}
		if !strings.Contains(warnings, "mock restore failure") {
			t.Errorf("Expected warning to include underlying cause, got: %q", warnings)
		}
	})

	t.Run("SkippedComponentsMergeAcrossStages", func(t *testing.T) {
		// Given stages that report overlapping skipped components
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
				{Path: "gitops"},
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
		mockCH.SetFunc = func(_ string, _ any) error { return nil }
		mockStack := terraforminfra.NewMockStack()
		destroyAllCalls := 0
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalls++
			if destroyAllCalls == 1 {
				return terraforminfra.DestroyOutcome{Skipped: []string{"gitops"}}, nil
			}
			return terraforminfra.DestroyOutcome{Skipped: []string{"backend"}}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			return []string{"backend"}, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		// When Teardown is called
		result, err := prov.Teardown(context.Background(), bp, true, false)
		// Then result.Skipped lists each component once in order
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expected := []string{"gitops", "backend"}
		if len(result.Skipped) != len(expected) {
			t.Fatalf("Expected %v, got %v", expected, result.Skipped)
		}
		for i, want := range expected {
			if result.Skipped[i] != want {
				t.Errorf("skipped[%d]: got %q, want %q", i, result.Skipped[i], want)
			}
		}
	})

	t.Run("ContinueDefersTerraformWhenNonBackendHasFailures", func(t *testing.T) {
		// Given a non-backend component that fails in Stage 1
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
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
		setCalled := false
		mockCH.SetFunc = func(_ string, _ any) error {
			setCalled = true
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		migrateCalled := false
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{
				Failed: []terraforminfra.ComponentFailure{{ID: "cluster", Err: fmt.Errorf("cluster destroy failed")}},
			}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			migrateCalled = true
			return nil, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		// When Teardown runs with continueOnError
		result, err := prov.Teardown(context.Background(), bp, true, true)

		// Then terraform is deferred and Stage 2 never engages
		if err != nil {
			t.Fatalf("Expected continueOnError to absorb per-component failure, got %v", err)
		}
		if !result.TerraformDeferred {
			t.Error("Expected TerraformDeferred=true when non-backend destroy left a failure")
		}
		if len(result.Failed) != 1 || result.Failed[0].ID != "cluster" {
			t.Errorf("Expected failures to include cluster, got %v", result.Failed)
		}
		if migrateCalled {
			t.Error("MigrateState must not run when non-backend failures left terraform deferred")
		}
		if setCalled {
			t.Error("Backend override must not engage when terraform is deferred")
		}
	})

	t.Run("ContinueAttemptsTerraformWhenStage1Clean", func(t *testing.T) {
		// Given a Stage 1 destroy that completes with zero failures
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
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
		mockCH.SetFunc = func(_ string, _ any) error { return nil }
		mockStack := terraforminfra.NewMockStack()
		migrateCalled := false
		destroyAllCalls := 0
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalls++
			return terraforminfra.DestroyOutcome{Destroyed: []string{fmt.Sprintf("pass%d", destroyAllCalls)}}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			migrateCalled = true
			return nil, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		// When Teardown runs with continueOnError
		result, err := prov.Teardown(context.Background(), bp, true, true)

		// Then Stage 2 runs and TerraformDeferred stays false
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result.TerraformDeferred {
			t.Error("Expected TerraformDeferred=false when Stage 1 was clean")
		}
		if !migrateCalled {
			t.Error("Expected MigrateState to run when Stage 1 produced no failures")
		}
		if destroyAllCalls != 2 {
			t.Errorf("Expected Stage 1 + Stage 2 DestroyAll calls (2), got %d", destroyAllCalls)
		}
	})

	t.Run("StillDefersTerraformAfterPrepareLocalTeardownOnRemoteBackend", func(t *testing.T) {
		// Given a gcs backend and a non-backend destroy failure
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}
		backendType := "gcs"
		mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetTerraformBackendTypeFunc = func() string { return backendType }
		mockCH.SetFunc = func(key string, value any) error {
			if key == "terraform.backend.type" {
				backendType = fmt.Sprintf("%v", value)
			}
			return nil
		}
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			return terraforminfra.DestroyOutcome{
				Failed: []terraforminfra.ComponentFailure{{ID: "cluster", Err: fmt.Errorf("cluster destroy failed")}},
			}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) { return nil, nil }
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		// When PrepareLocalTeardown runs, then Teardown with continueOnError
		if _, err := prov.PrepareLocalTeardown(context.Background(), bp); err != nil {
			t.Fatalf("Expected no error from PrepareLocalTeardown, got %v", err)
		}
		result, err := prov.Teardown(context.Background(), bp, false, true)

		// Then the backend type is untouched and terraform is deferred
		if err != nil {
			t.Fatalf("Expected continueOnError to absorb per-component failure, got %v", err)
		}
		if backendType != "gcs" {
			t.Errorf("Expected PrepareLocalTeardown to leave terraform.backend.type=gcs untouched, got %q", backendType)
		}
		if !result.TerraformDeferred {
			t.Error("Expected TerraformDeferred=true when non-backend destroy left a failure, even after PrepareLocalTeardown ran")
		}
	})

	t.Run("ContinueCollectsLocalBackendFailures", func(t *testing.T) {
		// Given a local backend and a stack that reports one failure
		mocks := setupProvisionerMocks(t)
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
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, continueOnError bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			if !continueOnError {
				t.Error("Expected continueOnError=true to be propagated to stack")
			}
			return terraforminfra.DestroyOutcome{
				Destroyed: []string{"vpc"},
				Failed:    []terraforminfra.ComponentFailure{{ID: "iam", Err: fmt.Errorf("permission denied")}},
			}, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		// When Teardown runs with continueOnError
		result, err := prov.Teardown(context.Background(), createTestBlueprint(), true, true)

		// Then the failure surfaces in result.Failed without aborting
		if err != nil {
			t.Fatalf("Expected continueOnError to absorb per-component failure, got %v", err)
		}
		if len(result.Failed) != 1 || result.Failed[0].ID != "iam" {
			t.Errorf("Expected failure list to contain iam, got %v", result.Failed)
		}
		if len(result.Destroyed) != 1 || result.Destroyed[0] != "vpc" {
			t.Errorf("Expected Destroyed=[vpc], got %v", result.Destroyed)
		}
	})

	t.Run("ContinueDefersTerraformWhenKustomizeFailsAndClusterReachable", func(t *testing.T) {
		// Given kustomize uninstall fails while the cluster stays reachable
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
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
		mockCH.SetFunc = func(_ string, _ any) error { return nil }
		mocks.KubernetesManager.DeleteBlueprintFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
			return fmt.Errorf("destroy aborted: timeout waiting for load balancer teardown")
		}
		mockStack := terraforminfra.NewMockStack()
		migrateCalled := false
		destroyAllCalls := 0
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalls++
			return terraforminfra.DestroyOutcome{Destroyed: []string{fmt.Sprintf("pass%d", destroyAllCalls)}}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			migrateCalled = true
			return nil, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{
			TerraformStack:    mockStack,
			KubernetesManager: mocks.KubernetesManager,
		})

		// When Teardown runs the full destroy with continueOnError
		result, err := prov.Teardown(context.Background(), bp, false, true)

		// Then terraform is deferred and no DestroyAll runs
		if err != nil {
			t.Fatalf("Expected continueOnError to absorb kustomize failure, got %v", err)
		}
		if !result.TerraformDeferred {
			t.Error("Expected TerraformDeferred=true when kustomize failed while the cluster is reachable")
		}
		if migrateCalled {
			t.Error("MigrateState must not run when terraform is deferred")
		}
		if destroyAllCalls != 0 {
			t.Errorf("Expected no terraform DestroyAll calls, got %d", destroyAllCalls)
		}
		var foundKustomize bool
		for _, f := range result.Failed {
			if f.ID == provisioner.KustomizeFailureID {
				foundKustomize = true
			}
		}
		if !foundKustomize {
			t.Errorf("Expected kustomize failure to remain in result.Failed, got %v", result.Failed)
		}
	})

	t.Run("ContinueDefersTerraformWhenKustomizeFailsAndClusterUnreachable", func(t *testing.T) {
		// Given kustomize uninstall fails and the cluster is unreachable
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
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
		mockCH.SetFunc = func(_ string, _ any) error { return nil }
		mocks.KubernetesManager.DeleteBlueprintFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
			return fmt.Errorf("cluster API unreachable")
		}
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(_ context.Context, _ string, _ func(string), _ ...string) error {
			return fmt.Errorf("cluster API unreachable")
		}
		mockStack := terraforminfra.NewMockStack()
		migrateCalled := false
		destroyAllCalls := 0
		mockStack.DestroyAllFunc = func(_ *blueprintv1alpha1.Blueprint, _ bool, _ ...string) (terraforminfra.DestroyOutcome, error) {
			destroyAllCalls++
			return terraforminfra.DestroyOutcome{Destroyed: []string{fmt.Sprintf("pass%d", destroyAllCalls)}}, nil
		}
		mockStack.MigrateStateFunc = func(_ *blueprintv1alpha1.Blueprint) ([]string, error) {
			migrateCalled = true
			return nil, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{
			TerraformStack:    mockStack,
			KubernetesManager: mocks.KubernetesManager,
		})

		// When Teardown runs the full destroy with continueOnError
		result, err := prov.Teardown(context.Background(), bp, false, true)

		// Then terraform is deferred and no DestroyAll runs
		if err != nil {
			t.Fatalf("Expected continueOnError to absorb kustomize failure, got %v", err)
		}
		if !result.TerraformDeferred {
			t.Error("Expected TerraformDeferred=true even when the cluster is unreachable")
		}
		if migrateCalled {
			t.Error("MigrateState must not run when terraform is deferred")
		}
		if destroyAllCalls != 0 {
			t.Errorf("Expected no terraform DestroyAll calls, got %d", destroyAllCalls)
		}
		var foundKustomize bool
		for _, f := range result.Failed {
			if f.ID == provisioner.KustomizeFailureID {
				foundKustomize = true
			}
		}
		if !foundKustomize {
			t.Errorf("Expected kustomize failure to remain in result.Failed, got %v", result.Failed)
		}
	})
}

func TestProvisioner_TeardownComponent(t *testing.T) {
	bpWithBackend := func() *blueprintv1alpha1.Blueprint {
		return &blueprintv1alpha1.Blueprint{
			Backend:  "backend",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}
	}

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// Given a provisioner
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		// When TeardownComponent is called with a nil blueprint
		_, err := prov.TeardownComponent(context.Background(), nil, "cluster")

		// Then a blueprint-not-provided error is returned
		if err == nil {
			t.Fatal("Expected error for nil blueprint, got nil")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("RefusesAnyBackendComponentOnRemoteBackend", func(t *testing.T) {
		for _, backendType := range []string{"kubernetes", "s3", "azurerm"} {
			t.Run(backendType, func(t *testing.T) {
				// Given a remote backend of the given type
				mocks := setupProvisionerMocks(t)
				mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
				mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
					if key == "terraform.backend.type" {
						return backendType
					}
					if len(defaultValue) > 0 {
						return defaultValue[0]
					}
					return ""
				}
				destroyCalled := false
				mockStack := terraforminfra.NewMockStack()
				mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) (bool, error) {
					destroyCalled = true
					return false, nil
				}
				prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

				// When TeardownComponent targets the backend component
				_, err := prov.TeardownComponent(context.Background(), bpWithBackend(), "backend")
				// Then it refuses with a full-teardown remediation
				if err == nil {
					t.Fatal("Expected refusal error, got nil")
				}
				if !strings.Contains(err.Error(), "backend component") {
					t.Errorf("Expected refusal message naming the backend component, got: %v", err)
				}
				if !strings.Contains(err.Error(), backendType) {
					t.Errorf("Expected refusal message naming %s backend, got: %v", backendType, err)
				}
				if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "windsor destroy") {
					t.Errorf("Expected remediation to point at full-cycle teardown, got: %v", err)
				}
				if destroyCalled {
					t.Error("Expected Destroy NOT to be called when refusing")
				}
			})
		}
	})

	t.Run("RefusesPreBackendComponentOnRemoteBackend", func(t *testing.T) {
		// Given an s3 backend and a component declared before the named backend
		mocks := setupProvisionerMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Backend:  "cluster",
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "networking/vpc"},
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
		destroyCalled := false
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) (bool, error) {
			destroyCalled = true
			return false, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When TeardownComponent targets that component
		_, err := prov.TeardownComponent(context.Background(), bp, "networking/vpc")
		// Then it refuses and names the component
		if err == nil {
			t.Fatal("Expected refusal for a pre-backend component, got nil")
		}
		if !strings.Contains(err.Error(), "networking/vpc") {
			t.Errorf("Expected error to name the backend component, got: %v", err)
		}
		if destroyCalled {
			t.Error("Destroy must not run when refusing")
		}
	})

	t.Run("AllowsBackendComponentOnLocalBackend", func(t *testing.T) {
		// Given a local backend
		mocks := setupProvisionerMocks(t)
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
		destroyCalled := false
		mockStack := terraforminfra.NewMockStack()
		mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
			destroyCalled = true
			if componentID != "backend" {
				t.Errorf("Expected destroy for backend, got %s", componentID)
			}
			return false, nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

		// When TeardownComponent targets the backend component
		// Then Destroy runs directly
		if _, err := prov.TeardownComponent(context.Background(), bpWithBackend(), "backend"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !destroyCalled {
			t.Error("Expected Destroy to be called on local backend")
		}
	})

	t.Run("NonBackendComponentUsesDirectDestroyOnAnyBackend", func(t *testing.T) {
		for _, backendType := range []string{"local", "kubernetes", "s3", "azurerm"} {
			t.Run(backendType, func(t *testing.T) {
				// Given a backend of the given type
				mocks := setupProvisionerMocks(t)
				mockCH := mocks.ConfigHandler.(*config.MockConfigHandler)
				mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
					if key == "terraform.backend.type" {
						return backendType
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
				migrateCalled := false
				mockStack := terraforminfra.NewMockStack()
				mockStack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) error {
					migrateCalled = true
					return nil
				}
				destroyCalled := false
				mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
					destroyCalled = true
					if componentID != "cluster" {
						t.Errorf("Expected destroy for cluster, got %s", componentID)
					}
					return false, nil
				}
				prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, TerraformStack: mockStack})

				// When TeardownComponent targets a non-backend component
				// Then Destroy runs with no migration or backend override
				if _, err := prov.TeardownComponent(context.Background(), bpWithBackend(), "cluster"); err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if !destroyCalled {
					t.Error("Expected Destroy to be called")
				}
				if migrateCalled {
					t.Error("Expected MigrateComponentState NOT to be called for non-backend component")
				}
				if setCalled {
					t.Error("Expected backend.type Set NOT to be called for non-backend component")
				}
			})
		}
	})

	t.Run("WarnsAboutOrphanedLocalStateBeforeDestroying", func(t *testing.T) {
		// Given local state with resources under a componentID not in the blueprint
		mocks := setupProvisionerMocks(t)
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
		mockStack := terraforminfra.NewMockStack()
		mockStack.ListLocalStateComponentIDsFunc = func() ([]string, error) {
			return []string{"cluster-old"}, nil
		}
		mockStack.HasLocalStateWithResourcesFunc = func(componentID string) (bool, error) {
			return componentID == "cluster-old", nil
		}
		mockStack.DestroyFunc = func(_ *blueprintv1alpha1.Blueprint, _ string) (bool, error) { return false, nil }
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When TeardownComponent runs while warnings are recorded
		var err error
		warnings := captureWarnings(t, func(ctx context.Context) {
			_, err = prov.TeardownComponent(ctx, bpWithBackend(), "cluster")
		})

		// Then a warning names the orphaned componentID
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !strings.Contains(warnings, "cluster-old") {
			t.Errorf("Expected a warning naming the orphaned componentID, got: %q", warnings)
		}
	})
}
