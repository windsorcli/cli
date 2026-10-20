package blueprint

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type ProcessorTestMocks struct {
	Shell         *shell.MockShell
	ConfigHandler *config.MockConfigHandler
	Evaluator     *evaluator.MockExpressionEvaluator
	Runtime       *runtime.Runtime
}

func setupProcessorMocks(t *testing.T) *ProcessorTestMocks {
	t.Helper()

	tmpDir := t.TempDir()
	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	realEvaluator := evaluator.NewExpressionEvaluator(mockConfigHandler, tmpDir, tmpDir)
	mockEvaluator := evaluator.NewMockExpressionEvaluator()

	mockEvaluator.EvaluateFunc = realEvaluator.Evaluate
	mockEvaluator.EvaluateMapFunc = realEvaluator.EvaluateMap
	mockEvaluator.SetTemplateDataFunc = realEvaluator.SetTemplateData
	mockEvaluator.RegisterFunc = realEvaluator.Register

	rt := &runtime.Runtime{
		Shims:         runtime.NewShims(),
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
		Evaluator:     mockEvaluator,
	}

	mocks := &ProcessorTestMocks{
		Shell:         mockShell,
		ConfigHandler: mockConfigHandler,
		Evaluator:     mockEvaluator,
		Runtime:       rt,
	}

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
	})

	return mocks
}

// evalTerraformInputsWithScope evaluates component inputs with the given scope (same as handler does
// with mergedScope). Used by processor config tests because the processor no longer evaluates inputs;
// inputs are evaluated once by the handler with the full merged config (Terraform-locals style).
func evalTerraformInputsWithScope(t *testing.T, rt *runtime.Runtime, inputs map[string]any, scope map[string]any) map[string]any {
	t.Helper()
	if rt == nil || rt.Evaluator == nil || inputs == nil {
		return inputs
	}
	evaluated, err := rt.Evaluator.EvaluateMap(inputs, "", scope, false)
	if err != nil {
		t.Fatalf("evalTerraformInputsWithScope: %v", err)
	}
	return evaluated
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewBlueprintProcessor(t *testing.T) {
	t.Run("CreatesProcessorWithDefaults", func(t *testing.T) {
		// Given a runtime with evaluator
		mocks := setupProcessorMocks(t)

		// When creating a new processor
		processor := NewBlueprintProcessor(mocks.Runtime)

		// Then processor should be created with defaults
		if processor == nil {
			t.Fatal("Expected processor to be created")
		}
		if processor.runtime != mocks.Runtime {
			t.Error("Expected runtime to be set")
		}
		if processor.evaluator != mocks.Evaluator {
			t.Error("Expected evaluator from runtime to be used")
		}
	})

	t.Run("UsesRuntimeEvaluator", func(t *testing.T) {
		// Given a processor
		mocks := setupProcessorMocks(t)

		// When creating a processor
		processor := NewBlueprintProcessor(mocks.Runtime)

		// Then processor should use runtime evaluator
		if processor.evaluator == nil {
			t.Error("Expected evaluator to be set")
		}
		if processor.evaluator != mocks.Runtime.Evaluator {
			t.Error("Expected processor to use runtime evaluator")
		}
	})

	t.Run("PanicsWhenRuntimeIsNil", func(t *testing.T) {
		// Given a nil runtime
		// When creating a processor
		// Then should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when runtime is nil")
			}
		}()
		NewBlueprintProcessor(nil)
	})

	t.Run("PanicsWhenEvaluatorIsNil", func(t *testing.T) {
		// Given a runtime without evaluator
		mocks := setupProcessorMocks(t)
		mocks.Runtime.Evaluator = nil

		// When creating a processor
		// Then should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when evaluator is nil")
			}
		}()
		NewBlueprintProcessor(mocks.Runtime)
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProcessor_ProcessFacets(t *testing.T) {
	t.Run("ReturnsEmptyBlueprintForNoFacets", func(t *testing.T) {
		// Given a processor and empty facets
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}

		// When processing empty facets
		_, err := processor.ProcessFacets(target, nil)

		// Then should return empty blueprint
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components, got %d", len(target.TerraformComponents))
		}
		if len(target.Kustomizations) != 0 {
			t.Errorf("Expected 0 kustomizations, got %d", len(target.Kustomizations))
		}
	})

	t.Run("IncludesFacetWithoutWhenCondition", func(t *testing.T) {
		// Given a facet without when condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "always-include"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"}},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then facet components should be included
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		if target.TerraformComponents[0].Path != "vpc" {
			t.Errorf("Expected path='vpc', got '%s'", target.TerraformComponents[0].Path)
		}
	})

	t.Run("IncludesFacetWhenConditionTrue", func(t *testing.T) {
		// Given a facet with true condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "conditional"},
				When:     "enabled == true",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "eks"}},
				},
			},
		}
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": true}, nil
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then facet should be included
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
	})

	t.Run("IncludesFacetWhenConditionUsesExtraScope", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": false}, nil
		}
		processor.SetExtraScope(map[string]any{"enabled": true})

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "extra-scope"},
				When:     "enabled == true",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "extra"}},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		if target.TerraformComponents[0].Path != "extra" {
			t.Errorf("Expected path='extra', got %s", target.TerraformComponents[0].Path)
		}
	})

	t.Run("ExcludesFacetWhenConditionFalse", func(t *testing.T) {
		// Given a facet with false condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "conditional"},
				When:     "enabled == true",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "eks"}},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then facet should be excluded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components, got %d", len(target.TerraformComponents))
		}
	})

	t.Run("RecordsExcludedFacetWithWhenAndProvidedNames", func(t *testing.T) {
		// Given one included and one when-excluded facet, the excluded one contributing a terraform
		// component, a kustomization, and a flux system.
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": false}, nil
		}
		facets := []blueprintv1alpha1.Facet{
			{Metadata: blueprintv1alpha1.Metadata{Name: "always"}, TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
				{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"}},
			}},
			{
				Metadata:            blueprintv1alpha1.Metadata{Name: "addon-private-dns"},
				When:                "enabled == true",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "cluster"}}},
				Kustomizations:      []blueprintv1alpha1.ConditionalKustomization{{Kustomization: blueprintv1alpha1.Kustomization{Name: "dns-config"}}},
				FluxSystems:         []blueprintv1alpha1.FluxSystem{{Name: "dns", Install: &blueprintv1alpha1.Kustomization{Components: []string{"external-dns"}}}},
			},
		}

		// When processing
		processor.ResetExcludedFacets()
		if _, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then only the excluded facet is recorded, with its when: and every name it would contribute.
		excluded := processor.GetExcludedFacets()
		if len(excluded) != 1 {
			t.Fatalf("Expected 1 excluded facet, got %d (%v)", len(excluded), excluded)
		}
		if excluded[0].Name != "addon-private-dns" || excluded[0].When != "enabled == true" {
			t.Errorf("Expected addon-private-dns/when recorded, got %+v", excluded[0])
		}
		for _, want := range []string{"cluster", "dns-config", "dns-install"} {
			if !slices.Contains(excluded[0].Provides, want) {
				t.Errorf("Expected Provides to contain %q, got %v", want, excluded[0].Provides)
			}
		}

		// And ResetExcludedFacets clears the record.
		processor.ResetExcludedFacets()
		if got := processor.GetExcludedFacets(); got != nil {
			t.Errorf("Expected nil after reset, got %v", got)
		}
	})

	t.Run("ExcludesFacetWhenConditionReferencesUndefinedVariable", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "conditional-undefined"},
				When:     "undefined_feature",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "eks"}},
				},
			},
		}

		mocks.Evaluator.EvaluateFunc = func(expression string, featurePath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return nil, nil
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components for undefined condition, got %d", len(target.TerraformComponents))
		}
	})

	t.Run("ProcessesFacetsInSortedOrder", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "z-facet"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "z-path"}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "a-facet"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "a-path"}},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then components should be in sorted facet order (ordinal then name; both ordinal 0 so name tiebreak: a before z)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 2 {
			t.Fatalf("Expected 2 terraform components, got %d", len(target.TerraformComponents))
		}
		if target.TerraformComponents[0].Path != "a-path" {
			t.Errorf("Expected first path='a-path', got '%s'", target.TerraformComponents[0].Path)
		}
		if target.TerraformComponents[1].Path != "z-path" {
			t.Errorf("Expected second path='z-path', got '%s'", target.TerraformComponents[1].Path)
		}
	})

	t.Run("ProcessesFacetsByOrdinalThenName", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		ord100 := 100
		ord200 := 200
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "high"},
				Ordinal:  &ord200,
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"from": "high"}}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "low"},
				Ordinal:  &ord100,
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"from": "low"}}},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component (same path), got %d", len(target.TerraformComponents))
		}
		if target.TerraformComponents[0].Inputs["from"] != "high" {
			t.Errorf("Expected higher-ordinal facet to win: inputs.from='high', got '%v'", target.TerraformComponents[0].Inputs["from"])
		}
	})

	t.Run("SameNameKustomizationHigherOrdinalWins", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		ord199 := 199
		ord400 := 400
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "provider-base"},
				Ordinal:  &ord199,
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{Kustomization: blueprintv1alpha1.Kustomization{Name: "observability", Substitutions: map[string]string{"stack": "fluentd"}}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "addon-observability"},
				Ordinal:  &ord400,
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{Kustomization: blueprintv1alpha1.Kustomization{Name: "observability", Substitutions: map[string]string{"stack": "grafana"}}},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization (same name), got %d", len(target.Kustomizations))
		}
		if target.Kustomizations[0].Substitutions["stack"] != "grafana" {
			t.Errorf("Expected addon (ordinal 400) to override provider-base (199): stack='grafana', got '%s'", target.Kustomizations[0].Substitutions["stack"])
		}
	})

	t.Run("SameNameKustomizationMergesByDefault", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		ord300 := 300
		ord400 := 400
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "option-ingress"},
				Ordinal:  &ord300,
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{
							Name:       "ingress",
							Components: []string{"nginx", "nginx/loadbalancer", "nginx/web", "nginx/prometheus"},
						},
					},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "addon-private-dns"},
				Ordinal:  &ord400,
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{
							Name:       "ingress",
							Components: []string{"nginx/coredns"},
						},
					},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization (same name), got %d", len(target.Kustomizations))
		}
		comps := target.Kustomizations[0].Components
		want := []string{"nginx", "nginx/coredns", "nginx/loadbalancer", "nginx/prometheus", "nginx/web"}
		if len(comps) != len(want) {
			t.Errorf("Expected merged components len %d, got %d: %v", len(want), len(comps), comps)
		}
		for _, w := range want {
			if !slices.Contains(comps, w) {
				t.Errorf("Expected merged components to contain %q, got %v", w, comps)
			}
		}
	})

	t.Run("EvaluatesInputExpressions", func(t *testing.T) {
		// Given a facet with input expressions and config data
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-inputs"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:   "vpc",
							Inputs: map[string]any{"region": "${aws.region}"},
						},
					},
				},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"aws": map[string]any{"region": "us-east-1"},
			}, nil
		}

		// When processing facets (inputs are not evaluated in processor; handler evaluates with merged scope)
		target := &blueprintv1alpha1.Blueprint{}
		scope, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		inputs := evalTerraformInputsWithScope(t, mocks.Runtime, target.TerraformComponents[0].Inputs, scope)
		if inputs["region"] != "us-east-1" {
			t.Errorf("Expected evaluated value 'us-east-1', got '%v'", inputs["region"])
		}
	})

	t.Run("IncludesKustomizationsWithoutCondition", func(t *testing.T) {
		// Given a facet with kustomizations
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-kustomization"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{Kustomization: blueprintv1alpha1.Kustomization{Name: "app"}},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then kustomization should be included
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(target.Kustomizations))
		}
		if target.Kustomizations[0].Name != "app" {
			t.Errorf("Expected name='app', got '%s'", target.Kustomizations[0].Name)
		}
	})

	t.Run("ExcludesComponentWhenComponentConditionFalse", func(t *testing.T) {
		// Given a facet with component-level condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "mixed"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						When:               "include_vpc == true",
						TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
					},
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "rds"},
					},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then only unconditional component should be included
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		if target.TerraformComponents[0].Path != "rds" {
			t.Errorf("Expected path='rds', got '%s'", target.TerraformComponents[0].Path)
		}
	})

	t.Run("ExcludesKustomizationWhenConditionFalse", func(t *testing.T) {
		// Given a facet with conditional kustomization
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "kust-facet"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						When:          "include_app == true",
						Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
					},
					{
						Kustomization: blueprintv1alpha1.Kustomization{Name: "base"},
					},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then only unconditional kustomization should be included
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(target.Kustomizations))
		}
		if target.Kustomizations[0].Name != "base" {
			t.Errorf("Expected name='base', got '%s'", target.Kustomizations[0].Name)
		}
	})

	t.Run("ReturnsErrorForInvalidFacetCondition", func(t *testing.T) {
		// Given a facet with invalid condition syntax
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "bad-condition"},
				When:     "invalid syntax {{{}",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"}},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid condition")
		}
	})

	t.Run("ReturnsErrorForInvalidComponentCondition", func(t *testing.T) {
		// Given a component with invalid condition syntax
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "facet"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						When:               "invalid syntax {{{}",
						TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
					},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid component condition")
		}
	})

	t.Run("ReturnsErrorForInvalidKustomizationCondition", func(t *testing.T) {
		// Given a kustomization with invalid condition syntax
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "facet"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						When:          "invalid syntax {{{}",
						Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
					},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid kustomization condition")
		}
	})

	t.Run("HandlesStringTrueConditionResult", func(t *testing.T) {
		// Given a condition that evaluates to string "true"
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "string-condition"},
				When:     `"true"`,
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"}},
				},
			},
		}
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then facet should be included (string "true" is truthy)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component for string 'true', got %d", len(target.TerraformComponents))
		}
	})

	t.Run("EvaluatesSubstitutionExpressions", func(t *testing.T) {
		// Given a facet with substitution expressions
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-subs"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{
							Name: "ingress",
							Substitutions: map[string]string{
								"domain": "${dns.domain}",
								"static": "unchanged",
							},
						},
					},
				},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"dns": map[string]any{"domain": "example.com"},
			}, nil
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then substitutions should be evaluated
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(target.Kustomizations))
		}
		subs := target.Kustomizations[0].Substitutions
		if subs["domain"] != "example.com" {
			t.Errorf("Expected 'example.com', got '%v'", subs["domain"])
		}
		if subs["static"] != "unchanged" {
			t.Errorf("Expected 'unchanged', got '%v'", subs["static"])
		}
	})

	t.Run("IncludesSubstitutionKeysForUndefinedPaths", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-undefined"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{
							Name: "app",
							Substitutions: map[string]string{
								"domain":   "${dns.domain}",
								"timezone": "${timezone}",
								"static":   "unchanged",
							},
						},
					},
				},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(target.Kustomizations))
		}
		subs := target.Kustomizations[0].Substitutions
		if _, exists := subs["domain"]; !exists {
			t.Error("Expected 'domain' key to exist even when undefined")
		}
		if _, exists := subs["timezone"]; !exists {
			t.Error("Expected 'timezone' key to exist even when undefined")
		}
		if subs["static"] != "unchanged" {
			t.Errorf("Expected 'unchanged', got '%v'", subs["static"])
		}
	})

	t.Run("CoalesceOperatorReturnsDefaultInSubstitutions", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-coalesce"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{
							Name: "observability",
							Substitutions: map[string]string{
								"admin_password": `${addons.observability.admin_password ?? "grafana"}`,
								"port":           `${addons.observability.port ?? "3000"}`,
							},
						},
					},
				},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"addons": map[string]any{},
			}, nil
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(target.Kustomizations))
		}
		subs := target.Kustomizations[0].Substitutions
		if subs["admin_password"] != "grafana" {
			t.Errorf("Expected 'grafana' from coalesce, got '%v'", subs["admin_password"])
		}
		if subs["port"] != "3000" {
			t.Errorf("Expected '3000' from coalesce, got '%v'", subs["port"])
		}
	})

	t.Run("PreservesNonStringInputs", func(t *testing.T) {
		// Given a facet with non-string inputs
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "mixed-inputs"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path: "vpc",
							Inputs: map[string]any{
								"count":   42,
								"enabled": true,
								"tags":    []string{"a", "b"},
							},
						},
					},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then non-string inputs should be preserved
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		inputs := target.TerraformComponents[0].Inputs
		if inputs["count"] != 42 {
			t.Errorf("Expected 42, got '%v'", inputs["count"])
		}
		if inputs["enabled"] != true {
			t.Errorf("Expected true, got '%v'", inputs["enabled"])
		}
	})

	t.Run("HandlesInputEvaluationError", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "bad-input"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:   "vpc",
							Inputs: map[string]any{"bad": "${unclosed"},
						},
					},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then should return error for malformed expression
		if err == nil {
			t.Error("Expected error for malformed expression")
		}
	})

	t.Run("HandlesSubstitutionEvaluationError", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "bad-sub"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{
							Name:          "test",
							Substitutions: map[string]string{"bad": "${unclosed"},
						},
					},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then should return error for malformed expression
		if err == nil {
			t.Error("Expected error for malformed substitution expression")
		}
	})

	t.Run("HandlesSourceAssignment", func(t *testing.T) {
		// Given a facet with components without source
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-source"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"}},
				},
			},
		}

		// When processing with sourceName
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets, "test-source")

		// Then source should be assigned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.TerraformComponents[0].Source != "test-source" {
			t.Errorf("Expected source='test-source', got '%s'", target.TerraformComponents[0].Source)
		}
	})

	t.Run("PreservesExistingSource", func(t *testing.T) {
		// Given a facet with components that already have source
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-source"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Source: "existing-source"}},
				},
			},
		}

		// When processing with sourceName
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets, "new-source")

		// Then existing source should be preserved
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.TerraformComponents[0].Source != "existing-source" {
			t.Errorf("Expected source='existing-source', got '%s'", target.TerraformComponents[0].Source)
		}
	})

	t.Run("ReturnsErrorForInvalidTerraformComponentStrategy", func(t *testing.T) {
		// Given a facet with invalid strategy
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "invalid-strategy"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						Strategy:           "invalid",
						TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
					},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid strategy")
		}
		if err != nil && !strings.Contains(err.Error(), "invalid strategy") {
			t.Errorf("Expected error about invalid strategy, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidKustomizationStrategy", func(t *testing.T) {
		// Given a facet with invalid kustomization strategy
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "invalid-strategy"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Strategy:      "typo",
						Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
					},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid strategy")
		}
		if err != nil && !strings.Contains(err.Error(), "invalid strategy") {
			t.Errorf("Expected error about invalid strategy, got: %v", err)
		}
	})

	t.Run("RemoveStrategyDeletesBlockFromScope", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "add-cluster"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "cluster", Body: map[string]any{"value": map[string]any{"endpoint": "x"}}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "remove-cluster"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "cluster", Strategy: "remove", Body: map[string]any{"value": nil}},
				},
			},
		}
		target := &blueprintv1alpha1.Blueprint{}
		scope, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := scope["cluster"]; ok {
			t.Error("Expected 'cluster' to be removed from scope by remove strategy")
		}
	})

	t.Run("RemoveStrategyLeavesNoNameForAnOperatorValueToCorrect", func(t *testing.T) {
		// Given a remove: strategy block wins, so no facet ends up contributing 'cluster' this
		// round, and the operator separately set a raw value under that same name
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetSetValuesFunc = func() map[string]any {
			return map[string]any{"cluster": map[string]any{"endpoint": "operator-value"}}
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "add-cluster"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "cluster", Body: map[string]any{"value": map[string]any{"endpoint": "x"}}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "remove-cluster"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "cluster", Strategy: "remove", Body: map[string]any{"value": nil}},
				},
			},
		}
		target := &blueprintv1alpha1.Blueprint{}
		scope, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Then 'cluster' stays absent: no facet claims the name, so the operator's raw value
		// is context, not config, and does not resurrect a config: scope entry
		if _, ok := scope["cluster"]; ok {
			t.Error("Expected 'cluster' to stay removed; an operator value alone must not resurrect it")
		}
	})

	t.Run("ReturnsErrorForInvalidConfigBlockStrategy", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "invalid-config-strategy"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "cluster", Strategy: "overwrite", Body: map[string]any{"value": map[string]any{"endpoint": "x"}}},
				},
			},
		}
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)
		if err == nil {
			t.Error("Expected error for invalid config block strategy")
		}
		if err != nil && !strings.Contains(err.Error(), "invalid strategy") {
			t.Errorf("Expected error about invalid strategy, got: %v", err)
		}
	})
}

func TestProcessor_ProcessFacets_Messages(t *testing.T) {
	t.Run("CollectsMessagesFromIncludedFacetsInOrder", func(t *testing.T) {
		// Given two included facets with post-run messages, ordered by ordinal
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) { return map[string]any{}, nil }
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		low, high := 100, 200
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "high"},
				Ordinal:  &high,
				Messages: []blueprintv1alpha1.Message{{Text: "second"}},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "low"},
				Ordinal:  &low,
				Messages: []blueprintv1alpha1.Message{{Text: "first"}},
			},
		}

		// When processing
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then messages carry through as raw templates in ordinal order
		if len(target.Messages) != 2 {
			t.Fatalf("Expected 2 messages, got %d: %v", len(target.Messages), target.Messages)
		}
		if target.Messages[0].Text != "first" || target.Messages[1].Text != "second" {
			t.Errorf("Expected [first, second] by ordinal, got [%q, %q]", target.Messages[0].Text, target.Messages[1].Text)
		}
	})

	t.Run("ExcludedFacetContributesNoMessages", func(t *testing.T) {
		// Given a facet gated out by its when: condition
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "aws"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "docker-only"},
				When:     "platform == 'docker'",
				Messages: []blueprintv1alpha1.Message{{Text: "docker note"}},
			},
		}

		// When processing
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the excluded facet's message does not leak into the blueprint
		if len(target.Messages) != 0 {
			t.Errorf("Expected no messages from excluded facet, got %v", target.Messages)
		}
	})

	t.Run("RawTemplatesCarryThroughUnevaluated", func(t *testing.T) {
		// Given a message whose text is an expression template
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) { return map[string]any{}, nil }
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "dns"},
				Messages: []blueprintv1alpha1.Message{
					{When: "dns.public_domain != ''", Text: "Delegate ${terraform_output('dns-zone', 'nameservers')}"},
				},
			},
		}

		// When processing
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the template is carried through verbatim for GenerateResolved to render later
		if len(target.Messages) != 1 {
			t.Fatalf("Expected 1 message, got %d", len(target.Messages))
		}
		if target.Messages[0].When != "dns.public_domain != ''" {
			t.Errorf("Expected when carried raw, got %q", target.Messages[0].When)
		}
		if target.Messages[0].Text != "Delegate ${terraform_output('dns-zone', 'nameservers')}" {
			t.Errorf("Expected text carried raw, got %q", target.Messages[0].Text)
		}
	})
}

func TestProcessor_ExpressionEvaluation(t *testing.T) {
	t.Run("EvaluatesDependsOnInTerraformComponents", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"base": "network",
			}, nil
		}

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-depends"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:      "cluster",
							DependsOn: []string{"${base}", "static-dep"},
						},
					},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 component, got %d", len(target.TerraformComponents))
		}
		deps := target.TerraformComponents[0].DependsOn
		if len(deps) != 2 {
			t.Fatalf("Expected 2 dependencies, got %d", len(deps))
		}
		if deps[0] != "network" {
			t.Errorf("Expected first dep to be 'network', got '%s'", deps[0])
		}
		if deps[1] != "static-dep" {
			t.Errorf("Expected second dep to be 'static-dep', got '%s'", deps[1])
		}
	})

	t.Run("EvaluatesDependsOnInKustomizations", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"app": "nginx",
			}, nil
		}

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-depends"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{
							Name:      "ingress",
							Path:      "apps/ingress",
							DependsOn: []string{"${app}", "dns"},
						},
					},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(target.Kustomizations))
		}
		deps := target.Kustomizations[0].DependsOn
		if len(deps) != 2 {
			t.Fatalf("Expected 2 dependencies, got %d", len(deps))
		}
		if deps[0] != "nginx" {
			t.Errorf("Expected first dep to be 'nginx', got '%s'", deps[0])
		}
		if deps[1] != "dns" {
			t.Errorf("Expected second dep to be 'dns', got '%s'", deps[1])
		}
	})

	t.Run("EvaluatesComponentsInKustomizations", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"base": "nginx",
			}, nil
		}

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-components"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{
							Name:       "app",
							Path:       "apps/app",
							Components: []string{"${base}", "static-comp"},
						},
					},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(target.Kustomizations))
		}
		comps := target.Kustomizations[0].Components
		if len(comps) != 2 {
			t.Fatalf("Expected 2 components, got %d", len(comps))
		}
		if comps[0] != "nginx" {
			t.Errorf("Expected first component to be 'nginx', got '%s'", comps[0])
		}
		if comps[1] != "static-comp" {
			t.Errorf("Expected second component to be 'static-comp', got '%s'", comps[1])
		}
	})

	t.Run("PreservesEmptyStringsInComponents", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"enabled": false,
			}, nil
		}

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-empty"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{
							Name:       "app",
							Path:       "apps/app",
							Components: []string{"nginx", "${enabled ? 'prometheus' : ''}", "cert-manager"},
						},
					},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		comps := target.Kustomizations[0].Components
		if len(comps) != 3 {
			t.Fatalf("Expected 3 components (empty preserved), got %d: %v", len(comps), comps)
		}
		if comps[0] != "nginx" {
			t.Errorf("Expected first component to be 'nginx', got '%s'", comps[0])
		}
		if comps[1] != "" {
			t.Errorf("Expected second component to be empty string, got %q", comps[1])
		}
		if comps[2] != "cert-manager" {
			t.Errorf("Expected third component to be 'cert-manager', got '%s'", comps[2])
		}
	})

	t.Run("FlattensArrayExpressionsInComponents", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"extra": []any{"comp1", "comp2"},
			}, nil
		}

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-array"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{
							Name:       "app",
							Path:       "apps/app",
							Components: []string{"nginx", "${extra}"},
						},
					},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		comps := target.Kustomizations[0].Components
		if len(comps) != 3 {
			t.Fatalf("Expected 3 components (array flattened), got %d: %v", len(comps), comps)
		}
		if comps[0] != "nginx" {
			t.Errorf("Expected first component to be 'nginx', got '%s'", comps[0])
		}
		if comps[1] != "comp1" {
			t.Errorf("Expected second component to be 'comp1', got '%s'", comps[1])
		}
		if comps[2] != "comp2" {
			t.Errorf("Expected third component to be 'comp2', got '%s'", comps[2])
		}
	})

	t.Run("EvaluatesDestroyInTerraformComponents", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"shouldDestroy": false,
			}, nil
		}

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-destroy"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:    "cluster",
							Destroy: &blueprintv1alpha1.BoolExpression{Expr: "${shouldDestroy}", IsExpr: true},
						},
					},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		destroy := target.TerraformComponents[0].Destroy.ToBool()
		if destroy == nil {
			t.Fatal("Expected destroy to be set")
		}
		if *destroy != false {
			t.Errorf("Expected destroy to be false, got %v", *destroy)
		}
	})

	t.Run("EvaluatesDestroyInKustomizations", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"shouldDestroy": true,
			}, nil
		}

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-destroy"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{
							Name:    "app",
							Path:    "apps/app",
							Destroy: &blueprintv1alpha1.BoolExpression{Expr: "${shouldDestroy}", IsExpr: true},
						},
					},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		destroy := target.Kustomizations[0].Destroy.ToBool()
		if destroy == nil {
			t.Fatal("Expected destroy to be set")
		}
		if *destroy != true {
			t.Errorf("Expected destroy to be true, got %v", *destroy)
		}
	})

	t.Run("EvaluatesParallelismInTerraformComponents", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"workers": 10,
			}, nil
		}

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-parallelism"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:        "cluster",
							Parallelism: &blueprintv1alpha1.IntExpression{Expr: "${workers / 2}", IsExpr: true},
						},
					},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		parallelism := target.TerraformComponents[0].Parallelism.ToInt()
		if parallelism == nil {
			t.Fatal("Expected parallelism to be set")
		}
		if *parallelism != 5 {
			t.Errorf("Expected parallelism to be 5, got %d", *parallelism)
		}
	})

	t.Run("ReturnsErrorForDeferredExpressionsInDependsOn", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if strings.Contains(expression, "terraform_output") {
				return nil, &evaluator.DeferredError{Expression: expression, Message: "deferred"}
			}
			return "value", nil
		}

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-deferred"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:      "cluster",
							DependsOn: []string{"${terraform_output('cluster', 'key')}"},
						},
					},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		if err == nil {
			t.Error("Expected error for deferred expression in dependsOn")
		}
		if err != nil && !strings.Contains(err.Error(), "dependsOn") {
			t.Errorf("Expected error about dependsOn, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForDeferredExpressionsInComponents", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if strings.Contains(expression, "terraform_output") {
				return nil, &evaluator.DeferredError{Expression: expression, Message: "deferred"}
			}
			return "value", nil
		}

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-deferred"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{
							Name:       "app",
							Path:       "apps/app",
							Components: []string{"${terraform_output('cluster', 'key')}"},
						},
					},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		if err == nil {
			t.Error("Expected error for deferred expression in components")
		}
		if err != nil && !strings.Contains(err.Error(), "components") {
			t.Errorf("Expected error about components, got: %v", err)
		}
	})

	t.Run("FiltersEmptyStringsFromTerraformDependsOnAfterEvaluation", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"optional_dep": "",
			}, nil
		}

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-empty-dep"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:      "cluster",
							DependsOn: []string{"${optional_dep}", "network"},
						},
					},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		deps := target.TerraformComponents[0].DependsOn
		for _, dep := range deps {
			if dep == "" {
				t.Errorf("Expected empty string to be filtered from dependsOn, got %v", deps)
			}
		}
		if len(deps) != 1 || deps[0] != "network" {
			t.Errorf("Expected [network], got %v", deps)
		}
	})

	t.Run("FiltersEmptyStringsFromKustomizationDependsOnAfterEvaluation", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"optional_dep": "",
			}, nil
		}

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-empty-dep"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{
							Name:      "ingress",
							DependsOn: []string{"${optional_dep}", "dns"},
						},
					},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		deps := target.Kustomizations[0].DependsOn
		for _, dep := range deps {
			if dep == "" {
				t.Errorf("Expected empty string to be filtered from dependsOn, got %v", deps)
			}
		}
		if len(deps) != 1 || deps[0] != "dns" {
			t.Errorf("Expected [dns], got %v", deps)
		}
	})

	t.Run("HandlesEmptyDependsOn", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "empty-depends"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:      "cluster",
							DependsOn: []string{},
						},
					},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents[0].DependsOn) != 0 {
			t.Errorf("Expected empty dependsOn, got %v", target.TerraformComponents[0].DependsOn)
		}
	})
}

func TestProcessor_ComponentConditionInheritance(t *testing.T) {
	t.Run("TerraformComponentInheritsFacetWhenCondition", func(t *testing.T) {
		// Given a facet with when condition and component without when
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "conditional-facet"},
				When:     "enabled == true",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
					},
				},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": true}, nil
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then component should be included (inherited facet condition is true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
	})

	t.Run("TerraformComponentExcludedWhenInheritedFacetConditionFalse", func(t *testing.T) {
		// Given a facet with when condition and component without when
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "conditional-facet"},
				When:     "enabled == true",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
					},
				},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": false}, nil
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then component should be excluded (inherited facet condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components, got %d", len(target.TerraformComponents))
		}
	})

	t.Run("KustomizationInheritsFacetWhenCondition", func(t *testing.T) {
		// Given a facet with when condition and kustomization without when
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "conditional-facet"},
				When:     "enabled == true",
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
					},
				},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": true}, nil
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then kustomization should be included (inherited facet condition is true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization, got %d", len(target.Kustomizations))
		}
	})

	t.Run("ComponentWithOwnWhenDoesNotInheritFacetWhen", func(t *testing.T) {
		// Given a facet with when condition and component with its own when
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "conditional-facet"},
				When:     "facet_enabled == true",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						When:               "component_enabled == true",
						TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
					},
				},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"facet_enabled":     true,
				"component_enabled": false,
			}, nil
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then component should be excluded (its own condition is false, even though facet condition is true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components, got %d", len(target.TerraformComponents))
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestBaseBlueprintProcessor_evaluateInputs(t *testing.T) {
	t.Run("SkipsUnresolvedExpressions", func(t *testing.T) {
		// Given a processor with evaluator that returns unresolved expressions
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			result := make(map[string]any)
			for key, value := range values {
				if key == "deferred" {
					continue
				}
				result[key] = value
			}
			return result, nil
		}

		inputs := map[string]any{
			"deferred": "${terraform_output('cluster', 'key')}",
			"normal":   "value",
		}

		// When evaluating inputs
		result, err := mocks.Evaluator.EvaluateMap(inputs, "", nil, false)

		// Then unresolved expression input should be skipped
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if _, exists := result["deferred"]; exists {
			t.Error("Expected unresolved expression input to be skipped")
		}

		if result["normal"] != "value" {
			t.Errorf("Expected normal input to be preserved, got %v", result["normal"])
		}
	})

	t.Run("SkipsUnresolvedExpressionsInInterpolatedString", func(t *testing.T) {
		// Given a processor with evaluator that returns unresolved expressions for interpolated strings
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			result := make(map[string]any)
			for key := range values {
				if key == "deferred" {
					continue
				}
				result[key] = values[key]
			}
			return result, nil
		}

		inputs := map[string]any{
			"deferred": "prefix-${terraform_output('cluster', 'key')}-suffix",
		}

		// When evaluating inputs
		result, err := mocks.Evaluator.EvaluateMap(inputs, "", nil, false)

		// Then unresolved expression input should be skipped
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if _, exists := result["deferred"]; exists {
			t.Error("Expected unresolved expression input to be skipped")
		}
	})

	t.Run("HandlesEmptyInputs", func(t *testing.T) {
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateFunc = func(expression string, featurePath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return expression, nil
		}

		inputs := map[string]any{}

		result, err := mocks.Evaluator.EvaluateMap(inputs, "", nil, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(result) != 0 {
			t.Errorf("Expected empty result, got %d entries", len(result))
		}
	})

	t.Run("PreservesNonStringValues", func(t *testing.T) {
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateFunc = func(expression string, featurePath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return expression, nil
		}

		inputs := map[string]any{
			"count":   42,
			"enabled": true,
			"tags":    []string{"a", "b"},
			"nested":  map[string]any{"key": "value"},
		}

		result, err := mocks.Evaluator.EvaluateMap(inputs, "", nil, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["count"] != 42 {
			t.Errorf("Expected count to be 42, got %v", result["count"])
		}

		if result["enabled"] != true {
			t.Errorf("Expected enabled to be true, got %v", result["enabled"])
		}

		if tags, ok := result["tags"].([]string); !ok || len(tags) != 2 {
			t.Errorf("Expected tags to be preserved, got %v", result["tags"])
		}
	})

	t.Run("EvaluatesStringExpressions", func(t *testing.T) {
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			result := make(map[string]any)
			for key, value := range values {
				if key == "expression" {
					result[key] = 42
				} else {
					result[key] = value
				}
			}
			return result, nil
		}

		inputs := map[string]any{
			"simple":     "value",
			"expression": "${value}",
		}

		result, err := mocks.Evaluator.EvaluateMap(inputs, "", nil, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["simple"] != "value" {
			t.Errorf("Expected simple to be 'value', got %v", result["simple"])
		}

		if result["expression"] != 42 {
			t.Errorf("Expected expression to be 42, got %v", result["expression"])
		}
	})

	t.Run("ReturnsErrorOnEvaluationFailure", func(t *testing.T) {
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			return nil, fmt.Errorf("failed to evaluate 'bad': evaluation failed")
		}

		inputs := map[string]any{
			"bad": "${invalid}",
		}

		result, err := mocks.Evaluator.EvaluateMap(inputs, "", nil, false)

		if err == nil {
			t.Fatal("Expected error on evaluation failure")
		}

		if result != nil {
			t.Error("Expected nil result on error")
		}

		if !strings.Contains(err.Error(), "failed to evaluate") {
			t.Errorf("Expected error message to contain 'failed to evaluate', got: %v", err)
		}
	})

	t.Run("HandlesMixedInputs", func(t *testing.T) {
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			result := make(map[string]any)
			for key, value := range values {
				if key == "evaluated" {
					result[key] = "evaluated"
				} else {
					result[key] = value
				}
			}
			return result, nil
		}

		inputs := map[string]any{
			"string":    "plain",
			"number":    42,
			"boolean":   true,
			"array":     []string{"a", "b"},
			"evaluated": "${value}",
		}

		result, err := mocks.Evaluator.EvaluateMap(inputs, "", nil, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["string"] != "plain" {
			t.Errorf("Expected string to be 'plain', got %v", result["string"])
		}

		if result["number"] != 42 {
			t.Errorf("Expected number to be 42, got %v", result["number"])
		}

		if result["boolean"] != true {
			t.Errorf("Expected boolean to be true, got %v", result["boolean"])
		}

		if result["evaluated"] != "evaluated" {
			t.Errorf("Expected evaluated to be 'evaluated', got %v", result["evaluated"])
		}
	})

	t.Run("PassesFacetPathToEvaluator", func(t *testing.T) {
		mocks := setupProcessorMocks(t)

		var receivedPath string
		mocks.Evaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			receivedPath = featurePath
			return values, nil
		}

		inputs := map[string]any{
			"test": "value",
		}

		expectedPath := "test/feature/path"
		_, err := mocks.Evaluator.EvaluateMap(inputs, expectedPath, nil, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if receivedPath != expectedPath {
			t.Errorf("Expected feature path to be '%s', got '%s'", expectedPath, receivedPath)
		}
	})
}
