package generators

import (
	"fmt"

	bundler "github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// The Generator is a core component that provides a unified interface for code generation.
// It provides a standardized way to initialize and write generated code to the filesystem.
// The Generator acts as the foundation for all code generation operations in the application,
// coordinating dependency injection, configuration handling, and blueprint processing.

// =============================================================================
// Interfaces
// =============================================================================

// Generator is the interface for all code generators
// It defines methods for initialization and file generation
// All generators must implement this interface
type Generator interface {
	Initialize() error
	Write(overwrite ...bool) error
	Generate(data map[string]any, overwrite ...bool) error
}

// =============================================================================
// Types
// =============================================================================

// BaseGenerator is a base implementation of the Generator interface
type BaseGenerator struct {
	injector         di.Injector
	configHandler    config.ConfigHandler
	blueprintHandler blueprint.BlueprintHandler
	shell            shell.Shell
	shims            *Shims
	artifactBuilder  bundler.Artifact
}

// =============================================================================
// Constructor
// =============================================================================

// NewGenerator creates a new BaseGenerator
func NewGenerator(injector di.Injector) *BaseGenerator {
	return &BaseGenerator{
		injector: injector,
		shims:    NewShims(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the BaseGenerator by resolving and storing required dependencies.
// It ensures that the config handler, blueprint handler, shell, and artifact builder are properly initialized.
func (g *BaseGenerator) Initialize() error {
	configHandler, ok := g.injector.Resolve("configHandler").(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("failed to resolve config handler")
	}
	g.configHandler = configHandler

	blueprintHandler, ok := g.injector.Resolve("blueprintHandler").(blueprint.BlueprintHandler)
	if !ok {
		return fmt.Errorf("failed to resolve blueprint handler")
	}
	g.blueprintHandler = blueprintHandler

	shellInstance, ok := g.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("failed to resolve shell instance")
	}
	g.shell = shellInstance

	artifactBuilder, ok := g.injector.Resolve("artifactBuilder").(bundler.Artifact)
	if !ok {
		return fmt.Errorf("failed to resolve artifact builder")
	}
	g.artifactBuilder = artifactBuilder

	return nil
}

// Write is a placeholder implementation of the Write method.
// Concrete implementations should override this method to provide specific generation logic.
func (g *BaseGenerator) Write(overwrite ...bool) error {
	return nil
}

// Generate is a placeholder implementation of the Generate method.
// Concrete implementations should override this method to provide specific generation logic.
// The data parameter contains the processed template data from pkg/template's Process function.
// The overwrite parameter controls whether existing files should be overwritten.
func (g *BaseGenerator) Generate(data map[string]any, overwrite ...bool) error {
	return nil
}
