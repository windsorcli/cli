package generators

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/zclconf/go-cty/cty"
)

// TerraformGenerator is a generator that writes Terraform files
type TerraformGenerator struct {
	BaseGenerator
}

// NewTerraformGenerator creates a new TerraformGenerator
func NewTerraformGenerator(injector di.Injector) *TerraformGenerator {
	return &TerraformGenerator{
		BaseGenerator: BaseGenerator{injector: injector},
	}
}

// Write generates the Terraform files
func (g *TerraformGenerator) Write() error {
	components := g.blueprintHandler.GetTerraformComponents()

	// Get the project root
	projectRoot, err := g.shell.GetProjectRoot()
	if err != nil {
		return err
	}

	// Get the context path
	contextPath, err := g.configHandler.GetConfigRoot()
	if err != nil {
		return err
	}

	// Ensure the "terraform" folder exists in the project root
	terraformFolderPath := filepath.Join(projectRoot, "terraform")
	if err := osMkdirAll(terraformFolderPath, os.ModePerm); err != nil {
		return err
	}

	// Write the Terraform files
	for _, component := range components {
		// Check if the component path is within the .tf_modules folder
		if component.Source != "" {
			// Ensure the parent directories exist
			if err := osMkdirAll(component.FullPath, os.ModePerm); err != nil {
				return err
			}

			// Write the module file
			if err := g.writeModuleFile(component.FullPath, component); err != nil {
				return err
			}

			// Write the variables file
			if err := g.writeVariableFile(component.FullPath, component); err != nil {
				return err
			}
		}

		// Write the tfvars file
		if err := g.writeTfvarsFile(contextPath, component); err != nil {
			return err
		}
	}

	return nil
}

// writeModule writes the Terraform module file for the given component
func (g *TerraformGenerator) writeModuleFile(dirPath string, component blueprintv1alpha1.TerraformComponent) error {
	// Create a new empty HCL file
	moduleContent := hclwrite.NewEmptyFile()

	// Append a new block for the module with the component's name
	block := moduleContent.Body().AppendNewBlock("module", []string{"main"})
	body := block.Body()

	// Set the source attribute
	body.SetAttributeValue("source", cty.StringVal(component.Source))

	// Get the keys from the Variables map and sort them alphabetically
	var keys []string
	for key := range component.Variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Directly map variable names to var.<variable_name> in alphabetical order
	for _, variableName := range keys {
		body.SetAttributeTraversal(variableName, hcl.Traversal{
			hcl.TraverseRoot{Name: "var"},
			hcl.TraverseAttr{Name: variableName},
		})
	}

	// Define the file path for the module file
	filePath := filepath.Join(dirPath, "main.tf")

	// Write the module content to the file
	if err := osWriteFile(filePath, moduleContent.Bytes(), 0644); err != nil {
		return err
	}

	return nil
}

// writeVariableFile generates and writes the Terraform variable definitions to a file.
func (g *TerraformGenerator) writeVariableFile(dirPath string, component blueprintv1alpha1.TerraformComponent) error {
	// Create a new empty HCL file to hold variable definitions.
	variablesContent := hclwrite.NewEmptyFile()
	body := variablesContent.Body()

	// Get the keys from the Variables map and sort them alphabetically
	var keys []string
	for key := range component.Variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Iterate over each key in the sorted order to define it as a variable in the HCL file.
	for _, variableName := range keys {
		variable := component.Variables[variableName]
		// Create a new block for each variable with its name.
		block := body.AppendNewBlock("variable", []string{variableName})
		blockBody := block.Body()

		// Set the type attribute if it exists (unquoted for Terraform 0.12+)
		if variable.Type != "" {
			// Use TokensForIdentifier to set the type attribute
			blockBody.SetAttributeRaw("type", hclwrite.TokensForIdentifier(variable.Type))
		}

		// Set the default attribute if it exists
		if variable.Default != nil {
			// Use a generic approach to handle various data types for the default value
			defaultValue := convertToCtyValue(variable.Default)
			blockBody.SetAttributeValue("default", defaultValue)
		}

		// Set the description attribute if it exists
		if variable.Description != "" {
			blockBody.SetAttributeValue("description", cty.StringVal(variable.Description))
		}

		// Set the sensitive attribute if it exists
		if variable.Sensitive {
			blockBody.SetAttributeValue("sensitive", cty.BoolVal(variable.Sensitive))
		}
	}

	// Define the path for the variables file.
	varFilePath := filepath.Join(dirPath, "variables.tf")

	// Write the variable definitions to the file.
	if err := osWriteFile(varFilePath, variablesContent.Bytes(), 0644); err != nil {
		return err
	}

	return nil
}

// writeTfvarsFile orchestrates writing a .tfvars file for the specified Terraform component,
// preserving existing attributes and integrating any new values. If the component includes a
// 'source' attribute, it indicates the component's origin or external module reference.
func (g *TerraformGenerator) writeTfvarsFile(dirPath string, component blueprintv1alpha1.TerraformComponent) error {
	// Define the path for the tfvars file relative to the component's path.
	componentPath := filepath.Join(dirPath, "terraform", component.Path)
	tfvarsFilePath := componentPath + ".tfvars"

	// Ensure the parent directories exist
	parentDir := filepath.Dir(tfvarsFilePath)
	if err := osMkdirAll(parentDir, os.ModePerm); err != nil {
		return fmt.Errorf("error creating directories for path %s: %w", parentDir, err)
	}

	// We'll define a unique token to identify our managed header line. This allows changing
	// the exact header message in the future as long as we keep this token present.
	windsorHeaderToken := "Managed by Windsor CLI:"

	// The actual user-facing header message. We can adjust this text in future changes
	// while still identifying the line via the token.
	headerComment := fmt.Sprintf("// %s This file is partially managed by the windsor CLI. Your changes will not be overwritten.", windsorHeaderToken)

	// Read the existing tfvars file if it exists. We do not remove existing lines or attributes
	// so that the original file content takes precedence.
	var existingContent []byte
	if _, err := osStat(tfvarsFilePath); err == nil {
		existingContent, err = osReadFile(tfvarsFilePath)
		if err != nil {
			return fmt.Errorf("error reading existing tfvars file: %w", err)
		}
	}

	// Use the existing file content as the basis for merging
	remainder := existingContent

	// Parse the existing file content to build the mergedFile
	mergedFile := hclwrite.NewEmptyFile()
	body := mergedFile.Body()

	if len(remainder) > 0 {
		parsedFile, parseErr := hclwrite.ParseConfig(remainder, tfvarsFilePath, hcl.Pos{Line: 1, Column: 1})
		if parseErr != nil {
			return fmt.Errorf("unable to parse existing tfvars content: %w", parseErr)
		}
		mergedFile = parsedFile
		body = mergedFile.Body()
	}

	// Collect existing comments from the merged file so we don't duplicate them
	existingComments := make(map[string]bool)
	for _, token := range mergedFile.Body().BuildTokens(nil) {
		if token.Type == hclsyntax.TokenComment {
			commentLine := string(bytes.TrimSpace(token.Bytes))
			existingComments[commentLine] = true
		}
	}

	// Create a map of variable names to comments from the component's variable definitions
	variableComments := make(map[string]string)
	var keys []string
	for key := range component.Variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Collect comments for each variable from component.Variables
	for _, variableName := range keys {
		if variableDef, hasVar := component.Variables[variableName]; hasVar && variableDef.Description != "" {
			commentText := fmt.Sprintf("// %s", variableDef.Description)
			variableComments[variableName] = commentText
		}
	}

	// Sort the values keys from the component so we add or update them in deterministic order
	keys = nil // reuse the slice
	for k := range component.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// For each key in component.Values, add or update only if it doesn't already exist in the merged file
	for _, variableName := range keys {
		// If an attribute already exists for this variable, keep the existing value; do not overwrite it.
		if body.GetAttribute(variableName) != nil {
			continue
		}

		// If we have a comment for the variable and it's not already present, add it
		if commentText, exists := variableComments[variableName]; exists && !existingComments[commentText] {
			body.AppendUnstructuredTokens(hclwrite.Tokens{
				{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
				{Type: hclsyntax.TokenComment, Bytes: []byte(commentText)},
				{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
			})
			existingComments[commentText] = true
		}

		// Convert and set the new value
		ctyVal := convertToCtyValue(component.Values[variableName])
		body.SetAttributeValue(variableName, ctyVal)
	}

	// Build the final content. If the header token isn't in the existing file, prepend it.
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

	// Ensure there's exactly one newline at the end
	finalOutput = bytes.TrimRight(finalOutput, "\n")
	finalOutput = append(finalOutput, '\n')

	// Write the merged content to disk
	if err := osWriteFile(tfvarsFilePath, finalOutput, 0644); err != nil {
		return fmt.Errorf("error writing tfvars file: %w", err)
	}

	return nil
}

// Ensure TerraformGenerator implements Generator
var _ Generator = (*TerraformGenerator)(nil)

// convertToCtyValue converts an any to a cty.Value, handling various data types.
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
