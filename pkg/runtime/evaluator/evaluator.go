// The ExpressionEvaluator is the resolver for ${...} expressions in Windsor configuration.
// It provides evaluation of single expressions, interpolated strings, and nested maps and lists.
// It reads values from the context config, or from a scope that the caller supplies.
// It keeps a deferred expression, such as a terraform_output() call, for a later evaluation.

package evaluator

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/config"
	secretsRuntime "github.com/windsorcli/cli/pkg/runtime/secrets"
)

// =============================================================================
// Types
// =============================================================================

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

// expressionEvaluator is the concrete implementation of ExpressionEvaluator.
type expressionEvaluator struct {
	configHandler config.ConfigHandler
	projectRoot   string
	templateRoot  string
	Shims         *Shims
	templateData  map[string][]byte
	helpers       []func(allowDeferred bool) expr.Option
	configScope   map[string]any
}

// =============================================================================
// Errors
// =============================================================================

// DeferredError signals that an expression is deferred and should be preserved for later evaluation.
// This is not an error condition but a signal that the expression cannot be evaluated at this time.
type DeferredError = secretsRuntime.DeferredError

// DeferredValue represents a value that could not be fully resolved because one or more
// deferred helpers (for example terraform_output) were unavailable at evaluation time.
// Expression stores the raw expression text preserved for later resolution.
type DeferredValue struct {
	Expression string
}

// IsDeferredValue reports whether value is a DeferredValue (value or pointer form).
func IsDeferredValue(value any) bool {
	switch value.(type) {
	case DeferredValue, *DeferredValue:
		return true
	default:
		return false
	}
}

// DeferredExpression returns the preserved raw expression string when value is DeferredValue.
func DeferredExpression(value any) (string, bool) {
	switch v := value.(type) {
	case DeferredValue:
		return v.Expression, true
	case *DeferredValue:
		if v == nil {
			return "", false
		}
		return v.Expression, true
	default:
		return "", false
	}
}

// ExpressionBody returns the inner expression for ${ ... } inputs.
// Double-brace ${{ ... }} inputs are not matched; callers should normalize those first.
// When value is not wrapped as a full expression, returns the original trimmed string and false.
func ExpressionBody(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "${{") {
		return trimmed, false
	}
	if strings.HasPrefix(trimmed, "${") && strings.HasSuffix(trimmed, "}") {
		return strings.TrimSpace(trimmed[2 : len(trimmed)-1]), true
	}
	return trimmed, false
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
	SetEnvLookup(lookup func(name string) (string, bool))
	SetConfigScope(scope map[string]any)
	Evaluate(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error)
	EvaluateMap(values map[string]any, facetPath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error)
}

// HelperRegistrar defines the interface for registering custom helpers with the evaluator.
// This allows providers to extend expression evaluation capabilities without the evaluator
// needing to know about provider-specific functionality.
type HelperRegistrar interface {
	Register(name string, helper func(params []any, deferred bool) (any, error), signatures ...any)
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

// SetEnvLookup replaces the source that env() expressions resolve against.
// The test runner uses this to supply a hermetic environment so composition never reads the host env.
func (e *expressionEvaluator) SetEnvLookup(lookup func(name string) (string, bool)) {
	e.Shims.LookupEnv = lookup
}

// SetConfigScope publishes the composed scope (facet config blocks merged over context config) that a
// nil-scope evaluation should use as its base. The composer calls this after composition so apply-time
// consumers — secret resolution in particular — dereference computed config values the same way
// substitutions do. Passing nil clears it, reverting nil-scope evaluations to context config alone.
func (e *expressionEvaluator) SetConfigScope(scope map[string]any) {
	e.configScope = scope
}

// Register adds a custom helper function to the evaluator for expression evaluation.
// The provided helper function receives the deferred flag as a closure variable. The name parameter
// specifies the function name used in expressions, helper is the custom function, and signatures define
// the helper's input/output signatures. Pass one signature per accepted argument count. This allows providers and consumers to
// extend evaluator capabilities with domain-specific helpers.
func (e *expressionEvaluator) Register(name string, helper func(params []any, deferred bool) (any, error), signatures ...any) {
	e.helpers = append(e.helpers, func(deferred bool) expr.Option {
		wrappedHelper := func(params ...any) (any, error) {
			return helper(params, deferred)
		}
		return expr.Function(name, wrappedHelper, signatures...)
	})
}

// Evaluate resolves Windsor expressions in the provided string s, supporting both complete and interpolated
// (${...}) expressions, arithmetic operations, and complex object types. The evaluation will process dynamic
// file and jsonnet loading as needed, and can defer unresolved expressions when evaluateDeferred is false.
// The facetPath parameter is used for relative expression resolution. When scope is nil, the environment is
// enriched config (getConfig plus context, project_root, etc.). When scope is non-nil, the environment is
// that scope (with runtime keys injected so helpers have context and paths). Returns the fully evaluated value,
// or an error if evaluation fails or the input is malformed.
func (e *expressionEvaluator) Evaluate(s string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
	return e.evaluate(s, facetPath, scope, evaluateDeferred)
}

// EvaluateMap evaluates a map of values using this expression evaluator. Each string value is evaluated as an
// expression; maps and arrays are recursively evaluated. When evaluateDeferred is false and evaluation fails
// with a DeferredError, the original value is preserved in the result. Environment is determined by scope the
// same way as Evaluate (nil = enriched config, non-nil = that scope with runtime keys). Returns a new map
// containing evaluated values, or an error if evaluation fails with a non-deferred error.
func (e *expressionEvaluator) EvaluateMap(values map[string]any, facetPath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
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
			return nil, werror.Wrap(err, "failed to evaluate '%s'", key)
		}
		result[key] = evaluated
	}
	return result, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// evaluate runs the expression parsing loop over s, resolving each ${...} with evaluateExpression.
// When scope is nil, the config context applies; a non-nil scope overrides it (e.g. yaml(path, input)).
// The loop stops after 20 iterations to avoid infinite loops. A DeferredError leaves its ${...} in
// place and advances to the next one, so sibling expressions can still resolve.
//
// A result still containing an unresolved ${...} never returns as if it were fully resolved: it
// wraps as a DeferredValue when evaluateDeferred is false, or errors when evaluateDeferred is true.
func (e *expressionEvaluator) evaluate(s string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
	if !strings.Contains(s, "${") {
		return s, nil
	}
	result := s
	deferredEncountered := false
	nestedResultRescan := false
	searchStart := 0
	for iter := 0; iter < 20; iter++ {
		idx := strings.Index(result[searchStart:], "${")
		if idx == -1 {
			break
		}
		start := searchStart + idx
		end := findExpressionEnd(result, start)
		if end == -1 {
			if nestedResultRescan {
				return "", fmt.Errorf("a nested expression's result still contains an unresolved expression that could not be re-parsed for further evaluation (likely an embedded reference that is not yet resolved): %.300s", result)
			}
			return "", werror.New(fmt.Sprintf("The string %.300q has an unclosed expression.", s),
				"Add the closing } to each ${ in the string.", nil)
		}
		expr := result[start+2 : end]
		if expr == "" {
			return nil, werror.New(fmt.Sprintf("The string %.300q has an empty expression.", s),
				"Put an expression between ${ and }, or remove the ${}.", nil)
		}
		value, err := e.evaluateExpression(expr, facetPath, scope, evaluateDeferred)
		if err != nil {
			if !evaluateDeferred {
				var deferredErr *DeferredError
				if errors.As(err, &deferredErr) {
					deferredEncountered = true
					searchStart = end + 1
					continue
				}
			}
			return "", werror.Wrap(err, "failed to evaluate expression '${%s}'", expr)
		}
		before := result[:start]
		after := result[end+1:]
		singleExpr := strings.TrimSpace(before) == "" && strings.TrimSpace(after) == ""
		if singleExpr {
			if str, ok := value.(string); ok && ContainsExpression(str) {
				if str == result {
					if !evaluateDeferred {
						return DeferredValue{Expression: str}, nil
					}
					return "", fmt.Errorf("expression '${%s}' resolved to a value that still contains an unresolved expression and cannot be resolved further: %.300s", expr, str)
				}
				result = str
				nestedResultRescan = true
				searchStart = 0
				continue
			}
			if isStructuredValue(value) {
				if marshaled, err := valueToInterpolationString(value, e.Shims.YamlMarshal); err == nil && ContainsExpression(marshaled) {
					if !evaluateDeferred {
						return DeferredValue{Expression: result}, nil
					}
					return "", fmt.Errorf("expression '${%s}' resolved to a value that still contains an unresolved expression and cannot be resolved further: %.300s", expr, marshaled)
				}
			}
			return value, nil
		}
		var replacement string
		if value != nil {
			var err error
			replacement, err = valueToInterpolationString(value, e.Shims.YamlMarshal)
			if err != nil {
				return "", werror.Wrap(err, "failed to marshal expression result to YAML")
			}
			if ContainsExpression(replacement) && isStructuredValue(value) {
				if !evaluateDeferred {
					return DeferredValue{Expression: result}, nil
				}
				return "", fmt.Errorf("expression '${%s}' resolved to a value that still contains an unresolved expression and cannot be resolved further: %.300s", expr, replacement)
			}
			replacement = indentForEmbeddedYAML(before, replacement, 2)
		}
		result = before + replacement + after
		searchStart = 0
	}
	if !evaluateDeferred && (deferredEncountered || ContainsExpression(result)) {
		return DeferredValue{Expression: result}, nil
	}
	if evaluateDeferred && ContainsExpression(result) {
		return "", fmt.Errorf("expression '%s' resolved to a value that still contains an unresolved expression and cannot be resolved further: %.300s", s, result)
	}
	return result, nil
}

// evaluateExpression compiles and evaluates a single expression. The environment base is chosen by
// precedence: an explicit non-nil scope wins; otherwise the composer-published configScope (which
// includes facet config blocks, so nil-scope consumers like secret resolution see computed config);
// otherwise enriched context config alone (pre-composition, or when no composed scope was published).
// Runtime keys (context, project_root, etc.) are injected so helpers like file() and jsonnet() have
// context and paths. The expression should not include ${} bookends. Returns the result or an error.
func (e *expressionEvaluator) evaluateExpression(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
	if rewritten, ok := secretsRuntime.NormalizeExpression(expression); ok {
		expression = rewritten
	}

	base := scope
	if base == nil {
		base = e.configScope
	}
	var merged map[string]any
	if base != nil {
		merged = make(map[string]any)
		maps.Copy(merged, base)
		injectRuntimeKeys(merged, e)
	} else {
		merged = e.enrichConfig(e.getConfig())
	}
	env := e.buildExprEnvironment(merged, facetPath, evaluateDeferred)
	program, err := expr.Compile(expression, env...)
	if err != nil {
		return nil, werror.New(fmt.Sprintf("Windsor cannot parse the expression ${%s}: %v", expression, err),
			"Correct the syntax of the expression.", err)
	}
	result, err := expr.Run(program, merged)
	if err != nil {
		var deferredErr *DeferredError
		if errors.As(err, &deferredErr) {
			return nil, deferredErr
		}
		return nil, werror.Wrap(err, "failed to evaluate expression '%s'", expression)
	}
	return result, nil
}

// getConfig retrieves the current configuration context as a map of string keys to values.
// If GetContextValues returns nil, an empty map is returned. Never returns a nil map.
func (e *expressionEvaluator) getConfig() map[string]any {
	config, _ := e.configHandler.GetContextValues()
	if config == nil {
		return make(map[string]any)
	}
	return config
}

// enrichConfig enriches the provided config map with Windsor runtime-specific values.
// It adds "context" (current context name), "context_path" if the config handler can provide one,
// "project_root" and "projectName" (basename of project root) when the evaluator has a project root set.
// All paths are normalized to use forward slashes for cross-platform consistency. The enriched config
// is used to make runtime context available in expression evaluation and Jsonnet (std.extVar("context"))
// without requiring explicit configuration.
func (e *expressionEvaluator) enrichConfig(config map[string]any) map[string]any {
	enrichedConfig := make(map[string]any)
	maps.Copy(enrichedConfig, config)
	injectRuntimeKeys(enrichedConfig, e)
	return enrichedConfig
}

// injectRuntimeKeys writes context, context_path, project_root, and projectName into the map so
// helpers (file(), jsonnet()) and expressions have runtime context when scope is used as the environment.
func injectRuntimeKeys(m map[string]any, e *expressionEvaluator) {
	m["context"] = e.configHandler.GetContext()
	if configRoot, err := e.configHandler.GetConfigRoot(); err == nil {
		m["context_path"] = strings.ReplaceAll(configRoot, "\\", "/")
	}
	if e.projectRoot != "" {
		m["project_root"] = strings.ReplaceAll(e.projectRoot, "\\", "/")
		m["projectName"] = filepath.Base(e.projectRoot)
	}
}

// buildExprEnvironment returns the expr options for one evaluation: AsAny mode, optional member
// chains, the built-in functions, and the helpers that Register added. The config and facetPath
// go to the functions that load files, so that relative paths resolve from the facet file.
func (e *expressionEvaluator) buildExprEnvironment(config map[string]any, facetPath string, deferred bool) []expr.Option {
	opts := []expr.Option{
		expr.AsAny(),
		expr.Patch(&optionalChainPatcher{}),
		e.stringFunction(),
		e.jsonnetFunction(config, facetPath),
		e.fileFunction(facetPath),
		e.envFunction(),
		e.yamlFunction(facetPath),
		e.yamlStringFunction(facetPath),
		e.jsonStringFunction(),
		e.splitFunction(),
		e.cidrHostFunction(),
		e.cidrSubnetFunction(),
		e.cidrSubnetsFunction(),
		e.cidrNetmaskFunction(),
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

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure expressionEvaluator implements the ExpressionEvaluator and HelperRegistrar interfaces
var _ ExpressionEvaluator = (*expressionEvaluator)(nil)
var _ HelperRegistrar = (*expressionEvaluator)(nil)

// =============================================================================
// Helpers
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
	case DeferredValue:
		return ContainsExpression(v.Expression)
	case *DeferredValue:
		if v == nil {
			return false
		}
		return ContainsExpression(v.Expression)
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
