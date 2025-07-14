package generators

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/go-jsonnet"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/zclconf/go-cty/cty"
)

// The TerraformGenerator is a specialized component that manages Terraform configuration files.
// It provides functionality to create and update Terraform modules, variables, and tfvars files.
// The TerraformGenerator ensures proper infrastructure-as-code configuration for Windsor projects,
// maintaining consistent Terraform structure across all contexts.

// =============================================================================
// Types
// =============================================================================

// TerraformGenerator is a generator that writes Terraform files
type TerraformGenerator struct {
	BaseGenerator
	reset bool
}

// VariableInfo holds metadata for a single Terraform variable
type VariableInfo struct {
	Name        string
	Description string
	Default     any
	Sensitive   bool
}

// =============================================================================
// Constructor
// =============================================================================

// NewTerraformGenerator creates a new TerraformGenerator with the provided dependency injector.
// It initializes the base generator and prepares it for Terraform file generation.
func NewTerraformGenerator(injector di.Injector) *TerraformGenerator {
	return &TerraformGenerator{
		BaseGenerator: *NewGenerator(injector),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Write generates Terraform configuration files for all components, including tfvars files.
// It processes jsonnet templates from the contexts/_template/terraform directory, merges template values into
// blueprint Terraform components, and delegates file generation to Generate. Module resolution is now handled
// by the pkg/terraform package.
func (g *TerraformGenerator) Write(overwrite ...bool) error {
	shouldOverwrite := false
	if len(overwrite) > 0 {
		shouldOverwrite = overwrite[0]
	}
	g.reset = shouldOverwrite

	templateValues, err := g.processTemplates(shouldOverwrite)
	if err != nil {
		return fmt.Errorf("failed to process terraform templates: %w", err)
	}

	components := g.blueprintHandler.GetTerraformComponents()
	generateData := make(map[string]any)

	for _, component := range components {
		componentValues := make(map[string]any)
		if component.Values != nil {
			maps.Copy(componentValues, component.Values)
		}
		if templateComponentValues, exists := templateValues[component.Path]; exists {
			maps.Copy(componentValues, templateComponentValues)
		}
		generateData["terraform/"+component.Path] = componentValues
	}

	return g.Generate(generateData)
}

// Generate produces Terraform configuration files, including tfvars files, for all blueprint components.
// It consumes template data keyed by "terraform/<module_path>", generating tfvars files at
// contexts/<context>/terraform/<module_path>.tfvars. The method utilizes the blueprint handler to retrieve
// TerraformComponents and determines the variables.tf location based on component source presence (remote or local).
// Module resolution is now handled by the pkg/terraform package.
func (g *TerraformGenerator) Generate(data map[string]any, overwrite ...bool) error {

	contextPath, err := g.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("failed to get config root: %w", err)
	}

	projectRoot, err := g.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	components := g.blueprintHandler.GetTerraformComponents()
	componentMap := make(map[string]blueprintv1alpha1.TerraformComponent)
	for _, component := range components {
		componentMap[component.Path] = component
	}

	for componentPath, componentData := range data {
		if !strings.HasPrefix(componentPath, "terraform/") {
			continue
		}

		componentValues, ok := componentData.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid data format for component %s: expected map[string]any", componentPath)
		}

		actualPath := strings.TrimPrefix(componentPath, "terraform/")

		component, exists := componentMap[actualPath]
		if !exists {
			return fmt.Errorf("component %s not found in blueprint", actualPath)
		}

		variablesTfPath, err := g.findVariablesTfFileForComponent(projectRoot, component)
		if err != nil {
			return fmt.Errorf("failed to find variables.tf for component %s: %w", componentPath, err)
		}

		tfvarsFilePath := filepath.Join(contextPath, componentPath+".tfvars")

		if err := g.generateTfvarsFile(tfvarsFilePath, variablesTfPath, componentValues, component.Source); err != nil {
			return fmt.Errorf("failed to generate tfvars file for component %s: %w", componentPath, err)
		}
	}

	for _, component := range components {
		terraformKey := "terraform/" + component.Path
		if _, exists := data[terraformKey]; !exists {
			variablesTfPath, err := g.findVariablesTfFileForComponent(projectRoot, component)
			if err != nil {
				return fmt.Errorf("failed to find variables.tf for component %s: %w", component.Path, err)
			}

			tfvarsFilePath := filepath.Join(contextPath, terraformKey+".tfvars")
			componentValues := component.Values
			if componentValues == nil {
				componentValues = make(map[string]any)
			}

			if err := g.generateTfvarsFile(tfvarsFilePath, variablesTfPath, componentValues, component.Source); err != nil {
				return fmt.Errorf("failed to generate tfvars file for component %s: %w", component.Path, err)
			}
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// processTemplates discovers and processes jsonnet template files from the contexts/_template/terraform directory.
// It checks for template directory existence, retrieves the current context configuration, and recursively
// walks through template files to generate corresponding .tfvars files. The function handles template
// discovery, context resolution, and delegates actual processing to walkTemplateDirectory.
func (g *TerraformGenerator) processTemplates(reset bool) (map[string]map[string]any, error) {
	projectRoot, err := g.shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to get project root: %w", err)
	}

	templateDir := filepath.Join(projectRoot, "contexts", "_template", "terraform")

	if _, err := g.shims.Stat(templateDir); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to check template directory: %w", err)
	}

	contextPath, err := g.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to get config root: %w", err)
	}

	contextName := g.configHandler.GetString("context")
	if contextName == "" {
		contextName = os.Getenv("WINDSOR_CONTEXT")
	}

	templateValues := make(map[string]map[string]any)

	return templateValues, g.walkTemplateDirectory(templateDir, contextPath, contextName, reset, templateValues)
}

// walkTemplateDirectory recursively traverses the template directory structure and processes jsonnet files.
// It handles both files and subdirectories, maintaining the directory structure in the output location.
// For each .jsonnet file found, it delegates processing to processJsonnetTemplate to collect template
// values that will be merged into terraform components.
func (g *TerraformGenerator) walkTemplateDirectory(templateDir, contextPath, contextName string, reset bool, templateValues map[string]map[string]any) error {
	entries, err := g.shims.ReadDir(templateDir)
	if err != nil {
		return fmt.Errorf("failed to read template directory: %w", err)
	}

	for _, entry := range entries {
		entryPath := filepath.Join(templateDir, entry.Name())

		if entry.IsDir() {
			if err := g.walkTemplateDirectory(entryPath, contextPath, contextName, reset, templateValues); err != nil {
				return err
			}
		} else if strings.HasSuffix(entry.Name(), ".jsonnet") {
			if err := g.processJsonnetTemplate(entryPath, contextName, templateValues); err != nil {
				return err
			}
		}
	}

	return nil
}

// processJsonnetTemplate processes a jsonnet template file and collects generated values
// for merging into blueprint terraform components. It evaluates the template with context data
// made available via std.extVar("context"), then stores the result in templateValues using
// the relative path from the template directory as the key.
// Templates must include: local context = std.extVar("context"); to access context data.
func (g *TerraformGenerator) processJsonnetTemplate(templateFile, contextName string, templateValues map[string]map[string]any) error {
	templateContent, err := g.shims.ReadFile(templateFile)
	if err != nil {
		return fmt.Errorf("error reading template file %s: %w", templateFile, err)
	}

	config := g.configHandler.GetConfig()

	contextYAML, err := g.configHandler.YamlMarshalWithDefinedPaths(config)
	if err != nil {
		return fmt.Errorf("error marshalling context to YAML: %w", err)
	}

	var contextMap map[string]any = make(map[string]any)
	if err := g.shims.YamlUnmarshal(contextYAML, &contextMap); err != nil {
		return fmt.Errorf("error unmarshalling context YAML: %w", err)
	}

	contextMap["name"] = contextName
	contextJSON, err := g.shims.JsonMarshal(contextMap)
	if err != nil {
		return fmt.Errorf("error marshalling context map to JSON: %w", err)
	}

	vm := jsonnet.MakeVM()
	vm.ExtCode("context", string(contextJSON))
	result, err := vm.EvaluateAnonymousSnippet("template.jsonnet", string(templateContent))
	if err != nil {
		return fmt.Errorf("error evaluating jsonnet template %s: %w", templateFile, err)
	}

	var values map[string]any
	if err := g.shims.JsonUnmarshal([]byte(result), &values); err != nil {
		return fmt.Errorf("jsonnet template must output valid JSON: %w", err)
	}

	projectRoot, err := g.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	templateDir := filepath.Join(projectRoot, "contexts", "_template", "terraform")
	relPath, err := g.shims.FilepathRel(templateDir, templateFile)
	if err != nil {
		return fmt.Errorf("failed to calculate relative path: %w", err)
	}

	componentPath := strings.TrimSuffix(relPath, ".jsonnet")
	componentPath = strings.ReplaceAll(componentPath, "\\", "/")
	templateValues[componentPath] = values

	return nil
}

// writeTfvarsFile creates or updates a .tfvars file with variable values for the Terraform module.
// It uses variables.tf as the basis for variable definitions and allows component.Values to override specific values.
// The function maintains a header indicating Windsor CLI management and handles module source comments.

// checkExistingTfvarsFile checks if a tfvars file exists and is readable.
// Returns os.ErrExist if the file exists and is readable, or an error if the file exists but is not readable.
func (g *TerraformGenerator) checkExistingTfvarsFile(tfvarsFilePath string) error {
	_, err := g.shims.Stat(tfvarsFilePath)
	if err == nil {
		_, err := g.shims.ReadFile(tfvarsFilePath)
		if err != nil {
			return fmt.Errorf("failed to read existing tfvars file: %w", err)
		}
		return os.ErrExist
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("error checking tfvars file: %w", err)
	}
	return nil
}

// parseVariablesFile parses variables.tf and returns metadata about the variables.
// It extracts variable names, descriptions, default values, and sensitivity flags.
// Protected values are excluded from the returned metadata.
func (g *TerraformGenerator) parseVariablesFile(variablesTfPath string, protectedValues map[string]bool) ([]VariableInfo, error) {
	variablesContent, err := g.shims.ReadFile(variablesTfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read variables.tf: %w", err)
	}

	variablesFile, diags := hclwrite.ParseConfig(variablesContent, variablesTfPath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse variables.tf: %w", diags)
	}

	var variables []VariableInfo
	for _, block := range variablesFile.Body().Blocks() {
		if block.Type() == "variable" && len(block.Labels()) > 0 {
			variableName := block.Labels()[0]

			if protectedValues[variableName] {
				continue
			}

			info := VariableInfo{
				Name: variableName,
			}

			if attr := block.Body().GetAttribute("description"); attr != nil {
				exprBytes := attr.Expr().BuildTokens(nil).Bytes()
				parsedExpr, diags := hclsyntax.ParseExpression(exprBytes, "description", hcl.Pos{Line: 1, Column: 1})
				if !diags.HasErrors() {
					val, diags := parsedExpr.Value(nil)
					if !diags.HasErrors() && val.Type() == cty.String {
						info.Description = val.AsString()
					}
				}
			}

			if attr := block.Body().GetAttribute("sensitive"); attr != nil {
				exprBytes := attr.Expr().BuildTokens(nil).Bytes()
				parsedExpr, diags := hclsyntax.ParseExpression(exprBytes, "sensitive", hcl.Pos{Line: 1, Column: 1})
				if !diags.HasErrors() {
					val, diags := parsedExpr.Value(nil)
					if !diags.HasErrors() && val.Type() == cty.Bool {
						info.Sensitive = val.True()
					}
				}
			}

			if attr := block.Body().GetAttribute("default"); attr != nil {
				exprBytes := attr.Expr().BuildTokens(nil).Bytes()
				parsedExpr, diags := hclsyntax.ParseExpression(exprBytes, "default", hcl.Pos{Line: 1, Column: 1})
				if !diags.HasErrors() {
					val, diags := parsedExpr.Value(nil)
					if !diags.HasErrors() {
						info.Default = convertFromCtyValue(val)
					}
				}
			}

			variables = append(variables, info)
		}
	}

	return variables, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// addTfvarsHeader adds a Windsor CLI management header to the tfvars file body.
// It includes a module source comment if provided, ensuring users are aware of CLI management and module provenance.
func addTfvarsHeader(body *hclwrite.Body, source string) {
	windsorHeaderToken := "Managed by Windsor CLI:"
	headerComment := fmt.Sprintf("# %s This file is partially managed by the windsor CLI. Your changes will not be overwritten.", windsorHeaderToken)
	body.AppendUnstructuredTokens(hclwrite.Tokens{
		{Type: hclsyntax.TokenComment, Bytes: []byte(headerComment + "\n")},
	})
	if source != "" {
		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenComment, Bytes: []byte(fmt.Sprintf("# Module source: %s\n", source))},
		})
	}
}

// writeComponentValues writes all component-provided or default variable values to the tfvars file body.
// It comments out default values and descriptions for unset variables, and writes explicit values for set variables.
// Handles sensitive variables and preserves variable order from variables.tf.
func writeComponentValues(body *hclwrite.Body, values map[string]any, protectedValues map[string]bool, variables []VariableInfo) {
	for _, info := range variables {
		if protectedValues[info.Name] {
			continue
		}

		body.AppendNewline()

		if info.Description != "" {
			body.AppendUnstructuredTokens(hclwrite.Tokens{
				{Type: hclsyntax.TokenComment, Bytes: []byte("# " + info.Description)},
			})
			body.AppendNewline()
		}

		if val, exists := values[info.Name]; exists {
			writeVariable(body, info.Name, val, []VariableInfo{})
			continue
		}

		if info.Sensitive {
			body.AppendUnstructuredTokens(hclwrite.Tokens{
				{Type: hclsyntax.TokenComment, Bytes: []byte(fmt.Sprintf("# %s = \"(sensitive)\"", info.Name))},
			})
			body.AppendNewline()
			continue
		}

		if info.Default != nil {
			defaultVal := convertToCtyValue(info.Default)
			if !defaultVal.IsNull() {
				var rendered string
				if defaultVal.Type().IsObjectType() || defaultVal.Type().IsMapType() {
					var mapStr strings.Builder
					mapStr.WriteString(fmt.Sprintf("%s = %s", info.Name, formatValue(convertFromCtyValue(defaultVal))))
					rendered = mapStr.String()
				} else {
					rendered = fmt.Sprintf("%s = %s", info.Name, string(hclwrite.TokensForValue(defaultVal).Bytes()))
				}
				for _, line := range strings.Split(rendered, "\n") {
					body.AppendUnstructuredTokens(hclwrite.Tokens{
						{Type: hclsyntax.TokenComment, Bytes: []byte("# " + line)},
					})
					body.AppendNewline()
				}
				continue
			}
		}

		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenComment, Bytes: []byte(fmt.Sprintf("# %s = null", info.Name))},
		})
		body.AppendNewline()
	}
}

// writeDefaultValues writes only the default values from variables.tf to the tfvars file body.
// This is an alias for writeComponentValues with no explicit values, ensuring all defaults are commented.
func writeDefaultValues(body *hclwrite.Body, variables []VariableInfo, componentValues map[string]any) {
	writeComponentValues(body, componentValues, map[string]bool{}, variables)
}

// writeHeredoc writes a multi-line string value as a heredoc assignment in the tfvars file body.
// Used for YAML or other multi-line string values to preserve formatting.
func writeHeredoc(body *hclwrite.Body, name string, content string) {
	tokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenOHeredoc, Bytes: []byte("<<EOF")},
		{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
		{Type: hclsyntax.TokenStringLit, Bytes: []byte(content)},
		{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
		{Type: hclsyntax.TokenCHeredoc, Bytes: []byte("EOF")},
	}
	body.SetAttributeRaw(name, tokens)
	body.AppendNewline()
}

// writeVariable writes a single variable assignment to the tfvars file body.
// Handles descriptions, sensitive flags, multi-line strings, and object/map formatting.
// Ensures correct HCL syntax for all supported value types.
func writeVariable(body *hclwrite.Body, name string, value any, variables []VariableInfo) {
	var info *VariableInfo
	for _, v := range variables {
		if v.Name == name {
			info = &v
			break
		}
	}

	if info != nil && info.Description != "" {
		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenComment, Bytes: []byte("# " + info.Description)},
		})
		body.AppendNewline()
	}

	if info != nil && info.Sensitive {
		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenComment, Bytes: []byte(fmt.Sprintf("# %s = \"(sensitive)\"", name))},
		})
		body.AppendNewline()
		return
	}

	switch v := value.(type) {
	case string:
		if strings.Contains(v, "\n") {
			writeHeredoc(body, name, v)
			return
		}
	case map[string]any:
		rendered := formatValue(v)
		assignment := fmt.Sprintf("%s = %s", name, rendered)
		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenIdent, Bytes: []byte(assignment)},
		})
		body.AppendNewline()
		return
	}

	body.SetAttributeValue(name, convertToCtyValue(value))
}

// formatValue formats a Go value as a valid HCL literal string for tfvars output.
// Handles strings, lists, maps, nested objects, and nil values with proper indentation and quoting.
func formatValue(value any) string {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	case []any:
		if len(v) == 0 {
			return "[]"
		}
		var items []string
		for _, item := range v {
			items = append(items, formatValue(item))
		}
		return fmt.Sprintf("[%s]", strings.Join(items, ", "))
	case map[string]any:
		if len(v) == 0 {
			return "{}"
		}
		var pairs []string
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			val := v[k]
			formattedVal := formatValue(val)
			if formattedVal == "{}" || formattedVal == "[]" {
				pairs = append(pairs, fmt.Sprintf("%s = %s", k, formattedVal))
			} else {
				if strings.HasPrefix(formattedVal, "{") {
					indented := strings.ReplaceAll(formattedVal, "\n", "\n  ")
					pairs = append(pairs, fmt.Sprintf("%s = %s", k, indented))
				} else {
					pairs = append(pairs, fmt.Sprintf("%s = %s", k, formattedVal))
				}
			}
		}
		return fmt.Sprintf("{\n  %s\n}", strings.Join(pairs, "\n  "))
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// convertToCtyValue converts a Go value to a cty.Value for HCL serialization.
// Supports strings, numbers, booleans, lists, and maps; returns NilVal for unsupported types.
func convertToCtyValue(value any) cty.Value {
	switch v := value.(type) {
	case string:
		return cty.StringVal(v)
	case int:
		return cty.NumberIntVal(int64(v))
	case float64:
		return cty.NumberFloatVal(v)
	case bool:
		return cty.BoolVal(v)
	case []any:
		if len(v) == 0 {
			return cty.ListValEmpty(cty.DynamicPseudoType)
		}
		var ctyList []cty.Value
		for _, item := range v {
			ctyList = append(ctyList, convertToCtyValue(item))
		}
		return cty.ListVal(ctyList)
	case map[string]any:
		ctyMap := make(map[string]cty.Value)
		for key, val := range v {
			ctyMap[key] = convertToCtyValue(val)
		}
		return cty.ObjectVal(ctyMap)
	default:
		return cty.NilVal
	}
}

// convertFromCtyValue converts a cty.Value to its Go representation for use in tfvars generation.
// Handles all supported HCL types, including lists, maps, objects, and primitives.
func convertFromCtyValue(val cty.Value) any {
	if !val.IsKnown() || val.IsNull() {
		return nil
	}

	switch {
	case val.Type() == cty.String:
		return val.AsString()
	case val.Type() == cty.Number:
		bf := val.AsBigFloat()
		if bf.IsInt() {
			i, _ := bf.Int64()
			return i
		}
		f, _ := bf.Float64()
		return f
	case val.Type() == cty.Bool:
		return val.True()
	case val.Type().IsListType() || val.Type().IsTupleType() || val.Type().IsSetType():
		var list []any
		it := val.ElementIterator()
		for it.Next() {
			_, v := it.Element()
			list = append(list, convertFromCtyValue(v))
		}
		return list
	case val.Type().IsMapType() || val.Type().IsObjectType():
		m := make(map[string]any)
		it := val.ElementIterator()
		for it.Next() {
			k, v := it.Element()
			m[k.AsString()] = convertFromCtyValue(v)
		}
		return m
	default:
		return nil
	}
}

// findVariablesTfFileForComponent locates the variables.tf file for a given terraform component.
// It determines the location based on whether the component has a source:
// - If component has a source: .windsor/.tf_modules/<path>/variables.tf (generated modules)
// - If component has no source: terraform/<path>/variables.tf (local modules)
// Returns the path to the variables.tf file if found, or an error if not found.
func (g *TerraformGenerator) findVariablesTfFileForComponent(projectRoot string, component blueprintv1alpha1.TerraformComponent) (string, error) {
	var variablesTfPath string

	if component.Source != "" {
		// Component has a source, so it's a generated module in .tf_modules
		variablesTfPath = filepath.Join(projectRoot, ".windsor", ".tf_modules", component.Path, "variables.tf")
	} else {
		// Component has no source, so it's a local module
		variablesTfPath = filepath.Join(projectRoot, "terraform", component.Path, "variables.tf")
	}

	// Check if the variables.tf file exists
	if _, err := g.shims.Stat(variablesTfPath); err != nil {
		return "", fmt.Errorf("variables.tf not found for component %s at %s", component.Path, variablesTfPath)
	}

	return variablesTfPath, nil
}

// generateTfvarsFile generates a tfvars file at the specified path using the provided variables.tf file and component values.
// It parses the variables.tf file to extract variable definitions, merges them with the given component values (excluding protected values),
// and writes a formatted tfvars file. If the file already exists and reset mode is not enabled, the function skips writing.
// The function ensures the parent directory exists and returns an error if any file or directory operation fails.
func (g *TerraformGenerator) generateTfvarsFile(tfvarsFilePath, variablesTfPath string, componentValues map[string]any, source string) error {
	protectedValues := map[string]bool{
		"context_path": true,
		"os_type":      true,
		"context_id":   true,
	}

	if !g.reset {
		if err := g.checkExistingTfvarsFile(tfvarsFilePath); err != nil {
			if err == os.ErrExist {
				return nil
			}
			return err
		}
	}

	variables, err := g.parseVariablesFile(variablesTfPath, protectedValues)
	if err != nil {
		return fmt.Errorf("failed to parse variables.tf: %w", err)
	}

	mergedFile := hclwrite.NewEmptyFile()
	body := mergedFile.Body()

	addTfvarsHeader(body, source)

	if len(componentValues) > 0 {
		writeComponentValues(body, componentValues, protectedValues, variables)
	} else {
		writeDefaultValues(body, variables, componentValues)
	}

	parentDir := filepath.Dir(tfvarsFilePath)
	if err := g.shims.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := g.shims.WriteFile(tfvarsFilePath, mergedFile.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write tfvars file: %w", err)
	}

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure TerraformGenerator implements Generator
var _ Generator = (*TerraformGenerator)(nil)
