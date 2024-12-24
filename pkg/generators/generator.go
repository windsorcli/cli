package generators

import (
	"fmt"

	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
)

// Generator is the interface that wraps the Write method
type Generator interface {
	Initialize() error
	Write() error
}

// BaseGenerator is a base implementation of the Generator interface
type BaseGenerator struct {
	injector         di.Injector
	contextHandler   context.ContextHandler
	blueprintHandler blueprint.BlueprintHandler
}

// NewGenerator creates a new BaseGenerator
func NewGenerator(injector di.Injector) *BaseGenerator {
	return &BaseGenerator{
		injector: injector,
	}
}

// Initialize initializes the BaseGenerator
func (g *BaseGenerator) Initialize() error {
	// Resolve the context handler
	contextHandler, ok := g.injector.Resolve("contextHandler").(context.ContextHandler)
	if !ok {
		return fmt.Errorf("failed to resolve context handler")
	}
	g.contextHandler = contextHandler

	// Resolve the blueprint handler
	blueprintHandler, ok := g.injector.Resolve("blueprintHandler").(blueprint.BlueprintHandler)
	if !ok {
		return fmt.Errorf("failed to resolve blueprint handler")
	}
	g.blueprintHandler = blueprintHandler

	return nil
}

// Write is a placeholder implementation of the Write method
func (g *BaseGenerator) Write() error {
	return nil
}
