// The ExpressionEvaluator provides unified expression evaluation for the Runtime system.
// It supplies expression evaluation using the Runtime's config handler, supporting features such as
// jsonnet, file loading, and string interpolation. All expression evaluation is performed through
// the provider system, enabling on-demand resolution of dynamic values.

package evaluator

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"net"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/google/go-jsonnet"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// optionalChainPatcher wraps member access chains in ChainNode and sets the Optional flag.
// This ensures that expressions like "addons.database.enabled" behave as if written
// "addons?.database?.enabled", returning nil instead of erroring when intermediate
// properties are missing.
type optionalChainPatcher struct{}

// Visit implements the ast.Visitor interface, wrapping MemberNode chains in ChainNode.
func (p *optionalChainPatcher) Visit(node *ast.Node) {
	if member, ok := (*node).(*ast.MemberNode); ok {
		member.Optional = true
		p.setNestedOptional(member.Node)
		chain := &ast.ChainNode{Node: member}
		ast.Patch(node, chain)
	}
}

// setNestedOptional recursively sets Optional=true on all nested MemberNodes.
func (p *optionalChainPatcher) setNestedOptional(node ast.Node) {
	if member, ok := node.(*ast.MemberNode); ok {
		member.Optional = true
		p.setNestedOptional(member.Node)
	}
}

// =============================================================================
// Types
// =============================================================================

// expressionEvaluator is the concrete implementation of ExpressionEvaluator.
type expressionEvaluator struct {
	configHandler config.ConfigHandler
	projectRoot   string
	templateRoot  string
	Shims         *Shims
	templateData  map[string][]byte
	helpers       []func(allowDeferred bool) expr.Option
}

// =============================================================================
// Errors
// =============================================================================

// DeferredError signals that an expression is deferred and should be preserved for later evaluation.
// This is not an error condition but a signal that the expression cannot be evaluated at this time.
type DeferredError struct {
	Expression string
	Message    string
}

// Error implements the error interface for DeferredError.
func (e *DeferredError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("deferred expression: %s", e.Expression)
}

// =============================================================================
// Interfaces
// =============================================================================

// ExpressionEvaluator provides unified expression evaluation for the Runtime system.
// It uses the config handler for accessing configuration values and supports
// dynamic value resolution through the provider system. The evaluator handles
// expression compilation, evaluation, and string interpolation with support
// for Jsonnet files and file loading functions.
type ExpressionEvaluator interface {
	HelperRegistrar
	SetTemplateData(templateData map[string][]byte)
	Evaluate(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error)
	EvaluateMap(values map[string]any, facetPath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error)
}

// HelperRegistrar defines the interface for registering custom helpers with the evaluator.
// This allows providers to extend expression evaluation capabilities without the evaluator
// needing to know about provider-specific functionality.
type HelperRegistrar interface {
	Register(name string, helper func(params []any, deferred bool) (any, error), signature any)
}

// =============================================================================
// Constructor
// =============================================================================

// NewExpressionEvaluator creates a new expression evaluator with the provided dependencies.
// The configHandler is used for accessing configuration values and provider resolution.
// The projectRoot and templateRoot paths are used for resolving relative file paths
// in expressions. Returns a fully initialized evaluator ready for use.
func NewExpressionEvaluator(configHandler config.ConfigHandler, projectRoot, templateRoot string) ExpressionEvaluator {
	if configHandler == nil {
		panic("config handler is required")
	}

	return &expressionEvaluator{
		configHandler: configHandler,
		projectRoot:   projectRoot,
		templateRoot:  templateRoot,
		Shims:         NewShims(),
		helpers:       []func(deferred bool) expr.Option{},
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// SetTemplateData sets the template data map for in-memory file resolution.
// This is used when loading blueprints from OCI artifacts where template files
// are stored in memory rather than on disk. The map keys should use paths
// relative to the template root, typically prefixed with "_template/".
func (e *expressionEvaluator) SetTemplateData(templateData map[string][]byte) {
	e.templateData = templateData
}

// Register adds a custom helper function to the evaluator for expression evaluation.
// The provided helper function receives the deferred flag as a closure variable. The name parameter
// specifies the function name used in expressions, helper is the custom function, and signature defines
// the helper's input/output signature for the evaluator's use. This allows providers and consumers to
// extend evaluator capabilities with domain-specific helpers.
func (e *expressionEvaluator) Register(name string, helper func(params []any, deferred bool) (any, error), signature any) {
	e.helpers = append(e.helpers, func(deferred bool) expr.Option {
		wrappedHelper := func(params ...any) (any, error) {
			return helper(params, deferred)
		}
		return expr.Function(name, wrappedHelper, signature)
	})
}

// Evaluate resolves Windsor expressions in the provided string s, supporting both complete and interpolated
// (${...}) expressions, arithmetic operations, and complex object types. The evaluation will process dynamic
// file and jsonnet loading as needed, and can defer unresolved expressions when evaluateDeferred is false.
// The facetPath parameter is used for relative expression resolution, and evaluateDeferred controls
// whether to process unresolved expressions immediately. When scope is non-nil, it is merged into the
// evaluation environment (e.g. facet config so expressions can reference talos.controlplanes). Returns the
// fully evaluated value, or an error if evaluation fails or the input is malformed. Empty strings are returned
// as-is. If the result is nil (such as from an undefined variable without a ?? fallback), nil is returned for
// complete expressions or an empty string is used for interpolation.
func (e *expressionEvaluator) Evaluate(s string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
	return e.evaluate(s, facetPath, scope, evaluateDeferred)
}

// EvaluateMap evaluates a map of values using this expression evaluator. Each string value is evaluated as an
// expression; maps and arrays are recursively evaluated. When evaluateDeferred is false and evaluation fails
// with a DeferredError, the original value is preserved in the result. The facetPath parameter is used for
// context during evaluation. When scope is non-nil, it is merged into the evaluation environment. Returns a
// new map containing evaluated values (or original values if deferred), or an error if evaluation fails with
// a non-deferred error.
func (e *expressionEvaluator) EvaluateMap(values map[string]any, facetPath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
	return e.evaluateMap(values, facetPath, scope, evaluateDeferred)
}

func (e *expressionEvaluator) evaluateMap(values map[string]any, facetPath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
	result := make(map[string]any)
	for key, value := range values {
		evaluated, err := e.evaluateValue(value, facetPath, evaluateDeferred, scope)
		if err != nil {
			if !evaluateDeferred {
				var deferredErr *DeferredError
				if errors.As(err, &deferredErr) {
					result[key] = value
					continue
				}
			}
			return nil, fmt.Errorf("failed to evaluate '%s': %w", key, err)
		}
		if !evaluateDeferred {
			if ContainsExpression(evaluated) && !isStructuredValue(evaluated) {
				result[key] = value
				continue
			}
		}
		result[key] = evaluated
	}
	return result, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// evaluate runs the expression parsing loop over s, resolving each ${...} with evaluateExpression.
// When scope is nil the config context is used; when non-nil (e.g. for yaml(path, input)) the given
// scope is used. Stops after 20 iterations to avoid infinite loops on circular or pathological input.
func (e *expressionEvaluator) evaluate(s string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
	if !strings.Contains(s, "${") {
		return s, nil
	}
	result := s
	for iter := 0; iter < 20 && strings.Contains(result, "${"); iter++ {
		start := strings.Index(result, "${")
		end := findExpressionEnd(result, start)
		if end == -1 {
			return "", fmt.Errorf("unclosed expression in string: %s", s)
		}
		expr := result[start+2 : end]
		if expr == "" {
			return nil, fmt.Errorf("expression cannot be empty")
		}
		value, err := e.evaluateExpression(expr, facetPath, scope, evaluateDeferred)
		if err != nil {
			if !evaluateDeferred {
				if _, ok := err.(*DeferredError); ok {
					return s, nil
				}
				var deferredErr *DeferredError
				if errors.As(err, &deferredErr) {
					return s, nil
				}
			}
			return "", fmt.Errorf("failed to evaluate expression '${%s}': %w", expr, err)
		}
		before := result[:start]
		after := result[end+1:]
		singleExpr := strings.TrimSpace(before) == "" && strings.TrimSpace(after) == ""
		if singleExpr {
			if str, ok := value.(string); ok && ContainsExpression(str) {
				if str == result {
					return value, nil
				}
				result = str
				continue
			}
			return value, nil
		}
		var replacement string
		if value != nil {
			var err error
			replacement, err = valueToInterpolationString(value, e.Shims.YamlMarshal)
			if err != nil {
				return "", fmt.Errorf("failed to marshal expression result to YAML: %w", err)
			}
			if !evaluateDeferred && ContainsExpression(replacement) && isStructuredValue(value) {
				return value, nil
			}
			replacement = indentForEmbeddedYAML(before, replacement, 2)
		}
		result = before + replacement + after
	}
	return result, nil
}

// evaluateExpression compiles and evaluates a single expression using the given scope as the variable
// environment. Context (getConfig + enrichConfig) is always used as the base; when scope is non-nil
// (e.g. for yaml(path, input) templating), it is merged on top so expressions see both context and
// scope (scope keys override). The expression should not include ${} bookends. Returns the evaluation
// result or an error if compilation or execution fails.
func (e *expressionEvaluator) evaluateExpression(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
	config := e.getConfig()
	merged := e.enrichConfig(config)
	if scope != nil {
		maps.Copy(merged, scope)
	}
	env := e.buildExprEnvironment(merged, facetPath, evaluateDeferred)
	program, err := expr.Compile(expression, env...)
	if err != nil {
		return nil, fmt.Errorf("failed to compile expression '%s': %w", expression, err)
	}
	result, err := expr.Run(program, merged)
	if err != nil {
		if deferredErr, ok := err.(*DeferredError); ok {
			return nil, deferredErr
		}
		var deferredErr *DeferredError
		if errors.As(err, &deferredErr) {
			return nil, deferredErr
		}
		return nil, fmt.Errorf("failed to evaluate expression '%s': %w", expression, err)
	}
	return result, nil
}

// getConfig retrieves the current configuration context as a map of string keys to values.
// If the config handler is not set or does not provide a configuration, an empty map is returned.
// This method never returns a nil map, ensuring callers always receive a usable configuration structure.
func (e *expressionEvaluator) getConfig() map[string]any {
	if e.configHandler == nil {
		return make(map[string]any)
	}
	config, _ := e.configHandler.GetContextValues()
	if config == nil {
		return make(map[string]any)
	}
	return config
}

// enrichConfig enriches the provided config map with Windsor runtime-specific values.
// It adds "project_root" if the evaluator has a project root set, and "context_path"
// if the config handler can provide one. All paths are normalized to use forward slashes
// for cross-platform consistency. The enriched config is used to make runtime context
// available in expression evaluation without requiring explicit configuration.
func (e *expressionEvaluator) enrichConfig(config map[string]any) map[string]any {
	enrichedConfig := make(map[string]any)
	maps.Copy(enrichedConfig, config)
	if e.projectRoot != "" {
		enrichedConfig["project_root"] = strings.ReplaceAll(e.projectRoot, "\\", "/")
	}
	if e.configHandler != nil {
		if configRoot, err := e.configHandler.GetConfigRoot(); err == nil {
			enrichedConfig["context_path"] = strings.ReplaceAll(configRoot, "\\", "/")
		}
	}
	return enrichedConfig
}

// buildExprEnvironment configures the expression evaluation environment with helper functions.
// It registers helper functions: jsonnet() for evaluating Jsonnet files, file() for
// loading raw file content, yaml() for parsing YAML from a file path or inline string,
// split() for string splitting, and CIDR functions (cidrhost, cidrsubnet, cidrsubnets,
// cidrnetmask) for IP address and network calculations. Helper functions registered via
// Register() are also included. Each helper includes argument validation and type checking.
// The config and facetPath are captured in closures to provide context for file resolution
// during expression evaluation. It also enables AsAny mode to allow dynamic property access.
func (e *expressionEvaluator) buildExprEnvironment(config map[string]any, facetPath string, deferred bool) []expr.Option {
	opts := []expr.Option{
		expr.AsAny(),
		expr.Patch(&optionalChainPatcher{}),
		expr.Function(
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
						return nil, fmt.Errorf("string() failed to marshal map/slice to YAML: %w", err)
					}
					return strings.TrimSpace(string(yamlData)), nil
				default:
					return fmt.Sprint(v), nil
				}
			},
			new(func(any) string),
		),
		expr.Function(
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
		),
		expr.Function(
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
		),
		expr.Function(
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
		),
		expr.Function(
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
					return nil, fmt.Errorf("yamlString() failed to marshal: %w", err)
				}
				return string(yamlBytes), nil
			},
			new(func(any) string),
			new(func(string, any) string),
		),
		expr.Function(
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
		),
		expr.Function(
			"cidrhost",
			func(params ...any) (any, error) {
				if len(params) != 2 {
					return nil, fmt.Errorf("cidrhost() requires exactly 2 arguments, got %d", len(params))
				}
				prefix, ok := params[0].(string)
				if !ok {
					return nil, fmt.Errorf("cidrhost() first argument must be a string, got %T", params[0])
				}
				var hostnum int
				switch v := params[1].(type) {
				case int:
					hostnum = v
				case float64:
					hostnum = int(v)
				default:
					return nil, fmt.Errorf("cidrhost() second argument must be a number, got %T", params[1])
				}
				return e.evaluateCidrHostFunction(prefix, hostnum)
			},
			new(func(string, int) string),
		),
		expr.Function(
			"cidrsubnet",
			func(params ...any) (any, error) {
				if len(params) != 3 {
					return nil, fmt.Errorf("cidrsubnet() requires exactly 3 arguments, got %d", len(params))
				}
				prefix, ok := params[0].(string)
				if !ok {
					return nil, fmt.Errorf("cidrsubnet() first argument must be a string, got %T", params[0])
				}
				var newbits, netnum int
				switch v := params[1].(type) {
				case int:
					newbits = v
				case float64:
					newbits = int(v)
				default:
					return nil, fmt.Errorf("cidrsubnet() second argument must be a number, got %T", params[1])
				}
				switch v := params[2].(type) {
				case int:
					netnum = v
				case float64:
					netnum = int(v)
				default:
					return nil, fmt.Errorf("cidrsubnet() third argument must be a number, got %T", params[2])
				}
				return e.evaluateCidrSubnetFunction(prefix, newbits, netnum)
			},
			new(func(string, int, int) string),
		),
		expr.Function(
			"cidrsubnets",
			func(params ...any) (any, error) {
				if len(params) < 2 {
					return nil, fmt.Errorf("cidrsubnets() requires at least 2 arguments, got %d", len(params))
				}
				prefix, ok := params[0].(string)
				if !ok {
					return nil, fmt.Errorf("cidrsubnets() first argument must be a string, got %T", params[0])
				}
				newbits := make([]int, len(params)-1)
				for i := 1; i < len(params); i++ {
					switch v := params[i].(type) {
					case int:
						newbits[i-1] = v
					case float64:
						newbits[i-1] = int(v)
					default:
						return nil, fmt.Errorf("cidrsubnets() argument %d must be a number, got %T", i+1, params[i])
					}
				}
				return e.evaluateCidrSubnetsFunction(prefix, newbits)
			},
			new(func(string, ...int) []any),
		),
		expr.Function(
			"cidrnetmask",
			func(params ...any) (any, error) {
				if len(params) != 1 {
					return nil, fmt.Errorf("cidrnetmask() requires exactly 1 argument, got %d", len(params))
				}
				prefix, ok := params[0].(string)
				if !ok {
					return nil, fmt.Errorf("cidrnetmask() argument must be a string, got %T", params[0])
				}
				return e.evaluateCidrNetmaskFunction(prefix)
			},
			new(func(string) string),
		),
	}

	for _, helperFactory := range e.helpers {
		opts = append(opts, helperFactory(deferred))
	}

	return opts
}

// evaluateValue recursively evaluates a value, handling strings, maps, and arrays.
// When evaluateDeferred is false and evaluation fails with a DeferredError, the original value
// is preserved. When scope is non-nil, it is merged into the expression environment. This
// ensures that entire input values containing deferred expressions are preserved rather than
// being partially evaluated or skipped.
func (e *expressionEvaluator) evaluateValue(value any, facetPath string, evaluateDeferred bool, scope map[string]any) (any, error) {
	switch v := value.(type) {
	case string:
		evaluated, err := e.evaluate(v, facetPath, scope, evaluateDeferred)
		if err != nil {
			if !evaluateDeferred {
				var deferredErr *DeferredError
				if errors.As(err, &deferredErr) {
					return v, nil
				}
			}
			return nil, err
		}
		return normalizeYamlResult(evaluated), nil
	case map[string]any:
		result := make(map[string]any)
		for k, val := range v {
			evaluated, err := e.evaluateValue(val, facetPath, evaluateDeferred, scope)
			if err != nil {
				if !evaluateDeferred {
					var deferredErr *DeferredError
					if errors.As(err, &deferredErr) {
						result[k] = val
						continue
					}
				}
				return nil, err
			}
			if !evaluateDeferred {
				if ContainsExpression(evaluated) && !isStructuredValue(evaluated) {
					result[k] = val
					continue
				}
			}
			result[k] = evaluated
		}
		return result, nil
	case []any:
		result := make([]any, 0, len(v))
		for _, item := range v {
			evaluated, err := e.evaluateValue(item, facetPath, evaluateDeferred, scope)
			if err != nil {
				if !evaluateDeferred {
					var deferredErr *DeferredError
					if errors.As(err, &deferredErr) {
						result = append(result, item)
						continue
					}
				}
				return nil, err
			}
			if !evaluateDeferred {
				if ContainsExpression(evaluated) && !isStructuredValue(evaluated) {
					result = append(result, item)
					continue
				}
			}
			result = append(result, evaluated)
		}
		return result, nil
	default:
		return normalizeYamlResult(value), nil
	}
}

// evaluateJsonnetFunction loads and evaluates a Jsonnet file at the specified path.
// It first attempts to load the file from template data if available, falling back to
// the filesystem if not found. The config is enriched with context information and
// passed to the Jsonnet VM as external code. Helper functions and import paths are
// configured, then the Jsonnet is evaluated and the JSON output is unmarshaled.
// Returns the evaluated value or an error if loading, evaluation, or parsing fails.
func (e *expressionEvaluator) evaluateJsonnetFunction(pathArg string, config map[string]any, facetPath string) (any, error) {
	var content []byte
	var err error

	if e.templateData != nil {
		content = e.lookupInTemplateData(pathArg, facetPath)
	}

	var path string
	if content == nil {
		path = e.resolvePath(pathArg, facetPath)
		content, err = e.Shims.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", path, err)
		}
	} else {
		if e.templateRoot != "" && facetPath != "" {
			facetAbsPath := facetPath
			if !filepath.IsAbs(facetPath) {
				facetAbsPath = filepath.Join(e.templateRoot, facetPath)
			}
			facetDir := filepath.Dir(facetAbsPath)
			resolvedAbsPath := filepath.Clean(filepath.Join(facetDir, pathArg))
			if relPath, err := filepath.Rel(e.templateRoot, resolvedAbsPath); err == nil && !strings.HasPrefix(relPath, "..") {
				path = filepath.Join(e.templateRoot, relPath)
			} else {
				path = resolvedAbsPath
			}
		} else {
			path = e.resolvePath(pathArg, facetPath)
		}
	}

	enrichedConfig := e.buildContextMap(config)

	configJSON, err := e.Shims.JsonMarshal(enrichedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config to JSON: %w", err)
	}

	vm := e.Shims.NewJsonnetVM()

	helpersLibrary := e.buildHelperLibrary()
	vm.ExtCode("helpers", helpersLibrary)
	vm.ExtCode("context", string(configJSON))

	if dir := filepath.Dir(path); dir != "" {
		vm.Importer(&jsonnet.FileImporter{
			JPaths: []string{dir},
		})
	}

	result, err := vm.EvaluateAnonymousSnippet(filepath.Base(path), string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate jsonnet file %s: %w", path, err)
	}

	var value any
	if err := e.Shims.JsonUnmarshal([]byte(result), &value); err != nil {
		return nil, fmt.Errorf("jsonnet file %s must output valid JSON: %w", path, err)
	}

	return value, nil
}

// buildContextMap enriches the config map with context information for Jsonnet evaluation.
// It adds "name" from the config handler's current context and "projectName" derived from
// the project root directory. These fields provide consistency with main template processing
// and enable Jsonnet code to access context and project information. Returns a new map
// with the original config plus the context fields.
func (e *expressionEvaluator) buildContextMap(config map[string]any) map[string]any {
	contextMap := make(map[string]any)
	maps.Copy(contextMap, config)

	if e.configHandler != nil {
		contextName := e.configHandler.GetContext()
		contextMap["name"] = contextName
	}

	if e.projectRoot != "" {
		contextMap["projectName"] = e.Shims.FilepathBase(e.projectRoot)
	}

	return contextMap
}

// buildHelperLibrary returns a Jsonnet library string containing helper functions.
// The library provides safe accessors for nested object paths (get, getString, getInt, etc.),
// type checking and conversion utilities, and data manipulation functions like baseUrl
// and removeEmptyKeys. These helpers enable safer and more expressive Jsonnet code
// when working with configuration data and context objects.
func (e *expressionEvaluator) buildHelperLibrary() string {
	return `{
  get(obj, path, default=null):
    if std.findSubstr(".", path) == [] then
      if std.type(obj) == "object" && path in obj then obj[path] else default
    else
      local parts = std.split(path, ".");
      local getValue(o, pathParts) =
        if std.length(pathParts) == 0 then o
        else if std.type(o) != "object" then null
        else if !(pathParts[0] in o) then null
        else getValue(o[pathParts[0]], pathParts[1:]);
      local result = getValue(obj, parts);
      if result == null then default else result,

  getString(obj, path, default=""):
    local val = self.get(obj, path, null);
    if val == null then default
    else if std.type(val) == "string" then val
    else error "Expected string for '" + path + "' but got " + std.type(val) + ": " + std.toString(val),

  getInt(obj, path, default=0):
    local val = self.get(obj, path, null);
    if val == null then default
    else if std.type(val) == "number" then std.floor(val)
    else error "Expected number for '" + path + "' but got " + std.type(val) + ": " + std.toString(val),

  getNumber(obj, path, default=0):
    local val = self.get(obj, path, null);
    if val == null then default
    else if std.type(val) == "number" then val
    else error "Expected number for '" + path + "' but got " + std.type(val) + ": " + std.toString(val),

  getBool(obj, path, default=false):
    local val = self.get(obj, path, null);
    if val == null then default
    else if std.type(val) == "boolean" then val
    else error "Expected boolean for '" + path + "' but got " + std.type(val) + ": " + std.toString(val),

  getObject(obj, path, default={}):
    local val = self.get(obj, path, null);
    if val == null then default
    else if std.type(val) == "object" then val
    else error "Expected object for '" + path + "' but got " + std.type(val) + ": " + std.toString(val),

  getArray(obj, path, default=[]):
    local val = self.get(obj, path, null);
    if val == null then default
    else if std.type(val) == "array" then val
    else error "Expected array for '" + path + "' but got " + std.type(val) + ": " + std.toString(val),

  has(obj, path):
    self.get(obj, path, null) != null,

  baseUrl(endpoint):
    if endpoint == "" then 
      ""
    else
      local withoutProtocol = if std.startsWith(endpoint, "https://") then
        std.substr(endpoint, 8, std.length(endpoint) - 8)
      else if std.startsWith(endpoint, "http://") then
        std.substr(endpoint, 7, std.length(endpoint) - 7)
      else
        endpoint;
      local colonPos = std.findSubstr(":", withoutProtocol);
      if std.length(colonPos) > 0 then
        std.substr(withoutProtocol, 0, colonPos[0])
      else
        withoutProtocol,

  removeEmptyKeys(obj):
    local _removeEmptyKeys(obj) =
      if std.type(obj) == "object" then
        local filteredFields = std.filter(
          function(key)
            local value = obj[key];
            if std.type(value) == "object" || std.type(value) == "array" then
              local cleaned = _removeEmptyKeys(value);
              if std.type(cleaned) == "object" then
                std.length(std.objectFields(cleaned)) > 0
              else
                std.length(cleaned) > 0
            else
              value != null && (std.type(value) != "string" || value != "")
          ,
          std.objectFields(obj)
        );
        {
          [key]: if std.type(obj[key]) == "object" || std.type(obj[key]) == "array" then _removeEmptyKeys(obj[key]) else obj[key]
          for key in filteredFields
        }
      else if std.type(obj) == "array" then
        local filteredElements = std.filter(
          function(element)
            if std.type(element) == "object" || std.type(element) == "array" then
              local cleaned = _removeEmptyKeys(element);
              if std.type(cleaned) == "object" then
                std.length(std.objectFields(cleaned)) > 0
              else
                std.length(cleaned) > 0
            else
              element != null && (std.type(element) != "string" || element != "")
          ,
          obj
        );
        [
          if std.type(element) == "object" || std.type(element) == "array" then _removeEmptyKeys(element) else element
          for element in filteredElements
        ]
      else
        obj;
    _removeEmptyKeys(obj),
}`
}

// evaluateFileFunction loads raw file content from the specified path.
// It first attempts to find the file in template data using lookupInTemplateData, then
// tries a fallback lookup relative to the template root if available. If not found in
// template data, it reads the file from the filesystem. The path is resolved relative
// to the facetPath or project root as appropriate. Returns the file content as a
// string or an error if the file cannot be found or read.
func (e *expressionEvaluator) evaluateFileFunction(pathArg string, facetPath string) (any, error) {
	var content []byte
	var err error

	if e.templateData != nil {
		content = e.lookupInTemplateData(pathArg, facetPath)
	}

	if content == nil {
		path := e.resolvePath(pathArg, facetPath)
		content, err = e.Shims.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", path, err)
		}
	}

	return string(content), nil
}

// evaluateYamlStringFromFile loads the file at pathArg, optionally templates it with input
// (scope = config + {"input": input}), and returns the result as a string without parsing YAML.
// Used by yamlString(path, input) for Terraform variables that expect a YAML string.
// Template expressions must use the input prefix, e.g. ${input.certSANs}, ${input.registryMirrors}.
func (e *expressionEvaluator) evaluateYamlStringFromFile(pathArg string, facetPath string, input any) (string, error) {
	var raw []byte
	if e.templateData != nil {
		raw = e.lookupInTemplateData(pathArg, facetPath)
	}
	if raw == nil {
		path := e.resolvePath(pathArg, facetPath)
		content, err := e.Shims.ReadFile(path)
		if err == nil {
			raw = content
		}
	}
	if raw == nil {
		raw = []byte(pathArg)
	}
	content := string(raw)
	if input != nil {
		config := e.getConfig()
		enrichedConfig := e.enrichConfig(config)
		scope := make(map[string]any)
		maps.Copy(scope, enrichedConfig)
		scope["input"] = input
		interpolated, err := e.evaluate(content, facetPath, scope, true)
		if err != nil {
			return "", fmt.Errorf("yamlString() failed to template: %w", err)
		}
		str, ok := interpolated.(string)
		if !ok {
			return "", fmt.Errorf("yamlString(path, input) expects template to evaluate to a string, got %T", interpolated)
		}
		content = str
	}
	return content, nil
}

// evaluateYamlFunction parses YAML from either a file path or an inline YAML string.
// If the argument contains a newline it is treated as inline YAML only (no file read), avoiding
// a failed filesystem call. Otherwise template data and the filesystem are tried first; if no
// file is found, the argument is unmarshaled as YAML. When input is non-nil, the raw content
// is templated with scope = context + {"input": input} so ${input.xyz} and context vars (e.g.
// provider) resolve; the result is then parsed as YAML. Returns the parsed value (map or slice)
// or an error if the file cannot be read or the YAML is invalid.
func (e *expressionEvaluator) evaluateYamlFunction(arg string, facetPath string, input any) (any, error) {
	var raw []byte
	if strings.Contains(arg, "\n") {
		raw = []byte(arg)
	} else {
		if e.templateData != nil {
			raw = e.lookupInTemplateData(arg, facetPath)
			if raw == nil && facetPath == "" && (strings.HasPrefix(arg, "../") || strings.HasPrefix(arg, "./")) {
				key := filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(arg, "../"), "./"))
				raw = e.templateDataLookup(key)
			}
		}
		if raw == nil {
			path := e.resolvePath(arg, facetPath)
			if facetPath == "" && e.templateData != nil && e.templateRoot != "" && strings.HasPrefix(path, e.templateRoot) {
				if relPath, err := filepath.Rel(e.templateRoot, path); err == nil {
					raw = e.templateDataLookup(filepath.ToSlash(relPath))
				}
			}
			if raw == nil {
				content, err := e.Shims.ReadFile(path)
				if err == nil {
					raw = content
				}
			}
		}
		if raw == nil {
			if input == nil {
				var fallback any
				if err := e.Shims.YamlUnmarshal([]byte(arg), &fallback); err != nil {
					return nil, fmt.Errorf("yaml() file not found: %s", arg)
				}
				if _, ok := fallback.(string); ok {
					return nil, fmt.Errorf("yaml() file not found: %s", arg)
				}
				normalized := normalizeYamlResult(fallback)
				unwrapped := unwrapSingleElementList(normalized)
				if unwrapped != nil {
					return unwrapped, nil
				}
				return normalized, nil
			}
			return nil, fmt.Errorf("yaml() file not found: %s", arg)
		}
	}
	content := string(raw)
	if input != nil {
		config := e.getConfig()
		enrichedConfig := e.enrichConfig(config)
		scope := make(map[string]any)
		maps.Copy(scope, enrichedConfig)
		scope["input"] = input
		interpolated, err := e.evaluate(content, facetPath, scope, true)
		if err != nil {
			return nil, fmt.Errorf("yaml() failed to template: %w", err)
		}
		str, ok := interpolated.(string)
		if !ok {
			return nil, fmt.Errorf("yaml() with input expects template to evaluate to a string, got %T", interpolated)
		}
		content = str
	}
	var value any
	if err := e.Shims.YamlUnmarshal([]byte(content), &value); err != nil {
		return nil, fmt.Errorf("yaml() failed to parse: %w", err)
	}
	normalized := normalizeYamlResult(value)
	unwrapped := unwrapSingleElementList(normalized)
	if unwrapped != nil {
		return unwrapped, nil
	}
	return normalized, nil
}

// normalizeYamlResult converts YAML-unmarshaled values to map[string]any and []any so expr's
// Fetch uses map lookup for string keys instead of ToInt (slice index). Handles
// map[interface{}]interface{} and []interface{} from goccy/go-yaml.
func normalizeYamlResult(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map:
		out := make(map[string]any)
		iter := rv.MapRange()
		for iter.Next() {
			k := iter.Key()
			var key string
			switch k.Kind() {
			case reflect.Interface:
				if k.IsNil() {
					continue
				}
				key = fmt.Sprint(k.Elem().Interface())
			default:
				key = fmt.Sprint(k.Interface())
			}
			out[key] = normalizeYamlResult(iter.Value().Interface())
		}
		return out
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.String {
			return v
		}
		out := make([]any, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			elem := rv.Index(i)
			if elem.Kind() == reflect.Interface && !elem.IsNil() {
				elem = elem.Elem()
			}
			out = append(out, normalizeYamlResult(elem.Interface()))
		}
		return out
	default:
		return v
	}
}

// unwrapSingleElementList returns the single element when v is a list of one map-like value.
// expr's Fetch treats slice.key as slice[ToInt(key)], so .controlplane_labels on a list causes
// int(string) panic. Unwrapping allows yaml(path).key to work when the file has a single-document list at root.
// Uses reflection so any slice/map types from the YAML library (e.g. []interface{}, map[interface{}]interface{}) are accepted.
func unwrapSingleElementList(v any) any {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil
	}
	if rv.Len() != 1 {
		return nil
	}
	first := rv.Index(0)
	if first.Kind() == reflect.Interface && !first.IsNil() {
		first = first.Elem()
	}
	if first.Kind() != reflect.Map {
		return nil
	}
	return first.Interface()
}

// templateDataLookup returns file content for the given key from templateData, trying both
// the key as-is and with a "_template/" prefix. The loader uses unprefixed keys (relative to
// template root); artifact bundles and tests may use "_template/"-prefixed keys.
func (e *expressionEvaluator) templateDataLookup(key string) []byte {
	if e.templateData == nil || key == "" {
		return nil
	}
	if data := e.templateData[key]; data != nil {
		return data
	}
	return e.templateData["_template/"+key]
}

// lookupInTemplateData attempts to find file content in the template data map.
// It resolves the path argument relative to the facet file's directory, converting
// the facet path to a relative path from the template root if available. For paths
// that go up directories (../), it ensures the resolved path stays within the template
// root. Returns the file content if found, or nil if not present.
func (e *expressionEvaluator) lookupInTemplateData(pathArg string, facetPath string) []byte {
	if e.templateData == nil {
		return nil
	}

	pathArg = strings.TrimSpace(pathArg)
	if filepath.IsAbs(pathArg) {
		return nil
	}

	if facetPath == "" {
		return nil
	}

	if e.templateRoot == "" {
		return nil
	}

	facetAbsPath := facetPath
	if !filepath.IsAbs(facetPath) {
		facetAbsPath = filepath.Join(e.templateRoot, facetPath)
	}

	facetDir := filepath.Dir(facetAbsPath)
	resolvedAbsPath := filepath.Clean(filepath.Join(facetDir, pathArg))

	if relPath, err := filepath.Rel(e.templateRoot, resolvedAbsPath); err == nil && !strings.HasPrefix(relPath, "..") {
		resolvedRelPath := strings.ReplaceAll(relPath, "\\", "/")
		if resolvedRelPath == "." {
			resolvedRelPath = ""
		}
		return e.templateDataLookup(resolvedRelPath)
	}

	return nil
}

// resolvePath resolves a file path to an absolute, cleaned path using the path to the
// file being processed (sourceFilePath) when provided. If path is already absolute
// it is cleaned and returned. For relative paths: when sourceFilePath is non-empty
// it is the path to the facet or blueprint file containing the expression; the
// relative path is resolved from that file's directory. If sourceFilePath is relative
// it is resolved against templateRoot. If the result is still relative it is resolved
// against projectRoot. When sourceFilePath is empty (e.g. GenerateTfvars) relative
// paths fall back to projectRoot. The result is always an absolute path when
// projectRoot or templateRoot are set.
func (e *expressionEvaluator) resolvePath(path string, sourceFilePath string) string {
	path = strings.TrimSpace(path)

	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	if sourceFilePath != "" {
		baseDir := sourceFilePath
		if !filepath.IsAbs(baseDir) && e.templateRoot != "" {
			baseDir = filepath.Join(e.templateRoot, baseDir)
		}
		baseDir = filepath.Dir(baseDir)
		result := filepath.Clean(filepath.Join(baseDir, path))
		if !filepath.IsAbs(result) && e.projectRoot != "" {
			result = filepath.Clean(filepath.Join(e.projectRoot, result))
		}
		return result
	}

	var result string
	if e.projectRoot != "" && (strings.HasPrefix(path, "..") || strings.HasPrefix(filepath.Clean(path), "..")) {
		trimmed := strings.TrimPrefix(path, "../")
		trimmed = strings.TrimPrefix(trimmed, "./")
		result = filepath.Clean(filepath.Join(e.projectRoot, trimmed))
	} else if e.projectRoot != "" {
		result = filepath.Clean(filepath.Join(e.projectRoot, path))
	} else if e.templateRoot != "" {
		result = filepath.Clean(filepath.Join(e.templateRoot, path))
	} else {
		result = filepath.Clean(path)
	}
	return result
}

// evaluateCidrHostFunction calculates the IP address for a specific host number within a given CIDR block.
// The prefix parameter must be a valid CIDR notation (e.g., "10.5.0.0/16"), and hostnum specifies the
// host number within that network. Returns the IP address as a string (e.g., "10.5.0.10" for hostnum 10
// in "10.5.0.0/16"), or an error if the prefix is invalid or the host number is out of range.
func (e *expressionEvaluator) evaluateCidrHostFunction(prefix string, hostnum int) (string, error) {
	_, ipnet, err := net.ParseCIDR(prefix)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR prefix: %w", err)
	}

	ones, bits := ipnet.Mask.Size()
	hostBits := bits - ones

	if hostnum < 0 {
		return "", fmt.Errorf("host number %d is out of range (must be non-negative) for CIDR %s", hostnum, prefix)
	}

	ip := make(net.IP, len(ipnet.IP))
	copy(ip, ipnet.IP)

	if len(ip) == net.IPv4len {
		if hostnum > math.MaxUint32 {
			return "", fmt.Errorf("host number %d is out of range for IPv4 address", hostnum)
		}
		hostnumUint32 := uint32(hostnum)
		ipInt := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
		if ipInt > math.MaxUint32-hostnumUint32 {
			return "", fmt.Errorf("host number %d causes overflow for CIDR %s", hostnum, prefix)
		}
		if hostBits < 64 {
			maxHosts := 1 << hostBits
			if hostnum >= maxHosts {
				return "", fmt.Errorf("host number %d is out of range (0-%d) for CIDR %s", hostnum, maxHosts-1, prefix)
			}
		}
		ipInt += hostnumUint32
		ip[0] = byte(ipInt >> 24)
		ip[1] = byte(ipInt >> 16)
		ip[2] = byte(ipInt >> 8)
		ip[3] = byte(ipInt)
	} else {
		if hostnum < 0 {
			return "", fmt.Errorf("host number %d is out of range for IPv6 address", hostnum)
		}
		hostnum64 := uint64(hostnum)
		for i := len(ip) - 1; i >= 0 && hostnum64 > 0; i-- {
			val := uint64(ip[i]) + (hostnum64 & 0xff)
			hostnum64 >>= 8
			if val > 255 {
				hostnum64 += val >> 8
				val &= 0xff
			}
			ip[i] = byte(val)
		}
		if hostnum64 > 0 {
			return "", fmt.Errorf("host number %d is too large for CIDR %s", hostnum, prefix)
		}
	}

	if !ipnet.Contains(ip) {
		return "", fmt.Errorf("host number %d produces IP %s which is outside CIDR %s", hostnum, ip.String(), prefix)
	}

	return ip.String(), nil
}

// evaluateCidrSubnetFunction calculates a subnet address within a given CIDR block by adding new bits
// to the prefix and specifying the subnet number. The prefix parameter must be a valid CIDR notation,
// newbits specifies how many additional bits to add for subnetting, and netnum is the subnet number.
// Returns the new subnet CIDR as a string (e.g., "10.5.1.0/24" for cidrsubnet("10.5.0.0/16", 8, 1)),
// or an error if the prefix is invalid or the subnet parameters are out of range.
func (e *expressionEvaluator) evaluateCidrSubnetFunction(prefix string, newbits int, netnum int) (string, error) {
	_, ipnet, err := net.ParseCIDR(prefix)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR prefix: %w", err)
	}

	ones, bits := ipnet.Mask.Size()
	if newbits < 0 || ones+newbits > bits {
		return "", fmt.Errorf("newbits %d is invalid for CIDR %s (prefix length %d, total bits %d)", newbits, prefix, ones, bits)
	}

	if netnum < 0 {
		return "", fmt.Errorf("netnum %d is out of range (must be non-negative) for newbits %d", netnum, newbits)
	}

	newPrefixLen := ones + newbits
	subnetSize := 1 << (bits - newPrefixLen)
	subnetIP := make(net.IP, len(ipnet.IP))
	copy(subnetIP, ipnet.IP)

	offset := netnum * subnetSize

	if len(subnetIP) == net.IPv4len {
		if offset > math.MaxUint32 {
			return "", fmt.Errorf("offset %d is out of range for IPv4 address", offset)
		}
		offsetUint32 := uint32(offset)
		ipInt := uint32(subnetIP[0])<<24 | uint32(subnetIP[1])<<16 | uint32(subnetIP[2])<<8 | uint32(subnetIP[3])
		if ipInt > math.MaxUint32-offsetUint32 {
			return "", fmt.Errorf("offset %d causes overflow for CIDR %s", offset, prefix)
		}
		if newbits < 64 {
			maxSubnets := 1 << newbits
			if netnum >= maxSubnets {
				return "", fmt.Errorf("netnum %d is out of range (0-%d) for newbits %d", netnum, maxSubnets-1, newbits)
			}
		}
		ipInt += offsetUint32
		subnetIP[0] = byte(ipInt >> 24)
		subnetIP[1] = byte(ipInt >> 16)
		subnetIP[2] = byte(ipInt >> 8)
		subnetIP[3] = byte(ipInt)
	} else {
		if offset < 0 {
			return "", fmt.Errorf("offset %d is out of range for IPv6 address", offset)
		}
		offset64 := uint64(offset)
		for i := len(subnetIP) - 1; i >= 0 && offset64 > 0; i-- {
			val := uint64(subnetIP[i]) + (offset64 & 0xff)
			offset64 >>= 8
			if val > 255 {
				offset64 += val >> 8
				val &= 0xff
			}
			subnetIP[i] = byte(val)
		}
		if offset64 > 0 {
			return "", fmt.Errorf("offset %d is too large for CIDR %s", offset, prefix)
		}
	}

	mask := net.CIDRMask(newPrefixLen, bits)
	subnetIP.Mask(mask)

	return fmt.Sprintf("%s/%d", subnetIP.String(), newPrefixLen), nil
}

// evaluateCidrSubnetsFunction generates multiple subnets from a single CIDR block by specifying
// multiple newbits values. The prefix parameter must be a valid CIDR notation, and newbits is
// a slice of integers specifying how many additional bits to add for each subnet. Returns a
// slice of subnet CIDR strings (e.g., ["10.5.0.0/24", "10.5.1.0/24"] for cidrsubnets("10.5.0.0/16", 8, 8)),
// or an error if the prefix is invalid or any subnet parameters are out of range.
func (e *expressionEvaluator) evaluateCidrSubnetsFunction(prefix string, newbits []int) ([]any, error) {
	_, ipnet, err := net.ParseCIDR(prefix)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR prefix: %w", err)
	}

	ones, bits := ipnet.Mask.Size()
	results := make([]any, len(newbits))
	baseIP := make(net.IP, len(ipnet.IP))
	copy(baseIP, ipnet.IP)
	cumulativeOffset := 0

	for i, nb := range newbits {
		if nb < 0 || ones+nb > bits {
			return nil, fmt.Errorf("newbits[%d]=%d is invalid for CIDR %s (prefix length %d, total bits %d)", i, nb, prefix, ones, bits)
		}

		newPrefixLen := ones + nb
		subnetSize := 1 << (bits - newPrefixLen)
		offset := cumulativeOffset

		subnetIP := make(net.IP, len(baseIP))
		copy(subnetIP, baseIP)

		if len(subnetIP) == net.IPv4len {
			if offset < 0 || offset > math.MaxUint32 {
				return nil, fmt.Errorf("offset %d is out of range for IPv4 address", offset)
			}
			offsetUint32 := uint32(offset)
			ipInt := uint32(subnetIP[0])<<24 | uint32(subnetIP[1])<<16 | uint32(subnetIP[2])<<8 | uint32(subnetIP[3])
			if ipInt > math.MaxUint32-offsetUint32 {
				return nil, fmt.Errorf("offset %d causes overflow for CIDR %s", offset, prefix)
			}
			ipInt += offsetUint32
			subnetIP[0] = byte(ipInt >> 24)
			subnetIP[1] = byte(ipInt >> 16)
			subnetIP[2] = byte(ipInt >> 8)
			subnetIP[3] = byte(ipInt)
		} else {
			if offset < 0 {
				return nil, fmt.Errorf("offset %d is out of range for IPv6 address", offset)
			}
			offset64 := uint64(offset)
			for j := len(subnetIP) - 1; j >= 0 && offset64 > 0; j-- {
				val := uint64(subnetIP[j]) + (offset64 & 0xff)
				offset64 >>= 8
				if val > 255 {
					offset64 += val >> 8
					val &= 0xff
				}
				subnetIP[j] = byte(val)
			}
			if offset64 > 0 {
				return nil, fmt.Errorf("offset %d is too large for CIDR %s", offset, prefix)
			}
		}

		mask := net.CIDRMask(newPrefixLen, bits)
		subnetIP.Mask(mask)

		results[i] = fmt.Sprintf("%s/%d", subnetIP.String(), newPrefixLen)

		cumulativeOffset += subnetSize
	}

	return results, nil
}

// evaluateCidrNetmaskFunction converts a CIDR prefix to a subnet mask in dotted-decimal notation.
// The prefix parameter must be a valid CIDR notation (e.g., "10.5.0.0/16"). Returns the subnet
// mask as a string (e.g., "255.255.0.0" for "/16"), or an error if the prefix is invalid.
func (e *expressionEvaluator) evaluateCidrNetmaskFunction(prefix string) (string, error) {
	_, ipnet, err := net.ParseCIDR(prefix)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR prefix: %w", err)
	}

	mask := ipnet.Mask
	if len(mask) == net.IPv4len {
		return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3]), nil
	}

	return "", fmt.Errorf("CIDR prefix must be IPv4, got IPv6")
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure expressionEvaluator implements the ExpressionEvaluator and HelperRegistrar interfaces
var _ ExpressionEvaluator = (*expressionEvaluator)(nil)
var _ HelperRegistrar = (*expressionEvaluator)(nil)

// =============================================================================
// Helper Functions
// =============================================================================

// valueToInterpolationString converts an expression result to a string for embedding in
// interpolated output. Maps and slices (any kind, including map[interface{}]interface{} from
// expr/YAML) are serialized as YAML so yamlString() output and facet templates get valid YAML.
func valueToInterpolationString(value any, yamlMarshal func(any) ([]byte, error)) (string, error) {
	if value == nil {
		return "", nil
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Map, reflect.Slice:
		yamlData, err := yamlMarshal(value)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(yamlData)), nil
	default:
		return fmt.Sprintf("%v", value), nil
	}
}

// indentForEmbeddedYAML returns replacement with each line indented so that when placed after
// "before" (the string up to the expression), the embedded YAML is valid and readable.
// baseIndent is the number of spaces at the start of the current line (from before).
func indentForEmbeddedYAML(before string, replacement string, extraIndent int) string {
	if replacement == "" || !strings.Contains(replacement, "\n") {
		return replacement
	}
	lineStart := strings.LastIndex(before, "\n")
	baseIndent := 0
	if lineStart >= 0 {
		for i := lineStart + 1; i < len(before) && before[i] == ' '; i++ {
			baseIndent++
		}
	}
	indent := baseIndent + extraIndent
	indentStr := strings.Repeat(" ", indent)
	lines := strings.Split(replacement, "\n")
	for i := range lines {
		if lines[i] != "" {
			lines[i] = indentStr + lines[i]
		}
	}
	return "\n" + strings.Join(lines, "\n")
}

// findExpressionEnd returns the index of the '}' that matches the '{' in "${" at start.
// It tracks brace depth and skips string literals so that "}" and '}' inside strings are ignored.
// start is the index of '$' in "${". Returns the index of the matching '}', or -1 if not found.
func findExpressionEnd(s string, start int) int {
	if start < 0 || start+2 > len(s) || s[start] != '$' || s[start+1] != '{' {
		return -1
	}
	depth := 1
	i := start + 2
	for i < len(s) {
		c := s[i]
		switch c {
		case '"':
			i++
			for i < len(s) {
				if s[i] == '\\' {
					i += 2
					continue
				}
				if s[i] == '"' {
					i++
					break
				}
				i++
			}
			continue
		case '\'':
			i++
			for i < len(s) {
				if s[i] == '\\' {
					i += 2
					continue
				}
				if s[i] == '\'' {
					i++
					break
				}
				i++
			}
			continue
		case '{':
			depth++
			i++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
			i++
		default:
			i++
		}
	}
	return -1
}

// isStructuredValue returns true when the value is a list or map ([]any, map[string]any,
// or reflection-equivalent types like []interface{} from expr/YAML). Used so that when
// evaluation produces a structured value (e.g. from yaml()), we keep it even if it contains
// nested expressions, instead of reverting to the original string.
func isStructuredValue(value any) bool {
	if value == nil {
		return false
	}
	switch value.(type) {
	case []any, map[string]any:
		return true
	}
	switch reflect.ValueOf(value).Kind() {
	case reflect.Slice, reflect.Map:
		return true
	}
	return false
}

// ContainsExpression determines whether the provided value is a string that contains an unresolved expression.
// An expression is identified by the pattern "${...}" anywhere in the string. This function returns true if
// the value is a string containing at least one properly closed "${...}" expression pattern, and false otherwise.
// Used to identify values containing unresolved expressions that should be skipped when evaluateDeferred is false.
func ContainsExpression(value any) bool {
	switch v := value.(type) {
	case string:
		if !strings.Contains(v, "${") {
			return false
		}
		start := strings.Index(v, "${")
		if start == -1 {
			return false
		}
		return findExpressionEnd(v, start) != -1
	case map[string]any:
		for _, val := range v {
			if ContainsExpression(val) {
				return true
			}
		}
		return false
	case []any:
		for _, item := range v {
			if ContainsExpression(item) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
