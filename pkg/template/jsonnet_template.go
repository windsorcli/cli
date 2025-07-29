package template

import (
	"fmt"
	"strings"

	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Types
// =============================================================================

// JsonnetTemplate handles jsonnet template processing with configurable rules
type JsonnetTemplate struct {
	*BaseTemplate
}

// =============================================================================
// Constructor
// =============================================================================

// NewJsonnetTemplate constructs a JsonnetTemplate with default processing rules for blueprint and terraform Jsonnet files.
// It initializes the embedded BaseTemplate and assigns rules for blueprint.jsonnet, terraform/*.jsonnet, and patches/*.jsonnet path handling.
// The blueprint rule matches the exact "blueprint.jsonnet" filename and generates the "blueprint" key.
// The terraform rule matches files under the "terraform/" directory with a ".jsonnet" extension and generates keys by stripping the extension.
// The patches rule matches files under the "patches/" directory with a ".jsonnet" extension and generates keys in the format "patches/<kustomization_name>".
func NewJsonnetTemplate(injector di.Injector) *JsonnetTemplate {
	template := &JsonnetTemplate{
		BaseTemplate: NewBaseTemplate(injector),
	}
	template.rules = []ProcessingRule{
		{
			PathMatcher: func(path string) bool {
				return path == "blueprint.jsonnet"
			},
			KeyGenerator: func(path string) string {
				return "blueprint"
			},
		},
		{
			PathMatcher: func(path string) bool {
				return strings.HasPrefix(path, "terraform/") && strings.HasSuffix(path, ".jsonnet")
			},
			KeyGenerator: func(path string) string {
				return strings.TrimSuffix(path, ".jsonnet")
			},
		},
		{
			PathMatcher: func(path string) bool {
				return strings.HasPrefix(path, "patches/") && strings.HasSuffix(path, ".jsonnet")
			},
			KeyGenerator: func(path string) string {
				trimmed := strings.TrimSuffix(path, ".jsonnet")
				return trimmed
			},
		},
	}
	return template
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the JsonnetTemplate dependencies
func (t *JsonnetTemplate) Initialize() error {
	return t.BaseTemplate.Initialize()
}

// Process applies configured processing rules to jsonnet templates and populates renderedData with evaluated results.
// For each template in templateData, the method checks all rules for a matching PathMatcher. If a rule matches,
// it processes the template using processJsonnetTemplate, stores the result in renderedData under the key generated
// by the rule's KeyGenerator, and skips further rule checks for that template. Returns an error if processing fails.
func (t *JsonnetTemplate) Process(templateData map[string][]byte, renderedData map[string]any) error {
	for templatePath, templateContent := range templateData {
		for _, rule := range t.rules {
			if rule.PathMatcher(templatePath) {
				values, err := t.processJsonnetTemplate(string(templateContent))
				if err != nil {
					return fmt.Errorf("failed to process jsonnet template %s: %w", templatePath, err)
				}
				outputKey := rule.KeyGenerator(templatePath)
				renderedData[outputKey] = values
				break
			}
		}
	}
	return nil
}

// processJsonnetTemplate evaluates a Jsonnet template with Windsor context and returns the resulting data as a map.
// It marshals the Windsor configuration to YAML, converts it to a map, injects context and project name,
// serializes the context to JSON, and evaluates the Jsonnet template with the context as an external variable.
// The output must be valid JSON and is unmarshaled into a map for downstream use.
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

	// Add context metadata to the merged config
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

// buildHelperLibrary creates a Jsonnet library with helper functions for safe context access
func (jt *JsonnetTemplate) buildHelperLibrary() string {
	return `{
  // Smart helpers - handle both path-based ("a.b.c") and key-based ("key") access
  get(obj, path, default=null):
    if std.findSubstr(".", path) == [] then
      // Simple key access (no dots)
      if std.type(obj) == "object" && path in obj then obj[path] else default
    else
      // Path-based access (with dots)
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

  // URL helper functions
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

// Ensure JsonnetTemplate implements Template interface
var _ Template = (*JsonnetTemplate)(nil)
