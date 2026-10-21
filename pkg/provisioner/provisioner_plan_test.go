package provisioner

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProvisioner_Plan(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.PlanFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) error { return nil }
		opts := &Provisioner{TerraformStack: mockStack}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		err := provisioner.Plan(createTestBlueprint(), "remote/path")

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		err := provisioner.Plan(nil, "remote/path")

		if err == nil {
			t.Error("Expected error for nil blueprint")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorWhenTerraformDisabled", func(t *testing.T) {
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
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		err := provisioner.Plan(createTestBlueprint(), "remote/path")

		if err == nil {
			t.Error("Expected error when terraform is disabled")
		}
		if !strings.Contains(err.Error(), "terraform is disabled") {
			t.Errorf("Expected 'terraform is disabled' error, got: %v", err)
		}
		if provisioner.TerraformStack != nil {
			t.Error("Expected TerraformStack to remain nil when terraform is disabled")
		}
	})

	t.Run("InitializesTerraformStackLazily", func(t *testing.T) {
		// Given a provisioner with no pre-injected TerraformStack and terraform enabled
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		if provisioner.TerraformStack != nil {
			t.Error("Expected TerraformStack to be nil before Plan() is called")
		}

		// When Plan() is called (it will fail because the directory doesn't exist, but
		// the stack should still be lazily initialized before the plan runs)
		_ = provisioner.Plan(createTestBlueprint(), "remote/path")

		// Then TerraformStack should be initialized regardless of the plan outcome
		if provisioner.TerraformStack == nil {
			t.Error("Expected TerraformStack to be initialized after Plan() when terraform is enabled")
		}
	})

	t.Run("ErrorTerraformStackPlan", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mockStack := terraforminfra.NewMockStack()
		mockStack.PlanFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) error {
			return fmt.Errorf("terraform stack plan failed")
		}
		opts := &Provisioner{TerraformStack: mockStack}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		err := provisioner.Plan(createTestBlueprint(), "remote/path")

		if err == nil {
			t.Error("Expected error for terraform stack plan failure")
		}
		if !strings.Contains(err.Error(), "failed to run terraform plan for") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestProvisioner_PlanKustomization(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a provisioner with a mock flux stack
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		// When PlanKustomization is called with a valid blueprint
		err := provisioner.PlanKustomization(createTestBlueprint(), "test-kustomization")

		// Then no error is returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("MaterializesCrdLayerForTheStack", func(t *testing.T) {
		// Given a blueprint whose only kustomize layer is the crds: list
		mocks := setupProvisionerMocks(t)
		var planned *blueprintv1alpha1.Blueprint
		mocks.FluxStack.PlanFunc = func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
			planned = blueprint
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		// When planning the crds component directly
		if err := provisioner.PlanKustomization(&blueprintv1alpha1.Blueprint{Crds: []string{"cert-manager-1.16.2"}}, "crds"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the synthesized crds kustomization is present so the stack can resolve "crds"
		if planned == nil || len(planned.Kustomizations) != 1 || planned.Kustomizations[0].Name != "crds" {
			t.Errorf("expected crds kustomization materialized for plan, got %+v", planned)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// Given a provisioner with a mock flux stack
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		// When PlanKustomization is called with a nil blueprint
		err := provisioner.PlanKustomization(nil, "test-kustomization")

		// Then an error is returned
		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
	})

	t.Run("ErrorFluxStackFails", func(t *testing.T) {
		// Given a provisioner whose flux stack returns an error
		mocks := setupProvisionerMocks(t)
		mocks.FluxStack.PlanFunc = func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
			return fmt.Errorf("flux diff failed")
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		// When PlanKustomization is called
		err := provisioner.PlanKustomization(createTestBlueprint(), "test-kustomization")

		// Then the error from the flux stack is returned
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "flux diff failed") {
			t.Errorf("expected flux error in message, got %v", err)
		}
	})
}

func TestProvisioner_PlanTerraformAll(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		err := p.PlanTerraformAll(nil)

		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
	})

	t.Run("DelegatestoStackPlanAll", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		called := false
		mocks.TerraformStack.(*terraforminfra.MockStack).PlanAllFunc = func(bp *blueprintv1alpha1.Blueprint) error {
			called = true
			return nil
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		err := p.PlanTerraformAll(createTestBlueprint())

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !called {
			t.Error("expected TerraformStack.PlanAll to be called")
		}
	})
}

func TestProvisioner_PlanTerraformComponentSummary(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		_, err := p.PlanTerraformComponentSummary(nil, "cluster")

		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
	})

	t.Run("PlansOnlyRequestedComponent", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		called := []string{}
		mocks.TerraformStack.(*terraforminfra.MockStack).PlanComponentSummaryFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) terraforminfra.TerraformComponentPlan {
			called = append(called, componentID)
			return terraforminfra.TerraformComponentPlan{ComponentID: componentID, Add: 2}
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		result, err := p.PlanTerraformComponentSummary(createTestBlueprint(), "cluster")

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result.ComponentID != "cluster" || result.Add != 2 {
			t.Errorf("unexpected result: %+v", result)
		}
		if len(called) != 1 || called[0] != "cluster" {
			t.Errorf("expected exactly one call for cluster, got %v", called)
		}
	})
}

func TestProvisioner_PlanKustomizeComponentSummary(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		_, err := p.PlanKustomizeComponentSummary(nil, "flux-system")

		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
	})

	t.Run("PlansOnlyRequestedKustomization", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		called := []string{}
		mocks.FluxStack.PlanComponentSummaryFunc = func(bp *blueprintv1alpha1.Blueprint, name string) fluxinfra.KustomizePlan {
			called = append(called, name)
			return fluxinfra.KustomizePlan{Name: name, Added: 5, IsNew: true}
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		result, err := p.PlanKustomizeComponentSummary(createTestBlueprint(), "flux-system")

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result.Name != "flux-system" || result.Added != 5 {
			t.Errorf("unexpected result: %+v", result)
		}
		if len(called) != 1 || called[0] != "flux-system" {
			t.Errorf("expected exactly one call for flux-system, got %v", called)
		}
	})
}

func TestProvisioner_PlanTerraformSummary(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		summary, err := p.PlanTerraformSummary(nil)

		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
		if summary != nil {
			t.Errorf("expected nil summary, got %v", summary)
		}
	})

	t.Run("ReturnsTerraformResultsOnly", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.TerraformStack.(*terraforminfra.MockStack).PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) []terraforminfra.TerraformComponentPlan {
			return []terraforminfra.TerraformComponentPlan{{ComponentID: "cluster", Add: 3}}
		}
		mocks.FluxStack.PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, []string) {
			t.Fatal("FluxStack.PlanSummary should not be called by PlanTerraformSummary")
			return nil, nil
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		summary, err := p.PlanTerraformSummary(createTestBlueprint())

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(summary.Terraform) != 1 || summary.Terraform[0].ComponentID != "cluster" {
			t.Errorf("expected terraform result for cluster, got %v", summary.Terraform)
		}
		if summary.Kustomize != nil {
			t.Errorf("expected nil kustomize slice, got %v", summary.Kustomize)
		}
	})
}

func TestProvisioner_PlanKustomizeSummary(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		summary, err := p.PlanKustomizeSummary(nil)

		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
		if summary != nil {
			t.Errorf("expected nil summary, got %v", summary)
		}
	})

	t.Run("ReturnsKustomizeResultsOnly", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.TerraformStack.(*terraforminfra.MockStack).PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) []terraforminfra.TerraformComponentPlan {
			t.Fatal("TerraformStack.PlanSummary should not be called by PlanKustomizeSummary")
			return nil
		}
		mocks.FluxStack.PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, []string) {
			return []fluxinfra.KustomizePlan{{Name: "flux-system", Added: 10, IsNew: true}}, []string{"hint1"}
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		summary, err := p.PlanKustomizeSummary(createTestBlueprint())

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(summary.Kustomize) != 1 || summary.Kustomize[0].Name != "flux-system" {
			t.Errorf("expected kustomize result for flux-system, got %v", summary.Kustomize)
		}
		if summary.Terraform != nil {
			t.Errorf("expected nil terraform slice, got %v", summary.Terraform)
		}
		if len(summary.Hints) != 1 || summary.Hints[0] != "hint1" {
			t.Errorf("expected hints, got %v", summary.Hints)
		}
	})
}

func TestProvisioner_PlanAll(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		// Given a properly configured provisioner
		mocks := setupProvisionerMocks(t)
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		// When PlanAll is called with nil blueprint
		summary, err := p.PlanAll(nil)

		// Then an error is returned and summary is nil
		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
		if summary != nil {
			t.Errorf("expected nil summary, got %v", summary)
		}
	})

	t.Run("ReturnsTerraformAndKustomizeResults", func(t *testing.T) {
		// Given stacks that return non-empty summaries
		mocks := setupProvisionerMocks(t)
		mocks.TerraformStack.(*terraforminfra.MockStack).PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) []terraforminfra.TerraformComponentPlan {
			return []terraforminfra.TerraformComponentPlan{{ComponentID: "cluster", Add: 3}}
		}
		mocks.FluxStack.PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, []string) {
			return []fluxinfra.KustomizePlan{{Name: "flux-system", Added: 10, IsNew: true}}, nil
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		// When PlanAll is called
		summary, err := p.PlanAll(createTestBlueprint())

		// Then both layers are present in the summary
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(summary.Terraform) != 1 || summary.Terraform[0].ComponentID != "cluster" {
			t.Errorf("expected terraform result for cluster, got %v", summary.Terraform)
		}
		if len(summary.Kustomize) != 1 || summary.Kustomize[0].Name != "flux-system" {
			t.Errorf("expected kustomize result for flux-system, got %v", summary.Kustomize)
		}
	})

	t.Run("OmitsTerraformWhenDisabled", func(t *testing.T) {
		// Given terraform.enabled=false in config
		mocks := setupProvisionerMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return false
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		mocks.FluxStack.PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, []string) {
			return []fluxinfra.KustomizePlan{{Name: "flux-system"}}, nil
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		// When PlanAll is called
		summary, err := p.PlanAll(createTestBlueprint())

		// Then Terraform slice is nil and Kustomize is populated
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if summary.Terraform != nil {
			t.Errorf("expected nil terraform slice when disabled, got %v", summary.Terraform)
		}
		if len(summary.Kustomize) == 0 {
			t.Errorf("expected kustomize results, got none")
		}
	})
}

func TestProvisioner_PlanDestroyTerraformSummary(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		summary, err := p.PlanDestroyTerraformSummary(nil)
		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
		if summary != nil {
			t.Errorf("expected nil summary, got %v", summary)
		}
	})

	t.Run("InvokesTerraformDestroyOnly", func(t *testing.T) {
		// PlanDestroyTerraformSummary must not touch the flux side — its
		// counterpart on the apply path is the same gate. Verify by failing the
		// test if FluxStack.PlanDestroySummary is called.
		mocks := setupProvisionerMocks(t)
		mocks.TerraformStack.(*terraforminfra.MockStack).PlanDestroySummaryFunc = func(bp *blueprintv1alpha1.Blueprint) []terraforminfra.TerraformComponentPlan {
			return []terraforminfra.TerraformComponentPlan{{ComponentID: "cluster", Destroy: 5}}
		}
		mocks.FluxStack.PlanDestroySummaryFunc = func(bp *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, error) {
			t.Fatal("FluxStack.PlanDestroySummary should not be called by PlanDestroyTerraformSummary")
			return nil, nil
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		summary, err := p.PlanDestroyTerraformSummary(createTestBlueprint())
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(summary.Terraform) != 1 || summary.Terraform[0].ComponentID != "cluster" {
			t.Errorf("expected terraform destroy result for cluster, got %v", summary.Terraform)
		}
		if summary.Kustomize != nil {
			t.Errorf("expected nil kustomize slice, got %v", summary.Kustomize)
		}
	})
}

func TestProvisioner_PlanDestroyKustomizeSummary(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		summary, err := p.PlanDestroyKustomizeSummary(nil)
		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
		if summary != nil {
			t.Errorf("expected nil summary, got %v", summary)
		}
	})

	t.Run("ReturnsKustomizeResultsOnly", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.TerraformStack.(*terraforminfra.MockStack).PlanDestroySummaryFunc = func(bp *blueprintv1alpha1.Blueprint) []terraforminfra.TerraformComponentPlan {
			t.Fatal("TerraformStack.PlanDestroySummary should not be called by PlanDestroyKustomizeSummary")
			return nil
		}
		mocks.FluxStack.PlanDestroySummaryFunc = func(bp *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, error) {
			return []fluxinfra.KustomizePlan{{Name: "monitoring", Removed: 12}}, nil
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		summary, err := p.PlanDestroyKustomizeSummary(createTestBlueprint())
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(summary.Kustomize) != 1 || summary.Kustomize[0].Name != "monitoring" {
			t.Errorf("expected kustomize destroy result for monitoring, got %v", summary.Kustomize)
		}
	})

	t.Run("PropagatesClusterErrorFromFluxStack", func(t *testing.T) {
		// The flux destroy plan errors on cluster failure (truthful — fall-back
		// would lie). The aggregator must propagate so the cmd layer can fail
		// before prompting for destroy.
		mocks := setupProvisionerMocks(t)
		mocks.FluxStack.PlanDestroySummaryFunc = func(bp *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, error) {
			return nil, fmt.Errorf("connection refused")
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		summary, err := p.PlanDestroyKustomizeSummary(createTestBlueprint())
		if err == nil || !strings.Contains(err.Error(), "connection refused") {
			t.Errorf("expected cluster error to propagate, got err=%v summary=%v", err, summary)
		}
	})

	t.Run("FlattensFluxSystemTiersBeforePlanning", func(t *testing.T) {
		// Regression: a blueprint that declares its kustomizations via FluxSystems
		// (not top-level kustomize:) must still preview every compiled install/
		// resources tier. The plan iterates blueprint.Kustomizations, so the
		// provisioner must pass the blueprint through withCrdLayer first — the
		// same flattening Uninstall applies before DeleteBlueprint. Without it the
		// plan saw an empty Kustomizations slice and rendered no Kustomize section
		// while the teardown still destroyed every tier.
		mocks := setupProvisionerMocks(t)
		var seen *blueprintv1alpha1.Blueprint
		mocks.FluxStack.PlanDestroySummaryFunc = func(bp *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, error) {
			seen = bp
			return nil, nil
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Crds:     []string{"cert-manager-crds"},
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name:   "pki",
					Source: "source1",
					Install: &blueprintv1alpha1.Kustomization{
						Components: []string{"controller"},
					},
				},
			},
		}
		if _, err := p.PlanDestroyKustomizeSummary(bp); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if seen == nil {
			t.Fatal("FluxStack.PlanDestroySummary was not called")
		}
		if seen.FluxSystems != nil {
			t.Errorf("expected FluxSystems flattened away, got %v", seen.FluxSystems)
		}
		names := make([]string, 0, len(seen.Kustomizations))
		for _, k := range seen.Kustomizations {
			names = append(names, k.Name)
		}
		if !slices.Contains(names, "pki-install") {
			t.Errorf("expected pki-install tier flattened into Kustomizations, got %v", names)
		}
	})
}

func TestProvisioner_PlanDestroyAll(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		summary, err := p.PlanDestroyAll(nil)
		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
		if summary != nil {
			t.Errorf("expected nil summary, got %v", summary)
		}
	})

	t.Run("AggregatesBothLayers", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.TerraformStack.(*terraforminfra.MockStack).PlanDestroySummaryFunc = func(bp *blueprintv1alpha1.Blueprint) []terraforminfra.TerraformComponentPlan {
			return []terraforminfra.TerraformComponentPlan{{ComponentID: "cluster", Destroy: 7}}
		}
		mocks.FluxStack.PlanDestroySummaryFunc = func(bp *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, error) {
			return []fluxinfra.KustomizePlan{{Name: "monitoring", Removed: 12}}, nil
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		summary, err := p.PlanDestroyAll(createTestBlueprint())
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(summary.Terraform) != 1 || summary.Terraform[0].ComponentID != "cluster" {
			t.Errorf("expected terraform destroy result for cluster, got %v", summary.Terraform)
		}
		if len(summary.Kustomize) != 1 || summary.Kustomize[0].Name != "monitoring" {
			t.Errorf("expected kustomize destroy result for monitoring, got %v", summary.Kustomize)
		}
	})

	t.Run("AbortsOnClusterFailure", func(t *testing.T) {
		// Flux returns error -> PlanDestroyAll must abort and return that error
		// so the cmd layer doesn't prompt for destroy.
		mocks := setupProvisionerMocks(t)
		mocks.FluxStack.PlanDestroySummaryFunc = func(bp *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, error) {
			return nil, fmt.Errorf("connection refused")
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		summary, err := p.PlanDestroyAll(createTestBlueprint())
		if err == nil || !strings.Contains(err.Error(), "connection refused") {
			t.Errorf("expected cluster error to propagate, got err=%v summary=%v", err, summary)
		}
	})
}

func TestProvisioner_PlanDestroyTerraformComponentSummary(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack: mocks.TerraformStack,
		})

		_, err := p.PlanDestroyTerraformComponentSummary(nil, "x")
		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
	})

	t.Run("DelegatesToTerraformStack", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.TerraformStack.(*terraforminfra.MockStack).PlanDestroyComponentSummaryFunc = func(bp *blueprintv1alpha1.Blueprint, id string) terraforminfra.TerraformComponentPlan {
			return terraforminfra.TerraformComponentPlan{ComponentID: id, Destroy: 4}
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack: mocks.TerraformStack,
		})

		got, err := p.PlanDestroyTerraformComponentSummary(createTestBlueprint(), "cluster")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ComponentID != "cluster" || got.Destroy != 4 {
			t.Errorf("expected delegated result, got %#v", got)
		}
	})
}

func TestProvisioner_PlanDestroyKustomizeComponentSummary(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			FluxStack: mocks.FluxStack,
		})

		_, err := p.PlanDestroyKustomizeComponentSummary(nil, "x")
		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
	})

	t.Run("DelegatesToFluxStack", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		mocks.FluxStack.PlanDestroyComponentSummaryFunc = func(bp *blueprintv1alpha1.Blueprint, name string) fluxinfra.KustomizePlan {
			return fluxinfra.KustomizePlan{Name: name, Removed: 8}
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			FluxStack: mocks.FluxStack,
		})

		got, err := p.PlanDestroyKustomizeComponentSummary(createTestBlueprint(), "monitoring")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "monitoring" || got.Removed != 8 {
			t.Errorf("expected delegated result, got %#v", got)
		}
	})

	t.Run("FlattensFluxSystemTiersBeforePlanning", func(t *testing.T) {
		// Regression: a blueprint that declares its kustomizations via FluxSystems
		// (not top-level kustomize:) must still preview the compiled install/
		// resources tier. The plan iterates blueprint.Kustomizations, so the
		// provisioner must pass the blueprint through withCrdLayer first — the
		// same flattening Uninstall applies before DeleteBlueprint. Without it the
		// component plan saw an empty Kustomizations slice while the teardown
		// still destroyed the tier.
		mocks := setupProvisionerMocks(t)
		var seen *blueprintv1alpha1.Blueprint
		mocks.FluxStack.PlanDestroyComponentSummaryFunc = func(bp *blueprintv1alpha1.Blueprint, name string) fluxinfra.KustomizePlan {
			seen = bp
			return fluxinfra.KustomizePlan{}
		}
		p := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{
			TerraformStack:    mocks.TerraformStack,
			FluxStack:         mocks.FluxStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
		})

		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Crds:     []string{"cert-manager-crds"},
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name:   "pki",
					Source: "source1",
					Install: &blueprintv1alpha1.Kustomization{
						Components: []string{"controller"},
					},
				},
			},
		}
		if _, err := p.PlanDestroyKustomizeComponentSummary(bp, "pki-install"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if seen == nil {
			t.Fatal("FluxStack.PlanDestroyComponentSummary was not called")
		}
		if seen.FluxSystems != nil {
			t.Errorf("expected FluxSystems flattened away, got %v", seen.FluxSystems)
		}
		names := make([]string, 0, len(seen.Kustomizations))
		for _, k := range seen.Kustomizations {
			names = append(names, k.Name)
		}
		if !slices.Contains(names, "pki-install") {
			t.Errorf("expected pki-install tier flattened into Kustomizations, got %v", names)
		}
	})
}
