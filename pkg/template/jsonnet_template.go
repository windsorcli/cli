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

// NewJsonnetTemplate creates a new JsonnetTemplate instance with default rules
func NewJsonnetTemplate(injector di.Injector) *JsonnetTemplate {
	template := &JsonnetTemplate{
		BaseTemplate: NewBaseTemplate(injector),
	}

	// Set up default processing rules
	template.rules = []ProcessingRule{
		{
			// Blueprint rule: exact match for blueprint.jsonnet
			PathMatcher: func(path string) bool {
				return path == "blueprint.jsonnet"
			},
			KeyGenerator: func(path string) string {
				return "blueprint"
			},
		},
		{
			// Terraform rule: files under terraform/ with .jsonnet extension
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

// Process processes jsonnet templates based on configured rules and adds results to renderedData
func (t *JsonnetTemplate) Process(templateData map[string][]byte, renderedData map[string]any) error {
	for templatePath, templateContent := range templateData {
		// Check each rule to see if it matches this file
		for _, rule := range t.rules {
			if rule.PathMatcher(templatePath) {
				values, err := t.processJsonnetTemplate(string(templateContent))
				if err != nil {
					return fmt.Errorf("failed to process jsonnet template %s: %w", templatePath, err)
				}

				outputKey := rule.KeyGenerator(templatePath)
				renderedData[outputKey] = values
				break // Only process with the first matching rule
			}
		}
	}

	return nil
}

// processJsonnetTemplate processes a jsonnet template with context data and returns parsed values
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
