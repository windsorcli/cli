// The ExpressionEvaluator provides unified expression evaluation for the Runtime system.
// It provides a single expression evaluator that uses the Runtime's config handler with providers
// for all expression evaluation, enabling on-demand resolution of dynamic values like terraform.*
// through the provider system. The evaluator supports all existing expression features including
// jsonnet, file loading, and string interpolation.

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
	helpers       []expr.Option
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
	Evaluate(expression string, config map[string]any, featurePath string) (any, error)
	EvaluateDefaults(defaults map[string]any, config map[string]any, featurePath string) (map[string]any, error)
	InterpolateString(s string, config map[string]any, featurePath string) (string, error)
}

// HelperRegistrar defines the interface for registering custom helpers with the evaluator.
// This allows providers to extend expression evaluation capabilities without the evaluator
// needing to know about provider-specific functionality.
type HelperRegistrar interface {
	Register(name string, helper func(params ...any) (any, error), signature any)
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
		helpers:       []expr.Option{},
	}
}

// SetTemplateData sets the template data map for in-memory file resolution.
// This is used when loading blueprints from OCI artifacts where template files
// are stored in memory rather than on disk. The map keys should use paths
// relative to the template root, typically prefixed with "_template/".
func (e *expressionEvaluator) SetTemplateData(templateData map[string][]byte) {
	e.templateData = templateData
}

// Register registers a custom helper with the evaluator.
// This allows providers to extend expression evaluation capabilities without
// the evaluator needing to know about provider-specific functionality.
func (e *expressionEvaluator) Register(name string, helper func(params ...any) (any, error), signature any) {
	e.helpers = append(e.helpers, expr.Function(name, helper, signature))
}

// =============================================================================
// Public Methods
// =============================================================================

// Evaluate evaluates a single expression and returns the result as any type.
// The expression can include arithmetic operations, string manipulation, array construction,
// and nested object access. It also supports file loading functions like jsonnet("path")
// and file("path") for dynamic content loading. The config map provides values accessible
// in the expression, and featurePath is used to resolve relative paths in file functions.
// Returns the evaluated value or an error if compilation or evaluation fails.
func (e *expressionEvaluator) Evaluate(expression string, config map[string]any, featurePath string) (any, error) {
	if expression == "" {
		return nil, fmt.Errorf("expression cannot be empty")
	}

	enrichedConfig := e.enrichConfig(config)

	env := e.buildExprEnvironment(enrichedConfig, featurePath)

	program, err := expr.Compile(expression, env...)
	if err != nil {
		return nil, fmt.Errorf("failed to compile expression '%s': %w", expression, err)
	}

	result, err := expr.Run(program, enrichedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate expression '%s': %w", expression, err)
	}

	return result, nil
}

// EvaluateDefaults recursively evaluates default values in a map structure.
// String values that are full expressions (e.g., "${value}") are evaluated as expressions,
// while strings containing partial expressions are interpolated. Literal strings and
// other types are preserved. The function handles nested maps and arrays recursively.
// The featurePath is used for resolving relative paths in file loading functions.
// Returns a new map with all defaults evaluated, or an error if evaluation fails.
func (e *expressionEvaluator) EvaluateDefaults(defaults map[string]any, config map[string]any, featurePath string) (map[string]any, error) {
	result := make(map[string]any)

	for key, value := range defaults {
		evaluated, err := e.evaluateDefaultValue(value, config, featurePath)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate default for key '%s': %w", key, err)
		}
		result[key] = evaluated
	}

	return result, nil
}

// InterpolateString replaces all ${expression} occurrences in a string with their evaluated values.
// This function processes template expressions embedded in strings, evaluating each expression
// and replacing it with the result. Complex values like maps and arrays are serialized to YAML.
// The function handles multiple expressions in a single string and processes them iteratively.
// Returns the fully interpolated string or an error if any expression evaluation fails.
func (e *expressionEvaluator) InterpolateString(s string, config map[string]any, featurePath string) (string, error) {
	result := s

	for strings.Contains(result, "${") {
		start := strings.Index(result, "${")
		end := strings.Index(result[start:], "}")

		if end == -1 {
			return "", fmt.Errorf("unclosed expression in string: %s", s)
		}

		end += start
		expr := result[start+2 : end]

		value, err := e.Evaluate(expr, config, featurePath)
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
	}

	return result, nil
}

// =============================================================================
// Private Methods
// =============================================================================

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
func (e *expressionEvaluator) buildExprEnvironment(config map[string]any, featurePath string) []expr.Option {
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

	opts = append(opts, e.helpers...)

	return opts
}

// evaluateDefaultValue recursively evaluates a single default value based on its type.
// String values are checked for full expressions (e.g., "${value}") which are evaluated
// directly, or partial expressions which are interpolated. Map and array types are processed
// recursively, evaluating each element. Other types are returned unchanged. This enables
// flexible default value specification with support for dynamic evaluation and nesting.
func (e *expressionEvaluator) evaluateDefaultValue(value any, config map[string]any, featurePath string) (any, error) {
	switch v := value.(type) {
	case string:
		if expr := e.extractExpression(v); expr != "" {
			return e.Evaluate(expr, config, featurePath)
		}
		if strings.Contains(v, "${") {
			return e.InterpolateString(v, config, featurePath)
		}
		return v, nil
	case map[string]any:
		result := make(map[string]any)
		for k, val := range v {
			evaluated, err := e.evaluateDefaultValue(val, config, featurePath)
			if err != nil {
				return nil, err
			}
			result[k] = evaluated
		}
		return result, nil
	case []any:
		result := make([]any, len(v))
		for i, val := range v {
			evaluated, err := e.evaluateDefaultValue(val, config, featurePath)
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
// It looks for the pattern "${...}" where the expression starts at the beginning and ends
// at the end of the string. If found, it returns the inner expression content. For partial
// expressions or strings with other content, it returns an empty string to indicate the
// value should be treated as interpolation rather than a direct expression.
func (e *expressionEvaluator) extractExpression(s string) string {
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
