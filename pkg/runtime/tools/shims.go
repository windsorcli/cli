// The Shims are the system call abstractions of the tools package.
// It provides the shared shims of internal/shims and the LookPath shim for the tool binary lookups.
// Tests replace the shims to simulate a missing tool or the content of an AWS config file.
// NewShims returns the shims with the real implementations.

package tools

import (
	"os/exec"

	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives and the tools-specific shims.
type Shims struct {
	shims.Shims
	LookPath func(file string) (string, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its real implementation.
func NewShims() *Shims {
	return &Shims{
		Shims:    *shims.NewShims(),
		LookPath: exec.LookPath,
	}
}
