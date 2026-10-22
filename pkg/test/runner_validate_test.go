package test

import (
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestTestRunner_validateBlueprint(t *testing.T) {
	t.Run("DetectsDuplicateTerraformComponents", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Path: "cluster/eks"},
				{Name: "cluster", Path: "cluster/gke"},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) == 0 {
			t.Error("Expected error for duplicate terraform component")
		}
		if !strings.Contains(errors[0], "duplicate terraform component ID") {
			t.Errorf("Expected duplicate component error, got: %v", errors)
		}
	})

	t.Run("DetectsDuplicateTerraformComponentsByPath", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/eks"},
				{Path: "cluster/eks"},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) == 0 {
			t.Error("Expected error for duplicate terraform component by path")
		}
	})

	t.Run("DetectsDuplicateKustomizations", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system", Path: "flux"},
				{Name: "flux-system", Path: "flux2"},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) == 0 {
			t.Error("Expected error for duplicate kustomization")
		}
		if !strings.Contains(errors[0], "duplicate kustomization name") {
			t.Errorf("Expected duplicate kustomization error, got: %v", errors)
		}
	})

	t.Run("DetectsDuplicateKustomizationAgainstFluxCompiledTier", func(t *testing.T) {
		// Given a plain kustomization whose name collides with a flux: system's compiled tier name
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "policy-install", Path: "app"},
			},
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name:    "policy",
					Path:    "policy",
					Install: &blueprintv1alpha1.Kustomization{Components: []string{"kyverno"}},
				},
			},
		}

		// When validating
		errors := runner.validateBlueprint(blueprint)

		// Then the collision is caught even though one side is compiled from a flux: system
		if len(errors) == 0 {
			t.Error("Expected error for duplicate kustomization name against flux-compiled tier")
		}
		if !strings.Contains(errors[0], "duplicate kustomization name") {
			t.Errorf("Expected duplicate kustomization error, got: %v", errors)
		}
	})

	t.Run("DetectsDuplicateKustomizationComponents", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:       "app",
					Path:       "app",
					Components: []string{"base", "monitoring", "base"},
				},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) == 0 {
			t.Error("Expected error for duplicate kustomization component")
		}
		if !strings.Contains(errors[0], "duplicate component") {
			t.Errorf("Expected duplicate component error, got: %v", errors)
		}
	})

	t.Run("DetectsCircularDependenciesInTerraform", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "a", Path: "a", DependsOn: []string{"b"}},
				{Name: "b", Path: "b", DependsOn: []string{"a"}},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) == 0 {
			t.Error("Expected error for circular dependency")
		}
		if !strings.Contains(errors[0], "circular dependency") {
			t.Errorf("Expected circular dependency error, got: %v", errors)
		}
	})

	t.Run("DetectsCircularDependenciesInKustomize", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "a", Path: "a", DependsOn: []string{"b"}},
				{Name: "b", Path: "b", DependsOn: []string{"a"}},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) == 0 {
			t.Error("Expected error for circular dependency")
		}
		if !strings.Contains(errors[0], "circular dependency") {
			t.Errorf("Expected circular dependency error, got: %v", errors)
		}
	})

	t.Run("DetectsCircularDependencyAcrossFluxTiers", func(t *testing.T) {
		// Given a plain kustomization and a flux: system install tier depending on each other
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "a", Path: "a", DependsOn: []string{"policy-install"}},
			},
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name:      "policy",
					Path:      "policy",
					DependsOn: []string{"a"},
					Install:   &blueprintv1alpha1.Kustomization{Components: []string{"kyverno"}},
				},
			},
		}

		// When validating
		errors := runner.validateBlueprint(blueprint)

		// Then the cycle is caught even though it spans a plain kustomization and a compiled flux tier
		if len(errors) == 0 {
			t.Error("Expected error for circular dependency across flux tiers")
		}
		if !strings.Contains(errors[0], "circular dependency") {
			t.Errorf("Expected circular dependency error, got: %v", errors)
		}
	})

	t.Run("DetectsInvalidTerraformDependencies", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "a", Path: "a", DependsOn: []string{"nonexistent"}},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) == 0 {
			t.Error("Expected error for invalid dependency")
		}
		if !strings.Contains(errors[0], "depends on non-existent component") {
			t.Errorf("Expected invalid dependency error, got: %v", errors)
		}
	})

	t.Run("DetectsInvalidKustomizeDependencies", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app", Path: "app", DependsOn: []string{"nonexistent"}},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) == 0 {
			t.Error("Expected error for invalid dependency")
		}
		if !strings.Contains(errors[0], "depends on non-existent kustomization") {
			t.Errorf("Expected invalid dependency error, got: %v", errors)
		}
	})

	t.Run("DetectsInvalidDependencyFromFluxResourcesVariant", func(t *testing.T) {
		// Given a flux: system resources variant depending on a nonexistent kustomization
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name: "policy",
					Path: "policy",
					Resources: []blueprintv1alpha1.FluxVariant{
						{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"resource-limits"}, DependsOn: []string{"nonexistent"}}},
					},
				},
			},
		}

		// When validating
		errors := runner.validateBlueprint(blueprint)

		// Then the invalid dependency is caught even though it originates from a compiled flux tier
		if len(errors) == 0 {
			t.Error("Expected error for invalid dependency from flux resources variant")
		}
		if !strings.Contains(errors[0], "depends on non-existent kustomization") {
			t.Errorf("Expected invalid dependency error, got: %v", errors)
		}
	})

	t.Run("AcceptsCrdLayerDependency", func(t *testing.T) {
		// Given a blueprint with a crds: layer and a root wired to it by the barrier
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Crds: []string{"cert-manager-1.16.2"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "cert-manager", Path: "pki/cert-manager", DependsOn: []string{"crds"}},
			},
		}

		// When validating
		errors := runner.validateBlueprint(blueprint)

		// Then the synthesized crds layer is a valid dependency target, not flagged as missing
		if len(errors) != 0 {
			t.Errorf("Expected no errors for crds dependency, got: %v", errors)
		}
	})

	t.Run("AcceptsDependencyOnFluxCompiledTier", func(t *testing.T) {
		// Given a plain kustomization depending on a name compiled from a flux: system
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name:    "policy",
					Path:    "policy",
					Install: &blueprintv1alpha1.Kustomization{Components: []string{"kyverno"}},
				},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app", Path: "app", DependsOn: []string{"policy-install"}},
			},
		}

		// When validating
		errors := runner.validateBlueprint(blueprint)

		// Then the compiled flux tier name is a valid dependency target, not flagged as missing
		if len(errors) != 0 {
			t.Errorf("Expected no errors for dependency on flux-compiled tier, got: %v", errors)
		}
	})

	t.Run("PassesValidBlueprint", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Path: "cluster/eks"},
				{Name: "network", Path: "network/vpc", DependsOn: []string{"cluster"}},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app", Path: "app", Components: []string{"base", "monitoring"}},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) > 0 {
			t.Errorf("Expected no errors for valid blueprint, got: %v", errors)
		}
	})
}
