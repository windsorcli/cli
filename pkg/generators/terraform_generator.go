package generators

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"

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

// =============================================================================
// Constructor
// =============================================================================

// NewTerraformGenerator creates a new TerraformGenerator
func NewTerraformGenerator(injector di.Injector) *TerraformGenerator {
	return &TerraformGenerator{
		BaseGenerator: BaseGenerator{injector: injector},
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Write generates the Terraform files for all components defined in the blueprint.
// It creates the necessary directory structure and writes module, variable, and tfvars files.
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
			if err := g.shims.MkdirAll(component.FullPath, 0755); err != nil {
				return fmt.Errorf("failed to create component directory: %w", err)
			}

			if err := g.writeModuleFile(component.FullPath, component); err != nil {
				return fmt.Errorf("failed to write module file: %w", err)
			}

			if err := g.writeVariableFile(component.FullPath, component); err != nil {
				return fmt.Errorf("failed to write variable file: %w", err)
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

// writeModuleFile creates a Terraform module file that defines the module source and variables.
// It generates a main.tf file with the module configuration and variable references.
func (g *TerraformGenerator) writeModuleFile(dirPath string, component blueprintv1alpha1.TerraformComponent) error {
	moduleContent := hclwrite.NewEmptyFile()

	block := moduleContent.Body().AppendNewBlock("module", []string{"main"})
	body := block.Body()

	body.SetAttributeValue("source", cty.StringVal(component.Source))

	var keys []string
	for key := range component.Variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, variableName := range keys {
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

// writeVariableFile generates a variables.tf file that defines all variables used by the module.
// It creates variable blocks with type, default value, description, and sensitivity settings.
func (g *TerraformGenerator) writeVariableFile(dirPath string, component blueprintv1alpha1.TerraformComponent) error {
	variablesContent := hclwrite.NewEmptyFile()
	body := variablesContent.Body()

	var keys []string
	for key := range component.Variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, variableName := range keys {
		variable := component.Variables[variableName]
		block := body.AppendNewBlock("variable", []string{variableName})
		blockBody := block.Body()

		if variable.Type != "" {
			blockBody.SetAttributeRaw("type", hclwrite.TokensForIdentifier(variable.Type))
		}

		if variable.Default != nil {
			defaultValue := convertToCtyValue(variable.Default)
			blockBody.SetAttributeValue("default", defaultValue)
		}

		if variable.Description != "" {
			blockBody.SetAttributeValue("description", cty.StringVal(variable.Description))
		}

		if variable.Sensitive {
			blockBody.SetAttributeValue("sensitive", cty.BoolVal(variable.Sensitive))
		}
	}

	varFilePath := filepath.Join(dirPath, "variables.tf")

	if err := g.shims.WriteFile(varFilePath, variablesContent.Bytes(), 0644); err != nil {
		return err
	}

	return nil
}

// writeTfvarsFile creates or updates a .tfvars file with variable values for the Terraform module.
// It preserves existing values while adding new ones, and includes descriptive comments for each variable.
func (g *TerraformGenerator) writeTfvarsFile(dirPath string, component blueprintv1alpha1.TerraformComponent) error {
	componentPath := filepath.Join(dirPath, "terraform", component.Path)
	tfvarsFilePath := componentPath + ".tfvars"

	parentDir := filepath.Dir(tfvarsFilePath)
	if err := g.shims.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	windsorHeaderToken := "Managed by Windsor CLI:"
	headerComment := fmt.Sprintf("// %s This file is partially managed by the windsor CLI. Your changes will not be overwritten.", windsorHeaderToken)

	var existingContent []byte
	if _, err := g.shims.Stat(tfvarsFilePath); err == nil {
		existingContent, err = g.shims.ReadFile(tfvarsFilePath)
		if err != nil {
			return fmt.Errorf("failed to read existing tfvars file: %w", err)
		}
	}

	remainder := existingContent

	mergedFile := hclwrite.NewEmptyFile()
	body := mergedFile.Body()

	if len(remainder) > 0 {
		parsedFile, parseErr := hclwrite.ParseConfig(remainder, tfvarsFilePath, hcl.Pos{Line: 1, Column: 1})
		if parseErr != nil {
			return fmt.Errorf("failed to parse existing tfvars content: %w", parseErr)
		}
		mergedFile = parsedFile
		body = mergedFile.Body()
	}

	existingComments := make(map[string]bool)
	for _, token := range mergedFile.Body().BuildTokens(nil) {
		if token.Type == hclsyntax.TokenComment {
			commentLine := string(bytes.TrimSpace(token.Bytes))
			existingComments[commentLine] = true
		}
	}

	variableComments := make(map[string]string)
	var keys []string
	for key := range component.Variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, variableName := range keys {
		if variableDef, hasVar := component.Variables[variableName]; hasVar && variableDef.Description != "" {
			commentText := fmt.Sprintf("// %s", variableDef.Description)
			variableComments[variableName] = commentText
		}
	}

	keys = nil
	for k := range component.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, variableName := range keys {
		if body.GetAttribute(variableName) != nil {
			continue
		}

		if commentText, exists := variableComments[variableName]; exists && !existingComments[commentText] {
			body.AppendUnstructuredTokens(hclwrite.Tokens{
				{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
				{Type: hclsyntax.TokenComment, Bytes: []byte(commentText)},
				{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
			})
			existingComments[commentText] = true
		}

		ctyVal := convertToCtyValue(component.Values[variableName])
		body.SetAttributeValue(variableName, ctyVal)
	}

	finalOutput := mergedFile.Bytes()

	if !bytes.Contains(bytes.ToLower(finalOutput), bytes.ToLower([]byte(windsorHeaderToken))) {
		var headerBuffer bytes.Buffer
		headerBuffer.WriteString(headerComment)
		headerBuffer.WriteByte('\n')
		if component.Source != "" && !bytes.Contains(bytes.ToLower(finalOutput), bytes.ToLower([]byte("// Module source:"))) {
			headerBuffer.WriteString(fmt.Sprintf("// Module source: %s\n", component.Source))
		}

		finalOutput = append(headerBuffer.Bytes(), finalOutput...)
	}

	finalOutput = bytes.TrimRight(finalOutput, "\n")
	finalOutput = append(finalOutput, '\n')

	if err := g.shims.WriteFile(tfvarsFilePath, finalOutput, 0644); err != nil {
		return fmt.Errorf("error writing tfvars file: %w", err)
	}

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure TerraformGenerator implements Generator
var _ Generator = (*TerraformGenerator)(nil)

// =============================================================================
// Helpers
// =============================================================================

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
