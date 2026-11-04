// The Shims are the system call abstractions of the inventory package.
// It provides the shared shims of internal/shims and the temporary directory shim.
// Tests replace the shims to simulate a file system failure.
// NewShims returns the shims with the real implementations.

package inventory

import (
	"os"

	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives and the inventory-specific shims.
type Shims struct {
	shims.Shims
	MkdirTemp func(dir, pattern string) (string, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its real implementation.
func NewShims() *Shims {
	return &Shims{
		Shims:     *shims.NewShims(),
		MkdirTemp: os.MkdirTemp,
	}
}
