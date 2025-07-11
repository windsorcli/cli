package template

import (
	"fmt"
	"strings"

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
// It initializes the embedded BaseTemplate and assigns rules for blueprint.jsonnet and terraform/*.jsonnet path handling.
// The blueprint rule matches the exact "blueprint.jsonnet" filename and generates the "blueprint" key.
// The terraform rule matches files under the "terraform/" directory with a ".jsonnet" extension and generates keys by stripping the extension.
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

	contextMap["name"] = t.configHandler.GetContext()
	contextMap["projectName"] = t.shims.FilepathBase(projectRoot)

	contextJSON, err := t.shims.JsonMarshal(contextMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal context map to JSON: %w", err)
	}

	vm := t.shims.NewJsonnetVM()
	vm.ExtCode("context", string(contextJSON))

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

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure JsonnetTemplate implements Template interface
var _ Template = (*JsonnetTemplate)(nil)
