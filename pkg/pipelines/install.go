package pipelines

import (
	"context"
	"fmt"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/resources/artifact"
	"github.com/windsorcli/cli/pkg/resources/blueprint"
)

// The InstallPipeline is a specialized component that manages blueprint installation functionality.
// It provides install-specific command execution for blueprint installation with optional waiting
// for kustomizations to be ready. The InstallPipeline assumes that the env pipeline has already
// been executed to handle environment variables and secrets setup.

// =============================================================================
// Types
// =============================================================================

// InstallPipeline provides blueprint installation functionality
type InstallPipeline struct {
	BasePipeline
	blueprintHandler blueprint.BlueprintHandler
	generators       []generators.Generator
	artifactBuilder  artifact.Artifact
}

// =============================================================================
// Constructor
// =============================================================================

// NewInstallPipeline creates a new InstallPipeline instance
func NewInstallPipeline() *InstallPipeline {
	return &InstallPipeline{
		BasePipeline: *NewBasePipeline(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize configures the InstallPipeline by setting up the blueprint handler, template renderer,
// and generators required for blueprint installation. Only components necessary for blueprint installation
// are initialized, as environment setup is handled by the env pipeline. The method initializes the
// Kubernetes manager and client, blueprint handler, template renderer, and generators, and ensures
// all are properly initialized before use. Returns an error if any component fails to initialize.
func (p *InstallPipeline) Initialize(injector di.Injector, ctx context.Context) error {
	if err := p.BasePipeline.Initialize(injector, ctx); err != nil {
		return err
	}

	kubernetesManager := p.withKubernetesManager()
	_ = p.withKubernetesClient()
	p.blueprintHandler = p.withBlueprintHandler()
	p.artifactBuilder = p.withArtifactBuilder()
	generators, err := p.withGenerators()
	if err != nil {
		return fmt.Errorf("failed to set up generators: %w", err)
	}
	p.generators = generators

	if kubernetesManager != nil {
		if err := kubernetesManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize kubernetes manager: %w", err)
		}
	}

	if p.blueprintHandler != nil {
		if err := p.blueprintHandler.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize blueprint handler: %w", err)
		}
	}

	for _, generator := range p.generators {
		if err := generator.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize generator: %w", err)
		}
	}

	return nil
}

// Execute runs the blueprint installation process for the InstallPipeline.
// It processes templates for kustomize data, installs the blueprint using the configured blueprint handler,
// and, if the "wait" flag is set in the context, waits for kustomizations to become ready.
// Returns an error if configuration is not loaded, the blueprint handler is missing, installation fails,
// or waiting for kustomizations fails.
func (p *InstallPipeline) Execute(ctx context.Context) error {
	if !p.configHandler.IsLoaded() {
		return fmt.Errorf("Nothing to install. Have you run \033[1mwindsor init\033[0m?")
	}

	if p.blueprintHandler == nil {
		return fmt.Errorf("No blueprint handler found")
	}

	// Phase 1: Load blueprint config
	if err := p.blueprintHandler.LoadConfig(); err != nil {
		return fmt.Errorf("Error loading blueprint config: %w", err)
	}

	// Phase 2: Generate files using generators
	for _, generator := range p.generators {
		if err := generator.Generate(map[string]any{}, false); err != nil {
			return fmt.Errorf("failed to generate from template data: %w", err)
		}
	}

	// Phase 3: Install blueprint
	if err := p.blueprintHandler.Install(); err != nil {
		return fmt.Errorf("Error installing blueprint: %w", err)
	}

	// Phase 4: Wait for kustomizations if requested
	waitFlag := ctx.Value("wait")
	if waitFlag != nil {
		if wait, ok := waitFlag.(bool); ok && wait {
			if err := p.blueprintHandler.WaitForKustomizations("⏳ Waiting for kustomizations to be ready"); err != nil {
				return fmt.Errorf("failed waiting for kustomizations: %w", err)
			}
		}
	}

	return nil
}
