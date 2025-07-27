package pipelines

import (
	"context"
	"fmt"
)

// The HookPipeline is a specialized component that manages shell hook installation functionality.
// It provides shell hook installation command execution including shell type validation and hook script generation.
// The HookPipeline handles shell integration for the Windsor CLI hook command.

// =============================================================================
// Types
// =============================================================================

// HookPipeline provides shell hook installation functionality
type HookPipeline struct {
	BasePipeline
}

// =============================================================================
// Constructor
// =============================================================================

// NewHookPipeline creates a new HookPipeline instance
func NewHookPipeline() *HookPipeline {
	return &HookPipeline{
		BasePipeline: *NewBasePipeline(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

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
