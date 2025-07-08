package pipelines

import (
	"context"
	"fmt"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// The ContextPipeline is a specialized component that manages context operations functionality.
// It provides context-specific command execution including context getting and setting operations,
// configuration validation, and shell integration for the Windsor CLI context command.
// The ContextPipeline handles context management operations with proper initialization and validation.

// =============================================================================
// Types
// =============================================================================

// ContextConstructors defines constructor functions for ContextPipeline dependencies
type ContextConstructors struct {
	NewConfigHandler func(di.Injector) config.ConfigHandler
	NewShell         func(di.Injector) shell.Shell
	NewShims         func() *Shims
}

// ContextPipeline provides context management functionality
type ContextPipeline struct {
	BasePipeline

	constructors ContextConstructors
}

// =============================================================================
// Constructor
// =============================================================================

// NewContextPipeline creates a new ContextPipeline instance with optional constructors
func NewContextPipeline(constructors ...ContextConstructors) *ContextPipeline {
	var ctors ContextConstructors
	if len(constructors) > 0 {
		ctors = constructors[0]
	} else {
		ctors = ContextConstructors{
			NewConfigHandler: func(injector di.Injector) config.ConfigHandler {
				return config.NewYamlConfigHandler(injector)
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewDefaultShell(injector)
			},
			NewShims: func() *Shims {
				return NewShims()
			},
		}
	}

	return &ContextPipeline{
		BasePipeline: *NewBasePipeline(),
		constructors: ctors,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize creates and registers all required components for the context pipeline.
// It sets up the config handler and shell in the correct order, registering each component
// with the dependency injector and initializing them sequentially to ensure proper dependency resolution.
func (p *ContextPipeline) Initialize(injector di.Injector, ctx context.Context) error {
	if err := p.BasePipeline.Initialize(injector, ctx); err != nil {
		return err
	}

	p.shims = p.constructors.NewShims()

	p.shell = resolveOrCreateDependency(injector, "shell", p.constructors.NewShell)
	p.configHandler = resolveOrCreateDependency(injector, "configHandler", p.constructors.NewConfigHandler)

	if err := p.shell.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize shell: %w", err)
	}

	if err := p.configHandler.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize config handler: %w", err)
	}

	if err := p.loadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	return nil
}

// Execute runs the context management logic based on the operation type.
// It supports both "get" and "set" operations specified through the context.
// For "get" operations, it returns the current context.
// For "set" operations, it sets the context and writes a reset token.
func (p *ContextPipeline) Execute(ctx context.Context) error {
	operation := ctx.Value("operation")
	if operation == nil {
		return fmt.Errorf("no operation specified")
	}

	operationStr, ok := operation.(string)
	if !ok {
		return fmt.Errorf("invalid operation type")
	}

	switch operationStr {
	case "get":
		return p.executeGet(ctx)
	case "set":
		return p.executeSet(ctx)
	default:
		return fmt.Errorf("unsupported operation: %s", operationStr)
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// executeGet handles the context get operation by checking if config is loaded
// and returning the current context name.
func (p *ContextPipeline) executeGet(ctx context.Context) error {
	if !p.configHandler.IsLoaded() {
		return fmt.Errorf("No context is available. Have you run `windsor init`?")
	}

	currentContext := p.configHandler.GetContext()

	output := ctx.Value("output")
	if output != nil {
		if outputFunc, ok := output.(func(string)); ok {
			outputFunc(currentContext)
		}
	}

	return nil
}

// executeSet handles the context set operation by validating the context name,
// setting the context, and writing a reset token.
func (p *ContextPipeline) executeSet(ctx context.Context) error {
	contextName := ctx.Value("contextName")
	if contextName == nil {
		return fmt.Errorf("no context name provided")
	}

	contextNameStr, ok := contextName.(string)
	if !ok {
		return fmt.Errorf("invalid context name type")
	}

	if !p.configHandler.IsLoaded() {
		return fmt.Errorf("No context is available. Have you run `windsor init`?")
	}

	if _, err := p.shell.WriteResetToken(); err != nil {
		return fmt.Errorf("Error writing reset token: %w", err)
	}

	if err := p.configHandler.SetContext(contextNameStr); err != nil {
		return fmt.Errorf("Error setting context: %w", err)
	}

	output := ctx.Value("output")
	if output != nil {
		if outputFunc, ok := output.(func(string)); ok {
			outputFunc(fmt.Sprintf("Context set to: %s", contextNameStr))
		}
	}

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Pipeline = (*ContextPipeline)(nil)
