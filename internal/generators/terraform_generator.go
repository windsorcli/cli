package generators

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/windsorcli/cli/internal/blueprint"
	"github.com/windsorcli/cli/internal/di"
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

	// Get the context path
	contextPath, err := g.contextHandler.GetConfigRoot()
	if err != nil {
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
func (g *TerraformGenerator) writeModuleFile(dirPath string, component blueprint.TerraformComponentV1Alpha1) error {
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
func (g *TerraformGenerator) writeVariableFile(dirPath string, component blueprint.TerraformComponentV1Alpha1) error {
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
			defaultValue, err := convertToCtyValue(variable.Default)
			if err != nil {
				return fmt.Errorf("error converting default value for variable %s: %w", variableName, err)
			}
			blockBody.SetAttributeValue("default", defaultValue)
		}

		// Set the description attribute if it exists
		if variable.Description != "" {
			blockBody.SetAttributeValue("description", cty.StringVal(variable.Description))
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

// writeTfvarsFile writes the Terraform tfvars file for the given component
func (g *TerraformGenerator) writeTfvarsFile(dirPath string, component blueprint.TerraformComponentV1Alpha1) error {

	// Define the path for the tfvars file relative to the component's path.
	componentPath := filepath.Join(dirPath, "terraform", component.Path)
	tfvarsFilePath := componentPath + ".tfvars"

	// Check if the file already exists. If it does, do nothing.
	if _, err := os.Stat(tfvarsFilePath); err == nil {
		return nil
	}

	// Create a new empty HCL file to hold variable definitions.
	variablesContent := hclwrite.NewEmptyFile()
	body := variablesContent.Body()

	// Get the keys from the Values map and sort them alphabetically
	var keys []string
	for key := range component.Values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Iterate over each key in the sorted order to define it as a variable in the HCL file.
	for _, variableName := range keys {
		value := component.Values[variableName]

		// Convert the value to a cty.Value
		ctyValue, err := convertToCtyValue(value)
		if err != nil {
			return fmt.Errorf("error converting value for variable %s: %w", variableName, err)
		}

		// Add a description comment before each variable
		if variable, exists := component.Variables[variableName]; exists && variable.Description != "" {
			body.AppendUnstructuredTokens(hclwrite.Tokens{
				{Type: hclsyntax.TokenComment, Bytes: []byte(fmt.Sprintf("// %s", variable.Description))},
				{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
			})
		}

		body.SetAttributeValue(variableName, ctyValue)

		// Add a newline after each variable definition for better spacing
		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
		})
	}

	// Write the variable definitions to the file.
	if err := osWriteFile(tfvarsFilePath, variablesContent.Bytes(), 0644); err != nil {
		return fmt.Errorf("error writing tfvars file: %w", err)
	}

	return nil
}

// Ensure TerraformGenerator implements Generator
var _ Generator = (*TerraformGenerator)(nil)

// convertToCtyValue converts an interface{} to a cty.Value, handling various data types.
func convertToCtyValue(value interface{}) (cty.Value, error) {
	switch v := value.(type) {
	case string:
		return cty.StringVal(v), nil
	case int:
		return cty.NumberIntVal(int64(v)), nil
	case float64:
		return cty.NumberFloatVal(v), nil
	case bool:
		return cty.BoolVal(v), nil
	case []interface{}:
		if len(v) == 0 {
			return cty.ListValEmpty(cty.DynamicPseudoType), nil
		}
		var ctyList []cty.Value
		for _, item := range v {
			ctyVal, err := convertToCtyValue(item)
			if err != nil {
				return cty.NilVal, err
			}
			ctyList = append(ctyList, ctyVal)
		}
		return cty.ListVal(ctyList), nil
	case map[string]interface{}:
		ctyMap := make(map[string]cty.Value)
		for key, val := range v {
			ctyVal, err := convertToCtyValue(val)
			if err != nil {
				return cty.NilVal, err
			}
			ctyMap[key] = ctyVal
		}
		return cty.ObjectVal(ctyMap), nil
	default:
		return cty.NilVal, fmt.Errorf("unsupported type: %T", v)
	}
}
