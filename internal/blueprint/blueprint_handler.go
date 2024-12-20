package blueprint

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/shell"
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
	contextHandler context.ContextHandler
	shell          shell.Shell
	blueprint      BlueprintV1Alpha1
	projectRoot    string
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

	// Resolve the shell
	shell, ok := b.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("error resolving shell")
	}
	b.shell = shell

	// Get the project root
	projectRoot, err := b.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error getting project root: %w", err)
	}
	b.projectRoot = projectRoot

	// Initialize with a default blueprint
	b.blueprint = DefaultBlueprint

	// Set the blueprint name to match the context name
	context := b.contextHandler.GetContext()
	b.blueprint.Metadata.Name = context

	// Set the default description
	b.blueprint.Metadata.Description = fmt.Sprintf("This blueprint outlines resources in the %s context", context)

	return nil
}

// LoadConfig Loads the blueprint from the specified path
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
	// Create a copy of the blueprint to avoid modifying the original
	resolvedBlueprint := b.blueprint
	return resolvedBlueprint.Metadata
}

// GetSources retrieves the sources for the blueprint
func (b *BaseBlueprintHandler) GetSources() []SourceV1Alpha1 {
	// Create a copy of the blueprint to avoid modifying the original
	resolvedBlueprint := b.blueprint
	return resolvedBlueprint.Sources
}

// GetTerraformComponents retrieves the Terraform components for the blueprint
func (b *BaseBlueprintHandler) GetTerraformComponents() []TerraformComponentV1Alpha1 {
	// Create a copy of the blueprint to avoid modifying the original
	resolvedBlueprint := b.blueprint

	// Resolve the component sources
	b.resolveComponentSources(&resolvedBlueprint)

	// Resolve the component paths
	b.resolveComponentPaths(&resolvedBlueprint)

	return resolvedBlueprint.TerraformComponents
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

// resolveComponentSources resolves the source for each Terraform component
func (b *BaseBlueprintHandler) resolveComponentSources(blueprint *BlueprintV1Alpha1) {
	// Create a copy of the TerraformComponents to avoid modifying the original components
	resolvedComponents := make([]TerraformComponentV1Alpha1, len(blueprint.TerraformComponents))
	copy(resolvedComponents, blueprint.TerraformComponents)

	for i, component := range resolvedComponents {
		for _, source := range blueprint.Sources {
			if component.Source == source.Name {
				pathPrefix := source.PathPrefix
				if pathPrefix == "" {
					pathPrefix = "terraform"
				}
				resolvedComponents[i].Source = source.Url + "//" + pathPrefix + "/" + component.Path + "@" + source.Ref
				break
			}
		}
	}

	// Replace the original components with the resolved ones
	blueprint.TerraformComponents = resolvedComponents
}

// resolveComponentPaths resolves the path for each Terraform component
func (b *BaseBlueprintHandler) resolveComponentPaths(blueprint *BlueprintV1Alpha1) {
	projectRoot := b.projectRoot

	// Create a copy of the TerraformComponents to avoid modifying the original components
	resolvedComponents := make([]TerraformComponentV1Alpha1, len(blueprint.TerraformComponents))
	copy(resolvedComponents, blueprint.TerraformComponents)

	for i, component := range resolvedComponents {
		// Create a copy of the component to avoid modifying the original component
		componentCopy := component

		if isValidTerraformRemoteSource(componentCopy.Source) {
			componentCopy.Path = filepath.Join(projectRoot, ".tf_modules", componentCopy.Path)
		} else {
			componentCopy.Path = filepath.Join(projectRoot, "terraform", componentCopy.Path)
		}

		// Update the resolved component in the slice
		resolvedComponents[i] = componentCopy
	}

	// Replace the original components with the resolved ones
	blueprint.TerraformComponents = resolvedComponents
}

// Ensure that BaseBlueprintHandler implements the BlueprintHandler interface
var _ BlueprintHandler = &BaseBlueprintHandler{}

// isValidTerraformRemoteSource checks if the source is a valid Terraform module reference
func isValidTerraformRemoteSource(source string) bool {
	// Define patterns for different valid source types
	patterns := []string{
		`^git::https://[^/]+/.*\.git(?:@.*)?$`, // Generic Git URL with .git suffix
		`^git@[^:]+:.*\.git(?:@.*)?$`,          // Generic SSH Git URL with .git suffix
		`^https?://[^/]+/.*\.git(?:@.*)?$`,     // HTTP URL with .git suffix
		`^https?://[^/]+/.*\.zip(?:@.*)?$`,     // HTTP URL pointing to a .zip archive
		`^https?://[^/]+/.*//.*(?:@.*)?$`,      // HTTP URL with double slashes and optional ref
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
