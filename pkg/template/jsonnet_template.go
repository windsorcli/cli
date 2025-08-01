package template

import (
	"fmt"
	"path/filepath"
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
	valuesRefs := t.extractValuesReferences(renderedData)
	patchSet := make(map[string]bool)
	valuesSet := make(map[string]bool)
	for _, ref := range patchRefs {
		patchSet[ref] = true
	}
	for _, ref := range valuesRefs {
		valuesSet[ref] = true
	}
	for templatePath := range templateData {
		if templatePath == "blueprint.jsonnet" {
			continue
		}
		if strings.HasPrefix(templatePath, "kustomize/") {
			if strings.HasSuffix(templatePath, "/values.jsonnet") {
				if !valuesSet[templatePath] {
					continue
				}
			} else if !patchSet[templatePath] {
				continue
			}
		}
		if strings.HasPrefix(templatePath, "values/") {
			if !valuesSet[templatePath] {
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

// processJsonnetTemplate evaluates a Jsonnet template string using the Windsor context and returns the resulting data as a map.
// The Windsor configuration is marshaled to YAML, converted to a map, and augmented with context and project name metadata.
// The context is then serialized to JSON and injected into the Jsonnet VM as an external variable, along with helper functions and the effective blueprint URL.
// The template is evaluated, and the output is unmarshaled from JSON into a map for downstream use.
// Returns the resulting map or an error if any step fails.
func (t *JsonnetTemplate) processJsonnetTemplate(templateContent string) (map[string]any, error) {
	config := t.configHandler.GetConfig()

	contextYAML, err := t.configHandler.YamlMarshalWithDefinedPaths(config)
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
	contextJSON, err := t.shims.JsonMarshal(contextMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal context map to JSON: %w", err)
	}
	vm := t.shims.NewJsonnetVM()
	vm.ExtCode("helpers", t.buildHelperLibrary())
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
	return values, nil
}

// processTemplate processes a single template file and stores the result in renderedData under a key determined by the template path.
// Recognized mappings:
//   - "blueprint.jsonnet" maps to "blueprint"
//   - "terraform/*.jsonnet" maps to "terraform/*" (without .jsonnet extension)
//   - "kustomize/*.jsonnet" maps to "kustomize/*" (without .jsonnet extension)
//   - "kustomize/<component>/values.jsonnet" maps to "values/<component>"
//   - "kustomize/values.jsonnet" maps to "values/global"
//   - "values/*.jsonnet" maps to "values/*" (without .jsonnet extension)
//
// If the template does not exist in templateData, no action is performed. Returns an error if processing fails. Unrecognized template types are ignored.
func (t *JsonnetTemplate) processTemplate(templatePath string, templateData map[string][]byte, renderedData map[string]any) error {
	content, exists := templateData[templatePath]
	if !exists {
		return nil
	}

	var outputKey string
	if templatePath == "blueprint.jsonnet" {
		outputKey = "blueprint"
	} else if strings.HasPrefix(templatePath, "terraform/") && strings.HasSuffix(templatePath, ".jsonnet") {
		outputKey = strings.TrimSuffix(templatePath, ".jsonnet")
	} else if strings.HasPrefix(templatePath, "kustomize/") && strings.HasSuffix(templatePath, ".jsonnet") {
		if strings.HasSuffix(templatePath, "/values.jsonnet") {
			pathParts := strings.Split(templatePath, "/")
			if len(pathParts) == 3 && pathParts[0] == "kustomize" && pathParts[2] == "values.jsonnet" {
				component := pathParts[1]
				if component == "values" {
					outputKey = "values/global"
				} else {
					outputKey = "values/" + component
				}
			} else if len(pathParts) == 2 && pathParts[0] == "kustomize" && pathParts[1] == "values.jsonnet" {
				outputKey = "values/global"
			} else {
				outputKey = strings.TrimSuffix(templatePath, ".jsonnet")
			}
		} else {
			outputKey = strings.TrimSuffix(templatePath, ".jsonnet")
		}
	} else if strings.HasPrefix(templatePath, "values/") && strings.HasSuffix(templatePath, ".jsonnet") {
		outputKey = strings.TrimSuffix(templatePath, ".jsonnet")
	} else {
		return nil
	}

	values, err := t.processJsonnetTemplate(string(content))
	if err != nil {
		return fmt.Errorf("failed to process template %s: %w", templatePath, err)
	}

	renderedData[outputKey] = values
	return nil
}

// extractPatchReferences returns a slice of template file paths for patch references found in the rendered blueprint within renderedData.
// The function inspects the "blueprint" key in renderedData, extracts the "kustomize" array, and collects patch paths from each kustomization's "patches" field.
// Patch paths are normalized to "kustomize/<path>.jsonnet" if not already fully qualified. Returns an empty slice if the blueprint or kustomize section is missing or malformed.
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
				var templatePath string
				if strings.HasPrefix(path, "kustomize/") {
					templatePath = path
				} else {
					templatePath = "kustomize/" + path + ".jsonnet"
				}
				templatePaths = append(templatePaths, templatePath)
			}
		}
	}
	return templatePaths
}

// extractValuesReferences returns a slice of values template file paths found in the kustomize directory structure.
// Always includes the global values template ("kustomize/values.jsonnet") and automatically discovers component-specific values templates.
// Returns an empty slice if the blueprint or kustomize section is missing or malformed.
func (t *JsonnetTemplate) extractValuesReferences(renderedData map[string]any) []string {
	var templatePaths []string
	blueprintData, ok := renderedData["blueprint"]
	if !ok {
		return templatePaths
	}
	blueprintMap, ok := blueprintData.(map[string]any)
	if !ok {
		return templatePaths
	}
	_, ok = blueprintMap["kustomize"].([]any)
	if !ok {
		return templatePaths
	}

	// Always include global values template
	templatePaths = append(templatePaths, "kustomize/values.jsonnet")

	// Automatically discover component-specific values templates from kustomize directory
	projectRoot, err := t.shell.GetProjectRoot()
	if err == nil {
		templateDir := filepath.Join(projectRoot, "contexts", "_template", "kustomize")
		if entries, err := t.shims.ReadDir(templateDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					componentValuesPath := filepath.Join(templateDir, entry.Name(), "values.jsonnet")
					if _, err := t.shims.Stat(componentValuesPath); err == nil {
						templatePath := fmt.Sprintf("kustomize/%s/values.jsonnet", entry.Name())
						templatePaths = append(templatePaths, templatePath)
					}
				}
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

// buildHelperLibrary returns a Jsonnet library as a string containing helper functions for safe context access.
// The library provides functions for retrieving values from objects by path or key, with type assertions and default values.
// It also includes a baseUrl helper for extracting the base URL from an endpoint string.
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
}`
}

// =============================================================================
// Interface Compliance
// =============================================================================

// JsonnetTemplate implements the Template interface.
var _ Template = (*JsonnetTemplate)(nil)
