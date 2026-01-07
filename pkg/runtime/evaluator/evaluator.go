// The ExpressionEvaluator provides unified expression evaluation for the Runtime system.
// It supplies expression evaluation using the Runtime's config handler, supporting features such as
// jsonnet, file loading, and string interpolation. All expression evaluation is performed through
// the provider system, enabling on-demand resolution of dynamic values.

package evaluator

import (
	"fmt"
	"maps"
	"path/filepath"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/google/go-jsonnet"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

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
	Evaluate(expression string, featurePath string, evaluateDeferred bool) (any, error)
	EvaluateMap(values map[string]any, featurePath string, evaluateDeferred bool) (map[string]any, error)
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
// The featurePath parameter is used for relative expression resolution, and evaluateDeferred controls
// whether to process unresolved expressions immediately. Returns the fully evaluated value, or an error if
// evaluation fails or the input is malformed. If s is empty, an error is returned. If no evaluation is triggered,
// or if the result is nil (such as from an undefined variable), the original string s is returned as-is.
func (e *expressionEvaluator) Evaluate(s string, featurePath string, evaluateDeferred bool) (any, error) {
	if s == "" {
		return nil, fmt.Errorf("expression cannot be empty")
	}
	if strings.Contains(s, "${") {
		if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") && strings.Count(s, "${") == 1 && strings.Count(s, "}") == 1 {
			expr := s[2 : len(s)-1]
			return e.evaluateExpression(expr, featurePath, evaluateDeferred)
		}
		result := s
		for strings.Contains(result, "${") {
			start := strings.Index(result, "${")
			end := strings.Index(result[start:], "}")
			if end == -1 {
				return "", fmt.Errorf("unclosed expression in string: %s", s)
			}
			end += start
			expr := result[start+2 : end]
			value, err := e.evaluateExpression(expr, featurePath, evaluateDeferred)
			if err != nil {
				return "", fmt.Errorf("failed to evaluate expression '${%s}': %w", expr, err)
			}
			var replacement string
			if value == nil {
				replacement = ""
			} else {
				switch value.(type) {
				case map[string]any, []any:
					yamlData, err := e.Shims.YamlMarshal(value)
					if err != nil {
						return "", fmt.Errorf("failed to marshal expression result to YAML: %w", err)
					}
					replacement = strings.TrimSpace(string(yamlData))
				default:
					replacement = fmt.Sprintf("%v", value)
				}
			}
			result = result[:start] + replacement + result[end+1:]
			if !evaluateDeferred && ContainsExpression(replacement) {
				break
			}
		}
		return result, nil
	}
	result, err := e.evaluateExpression(s, featurePath, evaluateDeferred)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return s, nil
	}
	return result, nil
}

// evaluateExpression compiles and evaluates a single expression string using the expressionEvaluator environment.
// The expression should not include ${} bookends. Configures the evaluation context with Windsor-specific helpers
// and config enrichment. If evaluateDeferred is false, and the result is a string matching the pattern for
// terraform_output(), the result is returned as an unresolved expression. Returns the evaluation result or an error
// if compilation or execution fails.
func (e *expressionEvaluator) evaluateExpression(expression string, featurePath string, evaluateDeferred bool) (any, error) {
	config := e.getConfig()
	enrichedConfig := e.enrichConfig(config)
	env := e.buildExprEnvironment(enrichedConfig, featurePath, evaluateDeferred)
	program, err := expr.Compile(expression, env...)
	if err != nil {
		return nil, fmt.Errorf("failed to compile expression '%s': %w", expression, err)
	}
	result, err := expr.Run(program, enrichedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate expression '%s': %w", expression, err)
	}
	if !evaluateDeferred {
		if str, isString := result.(string); isString && strings.HasPrefix(str, "terraform_output(") && strings.HasSuffix(str, ")") {
			return fmt.Sprintf("${%s}", str), nil
		}
	}
	return result, nil
}

// =============================================================================
// Private Methods
// =============================================================================

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
// loading raw file content, and split() for string splitting. Helper functions registered
// via Register() are also included. Each helper includes argument validation and
// type checking. The config and featurePath are captured in closures to provide context
// for file resolution during expression evaluation. It also enables AsAny mode to allow
// dynamic property access.
func (e *expressionEvaluator) buildExprEnvironment(config map[string]any, featurePath string, deferred bool) []expr.Option {
	opts := []expr.Option{
		expr.AsAny(),
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
				return e.evaluateJsonnetFunction(path, config, featurePath)
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
				return e.evaluateFileFunction(path, featurePath)
			},
			new(func(string) string),
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
	}

	for _, helperFactory := range e.helpers {
		opts = append(opts, helperFactory(deferred))
	}

	return opts
}

// EvaluateMap evaluates a map of values using this expression evaluator.
// Each string value is evaluated as an expression; non-string values are preserved as-is.
// When evaluateDeferred is false, evaluated values that contain unresolved expressions are
// skipped and not included in the result. The featurePath parameter is used for context
// during evaluation. Returns a new map containing successfully evaluated values, or an error
// if evaluation fails.
func (e *expressionEvaluator) EvaluateMap(values map[string]any, featurePath string, evaluateDeferred bool) (map[string]any, error) {
	result := make(map[string]any)
	for key, value := range values {
		strVal, isString := value.(string)
		if !isString {
			result[key] = value
			continue
		}
		evaluated, err := e.Evaluate(strVal, featurePath, evaluateDeferred)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate '%s': %w", key, err)
		}
		if !evaluateDeferred {
			if ContainsExpression(evaluated) {
				continue
			}
		}
		result[key] = evaluated
	}
	return result, nil
}

// evaluateJsonnetFunction loads and evaluates a Jsonnet file at the specified path.
// It first attempts to load the file from template data if available, falling back to
// the filesystem if not found. The config is enriched with context information and
// passed to the Jsonnet VM as external code. Helper functions and import paths are
// configured, then the Jsonnet is evaluated and the JSON output is unmarshaled.
// Returns the evaluated value or an error if loading, evaluation, or parsing fails.
func (e *expressionEvaluator) evaluateJsonnetFunction(pathArg string, config map[string]any, featurePath string) (any, error) {
	var content []byte
	var err error

	if e.templateData != nil {
		content = e.lookupInTemplateData(pathArg, featurePath)
	}

	var path string
	if content == nil {
		path = e.resolvePath(pathArg, featurePath)
		content, err = e.Shims.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", path, err)
		}
	} else {
		if e.templateRoot != "" && featurePath != "" {
			featureAbsPath := featurePath
			if !filepath.IsAbs(featurePath) {
				featureAbsPath = filepath.Join(e.templateRoot, featurePath)
			}
			featureDir := filepath.Dir(featureAbsPath)
			resolvedAbsPath := filepath.Clean(filepath.Join(featureDir, pathArg))
			if relPath, err := filepath.Rel(e.templateRoot, resolvedAbsPath); err == nil && !strings.HasPrefix(relPath, "..") {
				path = filepath.Join(e.templateRoot, relPath)
			} else {
				path = resolvedAbsPath
			}
		} else {
			path = e.resolvePath(pathArg, featurePath)
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
	vm.ExtCode("ociUrl", fmt.Sprintf("%q", constants.GetEffectiveBlueprintURL()))

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
// to the featurePath or project root as appropriate. Returns the file content as a
// string or an error if the file cannot be found or read.
func (e *expressionEvaluator) evaluateFileFunction(pathArg string, featurePath string) (any, error) {
	var content []byte
	var err error

	if e.templateData != nil {
		content = e.lookupInTemplateData(pathArg, featurePath)
	}

	if content == nil {
		path := e.resolvePath(pathArg, featurePath)
		content, err = e.Shims.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", path, err)
		}
	}

	return string(content), nil
}

// lookupInTemplateData attempts to find file content in the template data map.
// It resolves the path argument relative to the feature file's directory, converting
// the feature path to a relative path from the template root if available. The resolved
// path is used to look up the file in template data, checking both with and without
// the "_template/" prefix. For paths that go up directories (../), it ensures the
// resolved path stays within the template root. Returns the file content if found, or nil if not present.
func (e *expressionEvaluator) lookupInTemplateData(pathArg string, featurePath string) []byte {
	if e.templateData == nil {
		return nil
	}

	pathArg = strings.TrimSpace(pathArg)
	if filepath.IsAbs(pathArg) {
		return nil
	}

	if featurePath == "" {
		return nil
	}

	if e.templateRoot == "" {
		return nil
	}

	featureAbsPath := featurePath
	if !filepath.IsAbs(featurePath) {
		featureAbsPath = filepath.Join(e.templateRoot, featurePath)
	}

	featureDir := filepath.Dir(featureAbsPath)
	resolvedAbsPath := filepath.Clean(filepath.Join(featureDir, pathArg))

	if relPath, err := filepath.Rel(e.templateRoot, resolvedAbsPath); err == nil && !strings.HasPrefix(relPath, "..") {
		resolvedRelPath := strings.ReplaceAll(relPath, "\\", "/")
		if resolvedRelPath == "." {
			resolvedRelPath = ""
		}

		if data, exists := e.templateData["_template/"+resolvedRelPath]; exists {
			return data
		}
		if data, exists := e.templateData[resolvedRelPath]; exists {
			return data
		}
	}

	return nil
}

// resolvePath resolves a file path to an absolute, cleaned path.
// If the path is already absolute, it is cleaned and returned. For relative paths,
// it first tries to resolve relative to the featurePath's directory if provided.
// Otherwise, it falls back to the project root if set. The result is always an
// absolute path with normalized separators and cleaned of redundant elements.
func (e *expressionEvaluator) resolvePath(path string, featurePath string) string {
	path = strings.TrimSpace(path)

	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	if featurePath != "" {
		featureDir := filepath.Dir(featurePath)
		return filepath.Clean(filepath.Join(featureDir, path))
	}

	if e.projectRoot != "" {
		return filepath.Clean(filepath.Join(e.projectRoot, path))
	}

	return filepath.Clean(path)
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

// ContainsExpression determines whether the provided value is a string that represents an expression.
// An expression string is defined as any string starting with "${" and ending with "}", such as "${foo.bar}".
// This function returns true if the value is a string wrapped in "${...}" and false otherwise.
// If the expression is malformed (e.g., a missing closing brace), the function also returns false.
// Used to identify values containing expressions that should be evaluated or skipped if not fully resolved.
func ContainsExpression(value any) bool {
	str, isString := value.(string)
	if !isString {
		return false
	}
	if !strings.HasPrefix(str, "${") {
		return false
	}
	if !strings.HasSuffix(str, "}") {
		return false
	}
	if strings.Count(str, "${") != strings.Count(str, "}") {
		return false
	}
	return true
}
