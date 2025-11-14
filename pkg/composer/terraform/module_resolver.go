package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/zclconf/go-cty/cty"
)

// The ModuleResolver is a terraform module source resolution and configuration system.
// It provides functionality to process different types of terraform module sources including standard git/local sources and OCI artifacts.
// The ModuleResolver acts as the central orchestrator for terraform module management in Windsor projects,
// coordinating module extraction, shim generation, and configuration file creation for proper terraform initialization.

// =============================================================================
// Interfaces
// =============================================================================

// ModuleResolver processes terraform module sources and generates appropriate module configurations
type ModuleResolver interface {
	ProcessModules() error
	GenerateTfvars(overwrite bool) error
}

// =============================================================================
// Types
// =============================================================================

// BaseModuleResolver provides common functionality for all module resolvers
type BaseModuleResolver struct {
	shims            *Shims
	runtime          *runtime.Runtime
	blueprintHandler blueprint.BlueprintHandler
	reset            bool
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseModuleResolver creates a new base module resolver with the provided dependencies.
// If overrides are provided, any non-nil component in the override BaseModuleResolver will be used instead of creating a default.
func NewBaseModuleResolver(rt *runtime.Runtime, blueprintHandler blueprint.BlueprintHandler, opts ...*BaseModuleResolver) *BaseModuleResolver {
	resolver := &BaseModuleResolver{
		shims:            NewShims(),
		runtime:          rt,
		blueprintHandler: blueprintHandler,
	}

	if len(opts) > 0 && opts[0] != nil {
		overrides := opts[0]
		if overrides.shims != nil {
			resolver.shims = overrides.shims
		}
	}

	return resolver
}

// GenerateTfvars creates Terraform configuration files, including tfvars files, for all blueprint components.
// It processes template data keyed by "terraform/<module_path>", generating tfvars files at
// contexts/<context>/terraform/<module_path>.tfvars. For each entry in the input data, it skips keys
// not prefixed with "terraform/" and skips components not present in the blueprint. For all components
// in the blueprint, it ensures a tfvars file is generated if not already handled by the input data.
// The method uses the blueprint handler to retrieve TerraformComponents and determines the variables.tf
// location based on component source (remote or local). Module resolution is handled by pkg/terraform.
func (h *BaseModuleResolver) GenerateTfvars(overwrite bool) error {
	h.reset = overwrite

	contextPath := h.runtime.ConfigRoot
	projectRoot := h.runtime.ProjectRoot

	components := h.blueprintHandler.GetTerraformComponents()

	for _, component := range components {
		componentValues := component.Inputs
		if componentValues == nil {
			componentValues = make(map[string]any)
		}

		if err := h.generateComponentTfvars(projectRoot, contextPath, component, componentValues); err != nil {
			return fmt.Errorf("failed to generate tfvars for component %s: %w", component.Path, err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// VariableInfo holds metadata for a single Terraform variable
type VariableInfo struct {
	Name        string
	Description string
	Default     any
	Sensitive   bool
}

// checkExistingTfvarsFile checks if a tfvars file exists and is readable.
// Returns os.ErrExist if the file exists and is readable, or an error if the file exists but is not readable.
func (h *BaseModuleResolver) checkExistingTfvarsFile(tfvarsFilePath string) error {
	_, err := h.shims.Stat(tfvarsFilePath)
	if err == nil {
		_, err := h.shims.ReadFile(tfvarsFilePath)
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
func (h *BaseModuleResolver) parseVariablesFile(variablesTfPath string, protectedValues map[string]bool) ([]VariableInfo, error) {
	variablesContent, err := h.shims.ReadFile(variablesTfPath)
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

// generateComponentTfvars generates tfvars files for a single Terraform component.
// For components with a non-empty Source, only the module tfvars file is generated at .windsor/.tf_modules/<component.Path>/terraform.tfvars.
// For components with an empty Source, only the context tfvars file is generated at <contextPath>/terraform/<component.Path>.tfvars.
// Returns an error if variables.tf cannot be found or if tfvars file generation fails.
func (h *BaseModuleResolver) generateComponentTfvars(projectRoot, contextPath string, component blueprintv1alpha1.TerraformComponent, componentValues map[string]any) error {
	variablesTfPath, err := h.findVariablesTfFileForComponent(projectRoot, component)
	if err != nil {
		return fmt.Errorf("failed to find variables.tf for component %s: %w", component.Path, err)
	}

	if component.Source != "" {
		moduleTfvarsPath := filepath.Join(projectRoot, ".windsor", ".tf_modules", component.Path, "terraform.tfvars")
		if err := h.removeTfvarsFiles(filepath.Dir(moduleTfvarsPath)); err != nil {
			return fmt.Errorf("failed cleaning existing .tfvars in module dir %s: %w", filepath.Dir(moduleTfvarsPath), err)
		}
		if err := h.generateTfvarsFile(moduleTfvarsPath, variablesTfPath, componentValues, component.Source); err != nil {
			return fmt.Errorf("failed to generate module tfvars file: %w", err)
		}
	} else {
		terraformKey := "terraform/" + component.Path
		tfvarsFilePath := filepath.Join(contextPath, terraformKey+".tfvars")
		if err := h.generateTfvarsFile(tfvarsFilePath, variablesTfPath, componentValues, component.Source); err != nil {
			return fmt.Errorf("failed to generate context tfvars file: %w", err)
		}
	}

	return nil
}

// findVariablesTfFileForComponent returns the path to the variables.tf file for the specified Terraform component.
// If the component has a non-empty Source, the path is .windsor/.tf_modules/<component.Path>/variables.tf under the project root.
// If the component has an empty Source, the path is terraform/<component.Path>/variables.tf under the project root.
// Returns the variables.tf file path if it exists, or an error if not found.
func (h *BaseModuleResolver) findVariablesTfFileForComponent(projectRoot string, component blueprintv1alpha1.TerraformComponent) (string, error) {
	var variablesTfPath string

	if component.Source != "" {
		variablesTfPath = filepath.Join(projectRoot, ".windsor", ".tf_modules", component.Path, "variables.tf")
	} else {
		variablesTfPath = filepath.Join(projectRoot, "terraform", component.Path, "variables.tf")
	}

	if _, err := h.shims.Stat(variablesTfPath); err != nil {
		return "", fmt.Errorf("variables.tf not found for component %s at %s", component.Path, variablesTfPath)
	}

	return variablesTfPath, nil
}

// generateTfvarsFile generates a tfvars file at the specified path using the provided variables.tf file and component values.
// It parses the variables.tf file to extract variable definitions, merges them with the given component values (excluding protected values),
// and writes a formatted tfvars file. If the file already exists and reset mode is not enabled, the function skips writing.
// The function ensures the parent directory exists and returns an error if any file or directory operation fails.
func (h *BaseModuleResolver) generateTfvarsFile(tfvarsFilePath, variablesTfPath string, componentValues map[string]any, source string) error {
	protectedValues := map[string]bool{
		"context_path": true,
		"os_type":      true,
		"context_id":   true,
	}

	if !h.reset {
		if err := h.checkExistingTfvarsFile(tfvarsFilePath); err != nil {
			if err == os.ErrExist {
				return nil
			}
			return err
		}
	}

	variables, err := h.parseVariablesFile(variablesTfPath, protectedValues)
	if err != nil {
		return fmt.Errorf("failed to parse variables.tf: %w", err)
	}

	mergedFile := hclwrite.NewEmptyFile()
	body := mergedFile.Body()

	addTfvarsHeader(body, source)

	if len(componentValues) > 0 {
		writeComponentValues(body, componentValues, protectedValues, variables)
	} else {
		writeComponentValues(body, componentValues, map[string]bool{}, variables)
	}

	parentDir := filepath.Dir(tfvarsFilePath)
	if err := h.shims.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := h.shims.WriteFile(tfvarsFilePath, mergedFile.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write tfvars file: %w", err)
	}

	return nil
}

// removeTfvarsFiles removes any .tfvars files directly under the specified directory.
// This is used to ensure module directories do not retain stale tfvars prior to regeneration.
func (h *BaseModuleResolver) removeTfvarsFiles(dir string) error {
	if _, err := h.shims.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	entries, err := h.shims.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(strings.ToLower(name), ".tfvars") {
			fullPath := filepath.Join(dir, name)
			if err := h.shims.RemoveAll(fullPath); err != nil {
				return err
			}
		}
	}

	return nil
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
	case []string:
		if len(v) == 0 {
			return "[]"
		}
		var items []string
		for _, item := range v {
			items = append(items, fmt.Sprintf("%q", item))
		}
		return fmt.Sprintf("[%s]", strings.Join(items, ", "))
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
	case []string:
		if len(v) == 0 {
			return cty.ListValEmpty(cty.String)
		}
		var ctyList []cty.Value
		for _, item := range v {
			ctyList = append(ctyList, cty.StringVal(item))
		}
		return cty.ListVal(ctyList)
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

// writeShimMainTf creates the main.tf file for the shim module by generating a module block
// that references the specified source. It creates an HCL configuration with a single module
// block named "main" that points to the provided source location, then writes the generated
// configuration to main.tf in the specified module directory.
func (h *BaseModuleResolver) writeShimMainTf(moduleDir, source string) error {
	mainContent := hclwrite.NewEmptyFile()
	block := mainContent.Body().AppendNewBlock("module", []string{"main"})
	body := block.Body()
	body.SetAttributeValue("source", cty.StringVal(source))

	if err := h.shims.WriteFile(filepath.Join(moduleDir, "main.tf"), mainContent.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write main.tf: %w", err)
	}
	return nil
}

// writeShimVariablesTf creates the variables.tf file for the shim module by reading the original
// module's variables.tf file and generating corresponding variable blocks and module arguments.
// It parses variable definitions from the source module, creates shim variable blocks that preserve
// all attributes (description, type, default, sensitive), and configures the main module block
// to pass through all variables using var.variable_name references.
func (h *BaseModuleResolver) writeShimVariablesTf(moduleDir, modulePath, source string) error {
	shimMainContent := hclwrite.NewEmptyFile()
	shimBlock := shimMainContent.Body().AppendNewBlock("module", []string{"main"})
	shimBody := shimBlock.Body()
	shimBody.SetAttributeRaw("source", hclwrite.TokensForValue(cty.StringVal(source)))

	variablesPath := filepath.Join(modulePath, "variables.tf")
	if _, err := h.shims.Stat(variablesPath); err != nil {
		if err := h.shims.WriteFile(filepath.Join(moduleDir, "variables.tf"), []byte{}, 0644); err != nil {
			return fmt.Errorf("failed to write empty variables.tf: %w", err)
		}
		if err := h.shims.WriteFile(filepath.Join(moduleDir, "main.tf"), shimMainContent.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write shim main.tf: %w", err)
		}
		return nil
	}

	variablesContent, err := h.shims.ReadFile(variablesPath)
	if err != nil {
		return fmt.Errorf("failed to read variables.tf: %w", err)
	}

	variablesFile, diags := hclwrite.ParseConfig(variablesContent, variablesPath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("failed to parse variables.tf: %w", diags)
	}

	shimVariablesContent := hclwrite.NewEmptyFile()
	shimVariablesBody := shimVariablesContent.Body()

	for _, block := range variablesFile.Body().Blocks() {
		if block.Type() == "variable" {
			labels := block.Labels()
			if len(labels) > 0 {
				variableName := labels[0]

				shimBody.SetAttributeTraversal(variableName, hcl.Traversal{
					hcl.TraverseRoot{Name: "var"},
					hcl.TraverseAttr{Name: variableName},
				})

				shimBlock := shimVariablesBody.AppendNewBlock("variable", []string{variableName})
				shimBlockBody := shimBlock.Body()

				if attr := block.Body().GetAttribute("description"); attr != nil {
					shimBlockBody.SetAttributeRaw("description", attr.Expr().BuildTokens(nil))
				}

				if attr := block.Body().GetAttribute("type"); attr != nil {
					shimBlockBody.SetAttributeRaw("type", attr.Expr().BuildTokens(nil))
				}

				if attr := block.Body().GetAttribute("default"); attr != nil {
					shimBlockBody.SetAttributeRaw("default", attr.Expr().BuildTokens(nil))
				}

				if attr := block.Body().GetAttribute("sensitive"); attr != nil {
					shimBlockBody.SetAttributeRaw("sensitive", attr.Expr().BuildTokens(nil))
				}
			}
		}
	}

	if err := h.shims.WriteFile(filepath.Join(moduleDir, "variables.tf"), shimVariablesContent.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write shim variables.tf: %w", err)
	}

	if err := h.shims.WriteFile(filepath.Join(moduleDir, "main.tf"), shimMainContent.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write shim main.tf: %w", err)
	}

	return nil
}

// writeShimOutputsTf creates the outputs.tf file for the shim module by reading the original
// module's outputs.tf file and generating corresponding output blocks that reference the
// main module. It preserves output descriptions and creates value references using the
// module.main.output_name traversal pattern.
func (h *BaseModuleResolver) writeShimOutputsTf(moduleDir, modulePath string) error {
	outputsPath := filepath.Join(modulePath, "outputs.tf")
	if _, err := h.shims.Stat(outputsPath); err == nil {
		outputsContent, err := h.shims.ReadFile(outputsPath)
		if err != nil {
			return fmt.Errorf("failed to read outputs.tf: %w", err)
		}

		outputsFile, diags := hclwrite.ParseConfig(outputsContent, outputsPath, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return fmt.Errorf("failed to parse outputs.tf: %w", diags)
		}

		shimOutputsContent := hclwrite.NewEmptyFile()
		shimBody := shimOutputsContent.Body()

		for _, block := range outputsFile.Body().Blocks() {
			if block.Type() == "output" {
				labels := block.Labels()
				if len(labels) > 0 {
					outputName := labels[0]
					shimBlock := shimBody.AppendNewBlock("output", []string{outputName})
					shimBlockBody := shimBlock.Body()

					if attr := block.Body().GetAttribute("description"); attr != nil {
						shimBlockBody.SetAttributeRaw("description", attr.Expr().BuildTokens(nil))
					}

					if attr := block.Body().GetAttribute("sensitive"); attr != nil {
						shimBlockBody.SetAttributeRaw("sensitive", attr.Expr().BuildTokens(nil))
					}

					shimBlockBody.SetAttributeTraversal("value", hcl.Traversal{
						hcl.TraverseRoot{Name: "module"},
						hcl.TraverseAttr{Name: "main"},
						hcl.TraverseAttr{Name: outputName},
					})
				}
			}
		}

		if err := h.shims.WriteFile(filepath.Join(moduleDir, "outputs.tf"), shimOutputsContent.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write shim outputs.tf: %w", err)
		}
	}
	return nil
}
