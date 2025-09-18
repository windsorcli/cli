package blueprint

import (
	"fmt"
	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// The FeatureEvaluator is a CEL-based expression evaluator for blueprint feature conditions.
// It provides GitHub Actions-style conditional expression evaluation capabilities with support
// for nested object access, logical operators, and type-safe variable declarations.
// The FeatureEvaluator enables dynamic feature activation based on user configuration values.

// =============================================================================
// Types
// =============================================================================

// FeatureEvaluator provides CEL expression evaluation capabilities for feature conditions.
type FeatureEvaluator struct {
	env *cel.Env
}

// =============================================================================
// Constructor
// =============================================================================

// NewFeatureEvaluator creates a new CEL-based feature evaluator configured for evaluating feature conditions.
// The evaluator is pre-configured with standard libraries and custom functions needed
// for blueprint feature evaluation.
func NewFeatureEvaluator() (*FeatureEvaluator, error) {
	env, err := cel.NewEnv(
		cel.HomogeneousAggregateLiterals(),
		cel.EagerlyValidateDeclarations(true),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	return &FeatureEvaluator{
		env: env,
	}, nil
}

// =============================================================================
// Public Methods
// =============================================================================

// CompileExpression compiles a CEL expression string with variable declarations derived from the config structure.
// The expression should follow GitHub Actions-style syntax with support for:
// - Equality/inequality: ==, !=
// - Logical operators: &&, ||
// - Parentheses for grouping: (expression)
// - Nested object access: provider, observability.enabled, vm.driver
// Returns a compiled program that can be evaluated against configuration data.
func (e *FeatureEvaluator) CompileExpression(expression string, config map[string]any) (cel.Program, error) {
	if expression == "" {
		return nil, fmt.Errorf("expression cannot be empty")
	}

	var envOptions []cel.EnvOption
	envOptions = append(envOptions, cel.HomogeneousAggregateLiterals())
	envOptions = append(envOptions, cel.EagerlyValidateDeclarations(true))

	for key, value := range config {
		envOptions = append(envOptions, cel.Variable(key, e.getCELType(value)))
	}

	env, err := cel.NewEnv(envOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment with config: %w", err)
	}

	ast, issues := env.Compile(expression)
	if issues.Err() != nil {
		return nil, fmt.Errorf("failed to compile expression '%s': %w", expression, issues.Err())
	}

	program, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("failed to create program for expression '%s': %w", expression, err)
	}

	return program, nil
}

// EvaluateProgram executes a compiled CEL program against the provided configuration data.
// The configuration data should be a map containing the user's configuration values
// that the expression will be evaluated against.
// Returns true if the expression evaluates to true, false otherwise.
func (e *FeatureEvaluator) EvaluateProgram(program cel.Program, config map[string]any) (bool, error) {
	if config == nil {
		config = make(map[string]any)
	}

	result, _, err := program.Eval(config)
	if err != nil {
		return false, fmt.Errorf("failed to evaluate expression: %w", err)
	}

	return e.convertToBool(result)
}

// EvaluateExpression is a convenience method that compiles and evaluates an expression in one call.
// This is useful for one-time evaluations where the compiled program won't be reused.
func (e *FeatureEvaluator) EvaluateExpression(expression string, config map[string]any) (bool, error) {
	program, err := e.CompileExpression(expression, config)
	if err != nil {
		return false, err
	}

	return e.EvaluateProgram(program, config)
}

// =============================================================================
// Private Methods
// =============================================================================

// convertToBool converts a CEL result value to a boolean.
// CEL expressions should evaluate to boolean values for feature conditions.
func (e *FeatureEvaluator) convertToBool(result ref.Val) (bool, error) {
	if result.Type() == types.BoolType {
		return result.Value().(bool), nil
	}

	return false, fmt.Errorf("expression must evaluate to boolean, got %s", result.Type())
}

// getCELType determines the appropriate CEL type for a Go value.
// This is used to create variable declarations for the CEL environment.
func (e *FeatureEvaluator) getCELType(value any) *cel.Type {
	if value == nil {
		return cel.DynType
	}

	switch reflect.TypeOf(value).Kind() {
	case reflect.String:
		return cel.StringType
	case reflect.Bool:
		return cel.BoolType
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return cel.IntType
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return cel.UintType
	case reflect.Float32, reflect.Float64:
		return cel.DoubleType
	case reflect.Map:
		return cel.MapType(cel.StringType, cel.DynType)
	case reflect.Slice, reflect.Array:
		return cel.ListType(cel.DynType)
	default:
		return cel.DynType
	}
}
