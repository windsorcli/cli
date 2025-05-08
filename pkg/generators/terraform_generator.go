package generators

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

// Write generates Terraform configuration files for all components in the blueprint.
// It creates the necessary directory structure and writes three types of files:
// 1. main.tf - Contains module source and variable references
// 2. variables.tf - Defines all variables used by the module
// 3. .tfvars - Contains actual variable values for each context
// The function preserves existing values in .tfvars files while adding new ones.
func (g *TerraformGenerator) Write() error {
	components := g.blueprintHandler.GetTerraformComponents()

	projectRoot, err := g.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	contextPath, err := g.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("failed to get config root: %w", err)
	}

	terraformFolderPath := filepath.Join(projectRoot, "terraform")
	if err := g.shims.MkdirAll(terraformFolderPath, 0755); err != nil {
		return fmt.Errorf("failed to create terraform directory: %w", err)
	}

	for _, component := range components {
		if component.Source != "" {
			if err := g.generateModuleShim(component); err != nil {
				return fmt.Errorf("failed to generate module shim: %w", err)
			}
		}

		if err := g.writeTfvarsFile(contextPath, component); err != nil {
			return fmt.Errorf("failed to write tfvars file: %w", err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// generateModuleShim creates a local reference to a remote Terraform module.
// Algorithm:
//  1. Create module directory in .tf_modules/<component.Path>/
//  2. Generate main.tf with module reference to original source
//  3. Run 'terraform init' to download the module
//  4. Locate the downloaded module in .terraform directory
//  5. Extract variable definitions from the original module
//  6. Create variables.tf with all variables from original module
//  7. Extract and map outputs from the original module to outputs.tf
//  8. This creates a local reference that maintains all variable definitions
//     while allowing Windsor to manage the module configuration
func (g *TerraformGenerator) generateModuleShim(component blueprintv1alpha1.TerraformComponent) error {
	moduleDir := component.FullPath
	if err := g.shims.MkdirAll(moduleDir, 0755); err != nil {
		return fmt.Errorf("failed to create module directory: %w", err)
	}

	mainContent := hclwrite.NewEmptyFile()
	block := mainContent.Body().AppendNewBlock("module", []string{"main"})
	body := block.Body()
	body.SetAttributeValue("source", cty.StringVal(component.Source))

	if err := g.shims.WriteFile(filepath.Join(moduleDir, "main.tf"), mainContent.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write main.tf: %w", err)
	}

	if err := g.shims.Chdir(moduleDir); err != nil {
		return fmt.Errorf("failed to change to module directory: %w", err)
	}

	contextPath, err := g.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("failed to get config root: %w", err)
	}
	tfDataDir := filepath.Join(contextPath, ".terraform", component.Path)
	if err := g.shims.Setenv("TF_DATA_DIR", tfDataDir); err != nil {
		return fmt.Errorf("failed to set TF_DATA_DIR: %w", err)
	}

	output, err := g.shell.ExecSilent("terraform", "init", "-migrate-state", "-upgrade")
	if err != nil {
		return fmt.Errorf("failed to initialize terraform: %w", err)
	}

	modulePath := ""
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "- main in") {
			parts := strings.Split(line, "- main in ")
			if len(parts) == 2 {
				modulePath = strings.TrimSpace(parts[1])
				break
			}
		}
	}

	if modulePath == "" {
		tfModulesPath := filepath.Join(moduleDir, ".tf_modules")
		variablesPath := filepath.Join(tfModulesPath, "variables.tf")
		if _, err := g.shims.Stat(variablesPath); err == nil {
			modulePath = tfModulesPath
		} else {
			return fmt.Errorf("failed to find module path in terraform init output")
		}
	}

	variablesPath := filepath.Join(modulePath, "variables.tf")
	variablesContent, err := g.shims.ReadFile(variablesPath)
	if err != nil {
		return fmt.Errorf("failed to read variables.tf: %w", err)
	}

	variablesFile, diags := hclwrite.ParseConfig(variablesContent, variablesPath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("failed to parse variables.tf: %w", diags)
	}

	shimMainContent := hclwrite.NewEmptyFile()
	shimBlock := shimMainContent.Body().AppendNewBlock("module", []string{"main"})
	shimBody := shimBlock.Body()
	shimBody.SetAttributeValue("source", cty.StringVal(component.Source))

	for _, block := range variablesFile.Body().Blocks() {
		if block.Type() == "variable" {
			labels := block.Labels()
			if len(labels) > 0 {
				shimBody.SetAttributeTraversal(labels[0], hcl.Traversal{
					hcl.TraverseRoot{Name: "var"},
					hcl.TraverseAttr{Name: labels[0]},
				})
			}
		}
	}

	if err := g.shims.WriteFile(filepath.Join(moduleDir, "main.tf"), shimMainContent.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write shim main.tf: %w", err)
	}

	if err := g.shims.WriteFile(filepath.Join(moduleDir, "variables.tf"), variablesContent, 0644); err != nil {
		return fmt.Errorf("failed to write shim variables.tf: %w", err)
	}

	outputsPath := filepath.Join(modulePath, "outputs.tf")
	if _, err := g.shims.Stat(outputsPath); err == nil {
		outputsContent, err := g.shims.ReadFile(outputsPath)
		if err != nil {
			return fmt.Errorf("failed to read outputs.tf: %w", err)
		}

		if err := g.shims.WriteFile(filepath.Join(moduleDir, "outputs.tf"), outputsContent, 0644); err != nil {
			return fmt.Errorf("failed to write shim outputs.tf: %w", err)
		}
	}

	return nil
}

// writeModuleFile creates a main.tf file that defines the Terraform module configuration.
// It sets up the module source and creates variable references for all defined variables.
// The function ensures proper HCL syntax and maintains consistent module structure.
func (g *TerraformGenerator) writeModuleFile(dirPath string, component blueprintv1alpha1.TerraformComponent) error {
	moduleContent := hclwrite.NewEmptyFile()

	block := moduleContent.Body().AppendNewBlock("module", []string{"main"})
	body := block.Body()

	body.SetAttributeValue("source", cty.StringVal(component.Source))

	variablesTfPath := filepath.Join(dirPath, "variables.tf")
	variablesContent, err := g.shims.ReadFile(variablesTfPath)
	if err != nil {
		return fmt.Errorf("failed to read variables.tf: %w", err)
	}

	variablesFile, diags := hclwrite.ParseConfig(variablesContent, variablesTfPath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("failed to parse variables.tf: %w", diags)
	}

	var variableNames []string
	for _, block := range variablesFile.Body().Blocks() {
		if block.Type() == "variable" && len(block.Labels()) > 0 {
			variableNames = append(variableNames, block.Labels()[0])
		}
	}
	sort.Strings(variableNames)

	for _, variableName := range variableNames {
		body.SetAttributeTraversal(variableName, hcl.Traversal{
			hcl.TraverseRoot{Name: "var"},
			hcl.TraverseAttr{Name: variableName},
		})
	}

	filePath := filepath.Join(dirPath, "main.tf")

	if err := g.shims.WriteFile(filePath, moduleContent.Bytes(), 0644); err != nil {
		return err
	}

	return nil
}

// writeTfvarsFile creates or updates a .tfvars file with variable values for the Terraform module.
// It uses variables.tf as the basis for variable definitions and allows component.Values to override specific values.
// The function maintains a header indicating Windsor CLI management and handles module source comments.
// If the file already exists, it will not be overwritten.
func (g *TerraformGenerator) writeTfvarsFile(dirPath string, component blueprintv1alpha1.TerraformComponent) error {
	protectedValues := map[string]bool{
		"context_path": true,
		"os_type":      true,
		"context_id":   true,
	}

	componentPath := filepath.Join(dirPath, "terraform", component.Path)
	tfvarsFilePath := componentPath + ".tfvars"
	variablesTfPath := filepath.Join(component.FullPath, "variables.tf")

	if err := g.checkExistingTfvarsFile(tfvarsFilePath); err != nil {
		if err == os.ErrExist {
			return nil
		}
		return err
	}

	mergedFile := hclwrite.NewEmptyFile()
	body := mergedFile.Body()

	addTfvarsHeader(body, component.Source)

	variables, err := g.parseVariablesFile(variablesTfPath, protectedValues)
	if err != nil {
		return err
	}

	if len(component.Values) > 0 {
		writeComponentValues(body, component.Values, protectedValues, variables)
	} else {
		writeDefaultValues(body, variables, component.Values)
	}

	parentDir := filepath.Dir(tfvarsFilePath)
	if err := g.shims.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := g.shims.WriteFile(tfvarsFilePath, mergedFile.Bytes(), 0644); err != nil {
		return fmt.Errorf("error writing tfvars file: %w", err)
	}

	return nil
}

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

// writeComponentValues writes component-provided values to the tfvars file.
// It processes all variables in the order they appear in variables.tf.
func writeComponentValues(body *hclwrite.Body, values map[string]any, protectedValues map[string]bool, variables []VariableInfo) {
	for _, info := range variables {
		if protectedValues[info.Name] {
			continue
		}

		body.AppendNewline()

		// Write description if available
		if info.Description != "" {
			body.AppendUnstructuredTokens(hclwrite.Tokens{
				{Type: hclsyntax.TokenComment, Bytes: []byte("// " + info.Description)},
			})
			body.AppendNewline()
		}

		// If value is provided in component values, use it
		if val, exists := values[info.Name]; exists {
			writeVariable(body, info.Name, val, []VariableInfo{}) // Pass empty variables to avoid duplicate description
			continue
		}

		// For sensitive values, write them as commented sensitive
		if info.Sensitive {
			body.AppendUnstructuredTokens(hclwrite.Tokens{
				{Type: hclsyntax.TokenComment, Bytes: []byte(fmt.Sprintf("// %s = \"(sensitive)\"", info.Name))},
			})
			body.AppendNewline()
			continue
		}

		// Comment out the default value or null
		defaultStr := "null"
		if info.Default != nil {
			defaultVal := convertToCtyValue(info.Default)
			if !defaultVal.IsNull() {
				defaultStr = string(hclwrite.TokensForValue(defaultVal).Bytes())
			}
		}
		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenComment, Bytes: []byte(fmt.Sprintf("// %s = %s", info.Name, defaultStr))},
		})
		body.AppendNewline()
	}
}

// writeDefaultValues writes default values from variables.tf to the tfvars file.
// This is now just an alias for writeComponentValues with empty values
func writeDefaultValues(body *hclwrite.Body, variables []VariableInfo, componentValues map[string]any) {
	writeComponentValues(body, componentValues, map[string]bool{}, variables)
}

// writeHeredoc writes a variable value as a heredoc.
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

// writeVariable writes a single variable to the tfvars file.
func writeVariable(body *hclwrite.Body, name string, value any, variables []VariableInfo) {
	// Find variable info
	var info *VariableInfo
	for _, v := range variables {
		if v.Name == name {
			info = &v
			break
		}
	}

	// Write description if available
	if info != nil && info.Description != "" {
		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenComment, Bytes: []byte("// " + info.Description)},
		})
		body.AppendNewline()
	}

	// Handle sensitive variables
	if info != nil && info.Sensitive {
		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenComment, Bytes: []byte(fmt.Sprintf("// %s = \"(sensitive)\"", name))},
		})
		body.AppendNewline()
		return
	}

	// Handle multiline strings with heredoc
	if str, ok := value.(string); ok && strings.Contains(str, "\n") {
		writeHeredoc(body, name, str)
		return
	}

	// Write normal variable
	body.SetAttributeValue(name, convertToCtyValue(value))
}

// addTfvarsHeader adds a header comment to the tfvars file indicating Windsor CLI management.
// It includes the module source if provided.
func addTfvarsHeader(body *hclwrite.Body, source string) {
	windsorHeaderToken := "Managed by Windsor CLI:"
	headerComment := fmt.Sprintf("// %s This file is partially managed by the windsor CLI. Your changes will not be overwritten.", windsorHeaderToken)
	body.AppendUnstructuredTokens(hclwrite.Tokens{
		{Type: hclsyntax.TokenComment, Bytes: []byte(headerComment + "\n")},
	})
	if source != "" {
		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenComment, Bytes: []byte(fmt.Sprintf("// Module source: %s\n", source))},
		})
	}
}

// convertToCtyValue converts various Go types to their corresponding cty.Value representation.
// It handles strings, numbers, booleans, lists, and maps, returning a NilVal for unsupported types.
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

// convertFromCtyValue converts a cty.Value to its corresponding Go value.
// This is the inverse of convertToCtyValue and is used when reading values from HCL.
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

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure TerraformGenerator implements Generator
var _ Generator = (*TerraformGenerator)(nil)
