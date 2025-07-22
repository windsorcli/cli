package pipelines

import (
	"context"
	"fmt"
)

// The ExecPipeline is a specialized component that manages command execution with environment injection.
// It collects environment variables from all configured env printers and sets them in the process
// environment before executing commands, ensuring the same environment variables are injected
// that would be printed by the windsor env command.

// =============================================================================
// Types
// =============================================================================

// ExecPipeline provides command execution functionality with environment injection
type ExecPipeline struct {
	BasePipeline
}

// =============================================================================
// Constructor
// =============================================================================

// NewExecPipeline creates a new ExecPipeline instance
func NewExecPipeline() *ExecPipeline {
	return &ExecPipeline{
		BasePipeline: *NewBasePipeline(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Execute executes the command with the provided arguments.
// It expects the command and optional arguments to be provided in the context.
func (p *ExecPipeline) Execute(ctx context.Context) error {
	command, ok := ctx.Value("command").(string)
	if !ok || command == "" {
		return fmt.Errorf("no command provided in context")
	}

	var args []string
	if ctxArgs := ctx.Value("args"); ctxArgs != nil {
		args = ctxArgs.([]string)
	}

	_, err := p.shell.Exec(command, args...)
	if err != nil {
		return fmt.Errorf("command execution failed: %w", err)
	}

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Pipeline = (*ExecPipeline)(nil)
