package pipelines

import (
	"context"
	"fmt"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// The HookPipeline is a specialized component that manages shell hook installation functionality.
// It provides shell hook installation command execution including shell type validation and hook script generation.
// The HookPipeline handles shell integration for the Windsor CLI hook command.

// =============================================================================
// Types
// =============================================================================

// HookConstructors defines constructor functions for HookPipeline dependencies
type HookConstructors struct {
	NewShell func(di.Injector) shell.Shell
	NewShims func() *Shims
}

// HookPipeline provides shell hook installation functionality
type HookPipeline struct {
	BasePipeline

	constructors HookConstructors
}

// =============================================================================
// Constructor
// =============================================================================

// NewHookPipeline creates a new HookPipeline instance with optional constructors
func NewHookPipeline(constructors ...HookConstructors) *HookPipeline {
	var ctors HookConstructors
	if len(constructors) > 0 {
		ctors = constructors[0]
	} else {
		ctors = HookConstructors{
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewDefaultShell(injector)
			},
			NewShims: func() *Shims {
				return NewShims()
			},
		}
	}

	return &HookPipeline{
		BasePipeline: *NewBasePipeline(),
		constructors: ctors,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize creates and registers all required components for the hook pipeline.
// It sets up the shell in the correct order, registering each component
// with the dependency injector and initializing them sequentially to ensure proper dependency resolution.
func (p *HookPipeline) Initialize(injector di.Injector, ctx context.Context) error {
	if err := p.BasePipeline.Initialize(injector, ctx); err != nil {
		return err
	}

	p.shims = p.constructors.NewShims()

	p.shell = resolveOrCreateDependency(injector, "shell", p.constructors.NewShell)

	if err := p.shell.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize shell: %w", err)
	}

	return nil
}

// Execute installs the shell hook for the specified shell type.
// It validates the shell type argument and calls the shell's InstallHook method.
// The shell type is passed through the context with the key "shellType".
func (p *HookPipeline) Execute(ctx context.Context) error {
	shellType := ctx.Value("shellType")
	if shellType == nil {
		return fmt.Errorf("No shell name provided")
	}

	shellName, ok := shellType.(string)
	if !ok {
		return fmt.Errorf("Invalid shell name type")
	}

	return p.shell.InstallHook(shellName)
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Pipeline = (*HookPipeline)(nil)
