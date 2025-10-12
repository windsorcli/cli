package blueprint

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr"
)

// FeatureEvaluator provides lightweight expression evaluation for blueprint feature conditions.
// It uses the expr library for fast compilation and evaluation of simple comparison expressions
// without the overhead of a full expression language like CEL for basic equality checks.
// The FeatureEvaluator enables efficient feature activation based on user configuration values.

// =============================================================================
// Types
// =============================================================================

// FeatureEvaluator provides lightweight expression evaluation for feature conditions.
type FeatureEvaluator struct{}

// =============================================================================
// Constructor
// =============================================================================

// NewFeatureEvaluator creates a new lightweight feature evaluator for expression evaluation.
func NewFeatureEvaluator() *FeatureEvaluator {
	return &FeatureEvaluator{}
}

// =============================================================================
// Public Methods
// =============================================================================

// EvaluateExpression evaluates an expression string against the provided configuration data.
// The expression should use simple comparison syntax supported by expr:
// - Equality/inequality: ==, !=
// - Logical operators: &&, ||
// - Parentheses for grouping: (expression)
// - Nested object access: provider, observability.enabled, vm.driver
// Returns true if the expression evaluates to true, false otherwise.
func (e *FeatureEvaluator) EvaluateExpression(expression string, config map[string]any) (bool, error) {
	if expression == "" {
		return false, fmt.Errorf("expression cannot be empty")
	}

	program, err := expr.Compile(expression)
	if err != nil {
		return false, fmt.Errorf("failed to compile expression '%s': %w", expression, err)
	}

	result, err := expr.Run(program, config)
	if err != nil {
		return false, fmt.Errorf("failed to evaluate expression '%s': %w", expression, err)
	}

	boolResult, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("expression '%s' must evaluate to boolean, got %T", expression, result)
	}

	return boolResult, nil
}

// EvaluateValue evaluates an expression and returns the result as any type.
// Supports arithmetic, string operations, array construction, and nested object access.
// Returns the evaluated value or an error if evaluation fails.
func (e *FeatureEvaluator) EvaluateValue(expression string, config map[string]any) (any, error) {
	if expression == "" {
		return nil, fmt.Errorf("expression cannot be empty")
	}

	program, err := expr.Compile(expression)
	if err != nil {
		return nil, fmt.Errorf("failed to compile expression '%s': %w", expression, err)
	}

	result, err := expr.Run(program, config)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate expression '%s': %w", expression, err)
	}

	return result, nil
}

// EvaluateDefaults recursively evaluates default values, treating quoted strings as literals
// and unquoted values as expressions. Supports nested maps and arrays.
func (e *FeatureEvaluator) EvaluateDefaults(defaults map[string]any, config map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for key, value := range defaults {
		evaluated, err := e.evaluateDefaultValue(value, config)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate default for key '%s': %w", key, err)
		}
		result[key] = evaluated
	}

	return result, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// evaluateDefaultValue recursively evaluates a single default value.
func (e *FeatureEvaluator) evaluateDefaultValue(value any, config map[string]any) (any, error) {
	switch v := value.(type) {
	case string:
		if expr := e.extractExpression(v); expr != "" {
			return e.EvaluateValue(expr, config)
		}
		if strings.Contains(v, "${") {
			return e.interpolateString(v, config)
		}
		return v, nil

	case map[string]any:
		result := make(map[string]any)
		for k, val := range v {
			evaluated, err := e.evaluateDefaultValue(val, config)
			if err != nil {
				return nil, err
			}
			result[k] = evaluated
		}
		return result, nil

	case []any:
		result := make([]any, len(v))
		for i, val := range v {
			evaluated, err := e.evaluateDefaultValue(val, config)
			if err != nil {
				return nil, err
			}
			result[i] = evaluated
		}
		return result, nil

	default:
		return value, nil
	}
}

// extractExpression checks if a string contains a single expression spanning the entire string.
// If found, returns the expression content. Otherwise returns empty string.
func (e *FeatureEvaluator) extractExpression(s string) string {
	if !strings.Contains(s, "${") {
		return ""
	}

	start := strings.Index(s, "${")
	end := strings.Index(s[start:], "}")

	if end == -1 {
		return ""
	}

	end += start

	if start == 0 && end == len(s)-1 {
		return s[start+2 : end]
	}

	return ""
}

// interpolateString replaces all ${expression} occurrences in a string with their evaluated values.
func (e *FeatureEvaluator) interpolateString(s string, config map[string]any) (string, error) {
	result := s

	for strings.Contains(result, "${") {
		start := strings.Index(result, "${")
		end := strings.Index(result[start:], "}")

		if end == -1 {
			return "", fmt.Errorf("unclosed expression in string: %s", s)
		}

		end += start
		expr := result[start+2 : end]

		value, err := e.EvaluateValue(expr, config)
		if err != nil {
			return "", fmt.Errorf("failed to evaluate expression '${%s}': %w", expr, err)
		}

		var replacement string
		if value == nil {
			replacement = ""
		} else {
			replacement = fmt.Sprintf("%v", value)
		}

		result = result[:start] + replacement + result[end+1:]
	}

	return result, nil
}
