// The built-in functions are the expression helpers that every evaluation registers.
// It provides string(), jsonnet(), file(), env(), yaml(), yamlString(), jsonString(), and split().
// Each factory returns one expr option that checks the argument count and types.
// The CIDR functions are in cidr.go.

package evaluator

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Private Methods
// =============================================================================

// stringFunction returns the string(value) helper. It renders a map or a slice as YAML,
// renders nil as an empty string, and renders other values with fmt.Sprint.
func (e *expressionEvaluator) stringFunction() expr.Option {
	return expr.Function(
		"string",
		func(params ...any) (any, error) {
			if len(params) != 1 {
				return nil, fmt.Errorf("string() requires exactly 1 argument, got %d", len(params))
			}
			v := params[0]
			if v == nil {
				return "", nil
			}
			rv := reflect.ValueOf(v)
			switch rv.Kind() {
			case reflect.Map, reflect.Slice:
				yamlData, err := e.Shims.YamlMarshal(v)
				if err != nil {
					return nil, werror.Wrap(err, "string() failed to marshal map/slice to YAML")
				}
				return strings.TrimSpace(string(yamlData)), nil
			default:
				return fmt.Sprint(v), nil
			}
		},
		new(func(any) string),
	)
}

// jsonnetFunction returns the jsonnet(path) helper. The Jsonnet file gets config as std.extVar("context").
func (e *expressionEvaluator) jsonnetFunction(config map[string]any, facetPath string) expr.Option {
	return expr.Function(
		"jsonnet",
		func(params ...any) (any, error) {
			if len(params) != 1 {
				return nil, fmt.Errorf("jsonnet() requires exactly 1 argument, got %d", len(params))
			}
			path, ok := params[0].(string)
			if !ok {
				return nil, fmt.Errorf("jsonnet() path must be a string, got %T", params[0])
			}
			return e.evaluateJsonnetFunction(path, config, facetPath)
		},
		new(func(string) any),
	)
}

// fileFunction returns the file(path) helper, which returns the raw file content as a string.
func (e *expressionEvaluator) fileFunction(facetPath string) expr.Option {
	return expr.Function(
		"file",
		func(params ...any) (any, error) {
			if len(params) != 1 {
				return nil, fmt.Errorf("file() requires exactly 1 argument, got %d", len(params))
			}
			path, ok := params[0].(string)
			if !ok {
				return nil, fmt.Errorf("file() path must be a string, got %T", params[0])
			}
			return e.evaluateFileFunction(path, facetPath)
		},
		new(func(string) string),
	)
}

// envFunction returns the env(name) helper. It returns nil when the variable is not set.
func (e *expressionEvaluator) envFunction() expr.Option {
	return expr.Function(
		"env",
		func(params ...any) (any, error) {
			if len(params) != 1 {
				return nil, fmt.Errorf("env() requires exactly 1 argument, got %d", len(params))
			}
			name, ok := params[0].(string)
			if !ok {
				return nil, fmt.Errorf("env() variable name must be a string, got %T", params[0])
			}
			if value, present := e.Shims.LookupEnv(name); present {
				return value, nil
			}
			return nil, nil
		},
		new(func(string) any),
	)
}

// yamlFunction returns the yaml(pathOrContent, input?) helper, which parses YAML from a file or a string.
func (e *expressionEvaluator) yamlFunction(facetPath string) expr.Option {
	return expr.Function(
		"yaml",
		func(params ...any) (any, error) {
			if len(params) < 1 || len(params) > 2 {
				return nil, fmt.Errorf("yaml() requires 1 or 2 arguments, got %d", len(params))
			}
			arg, ok := params[0].(string)
			if !ok {
				return nil, fmt.Errorf("yaml() first argument must be a string (file path or YAML content), got %T", params[0])
			}
			var input any
			if len(params) == 2 {
				input = params[1]
			}
			return e.evaluateYamlFunction(arg, facetPath, input)
		},
		new(func(string, ...any) any),
	)
}

// yamlStringFunction returns the yamlString helper. yamlString(value) marshals value to YAML.
// yamlString(path, input) templates the file at path with input and returns the text unparsed.
func (e *expressionEvaluator) yamlStringFunction(facetPath string) expr.Option {
	return expr.Function(
		"yamlString",
		func(params ...any) (any, error) {
			if len(params) < 1 || len(params) > 2 {
				return nil, fmt.Errorf("yamlString() requires 1 or 2 arguments, got %d", len(params))
			}
			if len(params) == 2 {
				pathArg, ok := params[0].(string)
				if !ok {
					return nil, fmt.Errorf("yamlString(path, input) first argument must be a string, got %T", params[0])
				}
				return e.evaluateYamlStringFromFile(pathArg, facetPath, params[1])
			}
			yamlBytes, err := e.Shims.YamlMarshal(params[0])
			if err != nil {
				return nil, werror.Wrap(err, "yamlString() failed to marshal")
			}
			return string(yamlBytes), nil
		},
		new(func(any) string),
		new(func(string, any) string),
	)
}

// jsonStringFunction returns the jsonString(value) helper, which marshals value to JSON.
func (e *expressionEvaluator) jsonStringFunction() expr.Option {
	return expr.Function(
		"jsonString",
		func(params ...any) (any, error) {
			if len(params) != 1 {
				return nil, fmt.Errorf("jsonString() requires exactly 1 argument, got %d", len(params))
			}
			jsonBytes, err := e.Shims.JsonMarshal(normalizeYamlResult(params[0]))
			if err != nil {
				return nil, werror.Wrap(err, "jsonString() failed to marshal")
			}
			return string(jsonBytes), nil
		},
		new(func(any) string),
	)
}

// splitFunction returns the split(str, sep) helper, which returns the parts as a list.
func (e *expressionEvaluator) splitFunction() expr.Option {
	return expr.Function(
		"split",
		func(params ...any) (any, error) {
			if len(params) != 2 {
				return nil, fmt.Errorf("split() requires exactly 2 arguments, got %d", len(params))
			}
			str, ok := params[0].(string)
			if !ok {
				return nil, fmt.Errorf("split() first argument must be a string, got %T", params[0])
			}
			sep, ok := params[1].(string)
			if !ok {
				return nil, fmt.Errorf("split() second argument must be a string, got %T", params[1])
			}
			parts := strings.Split(str, sep)
			result := make([]any, len(parts))
			for i, part := range parts {
				result[i] = part
			}
			return result, nil
		},
		new(func(string, string) []any),
	)
}
