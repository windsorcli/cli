package blueprint

import (
	"fmt"
	"sort"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// BlueprintProcessor evaluates when: conditions on features, terraform components, and kustomizations.
// It determines inclusion/exclusion based on boolean condition results.
// The processor is stateless and shared across all loaders.
type BlueprintProcessor interface {
	ProcessFeatures(features []blueprintv1alpha1.Feature, config map[string]any, sourceName ...string) (*blueprintv1alpha1.Blueprint, error)
}

// =============================================================================
// Types
// =============================================================================

// BaseBlueprintProcessor provides the default implementation of the BlueprintProcessor interface.
type BaseBlueprintProcessor struct {
	runtime   *runtime.Runtime
	evaluator evaluator.ExpressionEvaluator
}

// =============================================================================
// Constructor
// =============================================================================

// NewBlueprintProcessor creates a new BlueprintProcessor using the runtime's expression evaluator.
// The evaluator is used to evaluate 'when' conditions on features and components. Optional
// overrides allow replacing the evaluator for testing. The processor is stateless and can
// be shared across multiple concurrent feature processing operations.
func NewBlueprintProcessor(rt *runtime.Runtime, opts ...*BaseBlueprintProcessor) *BaseBlueprintProcessor {
	processor := &BaseBlueprintProcessor{
		runtime:   rt,
		evaluator: rt.Evaluator,
	}

	if len(opts) > 0 && opts[0] != nil {
		overrides := opts[0]
		if overrides.evaluator != nil {
			processor.evaluator = overrides.evaluator
		}
	}

	return processor
}

// =============================================================================
// Public Methods
// =============================================================================

// ProcessFeatures iterates through a list of features, evaluating each feature's 'when' condition
// against the provided configuration data. Features whose conditions evaluate to true (or have no
// condition) contribute their terraform components and kustomizations to the result. Components
// within features can also have individual 'when' conditions for fine-grained control. Features
// are sorted by metadata.name before processing to ensure deterministic output. If sourceName is
// provided, it sets the Source field on components that don't already have one, linking them to
// their originating OCI artifact. Input expressions and substitutions are preserved as-is for
// later evaluation by their consumers.
func (p *BaseBlueprintProcessor) ProcessFeatures(features []blueprintv1alpha1.Feature, configData map[string]any, sourceName ...string) (*blueprintv1alpha1.Blueprint, error) {
	result := &blueprintv1alpha1.Blueprint{
		Kind:       "Blueprint",
		ApiVersion: "blueprints.windsorcli.dev/v1alpha1",
	}

	if len(features) == 0 {
		return result, nil
	}

	sortedFeatures := make([]blueprintv1alpha1.Feature, len(features))
	copy(sortedFeatures, features)
	sort.Slice(sortedFeatures, func(i, j int) bool {
		return sortedFeatures[i].Metadata.Name < sortedFeatures[j].Metadata.Name
	})

	for _, feature := range sortedFeatures {
		if feature.When != "" {
			matches, err := p.evaluateCondition(feature.When, configData, feature.Path)
			if err != nil {
				return nil, fmt.Errorf("error evaluating feature '%s' condition: %w", feature.Metadata.Name, err)
			}
			if !matches {
				continue
			}
		}

		for _, tc := range feature.TerraformComponents {
			if tc.When != "" {
				matches, err := p.evaluateCondition(tc.When, configData, feature.Path)
				if err != nil {
					return nil, fmt.Errorf("error evaluating terraform component condition: %w", err)
				}
				if !matches {
					continue
				}
			}
			component := tc.TerraformComponent
			if component.Inputs != nil {
				evaluated, err := p.evaluateInputs(component.Inputs, configData, feature.Path)
				if err != nil {
					return nil, fmt.Errorf("error evaluating inputs for component '%s': %w", component.GetID(), err)
				}
				component.Inputs = evaluated
			}
			if component.Source == "" && len(sourceName) > 0 && sourceName[0] != "" {
				component.Source = sourceName[0]
			}
			result.TerraformComponents = append(result.TerraformComponents, component)
		}

		for _, k := range feature.Kustomizations {
			if k.When != "" {
				matches, err := p.evaluateCondition(k.When, configData, feature.Path)
				if err != nil {
					return nil, fmt.Errorf("error evaluating kustomization condition: %w", err)
				}
				if !matches {
					continue
				}
			}
			kustomization := k.Kustomization
			if kustomization.Substitutions != nil {
				evaluated, err := p.evaluateSubstitutions(kustomization.Substitutions, configData, feature.Path)
				if err != nil {
					return nil, fmt.Errorf("error evaluating substitutions for kustomization '%s': %w", kustomization.Name, err)
				}
				kustomization.Substitutions = evaluated
			}
			if kustomization.Source == "" && len(sourceName) > 0 && sourceName[0] != "" {
				kustomization.Source = sourceName[0]
			}
			result.Kustomizations = append(result.Kustomizations, kustomization)
		}
	}

	return result, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// evaluateCondition uses the expression evaluator to evaluate a 'when' condition string against
// the provided configuration data. The path parameter provides context for error messages and
// helper function resolution. Returns true if the expression evaluates to boolean true or the
// string "true", false otherwise. Returns an error if the expression is invalid or evaluates
// to an unexpected type.
func (p *BaseBlueprintProcessor) evaluateCondition(expr string, configData map[string]any, path string) (bool, error) {
	evaluated, err := p.evaluator.Evaluate(expr, configData, path)
	if err != nil {
		return false, err
	}

	switch v := evaluated.(type) {
	case bool:
		return v, nil
	case string:
		return v == "true", nil
	default:
		return false, fmt.Errorf("condition must evaluate to boolean, got %T", evaluated)
	}
}

func (p *BaseBlueprintProcessor) evaluateInputs(inputs map[string]any, configData map[string]any, featurePath string) (map[string]any, error) {
	result := make(map[string]any)
	for key, value := range inputs {
		strVal, isString := value.(string)
		if !isString || !strings.Contains(strVal, "${") {
			result[key] = value
			continue
		}
		evaluated, err := p.evaluator.EvaluateValue(strVal, configData, featurePath)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate '%s': %w", key, err)
		}
		result[key] = evaluated
	}
	return result, nil
}

func (p *BaseBlueprintProcessor) evaluateSubstitutions(subs map[string]string, configData map[string]any, featurePath string) (map[string]string, error) {
	result := make(map[string]string)
	for key, value := range subs {
		if !strings.Contains(value, "${") {
			result[key] = value
			continue
		}
		evaluated, err := p.evaluator.InterpolateString(value, configData, featurePath)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate '%s': %w", key, err)
		}
		result[key] = evaluated
	}
	return result, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ BlueprintProcessor = (*BaseBlueprintProcessor)(nil)
