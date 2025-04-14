package stack

// The Stack is a core component that manages infrastructure component stacks.
// It provides a unified interface for initializing and managing infrastructure stacks,
// with support for dependency injection and component lifecycle management.
// The Stack acts as the primary orchestrator for infrastructure operations,
// coordinating shell operations, blueprint handling, and environment configuration.

import (
	"fmt"

	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Interfaces
// =============================================================================

// Stack is an interface that represents a stack of components.
type Stack interface {
	Initialize() error
	Up() error
}

// =============================================================================
// Types
// =============================================================================

// BaseStack is a struct that implements the Stack interface.
type BaseStack struct {
	injector         di.Injector
	blueprintHandler blueprint.BlueprintHandler
	shell            shell.Shell
	envPrinters      []env.EnvPrinter
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseStack creates a new base stack of components.
func NewBaseStack(injector di.Injector) *BaseStack {
	return &BaseStack{injector: injector}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize initializes the stack of components.
func (s *BaseStack) Initialize() error {
	// Resolve the shell
	shell, ok := s.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("error resolving shell")
	}
	s.shell = shell

	// Resolve the blueprint handler
	blueprintHandler, ok := s.injector.Resolve("blueprintHandler").(blueprint.BlueprintHandler)
	if !ok {
		return fmt.Errorf("error resolving blueprintHandler")
	}
	s.blueprintHandler = blueprintHandler

	// Resolve the envPrinters
	envPrinterInstances, err := s.injector.ResolveAll((*env.EnvPrinter)(nil))
	if err != nil {
		return fmt.Errorf("error resolving envPrinters: %v", err)
	}
	envPrinters := make([]env.EnvPrinter, len(envPrinterInstances))
	for i, instance := range envPrinterInstances {
		envPrinters[i], _ = instance.(env.EnvPrinter)
	}
	s.envPrinters = envPrinters

	return nil
}

// Up creates a new stack of components.
func (s *BaseStack) Up() error {
	return nil
}

// Ensure BaseStack implements Stack
var _ Stack = (*BaseStack)(nil)
