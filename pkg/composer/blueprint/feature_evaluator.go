package blueprint

import (
	"fmt"
	"maps"
	"path/filepath"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/google/go-jsonnet"
	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
)

// FeatureEvaluator provides lightweight expression evaluation for blueprint feature conditions.
// It uses the expr library for fast compilation and evaluation of simple comparison expressions
// without the overhead of a full expression language like CEL for basic equality checks.
// The FeatureEvaluator enables efficient feature activation based on user configuration values.

// =============================================================================
// Types
// =============================================================================

// FeatureEvaluator provides lightweight expression evaluation for feature conditions.
type FeatureEvaluator struct {
	runtime      *runtime.Runtime
	shims        *Shims
	templateData map[string][]byte
}

// =============================================================================
// Constructor
// =============================================================================

// NewFeatureEvaluator creates a new feature evaluator with the provided dependencies.
// If overrides are provided, any non-nil component in the override FeatureEvaluator will be used instead of creating a default.
func NewFeatureEvaluator(rt *runtime.Runtime) *FeatureEvaluator {
	evaluator := &FeatureEvaluator{
		runtime: rt,
		shims:   NewShims(),
	}

	return evaluator
}

// SetTemplateData sets the template data map for file resolution when loading from artifacts.
// This allows jsonnet() and file() functions to access files from in-memory template data
// instead of requiring them to exist on the filesystem.
func (e *FeatureEvaluator) SetTemplateData(templateData map[string][]byte) {
	e.templateData = templateData
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
// The featurePath is used to resolve relative paths in jsonnet() and file() functions.
// Returns true if the expression evaluates to true, false otherwise.
func (e *FeatureEvaluator) EvaluateExpression(expression string, config map[string]any, featurePath string) (bool, error) {
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
// Also supports file loading functions: jsonnet("path") and file("path").
// The featurePath is used to resolve relative paths in jsonnet() and file() functions.
// Returns the evaluated value or an error if evaluation fails.
func (e *FeatureEvaluator) EvaluateValue(expression string, config map[string]any, featurePath string) (any, error) {
	if expression == "" {
		return nil, fmt.Errorf("expression cannot be empty")
	}

	env := e.buildExprEnvironment(config, featurePath)

	program, err := expr.Compile(expression, env...)
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
// The featurePath is used to resolve relative paths in jsonnet() and file() functions.
func (e *FeatureEvaluator) EvaluateDefaults(defaults map[string]any, config map[string]any, featurePath string) (map[string]any, error) {
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

// ProcessFeature evaluates feature conditions and processes its Terraform components and Kustomizations.
// If the feature has a 'When' condition, it is evaluated against the provided config and feature path.
// Features or components whose conditions do not match are skipped. The returned Feature includes only
// the components and Kustomizations whose conditions have passed. If the root feature's condition is not met,
// ProcessFeature returns nil. Errors encountered in any evaluation are returned. Inputs for Terraform components
// and substitutions for Kustomizations are evaluated and updated; nil values from evaluated inputs are omitted.
func (e *FeatureEvaluator) ProcessFeature(feature *v1alpha1.Feature, config map[string]any) (*v1alpha1.Feature, error) {
	if feature.When != "" {
		matches, err := e.EvaluateExpression(feature.When, config, feature.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate feature condition '%s': %w", feature.When, err)
		}
		if !matches {
			return nil, nil
		}
	}

	processedFeature := feature.DeepCopy()

	var processedTerraformComponents []v1alpha1.ConditionalTerraformComponent
	for _, terraformComponent := range processedFeature.TerraformComponents {
		if terraformComponent.When != "" {
			matches, err := e.EvaluateExpression(terraformComponent.When, config, feature.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate terraform component condition '%s': %w", terraformComponent.When, err)
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
			matches, err := e.EvaluateExpression(kustomization.When, config, feature.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate kustomization condition '%s': %w", kustomization.When, err)
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

// buildExprEnvironment creates an expr environment with custom functions for file loading.
func (e *FeatureEvaluator) buildExprEnvironment(config map[string]any, featurePath string) []expr.Option {
	return []expr.Option{
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
}

// evaluateDefaultValue recursively evaluates a single default value.
func (e *FeatureEvaluator) evaluateDefaultValue(value any, config map[string]any, featurePath string) (any, error) {
	switch v := value.(type) {
	case string:
		if expr := e.extractExpression(v); expr != "" {
			return e.EvaluateValue(expr, config, featurePath)
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

// InterpolateString replaces all ${expression} occurrences in a string with their evaluated values.
// This is used to process template expressions in patch content and other string values.
func (e *FeatureEvaluator) InterpolateString(s string, config map[string]any, featurePath string) (string, error) {
	result := s

	for strings.Contains(result, "${") {
		start := strings.Index(result, "${")
		end := strings.Index(result[start:], "}")

		if end == -1 {
			return "", fmt.Errorf("unclosed expression in string: %s", s)
		}

		end += start
		expr := result[start+2 : end]

		value, err := e.EvaluateValue(expr, config, featurePath)
		if err != nil {
			return "", fmt.Errorf("failed to evaluate expression '${%s}': %w", expr, err)
		}

		var replacement string
		if value == nil {
			replacement = ""
		} else {
			switch value.(type) {
			case map[string]any, []any:
				yamlData, err := e.shims.YamlMarshal(value)
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

// evaluateJsonnetFunction loads and processes a jsonnet file from the given path.
func (e *FeatureEvaluator) evaluateJsonnetFunction(pathArg string, config map[string]any, featurePath string) (any, error) {
	path := e.resolvePath(pathArg, featurePath)

	var content []byte
	var err error

	if e.templateData != nil {
		content = e.lookupInTemplateData(pathArg, featurePath)
		if content == nil && e.runtime != nil && e.runtime.TemplateRoot != "" {
			if relPath, err := filepath.Rel(e.runtime.TemplateRoot, path); err == nil && !strings.HasPrefix(relPath, "..") {
				relPath = strings.ReplaceAll(relPath, "\\", "/")
				if data, exists := e.templateData["_template/"+relPath]; exists {
					content = data
				} else if data, exists := e.templateData[relPath]; exists {
					content = data
				}
			}
		}
	}

	if content == nil {
		content, err = e.shims.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", path, err)
		}
	}

	enrichedConfig := e.buildContextMap(config)

	configJSON, err := e.shims.JsonMarshal(enrichedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config to JSON: %w", err)
	}

	vm := e.shims.NewJsonnetVM()

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
	if err := e.shims.JsonUnmarshal([]byte(result), &value); err != nil {
		return nil, fmt.Errorf("jsonnet file %s must output valid JSON: %w", path, err)
	}

	return value, nil
}

// buildContextMap enriches the config with name and projectName fields for consistency with main template processing.
func (e *FeatureEvaluator) buildContextMap(config map[string]any) map[string]any {
	contextMap := make(map[string]any)
	maps.Copy(contextMap, config)

	if e.runtime != nil && e.runtime.ConfigHandler != nil {
		contextName := e.runtime.ConfigHandler.GetContext()
		contextMap["name"] = contextName
	}

	if e.runtime != nil && e.runtime.ProjectRoot != "" {
		contextMap["projectName"] = e.shims.FilepathBase(e.runtime.ProjectRoot)
	}

	return contextMap
}

// buildHelperLibrary returns a Jsonnet library string containing helper functions for safe context access and data manipulation.
func (e *FeatureEvaluator) buildHelperLibrary() string {
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

// evaluateFileFunction loads raw file content from the given path.
func (e *FeatureEvaluator) evaluateFileFunction(pathArg string, featurePath string) (any, error) {
	path := e.resolvePath(pathArg, featurePath)

	var content []byte
	var err error

	if e.templateData != nil {
		content = e.lookupInTemplateData(pathArg, featurePath)
		if content == nil && e.runtime != nil && e.runtime.TemplateRoot != "" {
			if relPath, err := filepath.Rel(e.runtime.TemplateRoot, path); err == nil && !strings.HasPrefix(relPath, "..") {
				relPath = strings.ReplaceAll(relPath, "\\", "/")
				if data, exists := e.templateData["_template/"+relPath]; exists {
					content = data
				} else if data, exists := e.templateData[relPath]; exists {
					content = data
				}
			}
		}
	}

	if content == nil {
		content, err = e.shims.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", path, err)
		}
	}

	return string(content), nil
}

// lookupInTemplateData looks up a file in templateData by resolving the relative path from featurePath.
// Returns the file content if found, nil otherwise.
func (e *FeatureEvaluator) lookupInTemplateData(pathArg string, featurePath string) []byte {
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

	var featureRelPath string
	if e.runtime != nil && e.runtime.TemplateRoot != "" {
		if rel, err := filepath.Rel(e.runtime.TemplateRoot, featurePath); err == nil && !strings.HasPrefix(rel, "..") {
			featureRelPath = strings.ReplaceAll(rel, "\\", "/")
		} else {
			featureRelPath = featurePath
		}
	} else {
		featureRelPath = featurePath
	}

	featureDir := filepath.Dir(featureRelPath)
	if featureDir == "." {
		featureDir = ""
	}
	resolvedRelPath := filepath.Clean(filepath.Join(featureDir, pathArg))
	resolvedRelPath = strings.ReplaceAll(resolvedRelPath, "\\", "/")

	if data, exists := e.templateData["_template/"+resolvedRelPath]; exists {
		return data
	}
	if data, exists := e.templateData[resolvedRelPath]; exists {
		return data
	}

	return nil
}

// resolvePath returns an absolute, cleaned file path based on the provided path and featurePath.
// If the path is absolute, it returns the cleaned version directly. If the path is relative and
// featurePath is non-empty, the result is the provided path joined to the feature directory.
// If featurePath is empty and a project root is available via the shell, the path is joined to
// the project root. In all cases, the result is normalized and cleaned. Paths are trimmed of
// whitespace before resolution.
func (e *FeatureEvaluator) resolvePath(path string, featurePath string) string {
	path = strings.TrimSpace(path)

	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	if featurePath != "" {
		featureDir := filepath.Dir(featurePath)
		return filepath.Clean(filepath.Join(featureDir, path))
	}

	if e.runtime != nil && e.runtime.ProjectRoot != "" {
		return filepath.Clean(filepath.Join(e.runtime.ProjectRoot, path))
	}

	return filepath.Clean(path)
}

// evaluateSubstitutions evaluates expressions in substitution values and converts all results to strings.
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
