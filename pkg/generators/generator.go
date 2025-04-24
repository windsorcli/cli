package generators

import (
	"fmt"

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

// Generator is the interface that wraps the Write method
type Generator interface {
	Initialize() error
	Write() error
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
// It ensures that the config handler, blueprint handler, and shell are properly initialized.
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

	return nil
}

// Write is a placeholder implementation of the Write method.
// Concrete implementations should override this method to provide specific generation logic.
func (g *BaseGenerator) Write() error {
	return nil
}
