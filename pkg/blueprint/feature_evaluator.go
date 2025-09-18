package blueprint

import (
	"fmt"

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

// MatchConditions evaluates whether all conditions in a map match the provided configuration.
// This provides a simple key-value matching interface for basic feature conditions.
// Each condition key-value pair must match for the overall result to be true.
func (e *FeatureEvaluator) MatchConditions(conditions map[string]any, config map[string]any) bool {
	if len(conditions) == 0 {
		return true
	}

	for key, expectedValue := range conditions {
		if !e.matchCondition(key, expectedValue, config) {
			return false
		}
	}

	return true
}

// =============================================================================
// Private Methods
// =============================================================================

// matchCondition checks if a single condition matches the configuration.
// Supports dot notation for nested field access and array matching for OR logic.
func (e *FeatureEvaluator) matchCondition(key string, expectedValue any, config map[string]any) bool {
	actualValue := e.getNestedValue(key, config)

	if expectedArray, ok := expectedValue.([]any); ok {
		for _, expected := range expectedArray {
			if e.valuesEqual(actualValue, expected) {
				return true
			}
		}
		return false
	}

	return e.valuesEqual(actualValue, expectedValue)
}

// getNestedValue retrieves a value from a nested map using dot notation.
// For example, "observability.enabled" retrieves config["observability"]["enabled"].
func (e *FeatureEvaluator) getNestedValue(key string, config map[string]any) any {
	keys := e.splitKey(key)
	current := config

	for i, k := range keys {
		if current == nil {
			return nil
		}

		value, exists := current[k]
		if !exists {
			return nil
		}

		if i == len(keys)-1 {
			return value
		}

		if nextMap, ok := value.(map[string]any); ok {
			current = nextMap
		} else {
			return nil
		}
	}

	return nil
}

// splitKey splits a dot-notation key into its component parts.
func (e *FeatureEvaluator) splitKey(key string) []string {
	if key == "" {
		return []string{}
	}

	var parts []string
	current := ""

	for _, char := range key {
		if char == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// valuesEqual compares two values for equality, handling type conversion.
func (e *FeatureEvaluator) valuesEqual(actual, expected any) bool {
	if actual == nil && expected == nil {
		return true
	}
	if actual == nil || expected == nil {
		return false
	}

	return fmt.Sprintf("%v", actual) == fmt.Sprintf("%v", expected)
}
