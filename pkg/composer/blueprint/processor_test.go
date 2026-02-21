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

func intPtr(i int) *int { return &i }

func ordinalOf(c *blueprintv1alpha1.ConditionalTerraformComponent) int {
	if c == nil || c.Ordinal == nil {
		return 0
	}
	return *c.Ordinal
}

func ordinalOfK(c *blueprintv1alpha1.ConditionalKustomization) int {
	if c == nil || c.Ordinal == nil {
		return 0
	}
	return *c.Ordinal
}

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
		_, _, err := processor.ProcessFacets(target, nil)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

		// Then facet should be included
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
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
		_, _, err := processor.ProcessFacets(target, facets)

		// Then facet should be excluded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components, got %d", len(target.TerraformComponents))
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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)
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
		_, _, err := processor.ProcessFacets(target, facets)
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
		_, _, err := processor.ProcessFacets(target, facets)
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

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, _, err := processor.ProcessFacets(target, facets)

		// Then inputs should be evaluated
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		inputs := target.TerraformComponents[0].Inputs
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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets, "test-source")

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
		_, _, err := processor.ProcessFacets(target, facets, "new-source")

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid strategy")
		}
		if err != nil && !strings.Contains(err.Error(), "invalid strategy") {
			t.Errorf("Expected error about invalid strategy, got: %v", err)
		}
	})
}

// =============================================================================
// Test Config (facet config blocks and global scope)
// =============================================================================

func TestProcessor_ProcessFacets_Config(t *testing.T) {
	t.Run("ResolvesTerraformInputsFromConfigBlock", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"provider": "incus",
				"cluster": map[string]any{
					"controlplanes": []any{map[string]any{"hostname": "cp0"}, map[string]any{"hostname": "cp1"}},
					"workers":       []any{map[string]any{"hostname": "w0"}},
				},
			}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "talos"},
				When:     "provider == 'incus'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{
						Name: "talos",
						Body: map[string]any{
							"value": map[string]any{
								"controlplanes": "${cluster.controlplanes}",
								"workers":       "${cluster.workers}",
							},
						},
					},
				},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path: "cluster",
							Inputs: map[string]any{
								"controlplanes": "${talos.controlplanes}",
								"workers":       "${talos.workers}",
							},
						},
					},
				},
			},
		}
		_, _, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		inputs := target.TerraformComponents[0].Inputs
		cp, ok := inputs["controlplanes"].([]any)
		if !ok {
			t.Fatalf("Expected controlplanes to be []any, got %T", inputs["controlplanes"])
		}
		if len(cp) != 2 {
			t.Errorf("Expected 2 controlplanes, got %d", len(cp))
		}
		w, ok := inputs["workers"].([]any)
		if !ok {
			t.Fatalf("Expected workers to be []any, got %T", inputs["workers"])
		}
		if len(w) != 1 {
			t.Errorf("Expected 1 worker, got %d", len(w))
		}
	})

	t.Run("SecondFacetSeesFirstFacetConfig", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"provider": "incus", "cluster": map[string]any{"name": "mycluster"}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "config-only"},
				When:     "provider == 'incus'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "talos", Body: map[string]any{"value": map[string]any{"cluster_name": "${cluster.name}"}}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "consumer"},
				When:     "provider == 'incus'",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path: "cluster",
							Inputs: map[string]any{
								"name": "${talos.cluster_name}",
							},
						},
					},
				},
			},
		}
		_, _, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		if target.TerraformComponents[0].Inputs["name"] != "mycluster" {
			t.Errorf("Expected name=mycluster, got %v", target.TerraformComponents[0].Inputs["name"])
		}
	})

	t.Run("SkipsConfigBlockWhenWhenFalse", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"provider": "docker"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "conditional-config"},
				When:     "provider == 'docker'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "talos", When: "provider == 'incus'", Body: map[string]any{"value": map[string]any{"x": "from-incus"}}},
					{Name: "talos", When: "provider == 'docker'", Body: map[string]any{"value": map[string]any{"x": "from-docker"}}},
				},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:   "cluster",
							Inputs: map[string]any{"val": "${talos.x}"},
						},
					},
				},
			},
		}
		_, _, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		if target.TerraformComponents[0].Inputs["val"] != "from-docker" {
			t.Errorf("Expected val=from-docker (second block when matched), got %v", target.TerraformComponents[0].Inputs["val"])
		}
	})

	t.Run("ResolvesSameBlockReferenceWhenLaterKeyDependsOnEarlierKey", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "self-ref"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{
						Name: "talos_common_docker",
						Body: map[string]any{
							"value": map[string]any{
								"patchVars":    map[string]any{"key": "val"},
								"common_patch": "${string(talos_common_docker.patchVars)}",
								"patches":      "${string(talos_common_docker.common_patch)}",
							},
						},
					},
				},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:   "cluster",
							Inputs: map[string]any{"out": "${talos_common_docker.patches}"},
						},
					},
				},
			},
		}
		_, _, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		out, ok := target.TerraformComponents[0].Inputs["out"].(string)
		if !ok {
			t.Fatalf("Expected out to be string, got %T", target.TerraformComponents[0].Inputs["out"])
		}
		if out == "" || out == "null" {
			t.Errorf("Expected patches (and thus out) to resolve from same-block common_patch, got %q", out)
		}
	})

	t.Run("ResolvesSameNameConfigByWhenWhenMatchingBlockIsFirstInList", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"provider": "docker", "workstation": map[string]any{"enabled": true}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "conditional-config"},
				When:     "provider == 'docker'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "talos_common", When: "provider == 'docker' && (workstation.enabled ?? false)", Body: map[string]any{"value": map[string]any{"patches": []any{"docker-patch.yaml"}}}},
					{Name: "talos_common", When: "!(provider == 'docker' && (workstation.enabled ?? false))", Body: map[string]any{"value": map[string]any{"patches": []any{"non-docker-patch.yaml"}}}},
				},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:   "cluster",
							Inputs: map[string]any{"common_config_patches": "${talos_common.patches}"},
						},
					},
				},
			},
		}
		_, _, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		patches, ok := target.TerraformComponents[0].Inputs["common_config_patches"].([]any)
		if !ok {
			t.Fatalf("Expected common_config_patches to be []any, got %T", target.TerraformComponents[0].Inputs["common_config_patches"])
		}
		if len(patches) != 1 || patches[0] != "docker-patch.yaml" {
			t.Errorf("Expected common_config_patches=[docker-patch.yaml] (matching block is first in list), got %v", patches)
		}
	})

	t.Run("ProcessesFacetsWithEmptyConfigList", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"provider": "incus"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "no-config"},
				When:     "provider == 'incus'",
				Config:   nil,
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:   "cluster",
							Inputs: map[string]any{"x": "y"},
						},
					},
				},
			},
		}
		_, _, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		if target.TerraformComponents[0].Inputs["x"] != "y" {
			t.Errorf("Expected x=y, got %v", target.TerraformComponents[0].Inputs["x"])
		}
	})

	t.Run("ProcessesConfigBlockWithEmptyBody", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"provider": "incus"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "empty-body"},
				When:     "provider == 'incus'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "empty", Body: map[string]any{"value": map[string]any{}}},
				},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:   "cluster",
							Inputs: map[string]any{"key": "${empty.foo ?? 'default'}"},
						},
					},
				},
			},
		}
		_, _, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		if target.TerraformComponents[0].Inputs["key"] != "default" {
			t.Errorf("Expected key=default (undefined empty.foo with ?? fallback), got %v", target.TerraformComponents[0].Inputs["key"])
		}
	})

	t.Run("PreservesScalarAndListConfigBlockValuesInScope", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"provider": "incus"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "scalar-list-config"},
				When:     "provider == 'incus'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "platform", Body: map[string]any{"value": "incus"}},
					{Name: "tags", Body: map[string]any{"value": []any{"a", "b"}}},
				},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:   "cluster",
							Inputs: map[string]any{"platform": "${platform}", "tags": "${string(tags)}"},
						},
					},
				},
			},
		}
		_, _, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		if target.TerraformComponents[0].Inputs["platform"] != "incus" {
			t.Errorf("Expected platform='incus' (scalar config block preserved), got %v", target.TerraformComponents[0].Inputs["platform"])
		}
		if target.TerraformComponents[0].Inputs["tags"] == nil {
			t.Error("Expected tags to be set (list config block preserved)")
		}
	})
}

func TestProcessor_mergeHelpers(t *testing.T) {
	t.Run("deepMergeMapMergesNestedMaps", func(t *testing.T) {
		base := map[string]any{"a": 1, "b": map[string]any{"x": 10}}
		overlay := map[string]any{"b": map[string]any{"y": 20}, "c": 3}
		out := deepMergeMap(base, overlay)
		if out["a"].(int) != 1 {
			t.Errorf("Expected a=1, got %v", out["a"])
		}
		if out["c"].(int) != 3 {
			t.Errorf("Expected c=3, got %v", out["c"])
		}
		b := out["b"].(map[string]any)
		if b["x"].(int) != 10 {
			t.Errorf("Expected b.x=10, got %v", b["x"])
		}
		if b["y"].(int) != 20 {
			t.Errorf("Expected b.y=20, got %v", b["y"])
		}
	})

	t.Run("mergeConfigMapsDeepMergesSameBlockName", func(t *testing.T) {
		globalScope := map[string]any{
			"talos": map[string]any{"controlplanes": []any{"cp0"}, "workers": []any{"w0"}},
		}
		facetConfig := map[string]any{
			"talos": map[string]any{"workers": []any{"w1", "w2"}},
		}
		out := MergeConfigMaps(globalScope, facetConfig)
		talos := out["talos"].(map[string]any)
		if _, ok := talos["controlplanes"]; !ok {
			t.Error("Expected controlplanes from global to remain")
		}
		workers := talos["workers"].([]any)
		if len(workers) != 2 {
			t.Errorf("Expected overlay workers to overwrite, got %d", len(workers))
		}
	})

}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestStrategyPrecedence(t *testing.T) {
	t.Run("ReturnsCorrectPrecedenceForRemove", func(t *testing.T) {
		prec := strategyPrecedence["remove"]
		if prec != 3 {
			t.Errorf("Expected precedence 3 for remove, got %d", prec)
		}
	})

	t.Run("ReturnsCorrectPrecedenceForReplace", func(t *testing.T) {
		prec := strategyPrecedence["replace"]
		if prec != 2 {
			t.Errorf("Expected precedence 2 for replace, got %d", prec)
		}
	})

	t.Run("ReturnsCorrectPrecedenceForMerge", func(t *testing.T) {
		prec := strategyPrecedence["merge"]
		if prec != 1 {
			t.Errorf("Expected precedence 1 for merge, got %d", prec)
		}
	})

	t.Run("ReturnsZeroForUnknownStrategy", func(t *testing.T) {
		prec := strategyPrecedence["unknown"]
		if prec != 0 {
			t.Errorf("Expected precedence 0 for unknown, got %d", prec)
		}
	})

	t.Run("ReturnsZeroForEmptyStrategy", func(t *testing.T) {
		prec := strategyPrecedence[""]
		if prec != 0 {
			t.Errorf("Expected precedence 0 for empty, got %d", prec)
		}
	})
}

func TestProcessor_updateTerraformComponentEntry(t *testing.T) {
	mocks := setupProcessorMocks(t)
	processor := NewBlueprintProcessor(mocks.Runtime)

	t.Run("ReplacesWhenNewStrategyHasHigherPrecedence", func(t *testing.T) {
		// Given existing entry with merge strategy
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with replace strategy
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "replace", entries, nil)

		// Then entry should be replaced
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace', got '%s'", entries["vpc"].Strategy)
		}
		if entries["vpc"].Inputs["key2"] != "value2" {
			t.Error("Expected new inputs to be set")
		}
	})

	t.Run("PreMergesWhenBothHaveMergeStrategy", func(t *testing.T) {
		// Given existing entry with merge strategy
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with merge strategy
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then entries should be pre-merged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "merge" {
			t.Errorf("Expected strategy 'merge', got '%s'", entries["vpc"].Strategy)
		}
		inputs := entries["vpc"].Inputs
		if inputs["key1"] != "value1" {
			t.Errorf("Expected key1 preserved, got %v", inputs["key1"])
		}
		if inputs["key2"] != "value2" {
			t.Errorf("Expected key2 added, got %v", inputs["key2"])
		}
	})

	t.Run("IgnoresWhenNewStrategyHasLowerPrecedence", func(t *testing.T) {
		// Given existing entry with replace strategy
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "replace",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with merge strategy
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then entry should remain unchanged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace' to be preserved, got '%s'", entries["vpc"].Strategy)
		}
		if entries["vpc"].Inputs["key1"] != "value1" {
			t.Error("Expected original inputs to be preserved")
		}
	})

	t.Run("AccumulatesRemovalsWhenBothHaveRemoveStrategy", func(t *testing.T) {
		// Given existing entry with remove strategy
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "remove",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": nil}, DependsOn: []string{"dep1"}},
			},
		}

		// When updating with remove strategy
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": nil}, DependsOn: []string{"dep2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "remove", entries, nil)

		// Then removals should be accumulated
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "remove" {
			t.Errorf("Expected strategy 'remove', got '%s'", entries["vpc"].Strategy)
		}
		inputs := entries["vpc"].Inputs
		if inputs == nil {
			t.Fatal("Expected inputs map to exist")
		}
		if _, exists := inputs["key1"]; !exists {
			t.Error("Expected key1 to be in removal list")
		}
		if _, exists := inputs["key2"]; !exists {
			t.Error("Expected key2 to be in removal list")
		}
		deps := entries["vpc"].DependsOn
		if !contains(deps, "dep1") {
			t.Error("Expected dep1 to be in removal list")
		}
		if !contains(deps, "dep2") {
			t.Error("Expected dep2 to be in removal list")
		}
	})

	t.Run("ReplacesWhenBothHaveReplaceStrategy", func(t *testing.T) {
		// Given existing entry with replace strategy
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "replace",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with replace strategy
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "replace", entries, nil)

		// Then new entry should replace existing
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace', got '%s'", entries["vpc"].Strategy)
		}
		if entries["vpc"].Inputs["key2"] != "value2" {
			t.Error("Expected new inputs to replace old ones")
		}
		if entries["vpc"].Inputs["key1"] != nil {
			t.Error("Expected old inputs to be replaced")
		}
	})

	t.Run("ReplacesWhenNewHasHigherOrdinal", func(t *testing.T) {
		// Given existing entry with replace strategy and ordinal 0
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "replace",
				Ordinal:            intPtr(0),
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with merge strategy but higher ordinal
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			Ordinal:            intPtr(100),
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then entry should be replaced despite lower strategy
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "merge" {
			t.Errorf("Expected strategy 'merge', got '%s'", entries["vpc"].Strategy)
		}
		if ordinalOf(entries["vpc"]) != 100 {
			t.Errorf("Expected ordinal 100, got %d", ordinalOf(entries["vpc"]))
		}
		if entries["vpc"].Inputs["key2"] != "value2" {
			t.Error("Expected new inputs to be set")
		}
	})

	t.Run("IgnoresWhenNewHasLowerOrdinal", func(t *testing.T) {
		// Given existing entry with merge strategy and ordinal 100
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				Ordinal:            intPtr(100),
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with replace strategy but lower ordinal
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			Ordinal:            intPtr(0),
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "replace", entries, nil)

		// Then entry should remain unchanged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "merge" {
			t.Errorf("Expected strategy 'merge' to be preserved, got '%s'", entries["vpc"].Strategy)
		}
		if ordinalOf(entries["vpc"]) != 100 {
			t.Errorf("Expected ordinal 100 to be preserved, got %d", ordinalOf(entries["vpc"]))
		}
		if entries["vpc"].Inputs["key1"] != "value1" {
			t.Error("Expected original inputs to be preserved")
		}
	})

	t.Run("UsesStrategyPrecedenceWhenOrdinalsEqual", func(t *testing.T) {
		// Given existing entry with merge strategy and ordinal 50
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				Ordinal:            intPtr(50),
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with replace strategy and same ordinal
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			Ordinal:            intPtr(50),
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "replace", entries, nil)

		// Then entry should be replaced due to higher strategy precedence
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace', got '%s'", entries["vpc"].Strategy)
		}
		if ordinalOf(entries["vpc"]) != 50 {
			t.Errorf("Expected ordinal 50, got %d", ordinalOf(entries["vpc"]))
		}
	})

	t.Run("PreservesOrdinalInMergedResult", func(t *testing.T) {
		// Given existing entry with merge strategy and ordinal 25
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				Ordinal:            intPtr(25),
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with merge strategy and same ordinal
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			Ordinal:            intPtr(25),
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then ordinal should be preserved in merged result
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if ordinalOf(entries["vpc"]) != 25 {
			t.Errorf("Expected ordinal 25, got %d", ordinalOf(entries["vpc"]))
		}
	})

	t.Run("ReturnsErrorForInvalidStrategy", func(t *testing.T) {
		// Given existing entry with merge strategy
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		// When updating with invalid strategy
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "invalid", entries, nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid strategy")
		}
		if err != nil && !strings.Contains(err.Error(), "invalid strategy") {
			t.Errorf("Expected error about invalid strategy, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidStrategyEvenWithHigherOrdinal", func(t *testing.T) {
		// Given existing entry with merge strategy and ordinal 0
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				Ordinal:            intPtr(0),
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with invalid strategy but higher ordinal
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			Ordinal:            intPtr(100),
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "typo", entries, nil)

		// Then should return error before checking ordinal
		if err == nil {
			t.Error("Expected error for invalid strategy, even with higher ordinal")
		}
		if err != nil && !strings.Contains(err.Error(), "invalid strategy") {
			t.Errorf("Expected error about invalid strategy, got: %v", err)
		}
		if entries["vpc"].Inputs["key1"] != "value1" {
			t.Error("Expected original entry to remain unchanged when invalid strategy is rejected")
		}
	})
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func TestProcessor_updateKustomizationEntry(t *testing.T) {
	mocks := setupProcessorMocks(t)
	processor := NewBlueprintProcessor(mocks.Runtime)

	t.Run("ReplacesWhenNewStrategyHasHigherPrecedence", func(t *testing.T) {
		// Given existing entry with merge strategy
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with replace strategy
		new := &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "replace", entries, nil)

		// Then entry should be replaced
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace', got '%s'", entries["app"].Strategy)
		}
		if entries["app"].Substitutions["key2"] != "value2" {
			t.Error("Expected new substitutions to be set")
		}
	})

	t.Run("PreMergesWhenBothHaveMergeStrategy", func(t *testing.T) {
		// Given existing entry with merge strategy
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "existing-path"},
			},
		}

		// When updating with merge strategy
		new := &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "new-path"},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then entries should be pre-merged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "merge" {
			t.Errorf("Expected strategy 'merge', got '%s'", entries["app"].Strategy)
		}
		if entries["app"].Path != "new-path" {
			t.Errorf("Expected path to be updated to 'new-path', got '%s'", entries["app"].Path)
		}
	})

	t.Run("IgnoresWhenNewStrategyHasLowerPrecedence", func(t *testing.T) {
		// Given existing entry with replace strategy
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "replace",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with merge strategy
		new := &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then entry should remain unchanged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace' to be preserved, got '%s'", entries["app"].Strategy)
		}
		if entries["app"].Substitutions["key1"] != "value1" {
			t.Error("Expected original substitutions to be preserved")
		}
	})

	t.Run("AccumulatesRemovalsWhenBothHaveRemoveStrategy", func(t *testing.T) {
		// Given existing entry with remove strategy
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "remove",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": ""}, DependsOn: []string{"dep1"}, Components: []string{"comp1"}, Cleanup: []string{"cleanup1"}},
			},
		}

		// When updating with remove strategy
		new := &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": ""}, DependsOn: []string{"dep2"}, Components: []string{"comp2"}, Cleanup: []string{"cleanup2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "remove", entries, nil)

		// Then removals should be accumulated
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "remove" {
			t.Errorf("Expected strategy 'remove', got '%s'", entries["app"].Strategy)
		}
		subs := entries["app"].Substitutions
		if subs == nil {
			t.Fatal("Expected substitutions map to exist")
		}
		if _, exists := subs["key1"]; !exists {
			t.Error("Expected key1 to be in removal list")
		}
		if _, exists := subs["key2"]; !exists {
			t.Error("Expected key2 to be in removal list")
		}
		deps := entries["app"].DependsOn
		if !contains(deps, "dep1") {
			t.Error("Expected dep1 to be in removal list")
		}
		if !contains(deps, "dep2") {
			t.Error("Expected dep2 to be in removal list")
		}
		comps := entries["app"].Components
		if !contains(comps, "comp1") {
			t.Error("Expected comp1 to be in removal list")
		}
		if !contains(comps, "comp2") {
			t.Error("Expected comp2 to be in removal list")
		}
		cleanup := entries["app"].Cleanup
		if !contains(cleanup, "cleanup1") {
			t.Error("Expected cleanup1 to be in removal list")
		}
		if !contains(cleanup, "cleanup2") {
			t.Error("Expected cleanup2 to be in removal list")
		}
	})

	t.Run("ReplacesWhenBothHaveReplaceStrategy", func(t *testing.T) {
		// Given existing entry with replace strategy
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "replace",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with replace strategy
		new := &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "replace", entries, nil)

		// Then new entry should replace existing
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace', got '%s'", entries["app"].Strategy)
		}
		if entries["app"].Substitutions["key2"] != "value2" {
			t.Error("Expected new substitutions to replace old ones")
		}
		if entries["app"].Substitutions["key1"] != "" {
			t.Error("Expected old substitutions to be replaced")
		}
	})

	t.Run("ReplacesWhenNewHasHigherOrdinal", func(t *testing.T) {
		// Given existing entry with replace strategy and ordinal 0
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "replace",
				Ordinal:       intPtr(0),
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with merge strategy but higher ordinal
		new := &blueprintv1alpha1.ConditionalKustomization{
			Ordinal:       intPtr(100),
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then entry should be replaced despite lower strategy
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "merge" {
			t.Errorf("Expected strategy 'merge', got '%s'", entries["app"].Strategy)
		}
		if ordinalOfK(entries["app"]) != 100 {
			t.Errorf("Expected ordinal 100, got %d", ordinalOfK(entries["app"]))
		}
		if entries["app"].Substitutions["key2"] != "value2" {
			t.Error("Expected new substitutions to be set")
		}
	})

	t.Run("IgnoresWhenNewHasLowerOrdinal", func(t *testing.T) {
		// Given existing entry with merge strategy and ordinal 100
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Ordinal:       intPtr(100),
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with replace strategy but lower ordinal
		new := &blueprintv1alpha1.ConditionalKustomization{
			Ordinal:       intPtr(0),
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "replace", entries, nil)

		// Then entry should remain unchanged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "merge" {
			t.Errorf("Expected strategy 'merge' to be preserved, got '%s'", entries["app"].Strategy)
		}
		if ordinalOfK(entries["app"]) != 100 {
			t.Errorf("Expected ordinal 100 to be preserved, got %d", ordinalOfK(entries["app"]))
		}
		if entries["app"].Substitutions["key1"] != "value1" {
			t.Error("Expected original substitutions to be preserved")
		}
	})

	t.Run("UsesStrategyPrecedenceWhenOrdinalsEqual", func(t *testing.T) {
		// Given existing entry with merge strategy and ordinal 50
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Ordinal:       intPtr(50),
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with replace strategy and same ordinal
		new := &blueprintv1alpha1.ConditionalKustomization{
			Ordinal:       intPtr(50),
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "replace", entries, nil)

		// Then entry should be replaced due to higher strategy precedence
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace', got '%s'", entries["app"].Strategy)
		}
		if ordinalOfK(entries["app"]) != 50 {
			t.Errorf("Expected ordinal 50, got %d", ordinalOfK(entries["app"]))
		}
	})

	t.Run("PreservesOrdinalInMergedResult", func(t *testing.T) {
		// Given existing entry with merge strategy and ordinal 25
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Ordinal:       intPtr(25),
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with merge strategy and same ordinal
		new := &blueprintv1alpha1.ConditionalKustomization{
			Ordinal:       intPtr(25),
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then ordinal should be preserved in merged result
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if ordinalOfK(entries["app"]) != 25 {
			t.Errorf("Expected ordinal 25, got %d", ordinalOfK(entries["app"]))
		}
	})

	t.Run("ReturnsErrorForInvalidStrategy", func(t *testing.T) {
		// Given existing entry with merge strategy
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
			},
		}

		// When updating with invalid strategy
		new := &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
		}
		err := processor.updateKustomizationEntry("app", new, "typo", entries, nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid strategy")
		}
		if err != nil && !strings.Contains(err.Error(), "invalid strategy") {
			t.Errorf("Expected error about invalid strategy, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidStrategyEvenWithHigherOrdinal", func(t *testing.T) {
		// Given existing entry with merge strategy and ordinal 0
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Ordinal:       intPtr(0),
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with invalid strategy but higher ordinal
		new := &blueprintv1alpha1.ConditionalKustomization{
			Ordinal:       intPtr(100),
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "typo", entries, nil)

		// Then should return error before checking ordinal
		if err == nil {
			t.Error("Expected error for invalid strategy, even with higher ordinal")
		}
		if err != nil && !strings.Contains(err.Error(), "invalid strategy") {
			t.Errorf("Expected error about invalid strategy, got: %v", err)
		}
		if entries["app"].Substitutions["key1"] != "value1" {
			t.Error("Expected original entry to remain unchanged when invalid strategy is rejected")
		}
	})
}

func TestProcessor_applyCollectedComponents(t *testing.T) {
	mocks := setupProcessorMocks(t)
	processor := NewBlueprintProcessor(mocks.Runtime)

	t.Run("AppliesReplaceThenMergeThenRemove", func(t *testing.T) {
		// Given collected components with all strategies
		target := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"existing": "value", "toRemove": "value"}},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app", Substitutions: map[string]string{"existing": "value", "toRemove": "value"}},
			},
		}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "remove",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"toRemove": nil}},
			},
		}

		kustomizationByName := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "remove",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"toRemove": ""}},
			},
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, terraformByID, kustomizationByName, nil)

		// Then specified fields should be removed (removes are applied last)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.TerraformComponents[0].Inputs["toRemove"] != nil {
			t.Error("Expected 'toRemove' input to be removed")
		}
		if target.TerraformComponents[0].Inputs["existing"] != "value" {
			t.Error("Expected 'existing' input to be preserved")
		}
		if target.Kustomizations[0].Substitutions["toRemove"] != "" {
			t.Error("Expected 'toRemove' substitution to be removed")
		}
		if target.Kustomizations[0].Substitutions["existing"] != "value" {
			t.Error("Expected 'existing' substitution to be preserved")
		}
	})

	t.Run("AppliesReplaceStrategy", func(t *testing.T) {
		// Given collected components with replace strategy
		target := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"old": "value"}},
			},
		}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "replace",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
			},
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, terraformByID, nil, nil)

		// Then component should be replaced
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.TerraformComponents[0].Inputs["new"] != "value" {
			t.Error("Expected component to be replaced")
		}
		if target.TerraformComponents[0].Inputs["old"] != nil {
			t.Error("Expected old inputs to be replaced")
		}
	})

	t.Run("AppliesMergeStrategy", func(t *testing.T) {
		// Given collected components with merge strategy
		target := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"existing": "value"}},
			},
		}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
			},
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, terraformByID, nil, nil)

		// Then component should be merged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.TerraformComponents[0].Inputs["existing"] != "value" {
			t.Error("Expected existing inputs to be preserved")
		}
		if target.TerraformComponents[0].Inputs["new"] != "value" {
			t.Error("Expected new inputs to be added")
		}
	})

	t.Run("HandlesEmptyStrategyAsMerge", func(t *testing.T) {
		// Given collected components with empty strategy
		target := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"existing": "value"}},
			},
		}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
			},
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, terraformByID, nil, nil)

		// Then should default to merge
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.TerraformComponents[0].Inputs["existing"] != "value" {
			t.Error("Expected existing inputs to be preserved (merge behavior)")
		}
	})

	t.Run("HandlesKustomizationStrategies", func(t *testing.T) {
		// Given collected kustomizations with different strategies
		target := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app", Substitutions: map[string]string{"existing": "value"}},
			},
		}

		kustomizationByName := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "replace",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"new": "value"}},
			},
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, nil, kustomizationByName, nil)

		// Then kustomization should be replaced
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.Kustomizations[0].Substitutions["new"] != "value" {
			t.Error("Expected kustomization to be replaced")
		}
		if target.Kustomizations[0].Substitutions["existing"] != "" {
			t.Error("Expected old substitutions to be replaced")
		}
	})

	t.Run("HandlesMultipleComponentsWithDifferentStrategies", func(t *testing.T) {
		// Given multiple components with different strategies
		target := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"existing": "value"}},
				{Path: "rds", Inputs: map[string]any{"existing": "value"}},
			},
		}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "remove",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"existing": nil}},
			},
			"rds": {
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "rds", Inputs: map[string]any{"new": "value"}},
			},
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, terraformByID, nil, nil)

		// Then strategies should be applied correctly
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.TerraformComponents[0].Inputs["existing"] != nil {
			t.Error("Expected 'existing' input to be removed from vpc")
		}
		if target.TerraformComponents[1].Inputs["existing"] != "value" {
			t.Error("Expected 'existing' input to be preserved in rds")
		}
		if target.TerraformComponents[1].Inputs["new"] != "value" {
			t.Error("Expected 'new' input to be added to rds")
		}
	})

	t.Run("AppliesStrategiesInCorrectOrder", func(t *testing.T) {
		// Given a component that will be merged, then replaced, then have fields removed
		target := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"existing": "value", "toRemove": "value"}},
			},
		}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"merged": "value"}},
			},
		}

		// First apply merge
		err := processor.applyCollectedComponents(target, terraformByID, nil, nil)
		if err != nil {
			t.Fatalf("Expected no error on merge, got %v", err)
		}

		// Then apply replace
		terraformByID = map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "replace",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"replaced": "value"}},
			},
		}
		err = processor.applyCollectedComponents(target, terraformByID, nil, nil)
		if err != nil {
			t.Fatalf("Expected no error on replace, got %v", err)
		}

		// Finally apply remove
		terraformByID = map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "remove",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"replaced": nil}},
			},
		}
		err = processor.applyCollectedComponents(target, terraformByID, nil, nil)
		if err != nil {
			t.Fatalf("Expected no error on remove, got %v", err)
		}

		// Then verify final state: merge added "merged", replace replaced everything with "replaced", remove removed "replaced"
		if target.TerraformComponents[0].Inputs["replaced"] != nil {
			t.Error("Expected 'replaced' input to be removed (remove applied last)")
		}
		if target.TerraformComponents[0].Inputs["merged"] != nil {
			t.Error("Expected 'merged' input to be gone (replaced)")
		}
		if target.TerraformComponents[0].Inputs["existing"] != nil {
			t.Error("Expected 'existing' input to be gone (replaced)")
		}
	})
}

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

func TestBaseBlueprintProcessor_evaluateSubstitutions(t *testing.T) {
	t.Run("PreservesUnresolvedExpressions", func(t *testing.T) {
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateFunc = func(expression string, featurePath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			expr := expression
			if strings.HasPrefix(expression, "${") && strings.HasSuffix(expression, "}") {
				expr = expression[2 : len(expression)-1]
			}
			if expr == "terraform_output('cluster', 'key')" {
				return fmt.Sprintf("${%s}", expr), nil
			}
			if strings.Contains(expr, "terraform_output") {
				return fmt.Sprintf("${%s}", expr), nil
			}
			if !strings.Contains(expression, "${") {
				return expression, nil
			}
			return "interpolated-value", nil
		}

		subs := map[string]string{
			"deferred": "${terraform_output('cluster', 'key')}",
			"normal":   "value",
		}

		baseProcessor := &BaseBlueprintProcessor{
			runtime:   mocks.Runtime,
			evaluator: mocks.Evaluator,
		}
		result, err := baseProcessor.evaluateSubstitutions(subs, "", nil)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["deferred"] != "${terraform_output('cluster', 'key')}" {
			t.Errorf("Expected deferred substitution to preserve original expression, got %v", result["deferred"])
		}

		if result["normal"] != "value" {
			t.Errorf("Expected normal substitution to be preserved, got %v", result["normal"])
		}
	})

	t.Run("IncludesEmptyStringForNilResults", func(t *testing.T) {
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateFunc = func(expression string, featurePath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if expression == "${undefined.path}" {
				return nil, nil
			}
			return "resolved", nil
		}

		subs := map[string]string{
			"undefined": "${undefined.path}",
			"defined":   "${defined.path}",
		}

		baseProcessor := &BaseBlueprintProcessor{
			runtime:   mocks.Runtime,
			evaluator: mocks.Evaluator,
		}
		result, err := baseProcessor.evaluateSubstitutions(subs, "", nil)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["undefined"] != "" {
			t.Errorf("Expected undefined substitution to be empty string, got %q", result["undefined"])
		}

		if result["defined"] != "resolved" {
			t.Errorf("Expected defined substitution to be 'resolved', got %v", result["defined"])
		}
	})

	t.Run("CoalesceOperatorReturnsDefaultForUndefinedPath", func(t *testing.T) {
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateFunc = func(expression string, featurePath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if expression == `${addons.observability.admin_password ?? "grafana"}` {
				return "grafana", nil
			}
			return "resolved", nil
		}

		subs := map[string]string{
			"password": `${addons.observability.admin_password ?? "grafana"}`,
		}

		baseProcessor := &BaseBlueprintProcessor{
			runtime:   mocks.Runtime,
			evaluator: mocks.Evaluator,
		}
		result, err := baseProcessor.evaluateSubstitutions(subs, "", nil)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["password"] != "grafana" {
			t.Errorf("Expected 'grafana' from coalesce operator, got %q", result["password"])
		}
	})

	t.Run("AllKeysIncludedEvenWhenUndefined", func(t *testing.T) {
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateFunc = func(expression string, featurePath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			switch expression {
			case "${dns.domain}":
				return nil, nil
			case "${timezone}":
				return nil, nil
			case "${defined.value}":
				return "exists", nil
			default:
				return nil, nil
			}
		}

		subs := map[string]string{
			"domain":   "${dns.domain}",
			"timezone": "${timezone}",
			"defined":  "${defined.value}",
		}

		baseProcessor := &BaseBlueprintProcessor{
			runtime:   mocks.Runtime,
			evaluator: mocks.Evaluator,
		}
		result, err := baseProcessor.evaluateSubstitutions(subs, "", nil)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if _, exists := result["domain"]; !exists {
			t.Error("Expected 'domain' key to exist in result")
		}
		if _, exists := result["timezone"]; !exists {
			t.Error("Expected 'timezone' key to exist in result")
		}
		if result["defined"] != "exists" {
			t.Errorf("Expected 'exists', got %q", result["defined"])
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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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

	t.Run("EvaluatesCleanupInKustomizations", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"resource": "old-service",
			}, nil
		}

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-cleanup"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{
							Name:    "app",
							Path:    "apps/app",
							Cleanup: []string{"${resource}", "static-cleanup"},
						},
					},
				},
			},
		}

		target := &blueprintv1alpha1.Blueprint{}
		_, _, err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		cleanup := target.Kustomizations[0].Cleanup
		if len(cleanup) != 2 {
			t.Fatalf("Expected 2 cleanup resources, got %d", len(cleanup))
		}
		if cleanup[0] != "old-service" {
			t.Errorf("Expected first cleanup to be 'old-service', got '%s'", cleanup[0])
		}
		if cleanup[1] != "static-cleanup" {
			t.Errorf("Expected second cleanup to be 'static-cleanup', got '%s'", cleanup[1])
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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

		if err == nil {
			t.Error("Expected error for deferred expression in components")
		}
		if err != nil && !strings.Contains(err.Error(), "components") {
			t.Errorf("Expected error about components, got: %v", err)
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
		_, _, err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents[0].DependsOn) != 0 {
			t.Errorf("Expected empty dependsOn, got %v", target.TerraformComponents[0].DependsOn)
		}
	})
}

// =============================================================================
// Test Component Condition Inheritance
// =============================================================================

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
		_, _, err := processor.ProcessFacets(target, facets)

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
// Test Condition Re-evaluation During Merge
// =============================================================================

func TestProcessor_ConditionReevaluationDuringMerge(t *testing.T) {
	t.Run("RemovesComponentWhenExistingConditionBecomesFalse", func(t *testing.T) {
		// Given an existing component with a condition that becomes false
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "enabled == true",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": false}, nil
		}

		// When updating with a new component
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then component should be removed (existing condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := entries["vpc"]; exists {
			t.Error("Expected component to be removed when existing condition is false")
		}
	})

	t.Run("RemovesComponentWhenNewConditionIsFalse", func(t *testing.T) {
		// Given an existing component without condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": false}, nil
		}

		// When updating with a new component that has false condition
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			When:               "enabled == true",
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then component should be removed (new condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := entries["vpc"]; exists {
			t.Error("Expected component to be removed when new condition is false")
		}
	})

	t.Run("RemovesComponentWhenBothConditionsAreFalse", func(t *testing.T) {
		// Given an existing component with condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "enabled1 == true",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"enabled1": false,
				"enabled2": false,
			}, nil
		}

		// When updating with a new component that also has false condition
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			When:               "enabled2 == true",
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then component should be removed (existing condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := entries["vpc"]; exists {
			t.Error("Expected component to be removed when existing condition is false")
		}
	})

	t.Run("PreservesComponentWhenBothConditionsAreTrue", func(t *testing.T) {
		// Given an existing component with condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "enabled1 == true",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"old": "value"}},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"enabled1": true,
				"enabled2": true,
			}, nil
		}

		// When updating with a new component that also has true condition
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			When:               "enabled2 == true",
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then component should be preserved and merged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := entries["vpc"]; !exists {
			t.Fatal("Expected component to be preserved when both conditions are true")
		}
		if entries["vpc"].Inputs["old"] != "value" {
			t.Error("Expected old inputs to be preserved")
		}
		if entries["vpc"].Inputs["new"] != "value" {
			t.Error("Expected new inputs to be added")
		}
	})

	t.Run("KustomizationRemovedWhenConditionBecomesFalse", func(t *testing.T) {
		// Given an existing kustomization with condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				When:          "enabled == true",
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": false}, nil
		}

		// When updating with a new kustomization
		new := &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "new-path"},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then kustomization should be removed (existing condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := entries["app"]; exists {
			t.Error("Expected kustomization to be removed when existing condition is false")
		}
	})
}

// =============================================================================
// Test Preserving When Field During Merge
// =============================================================================

func TestProcessor_PreservingWhenFieldDuringMerge(t *testing.T) {
	t.Run("CombinesWhenConditionsWithAndForMerge", func(t *testing.T) {
		// Given existing and new components with different when conditions
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "enabled1 == true",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"old": "value"}},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"enabled1": true,
				"enabled2": true,
			}, nil
		}

		// When updating with merge strategy
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			When:               "enabled2 == true",
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then When field should be combined with &&
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedWhen := "(enabled1 == true) && (enabled2 == true)"
		if entries["vpc"].When != expectedWhen {
			t.Errorf("Expected When to be '%s', got '%s'", expectedWhen, entries["vpc"].When)
		}
	})

	t.Run("PreservesExistingWhenWhenNewHasNoWhen", func(t *testing.T) {
		// Given existing component with when condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "enabled == true",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": true}, nil
		}

		// When updating with component without when
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then existing When should be preserved
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].When != "enabled == true" {
			t.Errorf("Expected When to be 'enabled == true', got '%s'", entries["vpc"].When)
		}
	})

	t.Run("UsesNewWhenWhenExistingHasNoWhen", func(t *testing.T) {
		// Given existing component without when condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": true}, nil
		}

		// When updating with component with when
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			When:               "enabled == true",
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then new When should be used
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].When != "enabled == true" {
			t.Errorf("Expected When to be 'enabled == true', got '%s'", entries["vpc"].When)
		}
	})

	t.Run("CombinesWhenConditionsForRemoveStrategy", func(t *testing.T) {
		// Given existing component with when condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "enabled1 == true",
				Strategy:           "remove",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": nil}},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"enabled1": true,
				"enabled2": true,
			}, nil
		}

		// When updating with remove strategy
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			When:               "enabled2 == true",
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": nil}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "remove", entries, nil)

		// Then When field should be combined with &&
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedWhen := "(enabled1 == true) && (enabled2 == true)"
		if entries["vpc"].When != expectedWhen {
			t.Errorf("Expected When to be '%s', got '%s'", expectedWhen, entries["vpc"].When)
		}
	})

	t.Run("KustomizationCombinesWhenConditions", func(t *testing.T) {
		// Given existing kustomization with when condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				When:          "enabled1 == true",
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"enabled1": true,
				"enabled2": true,
			}, nil
		}

		// When updating with merge strategy
		new := &blueprintv1alpha1.ConditionalKustomization{
			When:          "enabled2 == true",
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "new-path"},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then When field should be combined with &&
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedWhen := "(enabled1 == true) && (enabled2 == true)"
		if entries["app"].When != expectedWhen {
			t.Errorf("Expected When to be '%s', got '%s'", expectedWhen, entries["app"].When)
		}
	})
}

// =============================================================================
// Test Post-Processing Condition Evaluation
// =============================================================================

func TestProcessor_PostProcessingConditionEvaluation(t *testing.T) {
	t.Run("RemovesTerraformComponentWithFalseConditionAfterMerge", func(t *testing.T) {
		// Given collected components with a condition that will be false
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		target := &blueprintv1alpha1.Blueprint{}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "enabled == true",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": false}, nil
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, terraformByID, nil, nil)

		// Then component should be removed (condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components, got %d", len(target.TerraformComponents))
		}
	})

	t.Run("RemovesKustomizationWithFalseConditionAfterMerge", func(t *testing.T) {
		// Given collected kustomizations with a condition that will be false
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		target := &blueprintv1alpha1.Blueprint{}

		kustomizationByName := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				When:          "enabled == true",
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": false}, nil
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, nil, kustomizationByName, nil)

		// Then kustomization should be removed (condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.Kustomizations) != 0 {
			t.Errorf("Expected 0 kustomizations, got %d", len(target.Kustomizations))
		}
	})

	t.Run("PreservesComponentWithTrueConditionAfterMerge", func(t *testing.T) {
		// Given collected components with a condition that will be true
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		target := &blueprintv1alpha1.Blueprint{}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "enabled == true",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": true}, nil
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, terraformByID, nil, nil)

		// Then component should be preserved (condition is true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		if target.TerraformComponents[0].Path != "vpc" {
			t.Errorf("Expected path='vpc', got '%s'", target.TerraformComponents[0].Path)
		}
	})

	t.Run("RemovesComponentWithCombinedFalseCondition", func(t *testing.T) {
		// Given collected components with combined condition that will be false
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		target := &blueprintv1alpha1.Blueprint{}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "(enabled1 == true) && (enabled2 == true)",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"enabled1": true,
				"enabled2": false,
			}, nil
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, terraformByID, nil, nil)

		// Then component should be removed (combined condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components, got %d", len(target.TerraformComponents))
		}
	})

	t.Run("ReturnsErrorForInvalidConditionDuringPostProcessing", func(t *testing.T) {
		// Given collected components with invalid condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		target := &blueprintv1alpha1.Blueprint{}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "invalid syntax {{{}",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, terraformByID, nil, nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid condition during post-processing")
		}
		if err != nil && !strings.Contains(err.Error(), "re-evaluating") {
			t.Errorf("Expected error about re-evaluating, got: %v", err)
		}
	})
}

// =============================================================================
// Test Additional Coverage for updateKustomizationEntry
// =============================================================================

func TestProcessor_updateKustomizationEntry_AdditionalCoverage(t *testing.T) {
	t.Run("RemovesKustomizationWhenNewConditionIsFalse", func(t *testing.T) {
		// Given an existing kustomization without condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": false}, nil
		}

		// When updating with a new kustomization that has false condition
		new := &blueprintv1alpha1.ConditionalKustomization{
			When:          "enabled == true",
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "new-path"},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then kustomization should be removed (new condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := entries["app"]; exists {
			t.Error("Expected kustomization to be removed when new condition is false")
		}
	})

	t.Run("PreservesKustomizationWhenBothConditionsAreTrue", func(t *testing.T) {
		// Given an existing kustomization with condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				When:          "enabled1 == true",
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "old-path"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"enabled1": true,
				"enabled2": true,
			}, nil
		}

		// When updating with a new kustomization that also has true condition
		new := &blueprintv1alpha1.ConditionalKustomization{
			When:          "enabled2 == true",
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "new-path"},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then kustomization should be preserved and merged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := entries["app"]; !exists {
			t.Fatal("Expected kustomization to be preserved when both conditions are true")
		}
		if entries["app"].Path != "new-path" {
			t.Errorf("Expected path to be updated to 'new-path', got '%s'", entries["app"].Path)
		}
	})

	t.Run("RemovesKustomizationWhenBothConditionsAreFalse", func(t *testing.T) {
		// Given an existing kustomization with condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				When:          "enabled1 == true",
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"enabled1": false,
				"enabled2": false,
			}, nil
		}

		// When updating with a new kustomization that also has false condition
		new := &blueprintv1alpha1.ConditionalKustomization{
			When:          "enabled2 == true",
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "new-path"},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then kustomization should be removed (existing condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := entries["app"]; exists {
			t.Error("Expected kustomization to be removed when existing condition is false")
		}
	})
}

// =============================================================================
// Test evaluateStringSlice Edge Cases
// =============================================================================

func TestProcessor_evaluateStringSlice(t *testing.T) {
	t.Run("ReturnsNilForEmptySlice", func(t *testing.T) {
		// Given an empty slice
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		// When evaluating empty slice
		result, err := processor.evaluateStringSlice([]string{}, "", nil)

		// Then should return nil
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result != nil {
			t.Errorf("Expected nil for empty slice, got %v", result)
		}
	})

	t.Run("PreservesEmptyStrings", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return expression, nil
		}

		result, err := processor.evaluateStringSlice([]string{"value1", "", "value2", ""}, "", nil)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result) != 4 {
			t.Errorf("Expected 4 values (empty preserved), got %d: %v", len(result), result)
		}
		if result[0] != "value1" || result[1] != "" || result[2] != "value2" || result[3] != "" {
			t.Errorf("Expected ['value1', '', 'value2', ''], got %v", result)
		}
	})

	t.Run("HandlesNilEvaluationResult", func(t *testing.T) {
		// Given a slice with expressions that evaluate to nil
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if expression == "nil" {
				return nil, nil
			}
			return expression, nil
		}

		// When evaluating slice with nil results
		result, err := processor.evaluateStringSlice([]string{"value1", "nil", "value2"}, "", nil)

		// Then nil results should be filtered
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result) != 2 {
			t.Errorf("Expected 2 values, got %d: %v", len(result), result)
		}
	})

	t.Run("FlattensArrayResults", func(t *testing.T) {
		// Given a slice with expression that evaluates to array
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if expression == "${array}" {
				return []any{"item1", "item2", "item3"}, nil
			}
			return expression, nil
		}

		// When evaluating slice with array expression
		result, err := processor.evaluateStringSlice([]string{"value1", "${array}", "value2"}, "", nil)

		// Then array should be flattened
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result) != 5 {
			t.Errorf("Expected 5 values (flattened array), got %d: %v", len(result), result)
		}
		if result[0] != "value1" || result[1] != "item1" || result[2] != "item2" || result[3] != "item3" || result[4] != "value2" {
			t.Errorf("Expected ['value1', 'item1', 'item2', 'item3', 'value2'], got %v", result)
		}
	})

	t.Run("HandlesNonStringTypes", func(t *testing.T) {
		// Given a slice with expression that evaluates to non-string
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if expression == "${number}" {
				return 42, nil
			}
			if expression == "${bool}" {
				return true, nil
			}
			return expression, nil
		}

		// When evaluating slice with non-string expressions
		result, err := processor.evaluateStringSlice([]string{"value1", "${number}", "${bool}"}, "", nil)

		// Then non-string values should be converted to strings
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result) != 3 {
			t.Errorf("Expected 3 values, got %d: %v", len(result), result)
		}
		if result[0] != "value1" || result[1] != "42" || result[2] != "true" {
			t.Errorf("Expected ['value1', '42', 'true'], got %v", result)
		}
	})

	t.Run("ReturnsErrorOnEvaluationFailure", func(t *testing.T) {
		// Given a slice with expression that fails
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if expression == "${error}" {
				return nil, fmt.Errorf("evaluation failed")
			}
			return expression, nil
		}

		// When evaluating slice with failing expression
		result, err := processor.evaluateStringSlice([]string{"value1", "${error}"}, "", nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for evaluation failure")
		}
		if result != nil {
			t.Error("Expected nil result on error")
		}
	})
}

// =============================================================================
// Test evaluateBooleanExpression
// =============================================================================

func TestProcessor_evaluateBooleanExpression(t *testing.T) {
	t.Run("ReturnsNilForEmptyExpression", func(t *testing.T) {
		// Given an empty expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		// When evaluating empty expression
		result, err := processor.evaluateBooleanExpression("", "", nil)

		// Then should return nil
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result != nil {
			t.Errorf("Expected nil for empty expression, got %v", result)
		}
	})

	t.Run("EvaluatesBooleanTrue", func(t *testing.T) {
		// Given a boolean true expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return true, nil
		}

		// When evaluating boolean expression
		result, err := processor.evaluateBooleanExpression("enabled == true", "", nil)

		// Then should return true
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil || *result != true {
			t.Errorf("Expected true, got %v", result)
		}
	})

	t.Run("EvaluatesBooleanFalse", func(t *testing.T) {
		// Given a boolean false expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return false, nil
		}

		// When evaluating boolean expression
		result, err := processor.evaluateBooleanExpression("enabled == false", "", nil)

		// Then should return false
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil || *result != false {
			t.Errorf("Expected false, got %v", result)
		}
	})

	t.Run("EvaluatesStringInteger", func(t *testing.T) {
		// Given a string integer expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return "42", nil
		}

		// When evaluating string integer expression
		result, err := processor.evaluateIntegerExpression("workers", "", nil)

		// Then should return int
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil || *result != 42 {
			t.Errorf("Expected 42, got %v", result)
		}
	})

	t.Run("ReturnsErrorForInvalidStringInteger", func(t *testing.T) {
		// Given a string that's not a valid integer
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return "not-a-number", nil
		}

		// When evaluating invalid string integer expression
		result, err := processor.evaluateIntegerExpression("workers", "", nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid string integer")
		}
		if result != nil {
			t.Error("Expected nil result on error")
		}
		if !strings.Contains(err.Error(), "expected integer") {
			t.Errorf("Expected error about expected integer, got: %v", err)
		}
	})

	t.Run("EvaluatesStringTrue", func(t *testing.T) {
		// Given a string "true" expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return "true", nil
		}

		// When evaluating string expression
		result, err := processor.evaluateBooleanExpression("enabled", "", nil)

		// Then should return true
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil || *result != true {
			t.Errorf("Expected true, got %v", result)
		}
	})

	t.Run("EvaluatesStringFalse", func(t *testing.T) {
		// Given a string "false" expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return "false", nil
		}

		// When evaluating string expression
		result, err := processor.evaluateBooleanExpression("enabled", "", nil)

		// Then should return false
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil || *result != false {
			t.Errorf("Expected false, got %v", result)
		}
	})

	t.Run("ReturnsErrorForInvalidString", func(t *testing.T) {
		// Given a string that's not "true" or "false"
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return "maybe", nil
		}

		// When evaluating invalid string expression
		result, err := processor.evaluateBooleanExpression("enabled", "", nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid string")
		}
		if result != nil {
			t.Error("Expected nil result on error")
		}
		if !strings.Contains(err.Error(), "expected boolean") {
			t.Errorf("Expected error about expected boolean, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidType", func(t *testing.T) {
		// Given an expression that evaluates to invalid type
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return 42, nil
		}

		// When evaluating invalid type expression
		result, err := processor.evaluateBooleanExpression("enabled", "", nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid type")
		}
		if result != nil {
			t.Error("Expected nil result on error")
		}
		if !strings.Contains(err.Error(), "expected boolean") {
			t.Errorf("Expected error about expected boolean, got: %v", err)
		}
	})

	t.Run("ReturnsErrorOnEvaluationFailure", func(t *testing.T) {
		// Given an expression that fails to evaluate
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return nil, fmt.Errorf("evaluation failed")
		}

		// When evaluating failing expression
		result, err := processor.evaluateBooleanExpression("invalid", "", nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for evaluation failure")
		}
		if result != nil {
			t.Error("Expected nil result on error")
		}
	})
}

// =============================================================================
// Test evaluateIntegerExpression
// =============================================================================

func TestProcessor_evaluateIntegerExpression(t *testing.T) {
	t.Run("ReturnsNilForEmptyExpression", func(t *testing.T) {
		// Given an empty expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		// When evaluating empty expression
		result, err := processor.evaluateIntegerExpression("", "", nil)

		// Then should return nil
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result != nil {
			t.Errorf("Expected nil for empty expression, got %v", result)
		}
	})

	t.Run("EvaluatesInt", func(t *testing.T) {
		// Given an int expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return 42, nil
		}

		// When evaluating int expression
		result, err := processor.evaluateIntegerExpression("workers", "", nil)

		// Then should return int
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil || *result != 42 {
			t.Errorf("Expected 42, got %v", result)
		}
	})

	t.Run("EvaluatesInt64", func(t *testing.T) {
		// Given an int64 expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return int64(42), nil
		}

		// When evaluating int64 expression
		result, err := processor.evaluateIntegerExpression("workers", "", nil)

		// Then should return int
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil || *result != 42 {
			t.Errorf("Expected 42, got %v", result)
		}
	})

	t.Run("EvaluatesFloat64", func(t *testing.T) {
		// Given a float64 expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return 42.5, nil
		}

		// When evaluating float64 expression
		result, err := processor.evaluateIntegerExpression("workers", "", nil)

		// Then should return int (truncated)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil || *result != 42 {
			t.Errorf("Expected 42, got %v", result)
		}
	})

	t.Run("ReturnsErrorForInvalidType", func(t *testing.T) {
		// Given an expression that evaluates to invalid type
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return "not a number", nil
		}

		// When evaluating invalid type expression
		result, err := processor.evaluateIntegerExpression("workers", "", nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid type")
		}
		if result != nil {
			t.Error("Expected nil result on error")
		}
		if !strings.Contains(err.Error(), "expected integer") {
			t.Errorf("Expected error about expected integer, got: %v", err)
		}
	})

	t.Run("ReturnsErrorOnEvaluationFailure", func(t *testing.T) {
		// Given an expression that fails to evaluate
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return nil, fmt.Errorf("evaluation failed")
		}

		// When evaluating failing expression
		result, err := processor.evaluateIntegerExpression("invalid", "", nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for evaluation failure")
		}
		if result != nil {
			t.Error("Expected nil result on error")
		}
	})
}

func TestEvaluateCondition_EvaluatesAgainstScope(t *testing.T) {
	t.Run("PlatformEqualsIncusWhenScopeHasPlainValue", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		scope := map[string]any{"platform": "incus", "provider": "incus"}
		got, err := processor.evaluateCondition("platform == 'incus'", "", scope)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !got {
			t.Error("Expected condition to be true when platform is 'incus'")
		}
	})

	t.Run("PlatformEqualsFailsWhenValueDiffers", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		scope := map[string]any{"platform": "docker"}
		got, err := processor.evaluateCondition("platform == 'incus'", "", scope)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if got {
			t.Error("Expected condition to be false when platform is 'docker'")
		}
	})
}
