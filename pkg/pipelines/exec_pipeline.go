package pipelines

import (
	"context"
	"fmt"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// The ExecPipeline is a specialized component that manages command execution with environment injection.
// It collects environment variables from all configured env printers and sets them in the process
// environment before executing commands, ensuring the same environment variables are injected
// that would be printed by the windsor env command.

// =============================================================================
// Types
// =============================================================================

// ExecConstructors defines constructor functions for ExecPipeline dependencies
type ExecConstructors struct {
	NewShell func(di.Injector) shell.Shell
}

// ExecPipeline provides command execution functionality with environment injection
type ExecPipeline struct {
	BasePipeline

	constructors ExecConstructors
	shell        shell.Shell
	injector     di.Injector
}

// =============================================================================
// Constructor
// =============================================================================

// NewExecPipeline creates a new ExecPipeline instance with optional constructors
func NewExecPipeline(constructors ...ExecConstructors) *ExecPipeline {
	var ctors ExecConstructors
	if len(constructors) > 0 {
		ctors = constructors[0]
	} else {
		ctors = ExecConstructors{
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewDefaultShell(injector)
			},
		}
	}

	return &ExecPipeline{
		BasePipeline: *NewBasePipeline(),
		constructors: ctors,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize creates and registers the shell component for command execution.
func (p *ExecPipeline) Initialize(injector di.Injector) error {
	p.injector = injector

	if existing := injector.Resolve("shell"); existing != nil {
		p.shell = existing.(shell.Shell)
	} else {
		p.shell = p.constructors.NewShell(injector)
		injector.Register("shell", p.shell)
	}
	p.BasePipeline.shell = p.shell

	if err := p.shell.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize shell: %w", err)
	}

	return nil
}

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
