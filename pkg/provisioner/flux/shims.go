// The Shims are the system call abstractions of the flux stack.
// It provides the shared shims of internal/shims and the LookPath shim of the stack.
// Tests replace the shims to simulate file system state and the installed CLI tools.
// NewShims returns the shims with the real implementations.

package flux

import (
	"os/exec"

	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives and the flux stack shims.
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
