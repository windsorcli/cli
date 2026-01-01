package blueprint

import (
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupFeatureEvaluator(t *testing.T) *FeatureEvaluator {
	t.Helper()
	tmpDir := t.TempDir()
	configHandler := config.NewMockConfigHandler()
	configHandler.GetConfigRootFunc = func() (string, error) {
		return tmpDir, nil
	}
	mockShell := shell.NewMockShell()
	rt := &runtime.Runtime{
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir,
		TemplateRoot:  filepath.Join(tmpDir, "contexts", "_template"),
		ConfigHandler: configHandler,
		Shell:         mockShell,
	}
	rt, err := runtime.NewRuntime(rt)
	if err != nil {
		t.Fatalf("Failed to initialize runtime: %v", err)
	}
	return NewFeatureEvaluator(rt)
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewFeatureEvaluator(t *testing.T) {
	t.Run("CreatesNewFeatureEvaluatorSuccessfully", func(t *testing.T) {
		evaluator := setupFeatureEvaluator(t)
		if evaluator == nil {
			t.Fatal("Expected evaluator, got nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestFeatureEvaluator_Evaluate(t *testing.T) {
	t.Run("DelegatesToRuntimeEvaluator", func(t *testing.T) {
		evaluator := setupFeatureEvaluator(t)
		result, err := evaluator.Evaluate("provider == 'aws'", map[string]any{"provider": "aws"}, "")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result != true {
			t.Errorf("Expected true, got %v", result)
		}
	})

	t.Run("ReturnsErrorWhenRuntimeEvaluatorNotAvailable", func(t *testing.T) {
		evaluator := &FeatureEvaluator{runtime: nil}
		_, err := evaluator.Evaluate("true", map[string]any{}, "")
		if err == nil {
			t.Fatal("Expected error when runtime evaluator not available")
		}
		if !strings.Contains(err.Error(), "runtime evaluator not available") {
			t.Errorf("Expected error about runtime evaluator, got: %v", err)
		}
	})
}

func TestFeatureEvaluator_EvaluateDefaults(t *testing.T) {
	t.Run("DelegatesToRuntimeEvaluator", func(t *testing.T) {
		evaluator := setupFeatureEvaluator(t)
		defaults := map[string]any{
			"key": "${provider}",
		}
		config := map[string]any{"provider": "aws"}
		result, err := evaluator.EvaluateDefaults(defaults, config, "")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result["key"] != "aws" {
			t.Errorf("Expected 'aws', got %v", result["key"])
		}
	})

	t.Run("ReturnsErrorWhenRuntimeEvaluatorNotAvailable", func(t *testing.T) {
		evaluator := &FeatureEvaluator{runtime: nil}
		_, err := evaluator.EvaluateDefaults(map[string]any{"key": "value"}, map[string]any{}, "")
		if err == nil {
			t.Fatal("Expected error when runtime evaluator not available")
		}
		if !strings.Contains(err.Error(), "runtime evaluator not available") {
			t.Errorf("Expected error about runtime evaluator, got: %v", err)
		}
	})
}

func TestFeatureEvaluator_InterpolateString(t *testing.T) {
	t.Run("DelegatesToRuntimeEvaluator", func(t *testing.T) {
		evaluator := setupFeatureEvaluator(t)
		result, err := evaluator.InterpolateString("provider is ${provider}", map[string]any{"provider": "aws"}, "")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result != "provider is aws" {
			t.Errorf("Expected 'provider is aws', got '%s'", result)
		}
	})

	t.Run("ReturnsErrorWhenRuntimeEvaluatorNotAvailable", func(t *testing.T) {
		evaluator := &FeatureEvaluator{runtime: nil}
		_, err := evaluator.InterpolateString("test", map[string]any{}, "")
		if err == nil {
			t.Fatal("Expected error when runtime evaluator not available")
		}
		if !strings.Contains(err.Error(), "runtime evaluator not available") {
			t.Errorf("Expected error about runtime evaluator, got: %v", err)
		}
	})
}

// =============================================================================
// Test FeatureEvaluator-Specific Methods
// =============================================================================

func TestFeatureEvaluator_ProcessFeature(t *testing.T) {
	evaluator := setupFeatureEvaluator(t)

	t.Run("ReturnsNilWhenWhenConditionIsFalse", func(t *testing.T) {
		feature := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-feature"},
			When:     "provider == 'aws'",
		}
		config := map[string]any{"provider": "gcp"}

		result, err := evaluator.ProcessFeature(feature, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result != nil {
			t.Error("Expected nil when condition is false")
		}
	})

	t.Run("ReturnsErrorWhenWhenConditionEvaluationFails", func(t *testing.T) {
		feature := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-feature"},
			When:     "invalid expression",
		}
		config := map[string]any{}

		_, err := evaluator.ProcessFeature(feature, config)

		if err == nil {
			t.Fatal("Expected error when condition evaluation fails")
		}
	})

	t.Run("ProcessesFeatureWithoutWhenCondition", func(t *testing.T) {
		feature := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-feature"},
		}
		config := map[string]any{}

		result, err := evaluator.ProcessFeature(feature, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil {
			t.Fatal("Expected processed feature, got nil")
		}
		if result.Metadata.Name != "test-feature" {
			t.Errorf("Expected feature name 'test-feature', got '%s'", result.Metadata.Name)
		}
	})

	t.Run("FiltersTerraformComponentsByWhenCondition", func(t *testing.T) {
		feature := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-feature"},
			TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
				{
					TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "component1"},
					When:               "provider == 'aws'",
				},
				{
					TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "component2"},
					When:               "provider == 'gcp'",
				},
			},
		}
		config := map[string]any{"provider": "aws"}

		result, err := evaluator.ProcessFeature(feature, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 1 {
			t.Errorf("Expected 1 component, got %d", len(result.TerraformComponents))
		}
		if result.TerraformComponents[0].TerraformComponent.Path != "component1" {
			t.Errorf("Expected component1, got %s", result.TerraformComponents[0].TerraformComponent.Path)
		}
	})

	t.Run("FiltersKustomizationsByWhenCondition", func(t *testing.T) {
		feature := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-feature"},
			Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
				{
					Kustomization: blueprintv1alpha1.Kustomization{Name: "kustomization1"},
					When:          "provider == 'aws'",
				},
				{
					Kustomization: blueprintv1alpha1.Kustomization{Name: "kustomization2"},
					When:          "provider == 'gcp'",
				},
			},
		}
		config := map[string]any{"provider": "aws"}

		result, err := evaluator.ProcessFeature(feature, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization, got %d", len(result.Kustomizations))
		}
		if result.Kustomizations[0].Kustomization.Name != "kustomization1" {
			t.Errorf("Expected kustomization1, got %s", result.Kustomizations[0].Kustomization.Name)
		}
	})

	t.Run("HandlesEvaluateDefaultsErrorForTerraformComponent", func(t *testing.T) {
		feature := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-feature"},
			TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
				{
					TerraformComponent: blueprintv1alpha1.TerraformComponent{
						Path: "component1",
						Inputs: map[string]any{
							"key": "${invalid expression [[[",
						},
					},
				},
			},
		}
		config := map[string]any{}

		_, err := evaluator.ProcessFeature(feature, config)

		if err == nil {
			t.Fatal("Expected error when EvaluateDefaults fails")
		}
		if !strings.Contains(err.Error(), "failed to evaluate inputs") {
			t.Errorf("Expected error about evaluating inputs, got: %v", err)
		}
	})

	t.Run("HandlesEvaluateSubstitutionsErrorForKustomization", func(t *testing.T) {
		feature := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-feature"},
			Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
				{
					Kustomization: blueprintv1alpha1.Kustomization{
						Name: "kustomization1",
						Substitutions: map[string]string{
							"key": "${invalid expression [[[",
						},
					},
				},
			},
		}
		config := map[string]any{}

		_, err := evaluator.ProcessFeature(feature, config)

		if err == nil {
			t.Fatal("Expected error when evaluateSubstitutions fails")
		}
		if !strings.Contains(err.Error(), "failed to evaluate substitutions") {
			t.Errorf("Expected error about evaluating substitutions, got: %v", err)
		}
	})
}

func TestFeatureEvaluator_MergeFeatures(t *testing.T) {
	evaluator := setupFeatureEvaluator(t)

	t.Run("ReturnsNilWhenFeaturesIsEmpty", func(t *testing.T) {
		result := evaluator.MergeFeatures([]*blueprintv1alpha1.Feature{})

		if result != nil {
			t.Error("Expected nil when features is empty")
		}
	})

	t.Run("MergesMultipleFeatures", func(t *testing.T) {
		feature1 := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "feature1"},
			TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
				{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "component1"}},
			},
		}
		feature2 := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "feature2"},
			Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
				{Kustomization: blueprintv1alpha1.Kustomization{Name: "kustomization1"}},
			},
		}

		result := evaluator.MergeFeatures([]*blueprintv1alpha1.Feature{feature1, feature2})

		if result == nil {
			t.Fatal("Expected merged feature, got nil")
		}
		if result.Metadata.Name != "merged-features" {
			t.Errorf("Expected name 'merged-features', got '%s'", result.Metadata.Name)
		}
		if len(result.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(result.TerraformComponents))
		}
		if len(result.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization, got %d", len(result.Kustomizations))
		}
	})
}

func TestFeatureEvaluator_FeatureToBlueprint(t *testing.T) {
	evaluator := setupFeatureEvaluator(t)

	t.Run("ReturnsNilWhenFeatureIsNil", func(t *testing.T) {
		result := evaluator.FeatureToBlueprint(nil)

		if result != nil {
			t.Error("Expected nil when feature is nil")
		}
	})

	t.Run("ConvertsFeatureToBlueprint", func(t *testing.T) {
		feature := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-feature"},
			TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
				{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "component1"}},
			},
			Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
				{Kustomization: blueprintv1alpha1.Kustomization{Name: "kustomization1"}},
			},
		}

		result := evaluator.FeatureToBlueprint(feature)

		if result == nil {
			t.Fatal("Expected blueprint, got nil")
		}
		if result.Kind != "Blueprint" {
			t.Errorf("Expected kind 'Blueprint', got '%s'", result.Kind)
		}
		if result.ApiVersion != "v1alpha1" {
			t.Errorf("Expected apiVersion 'v1alpha1', got '%s'", result.ApiVersion)
		}
		if result.Metadata.Name != "test-feature" {
			t.Errorf("Expected name 'test-feature', got '%s'", result.Metadata.Name)
		}
		if len(result.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(result.TerraformComponents))
		}
		if len(result.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization, got %d", len(result.Kustomizations))
		}
	})
}

func TestFeatureEvaluator_evaluateSubstitutions(t *testing.T) {
	evaluator := setupFeatureEvaluator(t)

	t.Run("HandlesSubstitutionsWithoutExpressions", func(t *testing.T) {
		substitutions := map[string]string{
			"key1": "value1",
			"key2": "value2",
		}
		config := map[string]any{}

		result, err := evaluator.evaluateSubstitutions(substitutions, config, "")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result["key1"] != "value1" {
			t.Errorf("Expected 'value1', got '%s'", result["key1"])
		}
		if result["key2"] != "value2" {
			t.Errorf("Expected 'value2', got '%s'", result["key2"])
		}
	})

	t.Run("HandlesSubstitutionsWithExpressions", func(t *testing.T) {
		substitutions := map[string]string{
			"key1": "${provider}",
		}
		config := map[string]any{"provider": "aws"}

		result, err := evaluator.evaluateSubstitutions(substitutions, config, "")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result["key1"] != "aws" {
			t.Errorf("Expected 'aws', got '%s'", result["key1"])
		}
	})

	t.Run("HandlesNilEvaluatedValue", func(t *testing.T) {
		substitutions := map[string]string{
			"key1": "${nonexistent}",
		}
		config := map[string]any{}

		result, err := evaluator.evaluateSubstitutions(substitutions, config, "")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result["key1"] != "" {
			t.Errorf("Expected empty string for nil value, got '%s'", result["key1"])
		}
	})

	t.Run("ReturnsErrorWhenEvaluationFails", func(t *testing.T) {
		substitutions := map[string]string{
			"key1": "${invalid expression",
		}
		config := map[string]any{}

		_, err := evaluator.evaluateSubstitutions(substitutions, config, "")

		if err == nil {
			t.Fatal("Expected error when evaluation fails")
		}
	})
}
