package test

import (
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestTestRunner_matchBlueprint(t *testing.T) {
	t.Run("ReturnsNoDiffsWhenExpectationsMet", func(t *testing.T) {
		// Given a blueprint matching expectations
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Path: "cluster/eks", Source: "core"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system", Path: "flux", Components: []string{"base", "sync"}},
			},
		}

		expect := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system"},
			},
		}

		// When matching the blueprint
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("MatchesCrdReferences", func(t *testing.T) {
		// Given a blueprint whose crds: layer carries a reference
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{{Name: "core", Crds: []string{"cert-manager-1.16.2", "gateway-api-1.5.1"}}},
		}

		// When the expectation asserts a present and an absent reference (source-agnostic)
		present := runner.matchBlueprint(blueprint, &blueprintv1alpha1.Blueprint{Crds: []string{"cert-manager-1.16.2"}})
		absent := runner.matchBlueprint(blueprint, &blueprintv1alpha1.Blueprint{Crds: []string{"missing-crd"}})

		// Then a present reference passes and a missing one is reported
		if len(present) != 0 {
			t.Errorf("Expected no diffs for present crd, got: %v", present)
		}
		if len(absent) != 1 || !strings.Contains(absent[0], "crd reference not found: missing-crd") {
			t.Errorf("Expected missing-crd diff, got: %v", absent)
		}
	})

	t.Run("ReturnsDiffsWhenTerraformComponentNotFound", func(t *testing.T) {
		// Given a blueprint missing expected component
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{}

		expect := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "missing-component"},
			},
		}

		// When matching the blueprint
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then diffs should indicate missing component
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenTerraformComponentNotFoundByPath", func(t *testing.T) {
		// Given a blueprint missing expected component matched by path
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{}

		expect := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "missing/path"},
			},
		}

		// When matching the blueprint
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then diffs should use path as identifier
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenKustomizationNotFound", func(t *testing.T) {
		// Given a blueprint missing expected kustomization
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{}

		expect := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "missing-kustomization"},
			},
		}

		// When matching the blueprint
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then diffs should indicate missing kustomization
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ChecksPropertyMismatchesWhenComponentFound", func(t *testing.T) {
		// Given a blueprint with component having wrong properties
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Source: "wrong-source"},
			},
		}

		expect := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Source: "expected-source"},
			},
		}

		// When matching the blueprint
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then diffs should indicate property mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ChecksKustomizationPropertyMismatchesWhenFound", func(t *testing.T) {
		// Given a blueprint with kustomization having wrong properties
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system", Path: "wrong-path"},
			},
		}

		expect := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system", Path: "expected-path"},
			},
		}

		// When matching the blueprint
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then diffs should indicate property mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsNoDiffsWhenExpectIsEmpty", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster"},
			},
		}

		expect := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{},
			Kustomizations:      []blueprintv1alpha1.Kustomization{},
		}

		diffs := runner.matchBlueprint(blueprint, expect)

		if len(diffs) != 0 {
			t.Errorf("Expected no diffs when expect is empty, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffsWhenFluxSystemNotFound", func(t *testing.T) {
		// Given a blueprint missing the expected flux: system
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{}

		expect := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{Name: "missing-system"},
			},
		}

		// When matching the blueprint
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then diffs should indicate the missing system
		if len(diffs) != 1 || !strings.Contains(diffs[0], "flux system not found: missing-system") {
			t.Errorf("Expected flux system not found diff, got: %v", diffs)
		}
	})

	t.Run("MatchesFluxSystemInstallAndNamedResourcesVariant", func(t *testing.T) {
		// Given a composed blueprint carrying a flux: system with an install tier and a named resources variant
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name:      "cert-manager",
					DependsOn: []string{"policy-resources"},
					Install:   &blueprintv1alpha1.Kustomization{Components: []string{"cert-manager"}},
					Resources: []blueprintv1alpha1.FluxVariant{
						{Kustomization: blueprintv1alpha1.Kustomization{Name: "wildcard", Components: []string{"private-issuer/wildcard"}}},
					},
				},
			},
		}

		// When the expectation asserts the system in the author's own vocabulary — name, dependsOn,
		// install components, and a named resources variant's components
		expect := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name:      "cert-manager",
					DependsOn: []string{"policy-resources"},
					Install:   &blueprintv1alpha1.Kustomization{Components: []string{"cert-manager"}},
					Resources: []blueprintv1alpha1.FluxVariant{
						{Kustomization: blueprintv1alpha1.Kustomization{Name: "wildcard", Components: []string{"private-issuer/wildcard"}}},
					},
				},
			},
		}

		// Then no diffs should be returned
		diffs := runner.matchBlueprint(blueprint, expect)
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffsWhenFluxSystemInstallAbsent", func(t *testing.T) {
		// Given a flux: system whose install tier was excluded by a when: condition
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{Name: "cert-manager"},
			},
		}

		expect := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{Name: "cert-manager", Install: &blueprintv1alpha1.Kustomization{}},
			},
		}

		// When matching the blueprint
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then diffs should indicate the install tier is absent
		if len(diffs) != 1 || !strings.Contains(diffs[0], "flux[cert-manager].install: expected present, got absent") {
			t.Errorf("Expected install-absent diff, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffsWhenFluxSystemResourcesVariantNotFound", func(t *testing.T) {
		// Given a flux: system whose expected resources variant was gated off by when:
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{Name: "cert-manager"},
			},
		}

		expect := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name: "cert-manager",
					Resources: []blueprintv1alpha1.FluxVariant{
						{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"private-issuer/ca"}}},
					},
				},
			},
		}

		// When matching the blueprint
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then diffs should indicate the variant is missing
		if len(diffs) != 1 || !strings.Contains(diffs[0], "flux[cert-manager].resources: variant not found") {
			t.Errorf("Expected variant-not-found diff, got: %v", diffs)
		}
	})

	t.Run("ReturnsNoDiffsWhenConfigMatches", func(t *testing.T) {
		// Given a composed blueprint carrying a resolved config scope
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Config: map[string]any{
				"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 7}},
			},
		}

		// When the expectation asserts a nested subset of the config scope
		expect := &blueprintv1alpha1.Blueprint{
			Config: map[string]any{
				"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 7}},
			},
		}
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then no diffs are returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffWhenConfigValueMismatches", func(t *testing.T) {
		// Given a composed blueprint whose config scope disagrees with the expectation
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Config: map[string]any{
				"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 4}},
			},
		}

		expect := &blueprintv1alpha1.Blueprint{
			Config: map[string]any{
				"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 7}},
			},
		}

		// When matching the blueprint
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then a single diff reports the mismatched config value
		if len(diffs) != 1 || !strings.Contains(diffs[0], "config[cluster]:") {
			t.Errorf("Expected a config mismatch diff, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffWhenConfigBlockNotFound", func(t *testing.T) {
		// Given a composed blueprint that never resolved the expected config block, e.g. because
		// the contributing facet's when: evaluated to false
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{}

		expect := &blueprintv1alpha1.Blueprint{
			Config: map[string]any{
				"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 7}},
			},
		}

		// When matching the blueprint
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then a diff reports the missing config block
		if len(diffs) != 1 || !strings.Contains(diffs[0], "config[cluster]: key not found") {
			t.Errorf("Expected a config-not-found diff, got: %v", diffs)
		}
	})
}

func TestTestRunner_matchExclusions(t *testing.T) {
	t.Run("ReturnsNoDiffsWhenExcludeIsNil", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster"},
			},
		}

		diffs := runner.matchExclusions(blueprint, nil)

		if len(diffs) != 0 {
			t.Errorf("Expected no diffs when exclude is nil, got: %v", diffs)
		}
	})

	t.Run("ReturnsNoDiffsWhenExclusionsRespected", func(t *testing.T) {
		// Given a blueprint without excluded components
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster"},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "excluded-component"},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffsWhenExcludedTerraformExists", func(t *testing.T) {
		// Given a blueprint with an excluded component
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "should-not-exist"},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "should-not-exist"},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then diffs should indicate unwanted component
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenExcludedKustomizationExists", func(t *testing.T) {
		// Given a blueprint with an excluded kustomization
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "should-not-exist"},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "should-not-exist"},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then diffs should indicate unwanted kustomization
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("UsesPathWhenNameIsEmpty", func(t *testing.T) {
		// Given a blueprint with a component matched by path
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vm/colima"},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vm/colima"},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then diffs should use path as identifier
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("CatchesExcludedComponentCompiledFromFluxSystem", func(t *testing.T) {
		// Given a resources tier compiled from a flux: system carries the component a fixture excludes —
		// this is the scenario that silently passed before findKustomization searched AllKustomizations()
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name: "pki",
					Resources: []blueprintv1alpha1.FluxVariant{
						{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"private-issuer/ca"}}},
					},
				},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "pki-resources", Components: []string{"private-issuer/ca"}},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then a diff must be reported, not silently pass — and since the exclusion names both
		// the kustomization and the component, the diff asserts the component's absence rather
		// than the whole kustomization's absence (pki-resources is expected to exist)
		if len(diffs) != 1 || !strings.Contains(diffs[0], `kustomize[pki-resources].components: should not contain "private-issuer/ca"`) {
			t.Errorf("Expected excluded component diff, got: %v", diffs)
		}
	})

	t.Run("ReturnsNoDiffsWhenExcludedFluxSystemAbsent", func(t *testing.T) {
		// Given a blueprint without the excluded flux: system at all
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{}

		exclude := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{Name: "should-not-exist"},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffsWhenExcludedFluxSystemExistsEntirely", func(t *testing.T) {
		// Given a blueprint carrying a flux: system a fixture asserts must not exist at all
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{Name: "should-not-exist"},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{Name: "should-not-exist"},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then diffs should indicate the whole system should not exist
		if len(diffs) != 1 || !strings.Contains(diffs[0], "flux system should not exist: should-not-exist") {
			t.Errorf("Expected whole-system exclusion diff, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffsWhenExcludedResourcesVariantExistsWithinPresentSystem", func(t *testing.T) {
		// Given a flux: system whose install tier legitimately remains while one resources variant
		// should have been gated off by when: — the system itself must not be reported as excludable
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name:    "pki",
					Install: &blueprintv1alpha1.Kustomization{Components: []string{"trust-manager"}},
					Resources: []blueprintv1alpha1.FluxVariant{
						{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"private-issuer/ca"}}},
					},
				},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name: "pki",
					Resources: []blueprintv1alpha1.FluxVariant{
						{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"private-issuer/ca"}}},
					},
				},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then diffs should identify the specific component within the variant found by
		// its components-subset match, not the whole system or the whole variant
		if len(diffs) != 1 || !strings.Contains(diffs[0], `.components: should not contain "private-issuer/ca"`) {
			t.Errorf("Expected component-level exclusion diff, got: %v", diffs)
		}
	})

	t.Run("AllowsComponentAbsentFromOtherwiseExistingKustomization", func(t *testing.T) {
		// Given a kustomization that legitimately exists for an unrelated component, with the
		// excluded component genuinely absent — the previous whole-entry-presence check would
		// have always flagged this as a false positive merely because the kustomization exists,
		// making name+components exclusions unusable against a shared entry
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "pki-resources", Components: []string{"other-issuer/ca"}},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "pki-resources", Components: []string{"private-issuer/ca"}},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then no diff is reported — the excluded component is genuinely absent
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReportsOnlyTheExcludedComponentAmongOthers", func(t *testing.T) {
		// Given a kustomization with several components, only one of which is excluded
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "pki-resources", Components: []string{"other-issuer/ca", "private-issuer/ca"}},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "pki-resources", Components: []string{"private-issuer/ca"}},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then only the excluded component is flagged, naming both the kustomization and component
		if len(diffs) != 1 || !strings.Contains(diffs[0], `kustomize[pki-resources].components: should not contain "private-issuer/ca"`) {
			t.Errorf("Expected a single component-level diff, got: %v", diffs)
		}
	})

	t.Run("AllowsComponentAbsentFromOtherwiseExistingFluxInstallTier", func(t *testing.T) {
		// Given a flux system's install tier that legitimately exists for an unrelated component,
		// with the excluded component genuinely absent
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name:    "telemetry",
					Install: &blueprintv1alpha1.Kustomization{Components: []string{"logs"}},
				},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name:    "telemetry",
					Install: &blueprintv1alpha1.Kustomization{Components: []string{"prometheus"}},
				},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then no diff is reported — the excluded component is genuinely absent from the install
		// tier, which is itself expected to exist
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReportsExcludedComponentPresentInFluxInstallTier", func(t *testing.T) {
		// Given a flux system's install tier that still carries the excluded component
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name:    "telemetry",
					Install: &blueprintv1alpha1.Kustomization{Components: []string{"logs", "prometheus"}},
				},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name:    "telemetry",
					Install: &blueprintv1alpha1.Kustomization{Components: []string{"prometheus"}},
				},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then the diff names the install tier and the specific component, not "should not exist"
		if len(diffs) != 1 || !strings.Contains(diffs[0], `flux[telemetry].install.components: should not contain "prometheus"`) {
			t.Errorf("Expected a component-level diff, got: %v", diffs)
		}
	})

	t.Run("ReportsExcludedComponentPresentInNamedResourcesVariant", func(t *testing.T) {
		// Given a named resources variant that still carries the excluded component alongside
		// another one
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name: "pki",
					Resources: []blueprintv1alpha1.FluxVariant{
						{Kustomization: blueprintv1alpha1.Kustomization{Name: "extra", Components: []string{"other-issuer/ca", "private-issuer/ca"}}},
					},
				},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name: "pki",
					Resources: []blueprintv1alpha1.FluxVariant{
						{Kustomization: blueprintv1alpha1.Kustomization{Name: "extra", Components: []string{"private-issuer/ca"}}},
					},
				},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then the diff names the variant and the specific component, not the whole variant
		if len(diffs) != 1 || !strings.Contains(diffs[0], `flux[pki].resources[extra].components: should not contain "private-issuer/ca"`) {
			t.Errorf("Expected a component-level diff, got: %v", diffs)
		}
	})

	t.Run("AllowsComponentAbsentFromNamedResourcesVariant", func(t *testing.T) {
		// Given a named resources variant that legitimately exists for an unrelated component
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name: "pki",
					Resources: []blueprintv1alpha1.FluxVariant{
						{Kustomization: blueprintv1alpha1.Kustomization{Name: "extra", Components: []string{"other-issuer/ca"}}},
					},
				},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name: "pki",
					Resources: []blueprintv1alpha1.FluxVariant{
						{Kustomization: blueprintv1alpha1.Kustomization{Name: "extra", Components: []string{"private-issuer/ca"}}},
					},
				},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then no diff is reported — the excluded component is genuinely absent from the variant,
		// which is itself expected to exist
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReportsExcludedComponentPresentInUnnamedResourcesVariant", func(t *testing.T) {
		// Given an unnamed resources variant located by findFluxVariant's components-subset
		// match, which carries an extra, unrelated component alongside the excluded one — the
		// subset match already guarantees the excluded component is present, so this must not
		// fall back to flagging the whole variant as unexpected
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name: "pki",
					Resources: []blueprintv1alpha1.FluxVariant{
						{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"other-issuer/ca", "private-issuer/ca"}}},
					},
				},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name: "pki",
					Resources: []blueprintv1alpha1.FluxVariant{
						{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"private-issuer/ca"}}},
					},
				},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then the diff names the specific component, not "variant should not exist"
		if len(diffs) != 1 || !strings.Contains(diffs[0], `.components: should not contain "private-issuer/ca"`) {
			t.Errorf("Expected a component-level diff, got: %v", diffs)
		}
		if len(diffs) == 1 && strings.Contains(diffs[0], "variant should not exist") {
			t.Errorf("Expected a narrowed diff, not whole-variant-absence, got: %v", diffs)
		}
	})

	t.Run("ReportsExcludedConfigValuePresent", func(t *testing.T) {
		// Given a composed blueprint whose config scope still carries the excluded value, e.g.
		// because the contributing facet's when: was not actually forced off
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Config: map[string]any{
				"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 7}},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			Config: map[string]any{
				"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 7}},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then a diff reports the config value should not match
		if len(diffs) != 1 || !strings.Contains(diffs[0], "config[cluster] should not match:") {
			t.Errorf("Expected a config exclusion diff, got: %v", diffs)
		}
	})

	t.Run("ReturnsNoDiffsWhenExcludedConfigValueAbsent", func(t *testing.T) {
		// Given a composed blueprint whose config scope no longer matches the excluded value
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Config: map[string]any{
				"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 4}},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			Config: map[string]any{
				"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 7}},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then no diffs are returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})
}

func TestTestRunner_findTerraformComponent(t *testing.T) {
	t.Run("FindsByName", func(t *testing.T) {
		// Given a blueprint with named components
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Path: "cluster/eks"},
			},
		}

		// When finding by name
		found := runner.findTerraformComponent(blueprint, blueprintv1alpha1.TerraformComponent{Name: "cluster"})

		// Then component should be found
		if found == nil {
			t.Error("Expected component to be found")
		}
	})

	t.Run("FindsByPath", func(t *testing.T) {
		// Given a blueprint with path-only components
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/eks"},
			},
		}

		// When finding by path
		found := runner.findTerraformComponent(blueprint, blueprintv1alpha1.TerraformComponent{Path: "cluster/eks"})

		// Then component should be found
		if found == nil {
			t.Error("Expected component to be found")
		}
	})

	t.Run("ReturnsNilWhenNotFound", func(t *testing.T) {
		// Given an empty blueprint
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{}

		// When finding nonexistent component
		found := runner.findTerraformComponent(blueprint, blueprintv1alpha1.TerraformComponent{Name: "nonexistent"})

		// Then nil should be returned
		if found != nil {
			t.Error("Expected nil for nonexistent component")
		}
	})
}

func TestTestRunner_findKustomization(t *testing.T) {
	t.Run("FindsByName", func(t *testing.T) {
		// Given a blueprint with kustomizations
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system"},
			},
		}

		// When finding by name
		found := runner.findKustomization(blueprint, blueprintv1alpha1.Kustomization{Name: "flux-system"})

		// Then kustomization should be found
		if found == nil {
			t.Error("Expected kustomization to be found")
		}
	})

	t.Run("ReturnsNilWhenNotFound", func(t *testing.T) {
		// Given an empty blueprint
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{}

		// When finding nonexistent kustomization
		found := runner.findKustomization(blueprint, blueprintv1alpha1.Kustomization{Name: "nonexistent"})

		// Then nil should be returned
		if found != nil {
			t.Error("Expected nil for nonexistent kustomization")
		}
	})

	t.Run("FindsInstallTierCompiledFromFluxSystem", func(t *testing.T) {
		// Given a blueprint whose only kustomization comes from a flux: system's install tier
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name:    "cert-manager",
					Install: &blueprintv1alpha1.Kustomization{Components: []string{"cert-manager"}},
				},
			},
		}

		// When finding the compiled install tier by name
		found := runner.findKustomization(blueprint, blueprintv1alpha1.Kustomization{Name: "cert-manager-install"})

		// Then the compiled kustomization should be found
		if found == nil {
			t.Fatal("Expected compiled install tier to be found")
		}
		if !contains(found.Components, "cert-manager") {
			t.Errorf("Expected compiled install tier to carry components, got %v", found.Components)
		}
	})

	t.Run("FindsResourcesTierCompiledFromFluxSystem", func(t *testing.T) {
		// Given a blueprint whose flux: system has both an install and a resources tier
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name:    "cert-manager",
					Install: &blueprintv1alpha1.Kustomization{Components: []string{"cert-manager"}},
					Resources: []blueprintv1alpha1.FluxVariant{
						{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"private-issuer/ca"}}},
					},
				},
			},
		}

		// When finding the compiled resources tier by name
		found := runner.findKustomization(blueprint, blueprintv1alpha1.Kustomization{Name: "cert-manager-resources"})

		// Then the compiled kustomization should be found, depending on the install tier
		if found == nil {
			t.Fatal("Expected compiled resources tier to be found")
		}
		if !contains(found.DependsOn, "cert-manager-install") {
			t.Errorf("Expected compiled resources tier to depend on install tier, got %v", found.DependsOn)
		}
	})
}

func TestTestRunner_findFluxSystem(t *testing.T) {
	t.Run("FindsByName", func(t *testing.T) {
		// Given a blueprint with a flux: system
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{Name: "cert-manager"},
			},
		}

		// When finding by name
		found := runner.findFluxSystem(blueprint, blueprintv1alpha1.FluxSystem{Name: "cert-manager"})

		// Then the system should be found
		if found == nil {
			t.Error("Expected flux system to be found")
		}
	})

	t.Run("ReturnsNilWhenNotFound", func(t *testing.T) {
		// Given a blueprint without the named system
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{}

		// When finding a nonexistent system
		found := runner.findFluxSystem(blueprint, blueprintv1alpha1.FluxSystem{Name: "nonexistent"})

		// Then nil should be returned
		if found != nil {
			t.Error("Expected nil for nonexistent flux system")
		}
	})
}

func TestTestRunner_findFluxVariant(t *testing.T) {
	t.Run("FindsByName", func(t *testing.T) {
		// Given actual variants including one with a matching name
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := []blueprintv1alpha1.FluxVariant{
			{Kustomization: blueprintv1alpha1.Kustomization{Name: "wildcard", Components: []string{"private-issuer/wildcard"}}},
			{Kustomization: blueprintv1alpha1.Kustomization{Name: "ca", Components: []string{"private-issuer/ca"}}},
		}

		// When finding by name
		found := runner.findFluxVariant(actual, blueprintv1alpha1.FluxVariant{Kustomization: blueprintv1alpha1.Kustomization{Name: "ca"}})

		// Then the matching variant should be found
		if found == nil || found.Name != "ca" {
			t.Errorf("Expected to find variant \"ca\", got: %+v", found)
		}
	})

	t.Run("FindsByComponentSubsetWhenNameUnset", func(t *testing.T) {
		// Given multiple unnamed/merged variants distinguished only by their components
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := []blueprintv1alpha1.FluxVariant{
			{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"kyverno/policies", "kyverno/extra"}}},
			{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"private-issuer/ca"}}},
		}

		// When finding by a component subset
		found := runner.findFluxVariant(actual, blueprintv1alpha1.FluxVariant{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"kyverno/policies"}}})

		// Then the variant containing that component should be found
		if found == nil || !contains(found.Components, "kyverno/extra") {
			t.Errorf("Expected to find the kyverno variant, got: %+v", found)
		}
	})

	t.Run("FallsBackToSoleVariantWhenNeitherNameNorComponentsGiven", func(t *testing.T) {
		// Given exactly one actual variant and an expectation identifying it by neither name nor components
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := []blueprintv1alpha1.FluxVariant{
			{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"private-issuer/ca"}}},
		}

		// When finding with an empty expectation
		found := runner.findFluxVariant(actual, blueprintv1alpha1.FluxVariant{})

		// Then the sole variant should be returned
		if found == nil {
			t.Error("Expected the sole variant to be returned")
		}
	})

	t.Run("ReturnsNilWhenNoVariantMatches", func(t *testing.T) {
		// Given actual variants that share nothing with the expectation
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := []blueprintv1alpha1.FluxVariant{
			{Kustomization: blueprintv1alpha1.Kustomization{Name: "ca", Components: []string{"private-issuer/ca"}}},
		}

		// When finding by a name that doesn't exist
		found := runner.findFluxVariant(actual, blueprintv1alpha1.FluxVariant{Kustomization: blueprintv1alpha1.Kustomization{Name: "wildcard"}})

		// Then nil should be returned
		if found != nil {
			t.Error("Expected nil when no variant matches")
		}
	})
}

func TestTestRunner_matchTerraformComponent(t *testing.T) {
	t.Run("ReturnsNoDiffsWhenAllFieldsMatch", func(t *testing.T) {
		// Given matching component and expectation
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.TerraformComponent{
			Name:      "cluster",
			Path:      "cluster/eks",
			Source:    "core",
			DependsOn: []string{"network"},
		}

		expect := blueprintv1alpha1.TerraformComponent{
			Name:      "cluster",
			Path:      "cluster/eks",
			Source:    "core",
			DependsOn: []string{"network"},
		}

		// When matching
		diffs := runner.matchTerraformComponent(actual, expect)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffsWhenSourceMismatches", func(t *testing.T) {
		// Given component with wrong source
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.TerraformComponent{
			Name:   "cluster",
			Source: "wrong-source",
		}

		expect := blueprintv1alpha1.TerraformComponent{
			Name:   "cluster",
			Source: "expected-source",
		}

		// When matching
		diffs := runner.matchTerraformComponent(actual, expect)

		// Then diffs should indicate source mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenPathMismatches", func(t *testing.T) {
		// Given component with wrong path
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.TerraformComponent{
			Name: "cluster",
			Path: "wrong/path",
		}

		expect := blueprintv1alpha1.TerraformComponent{
			Name: "cluster",
			Path: "expected/path",
		}

		// When matching
		diffs := runner.matchTerraformComponent(actual, expect)

		// Then diffs should indicate path mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenDependencyMissing", func(t *testing.T) {
		// Given component missing a dependency
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.TerraformComponent{
			Name:      "cluster",
			DependsOn: []string{"network"},
		}

		expect := blueprintv1alpha1.TerraformComponent{
			Name:      "cluster",
			DependsOn: []string{"network", "vpc"},
		}

		// When matching
		diffs := runner.matchTerraformComponent(actual, expect)

		// Then diffs should indicate missing dependency
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("UsesPathAsIdentifierWhenNameEmpty", func(t *testing.T) {
		// Given expectation with only path
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.TerraformComponent{
			Path:   "cluster/eks",
			Source: "wrong",
		}

		expect := blueprintv1alpha1.TerraformComponent{
			Path:   "cluster/eks",
			Source: "expected",
		}

		// When matching
		diffs := runner.matchTerraformComponent(actual, expect)

		// Then diff message should use path as identifier
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsNoDiffsWhenInputsMatch", func(t *testing.T) {
		// Given component with matching inputs
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.TerraformComponent{
			Name: "cluster",
			Inputs: map[string]any{
				"region":     "us-west-2",
				"node_count": 3,
				"tags":       map[string]any{"env": "prod"},
			},
		}

		expect := blueprintv1alpha1.TerraformComponent{
			Name: "cluster",
			Inputs: map[string]any{
				"region":     "us-west-2",
				"node_count": 3,
			},
		}

		// When matching
		diffs := runner.matchTerraformComponent(actual, expect)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffsWhenInputValueMismatches", func(t *testing.T) {
		// Given component with mismatched input value
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.TerraformComponent{
			Name: "cluster",
			Inputs: map[string]any{
				"region": "us-east-1",
			},
		}

		expect := blueprintv1alpha1.TerraformComponent{
			Name: "cluster",
			Inputs: map[string]any{
				"region": "us-west-2",
			},
		}

		// When matching
		diffs := runner.matchTerraformComponent(actual, expect)

		// Then diffs should indicate value mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenInputKeyMissing", func(t *testing.T) {
		// Given component missing an input key
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.TerraformComponent{
			Name:   "cluster",
			Inputs: map[string]any{},
		}

		expect := blueprintv1alpha1.TerraformComponent{
			Name: "cluster",
			Inputs: map[string]any{
				"region": "us-west-2",
			},
		}

		// When matching
		diffs := runner.matchTerraformComponent(actual, expect)

		// Then diffs should indicate missing key
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})
}

func TestTestRunner_matchKustomization(t *testing.T) {
	t.Run("ReturnsNoDiffsWhenAllFieldsMatch", func(t *testing.T) {
		// Given matching kustomization and expectation
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name:       "flux-system",
			Path:       "flux",
			Source:     "core",
			DependsOn:  []string{"csi"},
			Components: []string{"base", "sync"},
		}

		expect := blueprintv1alpha1.Kustomization{
			Name:       "flux-system",
			Path:       "flux",
			Source:     "core",
			DependsOn:  []string{"csi"},
			Components: []string{"base"},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffsWhenPathMismatches", func(t *testing.T) {
		// Given kustomization with wrong path
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name: "flux-system",
			Path: "wrong/path",
		}

		expect := blueprintv1alpha1.Kustomization{
			Name: "flux-system",
			Path: "expected/path",
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then diffs should indicate path mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenSourceMismatches", func(t *testing.T) {
		// Given kustomization with wrong source
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name:   "flux-system",
			Source: "wrong-source",
		}

		expect := blueprintv1alpha1.Kustomization{
			Name:   "flux-system",
			Source: "expected-source",
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then diffs should indicate source mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenDependencyMissing", func(t *testing.T) {
		// Given kustomization missing a dependency
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name:      "flux-system",
			DependsOn: []string{"csi"},
		}

		expect := blueprintv1alpha1.Kustomization{
			Name:      "flux-system",
			DependsOn: []string{"csi", "pki"},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then diffs should indicate missing dependency
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenComponentMissing", func(t *testing.T) {
		// Given kustomization missing a component
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name:       "flux-system",
			Components: []string{"base"},
		}

		expect := blueprintv1alpha1.Kustomization{
			Name:       "flux-system",
			Components: []string{"base", "sync"},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then diffs should indicate missing component
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsNoDiffsWhenSubstitutionsMatch", func(t *testing.T) {
		// Given kustomization with matching substitutions
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name: "metallb",
			Substitutions: map[string]string{
				"loadbalancer_ip_range": "10.10.1.10-10.10.1.100",
				"namespace":             "metallb-system",
			},
		}

		expect := blueprintv1alpha1.Kustomization{
			Name: "metallb",
			Substitutions: map[string]string{
				"loadbalancer_ip_range": "10.10.1.10-10.10.1.100",
			},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffsWhenSubstitutionValueMismatches", func(t *testing.T) {
		// Given kustomization with mismatched substitution value
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name: "metallb",
			Substitutions: map[string]string{
				"loadbalancer_ip_range": "10.5.1.10-10.5.1.100",
			},
		}

		expect := blueprintv1alpha1.Kustomization{
			Name: "metallb",
			Substitutions: map[string]string{
				"loadbalancer_ip_range": "10.10.1.10-10.10.1.100",
			},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then diffs should indicate value mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenSubstitutionKeyMissing", func(t *testing.T) {
		// Given kustomization missing a substitution key
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name:          "metallb",
			Substitutions: map[string]string{},
		}

		expect := blueprintv1alpha1.Kustomization{
			Name: "metallb",
			Substitutions: map[string]string{
				"loadbalancer_ip_range": "10.10.1.10-10.10.1.100",
			},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then diffs should indicate missing key
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffWhenTimeoutMismatches", func(t *testing.T) {
		// Given a kustomization whose timeout differs from the expectation
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name:    "cert-manager",
			Timeout: &blueprintv1alpha1.DurationString{Duration: 5 * time.Minute},
		}

		expect := blueprintv1alpha1.Kustomization{
			Name:    "cert-manager",
			Timeout: &blueprintv1alpha1.DurationString{Duration: 20 * time.Minute},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then diffs should indicate the timeout mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffWhenTimeoutUnset", func(t *testing.T) {
		// Given an expectation naming a timeout the actual kustomization never sets
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{Name: "cert-manager"}
		expect := blueprintv1alpha1.Kustomization{
			Name:    "cert-manager",
			Timeout: &blueprintv1alpha1.DurationString{Duration: 20 * time.Minute},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then diffs should indicate the timeout is unset, not silently pass
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsNoDiffWhenTimeoutMatches", func(t *testing.T) {
		// Given a kustomization whose timeout matches the expectation
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name:    "cert-manager",
			Timeout: &blueprintv1alpha1.DurationString{Duration: 20 * time.Minute},
		}
		expect := blueprintv1alpha1.Kustomization{
			Name:    "cert-manager",
			Timeout: &blueprintv1alpha1.DurationString{Duration: 20 * time.Minute},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffWhenIntervalMismatches", func(t *testing.T) {
		// Given a kustomization whose interval differs from the expectation
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name:     "cert-manager",
			Interval: &blueprintv1alpha1.DurationString{Duration: time.Minute},
		}
		expect := blueprintv1alpha1.Kustomization{
			Name:     "cert-manager",
			Interval: &blueprintv1alpha1.DurationString{Duration: 5 * time.Minute},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then diffs should indicate the interval mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffWhenRetryIntervalMismatches", func(t *testing.T) {
		// Given a kustomization whose retryInterval differs from the expectation
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name:          "cert-manager",
			RetryInterval: &blueprintv1alpha1.DurationString{Duration: 30 * time.Second},
		}
		expect := blueprintv1alpha1.Kustomization{
			Name:          "cert-manager",
			RetryInterval: &blueprintv1alpha1.DurationString{Duration: time.Minute},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then diffs should indicate the retryInterval mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffWhenDeleteTimeoutMismatches", func(t *testing.T) {
		// Given a kustomization whose deleteTimeout differs from the expectation
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name:          "database",
			DeleteTimeout: &blueprintv1alpha1.DurationString{Duration: 10 * time.Minute},
		}
		expect := blueprintv1alpha1.Kustomization{
			Name:          "database",
			DeleteTimeout: &blueprintv1alpha1.DurationString{Duration: 45 * time.Minute},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then diffs should indicate the deleteTimeout mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsNoDiffWhenDeleteTimeoutMatches", func(t *testing.T) {
		// Given a kustomization whose deleteTimeout matches the expectation
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name:          "database",
			DeleteTimeout: &blueprintv1alpha1.DurationString{Duration: 45 * time.Minute},
		}
		expect := blueprintv1alpha1.Kustomization{
			Name:          "database",
			DeleteTimeout: &blueprintv1alpha1.DurationString{Duration: 45 * time.Minute},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffWhenWaitMismatches", func(t *testing.T) {
		// Given a kustomization whose wait differs from the expectation
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actualWait := false
		expectWait := true
		actual := &blueprintv1alpha1.Kustomization{Name: "cert-manager", Wait: &actualWait}
		expect := blueprintv1alpha1.Kustomization{Name: "cert-manager", Wait: &expectWait}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then diffs should indicate the wait mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsNoDiffWhenWaitMatches", func(t *testing.T) {
		// Given a kustomization whose wait matches the expectation
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		wait := true
		actual := &blueprintv1alpha1.Kustomization{Name: "cert-manager", Wait: &wait}
		expect := blueprintv1alpha1.Kustomization{Name: "cert-manager", Wait: &wait}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffWhenHealthCheckMissing", func(t *testing.T) {
		// Given a kustomization missing an expected health check
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name: "policy",
			HealthChecks: []blueprintv1alpha1.HealthCheckRef{
				{APIVersion: "apps/v1", Kind: "Deployment", Name: "other-controller", Namespace: "system-policy"},
			},
		}
		expect := blueprintv1alpha1.Kustomization{
			Name: "policy",
			HealthChecks: []blueprintv1alpha1.HealthCheckRef{
				{APIVersion: "apps/v1", Kind: "Deployment", Name: "kyverno-admission-controller", Namespace: "system-policy"},
			},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then diffs should indicate the missing health check
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsNoDiffWhenHealthCheckPresent", func(t *testing.T) {
		// Given a kustomization whose health checks include the expected one
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		check := blueprintv1alpha1.HealthCheckRef{APIVersion: "apps/v1", Kind: "Deployment", Name: "kyverno-admission-controller", Namespace: "system-policy"}
		actual := &blueprintv1alpha1.Kustomization{
			Name:         "policy",
			HealthChecks: []blueprintv1alpha1.HealthCheckRef{check},
		}
		expect := blueprintv1alpha1.Kustomization{
			Name:         "policy",
			HealthChecks: []blueprintv1alpha1.HealthCheckRef{check},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsNoDiffWhenHealthCheckLeavesNamespaceAndAPIVersionUnset", func(t *testing.T) {
		// Given a fixture that only names kind and name, leaving apiVersion and namespace
		// unset since the actual namespace defaults to the kustomization's own
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name: "policy",
			HealthChecks: []blueprintv1alpha1.HealthCheckRef{
				{APIVersion: "apps/v1", Kind: "Deployment", Name: "kyverno-admission-controller", Namespace: "system-policy"},
			},
		}
		expect := blueprintv1alpha1.Kustomization{
			Name: "policy",
			HealthChecks: []blueprintv1alpha1.HealthCheckRef{
				{Kind: "Deployment", Name: "kyverno-admission-controller"},
			},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then no diffs should be returned: the unset fields are not compared
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffWhenHealthCheckExprMissing", func(t *testing.T) {
		// Given a kustomization missing an expected CEL health check expression
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name: "crossplane",
			HealthCheckExprs: []blueprintv1alpha1.HealthCheckExpr{
				{APIVersion: "example.com/v1", Kind: "Widget", Current: "status.ready == false"},
			},
		}
		expect := blueprintv1alpha1.Kustomization{
			Name: "crossplane",
			HealthCheckExprs: []blueprintv1alpha1.HealthCheckExpr{
				{APIVersion: "example.com/v1", Kind: "Widget", Current: "status.ready == true"},
			},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then diffs should indicate the missing health check expression
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsNoDiffWhenHealthCheckExprPresent", func(t *testing.T) {
		// Given a kustomization whose health check expressions include the expected one
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		expr := blueprintv1alpha1.HealthCheckExpr{APIVersion: "example.com/v1", Kind: "Widget", Current: "status.ready == true"}
		actual := &blueprintv1alpha1.Kustomization{
			Name:             "crossplane",
			HealthCheckExprs: []blueprintv1alpha1.HealthCheckExpr{expr},
		}
		expect := blueprintv1alpha1.Kustomization{
			Name:             "crossplane",
			HealthCheckExprs: []blueprintv1alpha1.HealthCheckExpr{expr},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect, expect.Name)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})
}

func TestTestRunner_matchFluxSystem(t *testing.T) {
	t.Run("ReturnsNoDiffsWhenAllFieldsMatch", func(t *testing.T) {
		// Given an actual system matching every asserted field
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		ordinal := 5
		actual := &blueprintv1alpha1.FluxSystem{
			Name:      "policy",
			Path:      "policy",
			Source:    "core",
			When:      "${cluster.driver == 'talos'}",
			Strategy:  "merge",
			DependsOn: []string{"telemetry-install"},
			Ordinal:   &ordinal,
		}

		expect := blueprintv1alpha1.FluxSystem{
			Name:      "policy",
			Path:      "policy",
			Source:    "core",
			When:      "${cluster.driver == 'talos'}",
			Strategy:  "merge",
			DependsOn: []string{"telemetry-install"},
			Ordinal:   &ordinal,
		}

		// When matching
		diffs := runner.matchFluxSystem(actual, expect)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffWhenPathMismatches", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.FluxSystem{Name: "policy", Path: "wrong"}
		expect := blueprintv1alpha1.FluxSystem{Name: "policy", Path: "expected"}

		diffs := runner.matchFluxSystem(actual, expect)

		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("PathFallsBackToNameWhenActualPathOmitted", func(t *testing.T) {
		// Given an actual system with no Path, and an expected Path equal to the system's Name
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.FluxSystem{Name: "demo"}
		expect := blueprintv1alpha1.FluxSystem{Name: "demo", Path: "demo"}

		// When matching the systems
		diffs := runner.matchFluxSystem(actual, expect)

		// Then the match returns no diffs
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffWhenActualPathOmittedAndNameDoesNotMatchExpectedPath", func(t *testing.T) {
		// Given an actual system with no Path, and an expected Path different from the system's Name
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.FluxSystem{Name: "demo"}
		expect := blueprintv1alpha1.FluxSystem{Name: "demo", Path: "other"}

		// When matching the systems
		diffs := runner.matchFluxSystem(actual, expect)

		// Then the match returns one diff
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffWhenSourceMismatches", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.FluxSystem{Name: "policy", Source: "wrong"}
		expect := blueprintv1alpha1.FluxSystem{Name: "policy", Source: "expected"}

		diffs := runner.matchFluxSystem(actual, expect)

		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffWhenWhenMismatches", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.FluxSystem{Name: "policy", When: "wrong"}
		expect := blueprintv1alpha1.FluxSystem{Name: "policy", When: "expected"}

		diffs := runner.matchFluxSystem(actual, expect)

		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffWhenStrategyMismatches", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.FluxSystem{Name: "policy", Strategy: "merge"}
		expect := blueprintv1alpha1.FluxSystem{Name: "policy", Strategy: "remove"}

		diffs := runner.matchFluxSystem(actual, expect)

		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffWhenDependsOnMissing", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.FluxSystem{Name: "policy", DependsOn: []string{"telemetry-install"}}
		expect := blueprintv1alpha1.FluxSystem{Name: "policy", DependsOn: []string{"telemetry-install", "csi"}}

		diffs := runner.matchFluxSystem(actual, expect)

		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffWhenOrdinalUnset", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		ordinal := 3
		actual := &blueprintv1alpha1.FluxSystem{Name: "policy"}
		expect := blueprintv1alpha1.FluxSystem{Name: "policy", Ordinal: &ordinal}

		diffs := runner.matchFluxSystem(actual, expect)

		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffWhenOrdinalMismatches", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actualOrdinal, expectOrdinal := 1, 2
		actual := &blueprintv1alpha1.FluxSystem{Name: "policy", Ordinal: &actualOrdinal}
		expect := blueprintv1alpha1.FluxSystem{Name: "policy", Ordinal: &expectOrdinal}

		diffs := runner.matchFluxSystem(actual, expect)

		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffWhenGlobalDependencyExpectedTrueButActualFalse", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.FluxSystem{Name: "policy", GlobalDependency: false}
		expect := blueprintv1alpha1.FluxSystem{Name: "policy", GlobalDependency: true}

		diffs := runner.matchFluxSystem(actual, expect)

		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsNoDiffWhenGlobalDependencyNotAsserted", func(t *testing.T) {
		// The zero value (false) is indistinguishable from "not asserted" — a documented limitation
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.FluxSystem{Name: "policy", GlobalDependency: true}
		expect := blueprintv1alpha1.FluxSystem{Name: "policy"}

		diffs := runner.matchFluxSystem(actual, expect)

		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("DelegatesInstallMismatchesToMatchKustomization", func(t *testing.T) {
		// Given an install tier missing an expected component
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.FluxSystem{
			Name:    "cert-manager",
			Install: &blueprintv1alpha1.Kustomization{Components: []string{"cert-manager"}},
		}
		expect := blueprintv1alpha1.FluxSystem{
			Name:    "cert-manager",
			Install: &blueprintv1alpha1.Kustomization{Components: []string{"cert-manager", "cert-manager/prometheus"}},
		}

		diffs := runner.matchFluxSystem(actual, expect)

		if len(diffs) != 1 || !strings.Contains(diffs[0], "kustomize[cert-manager.install].components") {
			t.Errorf("Expected labeled install component diff, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffWhenResourcesVariantWhenMismatches", func(t *testing.T) {
		// Given a matched resources variant whose when: differs from expectation
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.FluxSystem{
			Name: "pki",
			Resources: []blueprintv1alpha1.FluxVariant{
				{Kustomization: blueprintv1alpha1.Kustomization{Name: "ca", Components: []string{"private-issuer/ca"}}, When: "${addons.private_ca.enabled}"},
			},
		}
		expect := blueprintv1alpha1.FluxSystem{
			Name: "pki",
			Resources: []blueprintv1alpha1.FluxVariant{
				{Kustomization: blueprintv1alpha1.Kustomization{Name: "ca"}, When: "${private_ca.enabled}"},
			},
		}

		diffs := runner.matchFluxSystem(actual, expect)

		if len(diffs) != 1 || !strings.Contains(diffs[0], "flux[pki].resources[ca].when") {
			t.Errorf("Expected variant when: mismatch diff, got: %v", diffs)
		}
	})
}

func TestTestRunner_matchFluxSystemExclusions(t *testing.T) {
	t.Run("ReturnsNoDiffsWhenNeitherInstallNorResourcesPresent", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.FluxSystem{Name: "pki"}
		exclude := blueprintv1alpha1.FluxSystem{Name: "pki", Install: &blueprintv1alpha1.Kustomization{}}

		diffs := runner.matchFluxSystemExclusions(actual, exclude)

		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffWhenExcludedInstallExists", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.FluxSystem{Name: "pki", Install: &blueprintv1alpha1.Kustomization{Components: []string{"cert-manager"}}}
		exclude := blueprintv1alpha1.FluxSystem{Name: "pki", Install: &blueprintv1alpha1.Kustomization{}}

		diffs := runner.matchFluxSystemExclusions(actual, exclude)

		if len(diffs) != 1 || !strings.Contains(diffs[0], "flux[pki].install: should not exist") {
			t.Errorf("Expected install-should-not-exist diff, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffWhenExcludedResourcesVariantExists", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.FluxSystem{
			Name: "pki",
			Resources: []blueprintv1alpha1.FluxVariant{
				{Kustomization: blueprintv1alpha1.Kustomization{Name: "ca", Components: []string{"private-issuer/ca"}}},
			},
		}
		exclude := blueprintv1alpha1.FluxSystem{
			Name: "pki",
			Resources: []blueprintv1alpha1.FluxVariant{
				{Kustomization: blueprintv1alpha1.Kustomization{Name: "ca"}},
			},
		}

		diffs := runner.matchFluxSystemExclusions(actual, exclude)

		if len(diffs) != 1 || !strings.Contains(diffs[0], "flux[pki].resources: variant should not exist: ca") {
			t.Errorf("Expected variant-should-not-exist diff, got: %v", diffs)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestContains(t *testing.T) {
	t.Run("ReturnsTrueWhenItemExists", func(t *testing.T) {
		// Given a slice with items
		slice := []string{"a", "b", "c"}

		// When checking for existing item
		result := contains(slice, "b")

		// Then true should be returned
		if !result {
			t.Error("Expected true for existing item")
		}
	})

	t.Run("ReturnsFalseWhenItemDoesNotExist", func(t *testing.T) {
		// Given a slice with items
		slice := []string{"a", "b", "c"}

		// When checking for nonexistent item
		result := contains(slice, "d")

		// Then false should be returned
		if result {
			t.Error("Expected false for nonexistent item")
		}
	})

	t.Run("ReturnsFalseForEmptySlice", func(t *testing.T) {
		// Given an empty slice
		slice := []string{}

		// When checking for any item
		result := contains(slice, "a")

		// Then false should be returned
		if result {
			t.Error("Expected false for empty slice")
		}
	})
}

func TestContainsAll(t *testing.T) {
	t.Run("ReturnsTrueWhenEveryNeedleIsPresent", func(t *testing.T) {
		// Given a haystack containing every needle
		haystack := []string{"a", "b", "c"}

		// When checking a subset
		result := containsAll(haystack, []string{"a", "c"})

		// Then true should be returned
		if !result {
			t.Error("Expected true when every needle is present")
		}
	})

	t.Run("ReturnsFalseWhenAnyNeedleIsMissing", func(t *testing.T) {
		// Given a haystack missing one needle
		haystack := []string{"a", "b"}

		// When checking a set including the missing item
		result := containsAll(haystack, []string{"a", "c"})

		// Then false should be returned
		if result {
			t.Error("Expected false when a needle is missing")
		}
	})

	t.Run("ReturnsTrueForEmptyNeedles", func(t *testing.T) {
		// Given an empty set of needles
		haystack := []string{"a"}

		// When checking with no needles
		result := containsAll(haystack, []string{})

		// Then true should be returned vacuously
		if !result {
			t.Error("Expected true for empty needles")
		}
	})
}

func TestDeepEqual(t *testing.T) {
	t.Run("ReturnsTrueForEqualStrings", func(t *testing.T) {
		if !deepEqual("hello", "hello") {
			t.Error("Expected true for equal strings")
		}
	})

	t.Run("ReturnsFalseForUnequalStrings", func(t *testing.T) {
		if deepEqual("hello", "world") {
			t.Error("Expected false for unequal strings")
		}
	})

	t.Run("ReturnsTrueForEqualIntegers", func(t *testing.T) {
		if !deepEqual(42, 42) {
			t.Error("Expected true for equal integers")
		}
	})

	t.Run("ReturnsTrueForBothNil", func(t *testing.T) {
		if !deepEqual(nil, nil) {
			t.Error("Expected true for both nil")
		}
	})

	t.Run("ReturnsFalseForOneNil", func(t *testing.T) {
		if deepEqual("hello", nil) {
			t.Error("Expected false when one is nil")
		}
		if deepEqual(nil, "hello") {
			t.Error("Expected false when one is nil")
		}
	})

	t.Run("ReturnsTrueForEqualMaps", func(t *testing.T) {
		a := map[string]any{"key": "value", "num": 42}
		b := map[string]any{"key": "value", "num": 42}
		if !deepEqual(a, b) {
			t.Error("Expected true for equal maps")
		}
	})

	t.Run("ReturnsFalseForUnequalMaps", func(t *testing.T) {
		a := map[string]any{"key": "value1"}
		b := map[string]any{"key": "value2"}
		if deepEqual(a, b) {
			t.Error("Expected false for unequal maps")
		}
	})

	t.Run("ReturnsFalseForMapsDifferentLength", func(t *testing.T) {
		a := map[string]any{"key": "value", "extra": "field"}
		b := map[string]any{"key": "value"}
		if deepEqual(a, b) {
			t.Error("Expected false for maps with different length")
		}
	})

	t.Run("ReturnsFalseForMapMissingKey", func(t *testing.T) {
		a := map[string]any{"key1": "value"}
		b := map[string]any{"key2": "value"}
		if deepEqual(a, b) {
			t.Error("Expected false for maps with different keys")
		}
	})

	t.Run("ReturnsTrueForEqualSlices", func(t *testing.T) {
		a := []any{"one", "two", 3}
		b := []any{"one", "two", 3}
		if !deepEqual(a, b) {
			t.Error("Expected true for equal slices")
		}
	})

	t.Run("ReturnsFalseForUnequalSlices", func(t *testing.T) {
		a := []any{"one", "two"}
		b := []any{"one", "three"}
		if deepEqual(a, b) {
			t.Error("Expected false for unequal slices")
		}
	})

	t.Run("ReturnsFalseForSlicesDifferentLength", func(t *testing.T) {
		a := []any{"one", "two", "three"}
		b := []any{"one", "two"}
		if deepEqual(a, b) {
			t.Error("Expected false for slices with different length")
		}
	})

	t.Run("ReturnsTrueForNestedStructures", func(t *testing.T) {
		a := map[string]any{
			"nested": map[string]any{"inner": "value"},
			"list":   []any{1, 2, 3},
		}
		b := map[string]any{
			"nested": map[string]any{"inner": "value"},
			"list":   []any{1, 2, 3},
		}
		if !deepEqual(a, b) {
			t.Error("Expected true for equal nested structures")
		}
	})

	t.Run("ReturnsTrueForEquivalentNumericTypes", func(t *testing.T) {
		if !deepEqual(42, 42.0) {
			t.Error("Expected true for equivalent numeric values via string comparison")
		}
	})

	t.Run("ReturnsFalseForMapVsNonMap", func(t *testing.T) {
		a := map[string]any{"key": "value"}
		b := "not a map"
		if deepEqual(a, b) {
			t.Error("Expected false for map vs non-map")
		}
	})

	t.Run("ReturnsFalseForSliceVsNonSlice", func(t *testing.T) {
		a := []any{"one", "two"}
		b := "not a slice"
		if deepEqual(a, b) {
			t.Error("Expected false for slice vs non-slice")
		}
	})
}
