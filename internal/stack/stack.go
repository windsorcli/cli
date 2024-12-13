package stack

import (
	"fmt"

	"github.com/windsorcli/cli/internal/blueprint"
	"github.com/windsorcli/cli/internal/di"
)

// Stack is an interface that represents a stack of components.
type Stack interface {
	Initialize() error
	Up() error
}

// BaseStack is a struct that implements the Stack interface.
type BaseStack struct {
	injector  di.Injector
	blueprint blueprint.BlueprintHandler
}

// NewBaseStack creates a new base stack of components.
func NewBaseStack(injector di.Injector) *BaseStack {
	return &BaseStack{injector: injector}
}

// Initialize initializes the stack of components.
func (s *BaseStack) Initialize() error {
	// Resolve the blueprint handler
	blueprintHandler, ok := s.injector.Resolve("blueprintHandler").(blueprint.BlueprintHandler)
	if !ok {
		return fmt.Errorf("error resolving blueprintHandler")
	}
	s.blueprint = blueprintHandler
	return nil
}

// Up creates a new stack of components.
func (s *BaseStack) Up() error {
	return nil
}
