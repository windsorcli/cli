package blueprint

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/di"
)

// BlueprintHandler defines the interface for handling blueprint operations
type BlueprintHandler interface {
	// Initialize initializes the blueprint handler
	Initialize() error

	// LoadConfig loads the blueprint from the specified path
	LoadConfig(path ...string) error

	// GetMetadata retrieves the metadata for the blueprint
	GetMetadata() MetadataV1Alpha1

	// GetSources retrieves the sources for the blueprint
	GetSources() []SourceV1Alpha1

	// GetTerraformComponents retrieves the Terraform components for the blueprint
	GetTerraformComponents() []TerraformComponentV1Alpha1

	// SetMetadata sets the metadata for the blueprint
	SetMetadata(metadata MetadataV1Alpha1) error

	// SetSources sets the sources for the blueprint
	SetSources(sources []SourceV1Alpha1) error

	// SetTerraformComponents sets the Terraform components for the blueprint
	SetTerraformComponents(terraformComponents []TerraformComponentV1Alpha1) error

	// WriteConfig writes the current blueprint to the specified path
	WriteConfig(path ...string) error
}

// BaseBlueprintHandler is a base implementation of the BlueprintHandler interface
type BaseBlueprintHandler struct {
	BlueprintHandler
	injector       di.Injector
	blueprint      BlueprintV1Alpha1
	contextHandler context.ContextHandler
}

// Create a new blueprint handler
func NewBlueprintHandler(injector di.Injector) *BaseBlueprintHandler {
	return &BaseBlueprintHandler{injector: injector}
}

// Initialize initializes the blueprint handler
func (b *BaseBlueprintHandler) Initialize() error {
	// Resolve the context handler
	contextHandler, ok := b.injector.Resolve("contextHandler").(context.ContextHandler)
	if !ok {
		return fmt.Errorf("error resolving contextHandler")
	}
	b.contextHandler = contextHandler

	// Initialize with a default blueprint
	b.blueprint = DefaultBlueprint

	// Set the blueprint name to match the context name
	context := b.contextHandler.GetContext()
	b.blueprint.Metadata.Name = context

	// Set the default description
	b.blueprint.Metadata.Description = fmt.Sprintf("This blueprint outlines resources in the %s context", context)

	return nil
}

// LoadConfig LoadConfigs the blueprint from the specified path
func (b *BaseBlueprintHandler) LoadConfig(path ...string) error {
	finalPath := ""
	// Check if a path is provided
	if len(path) > 0 && path[0] != "" {
		finalPath = path[0]
		// Check if the file exists at the provided path
		if _, err := osStat(finalPath); err != nil {
			return fmt.Errorf("specified path not found: %w", err)
		}
	} else {
		// Get the config root from the context handler
		configRoot, err := b.contextHandler.GetConfigRoot()
		if err != nil {
			return fmt.Errorf("error getting config root: %w", err)
		}
		// Set the final path to the default blueprint.yaml file
		finalPath = configRoot + "/blueprint.yaml"
		// Check if the file exists at the default path
		if _, err := osStat(finalPath); err != nil {
			// Do nothing if the default path does not exist
			return nil
		}
	}

	// Read the file from the final path
	data, err := osReadFile(finalPath)
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	// Unmarshal the YAML data into the blueprint struct
	if err := yamlUnmarshal(data, &b.blueprint); err != nil {
		return fmt.Errorf("error unmarshalling yaml: %w", err)
	}

	return nil
}

// WriteConfig writes the current blueprint to a specified path or a default location
func (b *BaseBlueprintHandler) WriteConfig(path ...string) error {
	finalPath := ""
	// Determine the final path to save the blueprint
	if len(path) > 0 && path[0] != "" {
		// Use the provided path if available
		finalPath = path[0]
	} else {
		// Otherwise, get the default config root path
		configRoot, err := b.contextHandler.GetConfigRoot()
		if err != nil {
			// Return an error if unable to get the config root
			return fmt.Errorf("error getting config root: %w", err)
		}
		// Set the final path to the default blueprint.yaml file
		finalPath = configRoot + "/blueprint.yaml"
	}

	// Ensure the parent directory exists
	dir := filepath.Dir(finalPath)
	if err := osMkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("error creating directory: %w", err)
	}

	// Convert the copied blueprint struct into YAML format, omitting null values
	data, err := yamlMarshalNonNull(b.blueprint)
	if err != nil {
		// Return an error if marshalling fails
		return fmt.Errorf("error marshalling yaml: %w", err)
	}

	// Write the YAML data to the determined path with appropriate permissions
	if err := osWriteFile(finalPath, data, 0644); err != nil {
		// Return an error if writing the file fails
		return fmt.Errorf("error writing blueprint file: %w", err)
	}
	return nil
}

// GetMetadata retrieves the metadata for the blueprint
func (b *BaseBlueprintHandler) GetMetadata() MetadataV1Alpha1 {
	return b.blueprint.Metadata
}

// GetSources retrieves the sources for the blueprint
func (b *BaseBlueprintHandler) GetSources() []SourceV1Alpha1 {
	return b.blueprint.Sources
}

// GetTerraformComponents retrieves the Terraform components for the blueprint
func (b *BaseBlueprintHandler) GetTerraformComponents() []TerraformComponentV1Alpha1 {
	return b.blueprint.TerraformComponents
}

// SetMetadata sets the metadata for the blueprint
func (b *BaseBlueprintHandler) SetMetadata(metadata MetadataV1Alpha1) error {
	b.blueprint.Metadata = metadata
	return nil
}

// SetSources sets the sources for the blueprint
func (b *BaseBlueprintHandler) SetSources(sources []SourceV1Alpha1) error {
	b.blueprint.Sources = sources
	return nil
}

// SetTerraformComponents sets the Terraform components for the blueprint
func (b *BaseBlueprintHandler) SetTerraformComponents(terraformComponents []TerraformComponentV1Alpha1) error {
	b.blueprint.TerraformComponents = terraformComponents
	return nil
}

// Ensure that BaseBlueprintHandler implements the BlueprintHandler interface
var _ BlueprintHandler = &BaseBlueprintHandler{}
