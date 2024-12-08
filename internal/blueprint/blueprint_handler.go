package blueprint

import (
	"fmt"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// BlueprintHandler defines the interface for handling blueprint operations
type BlueprintHandler interface {
	// Load loads the blueprint from the specified path
	Load(path ...string) error

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

	// Save saves the current blueprint to the specified path
	Save(path ...string) error
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
	return nil
}

// Load loads the blueprint from the specified path
func (b *BaseBlueprintHandler) Load(path ...string) error {
	finalPath := ""
	if len(path) > 0 && path[0] != "" {
		finalPath = path[0]
	} else {
		configRoot, err := b.contextHandler.GetConfigRoot()
		if err != nil {
			return fmt.Errorf("error getting config root: %w", err)
		}
		finalPath = configRoot + "/blueprint.yaml"
	}

	data, err := osReadFile(finalPath)
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	if err := yamlUnmarshal(data, &b.blueprint); err != nil {
		return fmt.Errorf("error unmarshalling yaml: %w", err)
	}

	return nil
}

// Save saves the current blueprint to the specified path
func (b *BaseBlueprintHandler) Save(path ...string) error {
	finalPath := ""
	if len(path) > 0 && path[0] != "" {
		finalPath = path[0]
	} else {
		configRoot, err := b.contextHandler.GetConfigRoot()
		if err != nil {
			return fmt.Errorf("error getting config root: %w", err)
		}
		finalPath = configRoot + "/blueprint.yaml"
	}

	// Marshal the blueprint struct into YAML data, omitting null values
	data, err := yamlMarshalNonNull(b.blueprint)
	if err != nil {
		return fmt.Errorf("error marshalling yaml: %w", err)
	}

	// Write the YAML data to the specified path
	if err := osWriteFile(finalPath, data, 0644); err != nil {
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
