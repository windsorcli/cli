package blueprint

import (
	"fmt"
	"strings"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
)

// FeatureEvaluator provides blueprint-specific feature processing and evaluation.
// It uses the Runtime's unified expression evaluator for all expression evaluation,
// while providing blueprint-specific functionality like feature processing, merging,
// and blueprint transformation. The evaluator handles conditional feature activation
// and component processing based on evaluated conditions.

// =============================================================================
// Types
// =============================================================================

// FeatureEvaluator provides blueprint-specific feature processing capabilities.
type FeatureEvaluator struct {
	runtime *runtime.Runtime
}

// =============================================================================
// Constructor
// =============================================================================

// NewFeatureEvaluator creates a new feature evaluator with the provided runtime.
// The evaluator uses the Runtime's unified expression evaluator for all expression
// evaluation operations. The runtime must have its evaluator initialized before use.
func NewFeatureEvaluator(rt *runtime.Runtime) *FeatureEvaluator {
	return &FeatureEvaluator{
		runtime: rt,
	}
}

// SetTemplateData sets the template data map for file resolution when loading from artifacts.
// This delegates to the Runtime's expression evaluator, allowing jsonnet() and file()
// functions to access files from in-memory template data instead of requiring them
// to exist on the filesystem.
func (e *FeatureEvaluator) SetTemplateData(templateData map[string][]byte) {
	if e.runtime != nil && e.runtime.Evaluator != nil {
		e.runtime.Evaluator.SetTemplateData(templateData)
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Evaluate evaluates an expression and returns the result as any type.
// This method delegates to the Runtime's unified expression evaluator, which
// handles all expression compilation, evaluation, and file loading functions.
// The featurePath is used to resolve relative paths in jsonnet() and file() functions.
// Returns the evaluated value or an error if evaluation fails.
func (e *FeatureEvaluator) Evaluate(expression string, config map[string]any, featurePath string) (any, error) {
	if e.runtime == nil || e.runtime.Evaluator == nil {
		return nil, fmt.Errorf("runtime evaluator not available")
	}
	return e.runtime.Evaluator.Evaluate(expression, config, featurePath)
}

// EvaluateDefaults recursively evaluates default values in a map structure.
// This method delegates to the Runtime's unified expression evaluator, which
// handles recursive evaluation of nested maps and arrays. The featurePath is
// used to resolve relative paths in jsonnet() and file() functions.
func (e *FeatureEvaluator) EvaluateDefaults(defaults map[string]any, config map[string]any, featurePath string) (map[string]any, error) {
	if e.runtime == nil || e.runtime.Evaluator == nil {
		return nil, fmt.Errorf("runtime evaluator not available")
	}
	return e.runtime.Evaluator.EvaluateDefaults(defaults, config, featurePath)
}

// InterpolateString replaces all ${expression} occurrences in a string with their evaluated values.
// This method delegates to the Runtime's unified expression evaluator, which handles
// expression evaluation and YAML marshaling for complex values. This is used to process
// template expressions in patch content and other string values.
func (e *FeatureEvaluator) InterpolateString(s string, config map[string]any, featurePath string) (string, error) {
	if e.runtime == nil || e.runtime.Evaluator == nil {
		return "", fmt.Errorf("runtime evaluator not available")
	}
	return e.runtime.Evaluator.InterpolateString(s, config, featurePath)
}

// ProcessFeature evaluates feature conditions and processes its Terraform components and Kustomizations.
// If the feature has a 'When' condition, it is evaluated against the provided config and feature path.
// Features or components whose conditions do not match are skipped. The returned Feature includes only
// the components and Kustomizations whose conditions have passed. If the root feature's condition is not met,
// ProcessFeature returns nil. Errors encountered in any evaluation are returned. Inputs for Terraform components
// and substitutions for Kustomizations are evaluated and updated; nil values from evaluated inputs are omitted.
func (e *FeatureEvaluator) ProcessFeature(feature *v1alpha1.Feature, config map[string]any) (*v1alpha1.Feature, error) {
	if feature.When != "" {
		result, err := e.Evaluate(feature.When, config, feature.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate feature condition '%s': %w", feature.When, err)
		}
		matches, ok := result.(bool)
		if !ok {
			return nil, fmt.Errorf("feature condition '%s' must evaluate to boolean, got %T", feature.When, result)
		}
		if !matches {
			return nil, nil
		}
	}

	processedFeature := feature.DeepCopy()

	var processedTerraformComponents []v1alpha1.ConditionalTerraformComponent
	for _, terraformComponent := range processedFeature.TerraformComponents {
		if terraformComponent.When != "" {
			result, err := e.Evaluate(terraformComponent.When, config, feature.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate terraform component condition '%s': %w", terraformComponent.When, err)
			}
			matches, ok := result.(bool)
			if !ok {
				return nil, fmt.Errorf("terraform component condition '%s' must evaluate to boolean, got %T", terraformComponent.When, result)
			}
			if !matches {
				continue
			}
		}

		if len(terraformComponent.Inputs) > 0 {
			evaluatedInputs, err := e.EvaluateDefaults(terraformComponent.Inputs, config, feature.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate inputs for component '%s': %w", terraformComponent.TerraformComponent.Path, err)
			}

			filteredInputs := make(map[string]any)
			for k, v := range evaluatedInputs {
				if v != nil {
					filteredInputs[k] = v
				}
			}
			terraformComponent.Inputs = filteredInputs
		}

		processedTerraformComponents = append(processedTerraformComponents, terraformComponent)
	}
	processedFeature.TerraformComponents = processedTerraformComponents

	var processedKustomizations []v1alpha1.ConditionalKustomization
	for _, kustomization := range processedFeature.Kustomizations {
		if kustomization.When != "" {
			result, err := e.Evaluate(kustomization.When, config, feature.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate kustomization condition '%s': %w", kustomization.When, err)
			}
			matches, ok := result.(bool)
			if !ok {
				return nil, fmt.Errorf("kustomization condition '%s' must evaluate to boolean, got %T", kustomization.When, result)
			}
			if !matches {
				continue
			}
		}

		if len(kustomization.Substitutions) > 0 {
			evaluatedSubstitutions, err := e.evaluateSubstitutions(kustomization.Substitutions, config, feature.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate substitutions for kustomization '%s': %w", kustomization.Kustomization.Name, err)
			}
			kustomization.Substitutions = evaluatedSubstitutions
		}

		processedKustomizations = append(processedKustomizations, kustomization)
	}
	processedFeature.Kustomizations = processedKustomizations

	return processedFeature, nil
}

// MergeFeatures creates a single "mega feature" by merging multiple processed features.
// It combines all Terraform components and Kustomizations from the input features into a consolidated feature.
// If the input slice is empty, it returns nil.
// The merged feature's metadata is given a default name of "merged-features".
func (e *FeatureEvaluator) MergeFeatures(features []*v1alpha1.Feature) *v1alpha1.Feature {
	if len(features) == 0 {
		return nil
	}

	megaFeature := &v1alpha1.Feature{
		Metadata: v1alpha1.Metadata{
			Name: "merged-features",
		},
	}

	var allTerraformComponents []v1alpha1.ConditionalTerraformComponent
	for _, feature := range features {
		allTerraformComponents = append(allTerraformComponents, feature.TerraformComponents...)
	}
	megaFeature.TerraformComponents = allTerraformComponents

	var allKustomizations []v1alpha1.ConditionalKustomization
	for _, feature := range features {
		allKustomizations = append(allKustomizations, feature.Kustomizations...)
	}
	megaFeature.Kustomizations = allKustomizations

	return megaFeature
}

// FeatureToBlueprint transforms a processed feature into a blueprint structure.
// It extracts and transfers all terraform components and kustomizations, removing
// any substitutions from the kustomization copies as those are only used for ConfigMap
// generation and are not included in the final blueprint output. Returns nil if the
// input feature is nil.
func (e *FeatureEvaluator) FeatureToBlueprint(feature *v1alpha1.Feature) *v1alpha1.Blueprint {
	if feature == nil {
		return nil
	}

	blueprint := &v1alpha1.Blueprint{
		Kind:       "Blueprint",
		ApiVersion: "v1alpha1",
		Metadata: v1alpha1.Metadata{
			Name: feature.Metadata.Name,
		},
	}

	var terraformComponents []v1alpha1.TerraformComponent
	for _, component := range feature.TerraformComponents {
		terraformComponent := component.TerraformComponent
		terraformComponents = append(terraformComponents, terraformComponent)
	}
	blueprint.TerraformComponents = terraformComponents

	var kustomizations []v1alpha1.Kustomization
	for _, kustomization := range feature.Kustomizations {
		kustomizationCopy := kustomization.Kustomization
		kustomizations = append(kustomizations, kustomizationCopy)
	}
	blueprint.Kustomizations = kustomizations

	return blueprint
}

// =============================================================================
// Private Methods
// =============================================================================

// evaluateSubstitutions evaluates expressions in substitution values and converts all results to strings.
// It processes each substitution value, evaluating any expressions it contains using EvaluateDefaults,
// then converts the results to strings as required by Flux's post-build substitution format.
func (e *FeatureEvaluator) evaluateSubstitutions(substitutions map[string]string, config map[string]any, featurePath string) (map[string]string, error) {
	result := make(map[string]string)

	for key, value := range substitutions {
		if strings.Contains(value, "${") {
			anyMap := map[string]any{key: value}
			evaluated, err := e.EvaluateDefaults(anyMap, config, featurePath)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate substitution for key '%s': %w", key, err)
			}

			evaluatedValue := evaluated[key]
			if evaluatedValue == nil {
				result[key] = ""
			} else {
				result[key] = fmt.Sprintf("%v", evaluatedValue)
			}
		} else {
			result[key] = value
		}
	}

	return result, nil
}
