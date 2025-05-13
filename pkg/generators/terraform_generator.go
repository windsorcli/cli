package generators

import (
	"encoding/json"
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

// TerraformInitOutput represents the JSON output from terraform init
type TerraformInitOutput struct {
	Level     string `json:"@level"`
	Message   string `json:"@message"`
	Module    string `json:"@module"`
	Timestamp string `json:"@timestamp"`
	Type      string `json:"type"`
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
// It provides a shim layer that maintains module configuration while allowing Windsor to manage it.
// The function orchestrates the creation of main.tf, variables.tf, and outputs.tf files.
// It ensures proper module initialization and state management.
func (g *TerraformGenerator) generateModuleShim(component blueprintv1alpha1.TerraformComponent) error {
	moduleDir := component.FullPath
	if err := g.shims.MkdirAll(moduleDir, 0755); err != nil {
		return fmt.Errorf("failed to create module directory: %w", err)
	}

	if err := g.writeShimMainTf(moduleDir, component.Source); err != nil {
		return err
	}

	if err := g.shims.Chdir(moduleDir); err != nil {
		return fmt.Errorf("failed to change to module directory: %w", err)
	}

	modulePath, err := g.initializeTerraformModule(component)
	if err != nil {
		return err
	}

	if err := g.writeShimVariablesTf(moduleDir, modulePath, component.Source); err != nil {
		return err
	}

	if err := g.writeShimOutputsTf(moduleDir, modulePath); err != nil {
		return err
	}

	return nil
}

// writeShimMainTf creates the main.tf file for the shim module.
// It provides the initial module configuration with source reference.
// The function ensures proper HCL syntax and maintains consistent module structure.
// It handles file writing with appropriate permissions and error handling.
func (g *TerraformGenerator) writeShimMainTf(moduleDir, source string) error {
	mainContent := hclwrite.NewEmptyFile()
	block := mainContent.Body().AppendNewBlock("module", []string{"main"})
	body := block.Body()
	body.SetAttributeValue("source", cty.StringVal(source))

	if err := g.shims.WriteFile(filepath.Join(moduleDir, "main.tf"), mainContent.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write main.tf: %w", err)
	}
	return nil
}

// initializeTerraformModule initializes the Terraform module and returns its path.
// It provides module initialization, path resolution, and environment setup.
// The function handles terraform init execution and module path detection.
// It ensures proper state directory configuration and error handling.
func (g *TerraformGenerator) initializeTerraformModule(component blueprintv1alpha1.TerraformComponent) (string, error) {
	contextPath, err := g.configHandler.GetConfigRoot()
	if err != nil {
		return "", fmt.Errorf("failed to get config root: %w", err)
	}

	tfDataDir := filepath.Join(contextPath, ".terraform", component.Path)
	if err := g.shims.Setenv("TF_DATA_DIR", tfDataDir); err != nil {
		return "", fmt.Errorf("failed to set TF_DATA_DIR: %w", err)
	}

	output, err := g.shell.ExecProgress(
		fmt.Sprintf("📥 Loading component %s", component.Path),
		"terraform",
		"init",
		"--backend=false",
		"-input=false",
		"-json",
	)
	if err != nil {
		return "", fmt.Errorf("failed to initialize terraform: %w", err)
	}

	detectedPath := ""
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var initOutput TerraformInitOutput
		if err := json.Unmarshal([]byte(line), &initOutput); err != nil {
			continue
		}
		if initOutput.Type == "log" {
			msg := initOutput.Message
			startIdx := strings.Index(msg, "- main in")
			if startIdx == -1 {
				continue
			}

			pathStart := startIdx + len("- main in")
			if pathStart >= len(msg) {
				continue
			}

			path := strings.TrimSpace(msg[pathStart:])
			if path == "" {
				continue
			}

			if _, err := g.shims.Stat(path); err == nil {
				detectedPath = path
				break
			}
		}
	}

	modulePath := filepath.Join(contextPath, ".terraform", component.Path, "modules", "main", "terraform", component.Path)
	if detectedPath != "" {
		if detectedPath != modulePath {
			fmt.Printf("\033[33mWarning: Using detected module path %s instead of standard path %s\033[0m\n", detectedPath, modulePath)
		}
		modulePath = detectedPath
	}

	return modulePath, nil
}

// writeShimVariablesTf creates the variables.tf file for the shim module.
// It provides variable definition extraction and shim generation.
// The function maintains variable references in main.tf and preserves descriptions.
// It handles file reading, parsing, and writing with proper error handling.
func (g *TerraformGenerator) writeShimVariablesTf(moduleDir, modulePath, source string) error {
	shimMainContent := hclwrite.NewEmptyFile()
	shimBlock := shimMainContent.Body().AppendNewBlock("module", []string{"main"})
	shimBody := shimBlock.Body()
	shimBody.SetAttributeRaw("source", hclwrite.TokensForValue(cty.StringVal(source)))

	variablesPath := filepath.Join(modulePath, "variables.tf")
	variablesContent, err := g.shims.ReadFile(variablesPath)
	if err == nil {
		variablesFile, diags := hclwrite.ParseConfig(variablesContent, variablesPath, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return fmt.Errorf("failed to parse variables.tf: %w", diags)
		}

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

		// Write variables.tf to shim dir
		shimVariablesPath := filepath.Join(moduleDir, "variables.tf")
		if err := g.shims.WriteFile(shimVariablesPath, variablesContent, 0644); err != nil {
			return fmt.Errorf("failed to write shim variables.tf: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read variables.tf: %w", err)
	}

	if err := g.shims.WriteFile(filepath.Join(moduleDir, "main.tf"), shimMainContent.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write shim main.tf: %w", err)
	}

	return nil
}

// writeShimOutputsTf creates the outputs.tf file for the shim module.
// It provides output definition extraction and shim generation.
// The function creates references to module.main outputs while preserving descriptions.
// It handles file reading, parsing, and writing with proper error handling.
func (g *TerraformGenerator) writeShimOutputsTf(moduleDir, modulePath string) error {
	outputsPath := filepath.Join(modulePath, "outputs.tf")
	if _, err := g.shims.Stat(outputsPath); err == nil {
		outputsContent, err := g.shims.ReadFile(outputsPath)
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

					// Copy description if present
					if attr := block.Body().GetAttribute("description"); attr != nil {
						shimBlockBody.SetAttributeRaw("description", attr.Expr().BuildTokens(nil))
					}

					// Set value to reference module.main output
					shimBlockBody.SetAttributeTraversal("value", hcl.Traversal{
						hcl.TraverseRoot{Name: "module"},
						hcl.TraverseAttr{Name: "main"},
						hcl.TraverseAttr{Name: outputName},
					})
				}
			}
		}

		if err := g.shims.WriteFile(filepath.Join(moduleDir, "outputs.tf"), shimOutputsContent.Bytes(), 0644); err != nil {
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
				if defaultVal.Type().IsObjectType() || defaultVal.Type().IsMapType() {
					// For objects/maps, format with proper indentation and comment each line
					var mapStr strings.Builder
					mapStr.WriteString(fmt.Sprintf("// %s = {\n", info.Name))
					it := defaultVal.ElementIterator()
					for it.Next() {
						k, v := it.Element()
						mapStr.WriteString(fmt.Sprintf("//   %s = %s\n", k.AsString(), formatValue(convertFromCtyValue(v))))
					}
					mapStr.WriteString("// }")
					body.AppendUnstructuredTokens(hclwrite.Tokens{
						{Type: hclsyntax.TokenComment, Bytes: []byte(mapStr.String())},
					})
					body.AppendNewline()
					continue
				} else {
					defaultStr = string(hclwrite.TokensForValue(defaultVal).Bytes())
				}
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

	// Handle multiline strings and maps with heredoc
	switch v := value.(type) {
	case string:
		if strings.Contains(v, "\n") {
			writeHeredoc(body, name, v)
			return
		}
	case map[string]any:
		// Convert map to HCL format string
		var mapStr strings.Builder
		mapStr.WriteString("{\n")
		for k, val := range v {
			mapStr.WriteString(fmt.Sprintf("  %s = %s\n", k, formatValue(val)))
		}
		mapStr.WriteString("}")
		writeHeredoc(body, name, mapStr.String())
		return
	}

	// Write normal variable
	body.SetAttributeValue(name, convertToCtyValue(value))
}

// formatValue formats a value for HCL output
func formatValue(value any) string {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	case []any:
		var items []string
		for _, item := range v {
			items = append(items, formatValue(item))
		}
		return fmt.Sprintf("[%s]", strings.Join(items, ", "))
	case map[string]any:
		var pairs []string
		for k, val := range v {
			pairs = append(pairs, fmt.Sprintf("%s = %s", k, formatValue(val)))
		}
		return fmt.Sprintf("{\n    %s\n  }", strings.Join(pairs, "\n    "))
	default:
		return fmt.Sprintf("%v", v)
	}
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
