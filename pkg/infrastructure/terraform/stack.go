package terraform

// The Stack is a core component that manages infrastructure component stacks.
// It provides a unified interface for initializing and managing infrastructure stacks,
// with support for dependency injection and component lifecycle management.
// The Stack acts as the primary orchestrator for infrastructure operations,
// coordinating shell operations and blueprint handling.

import (
	"fmt"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/resources/blueprint"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Interfaces
// =============================================================================

// Stack is an interface that represents a stack of components.
type Stack interface {
	Initialize() error
	Up() error
	Down() error
}

// =============================================================================
// Types
// =============================================================================

// BaseStack is a struct that implements the Stack interface.
type BaseStack struct {
	injector         di.Injector
	blueprintHandler blueprint.BlueprintHandler
	shell            shell.Shell
	shims            *Shims
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseStack creates a new base stack of components.
func NewBaseStack(injector di.Injector) *BaseStack {
	return &BaseStack{
		injector: injector,
		shims:    NewShims(),
	}
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

	return nil
}

// Up creates a new stack of components.
func (s *BaseStack) Up() error {
	return nil
}

// Down destroys a stack of components.
func (s *BaseStack) Down() error {
	return nil
}

// Ensure BaseStack implements Stack
var _ Stack = (*BaseStack)(nil)
