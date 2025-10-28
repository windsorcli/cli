package resources

import (
	"fmt"

	"github.com/windsorcli/cli/pkg/resources/artifact"
	"github.com/windsorcli/cli/pkg/resources/blueprint"
	"github.com/windsorcli/cli/pkg/resources/terraform"
	"github.com/windsorcli/cli/pkg/types"
)

// The Resources package provides high-level resource management functionality
// for artifact, blueprint, and terraform operations. It consolidates the creation
// and management of these core resources, providing a unified interface for
// resource lifecycle operations across the Windsor CLI.

// =============================================================================
// Types
// =============================================================================

// ResourcesExecutionContext holds the execution context for resource operations.
// It embeds the base ExecutionContext and includes all resource-specific dependencies.
type ResourcesExecutionContext struct {
	types.ExecutionContext

	// Resource-specific dependencies
	ArtifactBuilder   artifact.Artifact
	BlueprintHandler  blueprint.BlueprintHandler
	TerraformResolver terraform.ModuleResolver
}

// Resources manages the lifecycle of all resource types (artifact, blueprint, terraform).
// It provides a unified interface for creating, initializing, and managing these resources
// with proper dependency injection and error handling.
type Resources struct {
	*ResourcesExecutionContext
}

// =============================================================================
// Constructor
// =============================================================================

// NewResources creates and initializes a new Resources instance with the provided execution context.
// It sets up all required resource handlers—artifact builder, blueprint handler, and terraform resolver—
// and registers each handler with the dependency injector for use throughout the resource lifecycle.
// Returns a pointer to the fully initialized Resources struct.
func NewResources(ctx *ResourcesExecutionContext) *Resources {
	resources := &Resources{
		ResourcesExecutionContext: ctx,
	}

	resources.ArtifactBuilder = artifact.NewArtifactBuilder()
	resources.Injector.Register("artifactBuilder", resources.ArtifactBuilder)

	resources.BlueprintHandler = blueprint.NewBlueprintHandler(resources.Injector)
	resources.Injector.Register("blueprintHandler", resources.BlueprintHandler)

	resources.TerraformResolver = terraform.NewStandardModuleResolver(resources.Injector)
	resources.Injector.Register("terraformResolver", resources.TerraformResolver)

	return resources
}

// =============================================================================
// Public Methods
// =============================================================================

// Bundle creates a complete artifact bundle from the project's templates, kustomize, and terraform files.
// It initializes the artifact builder and creates a distributable artifact.
// The outputPath specifies where to save the bundle file. Returns the actual output path or an error.
func (r *Resources) Bundle(outputPath, tag string) (string, error) {
	if err := r.ArtifactBuilder.Initialize(r.Injector); err != nil {
		return "", fmt.Errorf("failed to initialize artifact builder: %w", err)
	}

	actualOutputPath, err := r.ArtifactBuilder.Write(outputPath, tag)
	if err != nil {
		return "", fmt.Errorf("failed to create artifact bundle: %w", err)
	}

	return actualOutputPath, nil
}

// Push creates and pushes an artifact to a container registry.
// It bundles all project files and pushes them to the specified registry with the given tag.
// Returns the registry URL or an error.
func (r *Resources) Push(registryBase, repoName, tag string) (string, error) {
	if err := r.ArtifactBuilder.Initialize(r.Injector); err != nil {
		return "", fmt.Errorf("failed to initialize artifact builder: %w", err)
	}

	if err := r.ArtifactBuilder.Bundle(); err != nil {
		return "", fmt.Errorf("failed to bundle artifacts: %w", err)
	}

	if err := r.ArtifactBuilder.Push(registryBase, repoName, tag); err != nil {
		return "", fmt.Errorf("failed to push artifact: %w", err)
	}

	registryURL := fmt.Sprintf("%s/%s", registryBase, repoName)
	if tag != "" {
		registryURL = fmt.Sprintf("%s:%s", registryURL, tag)
	}

	return registryURL, nil
}

// Generate processes and deploys the complete project infrastructure.
// It initializes all core resources, processes blueprints, and handles terraform modules
// for the project. The optional overwrite parameter determines whether existing files
// should be overwritten during blueprint processing. This is the main deployment method.
// Returns an error if any initialization or processing step fails.
func (r *Resources) Generate(overwrite ...bool) error {
	shouldOverwrite := false
	if len(overwrite) > 0 {
		shouldOverwrite = overwrite[0]
	}

	if err := r.BlueprintHandler.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize blueprint handler: %w", err)
	}
	if err := r.BlueprintHandler.LoadBlueprint(); err != nil {
		return fmt.Errorf("failed to load blueprint data: %w", err)
	}

	r.Blueprint = r.BlueprintHandler.Generate()

	if err := r.TerraformResolver.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize terraform resolver: %w", err)
	}

	if err := r.BlueprintHandler.Write(shouldOverwrite); err != nil {
		return fmt.Errorf("failed to write blueprint files: %w", err)
	}

	if err := r.TerraformResolver.ProcessModules(); err != nil {
		return fmt.Errorf("failed to process terraform modules: %w", err)
	}

	return nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// CreateResources creates a new Resources instance with all dependencies properly initialized.
// This is a convenience function that creates a fully configured Resources
// with the provided execution context.
func CreateResources(ctx *ResourcesExecutionContext) *Resources {
	return NewResources(ctx)
}
