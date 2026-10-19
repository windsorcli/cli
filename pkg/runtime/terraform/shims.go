// The Shims are the system call abstractions of the terraform package.
// It provides the shared shims of internal/shims and the Unsetenv shim.
// Tests replace the shims to simulate file access and environment state.
// NewShims returns the shims with the real implementations.

package terraform

import (
	"os"

	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives and the terraform-specific shims.
type Shims struct {
	shims.Shims
	Unsetenv func(key string) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its real implementation.
func NewShims() *Shims {
	return &Shims{
		Shims:    *shims.NewShims(),
		Unsetenv: os.Unsetenv,
	}
}
