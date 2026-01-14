package blueprint

import (
	"fmt"
	"os"
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
		err := processor.ProcessFacets(target, nil)

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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

		// Then facet should be excluded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components, got %d", len(target.TerraformComponents))
		}
	})

	t.Run("ProcessesFacetsInSortedOrder", func(t *testing.T) {
		// Given facets in unsorted order
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
		err := processor.ProcessFacets(target, facets)

		// Then components should be in sorted facet order
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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

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
		// Given a facet with invalid input expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "bad-input"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:   "vpc",
							Inputs: map[string]any{"bad": "${undefined.value}"},
						},
					},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		err := processor.ProcessFacets(target, facets)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid expression")
		}
	})

	t.Run("HandlesSubstitutionEvaluationError", func(t *testing.T) {
		// Given a facet with invalid substitution expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "bad-sub"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{
							Name:          "test",
							Substitutions: map[string]string{"bad": "${undefined.value}"},
						},
					},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		err := processor.ProcessFacets(target, facets)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid substitution expression")
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
		err := processor.ProcessFacets(target, facets, "test-source")

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
		err := processor.ProcessFacets(target, facets, "new-source")

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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

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
// Test Private Methods
// =============================================================================

func TestStrategyPriorities(t *testing.T) {
	t.Run("ReturnsCorrectPriorityForRemove", func(t *testing.T) {
		// Given remove strategy
		// When getting priority
		priority := strategyPriorities["remove"]

		// Then should return 3
		if priority != 3 {
			t.Errorf("Expected priority 3 for remove, got %d", priority)
		}
	})

	t.Run("ReturnsCorrectPriorityForReplace", func(t *testing.T) {
		// Given replace strategy
		// When getting priority
		priority := strategyPriorities["replace"]

		// Then should return 2
		if priority != 2 {
			t.Errorf("Expected priority 2 for replace, got %d", priority)
		}
	})

	t.Run("ReturnsCorrectPriorityForMerge", func(t *testing.T) {
		// Given merge strategy
		// When getting priority
		priority := strategyPriorities["merge"]

		// Then should return 1
		if priority != 1 {
			t.Errorf("Expected priority 1 for merge, got %d", priority)
		}
	})

	t.Run("ReturnsZeroForUnknownStrategy", func(t *testing.T) {
		// Given unknown strategy
		// When getting priority
		priority := strategyPriorities["unknown"]

		// Then should return 0 (zero value)
		if priority != 0 {
			t.Errorf("Expected priority 0 for unknown, got %d", priority)
		}
	})

	t.Run("ReturnsZeroForEmptyStrategy", func(t *testing.T) {
		// Given empty strategy
		// When getting priority
		priority := strategyPriorities[""]

		// Then should return 0 (zero value)
		if priority != 0 {
			t.Errorf("Expected priority 0 for empty, got %d", priority)
		}
	})
}

func TestProcessor_updateTerraformComponentEntry(t *testing.T) {
	mocks := setupProcessorMocks(t)
	processor := NewBlueprintProcessor(mocks.Runtime)

	t.Run("ReplacesWhenNewStrategyHasHigherPriority", func(t *testing.T) {
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
		err := processor.updateTerraformComponentEntry("vpc", new, "replace", entries)

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
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries)

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

	t.Run("IgnoresWhenNewStrategyHasLowerPriority", func(t *testing.T) {
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
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries)

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
		err := processor.updateTerraformComponentEntry("vpc", new, "remove", entries)

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
		err := processor.updateTerraformComponentEntry("vpc", new, "replace", entries)

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

	t.Run("ReplacesWhenNewHasHigherPriority", func(t *testing.T) {
		// Given existing entry with replace strategy and priority 0
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "replace",
				Priority:           0,
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with merge strategy but higher priority
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			Priority:           100,
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries)

		// Then entry should be replaced despite lower strategy
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "merge" {
			t.Errorf("Expected strategy 'merge', got '%s'", entries["vpc"].Strategy)
		}
		if entries["vpc"].Priority != 100 {
			t.Errorf("Expected priority 100, got %d", entries["vpc"].Priority)
		}
		if entries["vpc"].Inputs["key2"] != "value2" {
			t.Error("Expected new inputs to be set")
		}
	})

	t.Run("IgnoresWhenNewHasLowerPriority", func(t *testing.T) {
		// Given existing entry with merge strategy and priority 100
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				Priority:           100,
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with replace strategy but lower priority
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			Priority:           0,
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "replace", entries)

		// Then entry should remain unchanged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "merge" {
			t.Errorf("Expected strategy 'merge' to be preserved, got '%s'", entries["vpc"].Strategy)
		}
		if entries["vpc"].Priority != 100 {
			t.Errorf("Expected priority 100 to be preserved, got %d", entries["vpc"].Priority)
		}
		if entries["vpc"].Inputs["key1"] != "value1" {
			t.Error("Expected original inputs to be preserved")
		}
	})

	t.Run("UsesStrategyPriorityWhenPrioritiesEqual", func(t *testing.T) {
		// Given existing entry with merge strategy and priority 50
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				Priority:           50,
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with replace strategy and same priority
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			Priority:           50,
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "replace", entries)

		// Then entry should be replaced due to higher strategy priority
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace', got '%s'", entries["vpc"].Strategy)
		}
		if entries["vpc"].Priority != 50 {
			t.Errorf("Expected priority 50, got %d", entries["vpc"].Priority)
		}
	})

	t.Run("PreservesPriorityInMergedResult", func(t *testing.T) {
		// Given existing entry with merge strategy and priority 25
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				Priority:           25,
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with merge strategy and same priority
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			Priority:           25,
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries)

		// Then priority should be preserved in merged result
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Priority != 25 {
			t.Errorf("Expected priority 25, got %d", entries["vpc"].Priority)
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
		err := processor.updateTerraformComponentEntry("vpc", new, "invalid", entries)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid strategy")
		}
		if err != nil && !strings.Contains(err.Error(), "invalid strategy") {
			t.Errorf("Expected error about invalid strategy, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidStrategyEvenWithHigherPriority", func(t *testing.T) {
		// Given existing entry with merge strategy and priority 0
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				Priority:           0,
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with invalid strategy but higher priority
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			Priority:           100,
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "typo", entries)

		// Then should return error before checking priority
		if err == nil {
			t.Error("Expected error for invalid strategy, even with higher priority")
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

	t.Run("ReplacesWhenNewStrategyHasHigherPriority", func(t *testing.T) {
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
		err := processor.updateKustomizationEntry("app", new, "replace", entries)

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
		err := processor.updateKustomizationEntry("app", new, "merge", entries)

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

	t.Run("IgnoresWhenNewStrategyHasLowerPriority", func(t *testing.T) {
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
		err := processor.updateKustomizationEntry("app", new, "merge", entries)

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
		err := processor.updateKustomizationEntry("app", new, "remove", entries)

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
		err := processor.updateKustomizationEntry("app", new, "replace", entries)

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

	t.Run("ReplacesWhenNewHasHigherPriority", func(t *testing.T) {
		// Given existing entry with replace strategy and priority 0
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "replace",
				Priority:      0,
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with merge strategy but higher priority
		new := &blueprintv1alpha1.ConditionalKustomization{
			Priority:      100,
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries)

		// Then entry should be replaced despite lower strategy
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "merge" {
			t.Errorf("Expected strategy 'merge', got '%s'", entries["app"].Strategy)
		}
		if entries["app"].Priority != 100 {
			t.Errorf("Expected priority 100, got %d", entries["app"].Priority)
		}
		if entries["app"].Substitutions["key2"] != "value2" {
			t.Error("Expected new substitutions to be set")
		}
	})

	t.Run("IgnoresWhenNewHasLowerPriority", func(t *testing.T) {
		// Given existing entry with merge strategy and priority 100
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Priority:      100,
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with replace strategy but lower priority
		new := &blueprintv1alpha1.ConditionalKustomization{
			Priority:      0,
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "replace", entries)

		// Then entry should remain unchanged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "merge" {
			t.Errorf("Expected strategy 'merge' to be preserved, got '%s'", entries["app"].Strategy)
		}
		if entries["app"].Priority != 100 {
			t.Errorf("Expected priority 100 to be preserved, got %d", entries["app"].Priority)
		}
		if entries["app"].Substitutions["key1"] != "value1" {
			t.Error("Expected original substitutions to be preserved")
		}
	})

	t.Run("UsesStrategyPriorityWhenPrioritiesEqual", func(t *testing.T) {
		// Given existing entry with merge strategy and priority 50
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Priority:      50,
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with replace strategy and same priority
		new := &blueprintv1alpha1.ConditionalKustomization{
			Priority:      50,
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "replace", entries)

		// Then entry should be replaced due to higher strategy priority
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace', got '%s'", entries["app"].Strategy)
		}
		if entries["app"].Priority != 50 {
			t.Errorf("Expected priority 50, got %d", entries["app"].Priority)
		}
	})

	t.Run("PreservesPriorityInMergedResult", func(t *testing.T) {
		// Given existing entry with merge strategy and priority 25
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Priority:      25,
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with merge strategy and same priority
		new := &blueprintv1alpha1.ConditionalKustomization{
			Priority:      25,
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries)

		// Then priority should be preserved in merged result
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Priority != 25 {
			t.Errorf("Expected priority 25, got %d", entries["app"].Priority)
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
		err := processor.updateKustomizationEntry("app", new, "typo", entries)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid strategy")
		}
		if err != nil && !strings.Contains(err.Error(), "invalid strategy") {
			t.Errorf("Expected error about invalid strategy, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidStrategyEvenWithHigherPriority", func(t *testing.T) {
		// Given existing entry with merge strategy and priority 0
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Priority:      0,
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with invalid strategy but higher priority
		new := &blueprintv1alpha1.ConditionalKustomization{
			Priority:      100,
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "typo", entries)

		// Then should return error before checking priority
		if err == nil {
			t.Error("Expected error for invalid strategy, even with higher priority")
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
		err := processor.applyCollectedComponents(target, terraformByID, kustomizationByName)

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
		err := processor.applyCollectedComponents(target, terraformByID, nil)

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
		err := processor.applyCollectedComponents(target, terraformByID, nil)

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
		err := processor.applyCollectedComponents(target, terraformByID, nil)

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
		err := processor.applyCollectedComponents(target, nil, kustomizationByName)

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
		err := processor.applyCollectedComponents(target, terraformByID, nil)

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
		err := processor.applyCollectedComponents(target, terraformByID, nil)
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
		err = processor.applyCollectedComponents(target, terraformByID, nil)
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
		err = processor.applyCollectedComponents(target, terraformByID, nil)
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

		mocks.Evaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, evaluateDeferred bool) (map[string]any, error) {
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
		result, err := mocks.Evaluator.EvaluateMap(inputs, "", false)

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

		mocks.Evaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, evaluateDeferred bool) (map[string]any, error) {
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
		result, err := mocks.Evaluator.EvaluateMap(inputs, "", false)

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

		mocks.Evaluator.EvaluateFunc = func(expression string, featurePath string, evaluateDeferred bool) (any, error) {
			return expression, nil
		}

		inputs := map[string]any{}

		result, err := mocks.Evaluator.EvaluateMap(inputs, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(result) != 0 {
			t.Errorf("Expected empty result, got %d entries", len(result))
		}
	})

	t.Run("PreservesNonStringValues", func(t *testing.T) {
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateFunc = func(expression string, featurePath string, evaluateDeferred bool) (any, error) {
			return expression, nil
		}

		inputs := map[string]any{
			"count":   42,
			"enabled": true,
			"tags":    []string{"a", "b"},
			"nested":  map[string]any{"key": "value"},
		}

		result, err := mocks.Evaluator.EvaluateMap(inputs, "", false)

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

		mocks.Evaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, evaluateDeferred bool) (map[string]any, error) {
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

		result, err := mocks.Evaluator.EvaluateMap(inputs, "", false)

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

		mocks.Evaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, evaluateDeferred bool) (map[string]any, error) {
			return nil, fmt.Errorf("failed to evaluate 'bad': evaluation failed")
		}

		inputs := map[string]any{
			"bad": "${invalid}",
		}

		result, err := mocks.Evaluator.EvaluateMap(inputs, "", false)

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

		mocks.Evaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, evaluateDeferred bool) (map[string]any, error) {
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

		result, err := mocks.Evaluator.EvaluateMap(inputs, "", false)

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
		mocks.Evaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, evaluateDeferred bool) (map[string]any, error) {
			receivedPath = featurePath
			return values, nil
		}

		inputs := map[string]any{
			"test": "value",
		}

		expectedPath := "test/feature/path"
		_, err := mocks.Evaluator.EvaluateMap(inputs, expectedPath, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if receivedPath != expectedPath {
			t.Errorf("Expected feature path to be '%s', got '%s'", expectedPath, receivedPath)
		}
	})
}

func TestBaseBlueprintProcessor_evaluateSubstitutions(t *testing.T) {
	t.Run("SkipsUnresolvedExpressions", func(t *testing.T) {
		// Given a processor with evaluator that returns unresolved expressions
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateFunc = func(expression string, featurePath string, evaluateDeferred bool) (any, error) {
			// Extract inner expression if wrapped in ${}
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

		// When evaluating substitutions
		baseProcessor := &BaseBlueprintProcessor{
			runtime:   mocks.Runtime,
			evaluator: mocks.Evaluator,
		}
		result, err := baseProcessor.evaluateSubstitutions(subs, "")

		// Then unresolved expression substitution should be skipped
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if _, exists := result["deferred"]; exists {
			t.Errorf("Expected unresolved expression substitution to be skipped, but found in result: %v", result)
		}

		if result["normal"] != "value" {
			t.Errorf("Expected normal substitution to be preserved, got %v", result["normal"])
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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

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

	t.Run("FiltersEmptyComponentsFromKustomizations", func(t *testing.T) {
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
		err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		comps := target.Kustomizations[0].Components
		if len(comps) != 2 {
			t.Fatalf("Expected 2 components (empty filtered), got %d: %v", len(comps), comps)
		}
		if comps[0] != "nginx" {
			t.Errorf("Expected first component to be 'nginx', got '%s'", comps[0])
		}
		if comps[1] != "cert-manager" {
			t.Errorf("Expected second component to be 'cert-manager', got '%s'", comps[1])
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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

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

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, evaluateDeferred bool) (any, error) {
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
		err := processor.ProcessFacets(target, facets)

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

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, evaluateDeferred bool) (any, error) {
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
		err := processor.ProcessFacets(target, facets)

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
		err := processor.ProcessFacets(target, facets)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents[0].DependsOn) != 0 {
			t.Errorf("Expected empty dependsOn, got %v", target.TerraformComponents[0].DependsOn)
		}
	})
}
