package pipelines

import (
	"context"
	"fmt"

	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/di"
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

// Initialize sets up the install pipeline components including blueprint handler.
// It only initializes the components needed for blueprint installation functionality
// since environment setup is handled by the env pipeline.
func (p *InstallPipeline) Initialize(injector di.Injector, ctx context.Context) error {
	if err := p.BasePipeline.Initialize(injector, ctx); err != nil {
		return err
	}

	// Set up kubernetes manager and client (required by blueprint handler)
	kubernetesManager := p.withKubernetesManager()
	_ = p.withKubernetesClient()

	// Set up blueprint handler
	p.blueprintHandler = p.withBlueprintHandler()

	// Initialize kubernetes manager before blueprint handler
	if kubernetesManager != nil {
		if err := kubernetesManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize kubernetes manager: %w", err)
		}
	}

	// Initialize blueprint handler
	if p.blueprintHandler != nil {
		if err := p.blueprintHandler.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize blueprint handler: %w", err)
		}
	}

	return nil
}

// Execute runs the blueprint installation process for the InstallPipeline.
// It installs the blueprint using the configured blueprint handler and, if the "wait" flag
// is set in the context, waits for kustomizations to become ready. Returns an error if
// configuration is not loaded, the blueprint handler is missing, installation fails, or
// waiting for kustomizations fails.
func (p *InstallPipeline) Execute(ctx context.Context) error {
	if !p.configHandler.IsLoaded() {
		return fmt.Errorf("Nothing to install. Have you run \033[1mwindsor init\033[0m?")
	}

	if p.blueprintHandler == nil {
		return fmt.Errorf("No blueprint handler found")
	}

	if err := p.blueprintHandler.Install(); err != nil {
		return fmt.Errorf("Error installing blueprint: %w", err)
	}

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
