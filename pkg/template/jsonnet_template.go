package template

import (
	"fmt"
	"maps"
	"strings"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Interfaces
// =============================================================================

// Template defines the interface for template processors
type Template interface {
	Initialize() error
	Process(templateData map[string][]byte, renderedData map[string]any) error
}

// =============================================================================
// Types
// =============================================================================

// JsonnetTemplate provides processing for Jsonnet templates for blueprint, terraform, and kustomize files.
// It applies path-based logic to determine how each template file is processed and keyed in the output.
type JsonnetTemplate struct {
	injector      di.Injector
	configHandler config.ConfigHandler
	shell         shell.Shell
	shims         *Shims
}

// =============================================================================
// Constructor
// =============================================================================

// NewJsonnetTemplate constructs a JsonnetTemplate with the provided dependency injector.
func NewJsonnetTemplate(injector di.Injector) *JsonnetTemplate {
	return &JsonnetTemplate{
		injector: injector,
		shims:    NewShims(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the JsonnetTemplate dependencies by resolving them from the injector.
// Returns an error if initialization fails.
func (t *JsonnetTemplate) Initialize() error {
	if t.injector != nil {
		if configHandler := t.injector.Resolve("configHandler"); configHandler != nil {
			t.configHandler = configHandler.(config.ConfigHandler)
		}
		if shellService := t.injector.Resolve("shell"); shellService != nil {
			t.shell = shellService.(shell.Shell)
		}
	}
	return nil
}

// Process performs two-phase Jsonnet template processing for blueprint and related files.
// Phase 1: Processes "blueprint.jsonnet" to extract patch and values references.
// Phase 2: Processes only referenced patch and values templates, omitting unreferenced files, and removes the patches field from the blueprint in renderedData.
// Returns an error if any processing step fails.
func (t *JsonnetTemplate) Process(templateData map[string][]byte, renderedData map[string]any) error {
	if err := t.processTemplate("blueprint.jsonnet", templateData, renderedData); err != nil {
		return err
	}
	patchRefs := t.extractPatchReferences(renderedData)
	patchSet := make(map[string]bool)
	for _, ref := range patchRefs {
		patchSet[ref] = true
	}
	for templatePath := range templateData {
		if templatePath == "blueprint.jsonnet" {
			continue
		}
		if strings.HasPrefix(templatePath, "patches/") {
			if !patchSet[templatePath] {
				continue
			}
		}
		if err := t.processTemplate(templatePath, templateData, renderedData); err != nil {
			return err
		}
	}
	t.cleanupBlueprint(renderedData)
	return nil
}

// processJsonnetTemplate evaluates a Jsonnet template string using the Windsor context and values data.
// The Windsor configuration is marshaled to YAML, converted to a map, and augmented with context and project name metadata.
// The context is serialized to JSON and injected into the Jsonnet VM as an external variable, along with helper functions and the effective blueprint URL.
// If values data is provided, it is merged into the context map before serialization.
// The template is evaluated, and the output is unmarshaled from JSON into a map.
// Returns the resulting map or an error if any step fails.
func (t *JsonnetTemplate) processJsonnetTemplate(templateContent string, valuesData []byte) (map[string]any, error) {
	config := t.configHandler.GetConfig()
	contextYAML, err := t.shims.YamlMarshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal context to YAML: %w", err)
	}
	projectRoot, err := t.shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to get project root: %w", err)
	}
	var contextMap map[string]any = make(map[string]any)
	if err := t.shims.YamlUnmarshal(contextYAML, &contextMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal context YAML: %w", err)
	}
	contextName := t.configHandler.GetContext()
	contextMap["name"] = contextName
	contextMap["projectName"] = t.shims.FilepathBase(projectRoot)

	if valuesData != nil {
		var valuesMap map[string]any
		if err := t.shims.YamlUnmarshal(valuesData, &valuesMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal values YAML: %w", err)
		}
		maps.Copy(contextMap, valuesMap)
	}

	contextJSON, err := t.shims.JsonMarshal(contextMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal context map to JSON: %w", err)
	}
	vm := t.shims.NewJsonnetVM()
	helpersLibrary := t.buildHelperLibrary()
	vm.ExtCode("helpers", helpersLibrary)
	vm.ExtCode("context", string(contextJSON))
	vm.ExtCode("ociUrl", fmt.Sprintf("%q", constants.GetEffectiveBlueprintURL()))
	result, err := vm.EvaluateAnonymousSnippet("template.jsonnet", templateContent)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate jsonnet template: %w", err)
	}
	var values map[string]any
	if err := t.shims.JsonUnmarshal([]byte(result), &values); err != nil {
		return nil, fmt.Errorf("jsonnet template must output valid JSON: %w", err)
	}
	cleanedValues := t.removeEmptyKeysFromOutput(values)
	if cleanedMap, ok := cleanedValues.(map[string]any); ok {
		return cleanedMap, nil
	}
	return values, nil
}

// removeEmptyKeysFromOutput recursively removes empty keys from the output data.
// This method implements the same logic as the Jsonnet removeEmptyKeys helper function
// but operates on Go data structures after template processing.
func (t *JsonnetTemplate) removeEmptyKeysFromOutput(data any) any {
	switch v := data.(type) {
	case map[string]any:
		cleaned := make(map[string]any)
		for key, value := range v {
			cleanedValue := t.removeEmptyKeysFromOutput(value)
			if !t.isEmptyValue(cleanedValue) {
				cleaned[key] = cleanedValue
			}
		}
		return cleaned
	case []any:
		cleaned := make([]any, 0, len(v))
		for _, item := range v {
			cleanedItem := t.removeEmptyKeysFromOutput(item)
			if !t.isEmptyValue(cleanedItem) {
				cleaned = append(cleaned, cleanedItem)
			}
		}
		return cleaned
	default:
		return data
	}
}

// isEmptyValue determines if a value should be considered empty and removed.
// Returns true for null, empty maps, and empty slices.
// Empty strings are preserved as they may be valid function results.
func (t *JsonnetTemplate) isEmptyValue(value any) bool {
	if value == nil {
		return true
	}

	switch v := value.(type) {
	case map[string]any:
		return len(v) == 0
	case []any:
		return len(v) == 0
	default:
		return false
	}
}

// processTemplate processes a single template file and stores the result in renderedData under a key determined by the template path.
// Recognized mappings:
//   - "blueprint.jsonnet" → "blueprint"
//   - "terraform/*.jsonnet" → "terraform/*" (without .jsonnet extension)
//   - "patches/<kustomization_name>/*.jsonnet" → "patches/<kustomization_name>/*" (without .jsonnet extension)
//
// Templates exclusively contain:
//   - blueprint.jsonnet
//   - terraform/<rel/path>.jsonnet
//   - patches/<kustomization_name>/*.jsonnet
//   - values.yaml (processed separately, not here)
//
// If the template does not exist in templateData, no action is performed. Returns an error if processing fails. Unrecognized template types are ignored.
func (t *JsonnetTemplate) processTemplate(templatePath string, templateData map[string][]byte, renderedData map[string]any) error {
	content, exists := templateData[templatePath]
	if !exists {
		return nil
	}

	var outputKey string
	switch {
	case templatePath == "blueprint.jsonnet":
		outputKey = "blueprint"
	case templatePath == "substitution.jsonnet":
		outputKey = "substitution"
	case strings.HasPrefix(templatePath, "terraform/") && strings.HasSuffix(templatePath, ".jsonnet"):
		outputKey = strings.TrimSuffix(templatePath, ".jsonnet")
	case strings.HasPrefix(templatePath, "patches/") && strings.HasSuffix(templatePath, ".jsonnet"):
		outputKey = strings.TrimSuffix(templatePath, ".jsonnet")
	default:
		return nil
	}

	var valuesData []byte
	if data, exists := templateData["values"]; exists {
		valuesData = data
	}

	values, err := t.processJsonnetTemplate(string(content), valuesData)
	if err != nil {
		return fmt.Errorf("failed to process template %s: %w", templatePath, err)
	}

	if t.isEmptyValue(values) {
		return nil
	}

	renderedData[outputKey] = values
	return nil
}

// extractPatchReferences returns a slice of template file paths for patch references found in the rendered blueprint within renderedData.
// The function inspects the "blueprint" key in renderedData, extracts the "kustomize" array, and collects patch paths from each kustomization's "patches" field.
// Patch paths are normalized to "patches/<kustomization_name>/<path>.jsonnet" format. Returns an empty slice if the blueprint or kustomize section is missing or malformed.
func (t *JsonnetTemplate) extractPatchReferences(renderedData map[string]any) []string {
	var templatePaths []string
	blueprintData, ok := renderedData["blueprint"]
	if !ok {
		return templatePaths
	}
	blueprintMap, ok := blueprintData.(map[string]any)
	if !ok {
		return templatePaths
	}
	kustomizeArr, ok := blueprintMap["kustomize"].([]any)
	if !ok {
		return templatePaths
	}
	for _, k := range kustomizeArr {
		kMap, ok := k.(map[string]any)
		if !ok {
			continue
		}
		kustomizationName, ok := kMap["name"].(string)
		if !ok {
			continue
		}
		patches, ok := kMap["patches"].([]any)
		if !ok {
			continue
		}
		for _, p := range patches {
			pMap, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if path, ok := pMap["path"].(string); ok && path != "" {
				templatePath := "patches/" + kustomizationName + "/" + path + ".jsonnet"
				templatePaths = append(templatePaths, templatePath)
			}
		}
	}
	return templatePaths
}

// cleanupBlueprint removes the patches field from each kustomization in the blueprint within renderedData.
// This is used to clean up the output after all referenced patches have been processed.
func (t *JsonnetTemplate) cleanupBlueprint(renderedData map[string]any) {
	blueprintData, ok := renderedData["blueprint"]
	if !ok {
		return
	}
	blueprintMap, ok := blueprintData.(map[string]any)
	if !ok {
		return
	}
	kustomizeArr, ok := blueprintMap["kustomize"].([]any)
	if !ok {
		return
	}
	for i, k := range kustomizeArr {
		if kMap, ok := k.(map[string]any); ok {
			delete(kMap, "patches")
			kustomizeArr[i] = kMap
		}
	}
	blueprintMap["kustomize"] = kustomizeArr
	renderedData["blueprint"] = blueprintMap
}

// buildHelperLibrary returns a Jsonnet library string containing helper functions for safe context access and data manipulation.
// Helpers provided:
//   - get: Retrieve value by path from object, with default fallback.
//   - getString, getInt, getNumber, getBool, getObject, getArray: Typed retrieval with type assertion and default fallback.
//   - has: Check if a value exists at a given path.
//   - baseUrl: Extract base URL from an endpoint string, removing protocol and port.
//   - removeEmptyKeys: Recursively remove empty keys from objects, preserving non-empty values.
func (jt *JsonnetTemplate) buildHelperLibrary() string {
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

// =============================================================================
// Interface Compliance
// =============================================================================

// JsonnetTemplate implements the Template interface.
var _ Template = (*JsonnetTemplate)(nil)
