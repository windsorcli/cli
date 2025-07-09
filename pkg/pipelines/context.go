package pipelines

import (
	"context"
	"fmt"
)

// The ContextPipeline is a specialized component that manages context operations functionality.
// It provides context-specific command execution including context getting and setting operations,
// configuration validation, and shell integration for the Windsor CLI context command.
// The ContextPipeline handles context management operations with proper initialization and validation.

// =============================================================================
// Types
// =============================================================================

// ContextPipeline provides context management functionality
// It embeds BasePipeline and manages context-specific dependencies
// for Windsor CLI context operations.
type ContextPipeline struct {
	BasePipeline
}

// =============================================================================
// Constructor
// =============================================================================

// NewContextPipeline creates a new ContextPipeline instance
func NewContextPipeline() *ContextPipeline {
	return &ContextPipeline{
		BasePipeline: *NewBasePipeline(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

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
