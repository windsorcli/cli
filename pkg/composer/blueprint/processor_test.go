package blueprint

import (
	"os"
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
	mockEvaluator.EvaluateDefaultsFunc = realEvaluator.EvaluateDefaults
	mockEvaluator.EvaluateValueFunc = realEvaluator.EvaluateValue
	mockEvaluator.InterpolateStringFunc = realEvaluator.InterpolateString
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

	t.Run("AcceptsEvaluatorOverride", func(t *testing.T) {
		// Given a custom evaluator
		mocks := setupProcessorMocks(t)
		customEval := evaluator.NewExpressionEvaluator(mocks.ConfigHandler, mocks.Runtime.ProjectRoot, mocks.Runtime.ConfigRoot)

		// When creating a processor with override
		processor := NewBlueprintProcessor(mocks.Runtime, &BaseBlueprintProcessor{evaluator: customEval})

		// Then processor should use custom evaluator
		if processor.evaluator != customEval {
			t.Error("Expected custom evaluator to be used")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProcessor_ProcessFeatures(t *testing.T) {
	t.Run("ReturnsEmptyBlueprintForNoFeatures", func(t *testing.T) {
		// Given a processor and empty features
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		// When processing empty features
		result, err := processor.ProcessFeatures(nil, nil)

		// Then should return empty blueprint
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil {
			t.Fatal("Expected non-nil blueprint")
		}
		if len(result.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components, got %d", len(result.TerraformComponents))
		}
		if len(result.Kustomizations) != 0 {
			t.Errorf("Expected 0 kustomizations, got %d", len(result.Kustomizations))
		}
	})

	t.Run("IncludesFeatureWithoutWhenCondition", func(t *testing.T) {
		// Given a feature without when condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		features := []blueprintv1alpha1.Feature{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "always-include"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"}},
				},
			},
		}

		// When processing features
		result, err := processor.ProcessFeatures(features, nil)

		// Then feature components should be included
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(result.TerraformComponents))
		}
		if result.TerraformComponents[0].Path != "vpc" {
			t.Errorf("Expected path='vpc', got '%s'", result.TerraformComponents[0].Path)
		}
	})

	t.Run("IncludesFeatureWhenConditionTrue", func(t *testing.T) {
		// Given a feature with true condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		features := []blueprintv1alpha1.Feature{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "conditional"},
				When:     "enabled == true",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "eks"}},
				},
			},
		}
		configData := map[string]any{"enabled": true}

		// When processing features
		result, err := processor.ProcessFeatures(features, configData)

		// Then feature should be included
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(result.TerraformComponents))
		}
	})

	t.Run("ExcludesFeatureWhenConditionFalse", func(t *testing.T) {
		// Given a feature with false condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		features := []blueprintv1alpha1.Feature{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "conditional"},
				When:     "enabled == true",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "eks"}},
				},
			},
		}
		configData := map[string]any{"enabled": false}

		// When processing features
		result, err := processor.ProcessFeatures(features, configData)

		// Then feature should be excluded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components, got %d", len(result.TerraformComponents))
		}
	})

	t.Run("ProcessesFeaturesInSortedOrder", func(t *testing.T) {
		// Given features in unsorted order
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		features := []blueprintv1alpha1.Feature{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "z-feature"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "z-path"}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "a-feature"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "a-path"}},
				},
			},
		}

		// When processing features
		result, err := processor.ProcessFeatures(features, nil)

		// Then components should be in sorted feature order
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 2 {
			t.Fatalf("Expected 2 terraform components, got %d", len(result.TerraformComponents))
		}
		if result.TerraformComponents[0].Path != "a-path" {
			t.Errorf("Expected first path='a-path', got '%s'", result.TerraformComponents[0].Path)
		}
		if result.TerraformComponents[1].Path != "z-path" {
			t.Errorf("Expected second path='z-path', got '%s'", result.TerraformComponents[1].Path)
		}
	})

	t.Run("EvaluatesInputExpressions", func(t *testing.T) {
		// Given a feature with input expressions and config data
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		features := []blueprintv1alpha1.Feature{
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

		configData := map[string]any{
			"aws": map[string]any{"region": "us-east-1"},
		}

		// When processing features
		result, err := processor.ProcessFeatures(features, configData)

		// Then inputs should be evaluated
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(result.TerraformComponents))
		}
		inputs := result.TerraformComponents[0].Inputs
		if inputs["region"] != "us-east-1" {
			t.Errorf("Expected evaluated value 'us-east-1', got '%v'", inputs["region"])
		}
	})

	t.Run("IncludesKustomizationsWithoutCondition", func(t *testing.T) {
		// Given a feature with kustomizations
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		features := []blueprintv1alpha1.Feature{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "with-kustomization"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{Kustomization: blueprintv1alpha1.Kustomization{Name: "app"}},
				},
			},
		}

		// When processing features
		result, err := processor.ProcessFeatures(features, nil)

		// Then kustomization should be included
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(result.Kustomizations))
		}
		if result.Kustomizations[0].Name != "app" {
			t.Errorf("Expected name='app', got '%s'", result.Kustomizations[0].Name)
		}
	})

	t.Run("ExcludesComponentWhenComponentConditionFalse", func(t *testing.T) {
		// Given a feature with component-level condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		features := []blueprintv1alpha1.Feature{
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
		configData := map[string]any{"include_vpc": false}

		// When processing features
		result, err := processor.ProcessFeatures(features, configData)

		// Then only unconditional component should be included
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(result.TerraformComponents))
		}
		if result.TerraformComponents[0].Path != "rds" {
			t.Errorf("Expected path='rds', got '%s'", result.TerraformComponents[0].Path)
		}
	})

	t.Run("ExcludesKustomizationWhenConditionFalse", func(t *testing.T) {
		// Given a feature with conditional kustomization
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		features := []blueprintv1alpha1.Feature{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "kust-feature"},
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
		configData := map[string]any{"include_app": false}

		// When processing features
		result, err := processor.ProcessFeatures(features, configData)

		// Then only unconditional kustomization should be included
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(result.Kustomizations))
		}
		if result.Kustomizations[0].Name != "base" {
			t.Errorf("Expected name='base', got '%s'", result.Kustomizations[0].Name)
		}
	})

	t.Run("ReturnsErrorForInvalidFeatureCondition", func(t *testing.T) {
		// Given a feature with invalid condition syntax
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		features := []blueprintv1alpha1.Feature{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "bad-condition"},
				When:     "invalid syntax {{{}",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"}},
				},
			},
		}

		// When processing features
		_, err := processor.ProcessFeatures(features, nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid condition")
		}
	})

	t.Run("ReturnsErrorForInvalidComponentCondition", func(t *testing.T) {
		// Given a component with invalid condition syntax
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		features := []blueprintv1alpha1.Feature{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "feature"},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						When:               "invalid syntax {{{}",
						TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
					},
				},
			},
		}

		// When processing features
		_, err := processor.ProcessFeatures(features, nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid component condition")
		}
	})

	t.Run("ReturnsErrorForInvalidKustomizationCondition", func(t *testing.T) {
		// Given a kustomization with invalid condition syntax
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		features := []blueprintv1alpha1.Feature{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "feature"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						When:          "invalid syntax {{{}",
						Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
					},
				},
			},
		}

		// When processing features
		_, err := processor.ProcessFeatures(features, nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid kustomization condition")
		}
	})

	t.Run("HandlesStringTrueConditionResult", func(t *testing.T) {
		// Given a condition that evaluates to string "true"
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		features := []blueprintv1alpha1.Feature{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "string-condition"},
				When:     `"true"`,
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"}},
				},
			},
		}

		// When processing features
		result, err := processor.ProcessFeatures(features, nil)

		// Then feature should be included (string "true" is truthy)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component for string 'true', got %d", len(result.TerraformComponents))
		}
	})

	t.Run("EvaluatesSubstitutionExpressions", func(t *testing.T) {
		// Given a feature with substitution expressions
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		features := []blueprintv1alpha1.Feature{
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

		configData := map[string]any{
			"dns": map[string]any{"domain": "example.com"},
		}

		// When processing features
		result, err := processor.ProcessFeatures(features, configData)

		// Then substitutions should be evaluated
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(result.Kustomizations))
		}
		subs := result.Kustomizations[0].Substitutions
		if subs["domain"] != "example.com" {
			t.Errorf("Expected 'example.com', got '%v'", subs["domain"])
		}
		if subs["static"] != "unchanged" {
			t.Errorf("Expected 'unchanged', got '%v'", subs["static"])
		}
	})

	t.Run("PreservesNonStringInputs", func(t *testing.T) {
		// Given a feature with non-string inputs
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		features := []blueprintv1alpha1.Feature{
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

		// When processing features
		result, err := processor.ProcessFeatures(features, nil)

		// Then non-string inputs should be preserved
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		inputs := result.TerraformComponents[0].Inputs
		if inputs["count"] != 42 {
			t.Errorf("Expected 42, got '%v'", inputs["count"])
		}
		if inputs["enabled"] != true {
			t.Errorf("Expected true, got '%v'", inputs["enabled"])
		}
	})

	t.Run("HandlesInputEvaluationError", func(t *testing.T) {
		// Given a feature with invalid input expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		features := []blueprintv1alpha1.Feature{
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

		// When processing features
		_, err := processor.ProcessFeatures(features, map[string]any{})

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid expression")
		}
	})

	t.Run("HandlesSubstitutionEvaluationError", func(t *testing.T) {
		// Given a feature with invalid substitution expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		features := []blueprintv1alpha1.Feature{
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

		// When processing features
		_, err := processor.ProcessFeatures(features, map[string]any{})

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid substitution expression")
		}
	})
}
