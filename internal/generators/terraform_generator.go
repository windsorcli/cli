package generators

import (
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
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

	// Get the project root from the shell
	projectRoot, err := g.shell.GetProjectRoot()
	if err != nil {
		return err
	}

	// Write the Terraform files
	for _, component := range components {
		if isValidTerraformRemoteSource(component.Source) {
			// Parse the component source to create the directory path
			dirPath := filepath.Join(projectRoot, ".tf_modules", "terraform", component.Path)

			// Ensure the parent directories exist
			if err := osMkdirAll(dirPath, os.ModePerm); err != nil {
				return err
			}

			// Write the module file
			if err := g.writeModuleFile(dirPath, component); err != nil {
				return err
			}

			// Write the variables file
			if err := g.writeVariableFile(dirPath, component); err != nil {
				return err
			}
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

	// Directly map value names to var.<value_name>
	for valueName := range component.Values {
		body.SetAttributeTraversal(valueName, hcl.Traversal{
			hcl.TraverseRoot{Name: "var"},
			hcl.TraverseAttr{Name: valueName},
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

	// Iterate over each key in the Values map to define it as a variable in the HCL file.
	for variableName := range component.Values {
		// Create a new block for each variable with its name.
		block := body.AppendNewBlock("variable", []string{variableName})
		block.Body() // Create empty body to avoid extra newline
	}

	// Define the path for the variables file.
	varFilePath := filepath.Join(dirPath, "variables.tf")

	// Write the variable definitions to the file.
	if err := osWriteFile(varFilePath, variablesContent.Bytes(), 0644); err != nil {
		return err
	}

	return nil
}

// Ensure TerraformGenerator implements Generator
var _ Generator = (*TerraformGenerator)(nil)

// isValidTerraformRemoteSource checks if the source is a valid Terraform module reference
func isValidTerraformRemoteSource(source string) bool {
	// Define patterns for different valid source types
	patterns := []string{
		`^git::https://[^/]+/.*\.git(?:@.*)?$`, // Generic Git URL with .git suffix
		`^git@[^:]+:.*\.git(?:@.*)?$`,          // Generic SSH Git URL with .git suffix
		`^https?://.*\.git(?:@.*)?$`,           // HTTP URL with .git suffix
		`^https?://.*\.zip(?:@.*)?$`,           // HTTP URL pointing to a .zip archive
		`^registry\.terraform\.io/.*`,          // Terraform Registry
		`^[^/]+\.com/.*`,                       // Generic domain reference
	}

	// Check if the source matches any of the valid patterns
	for _, pattern := range patterns {
		matched, err := regexpMatchString(pattern, source)
		if err != nil {
			return false
		}
		if matched {
			return true
		}
	}

	return false
}
