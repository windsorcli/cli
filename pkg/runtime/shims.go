// The Shims are the system call abstractions of the runtime package.
// It provides the shared shims of internal/shims and the OpenRoot shim.
// Tests replace the shims to simulate environment state, the host OS, and project root access.
// NewShims returns the shims with the real implementations.

package runtime

import (
	"os"

	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives and the runtime-specific shims.
type Shims struct {
	shims.Shims
	OpenRoot func(name string) (*os.Root, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its real implementation.
func NewShims() *Shims {
	return &Shims{
		Shims:    *shims.NewShims(),
		OpenRoot: os.OpenRoot,
	}
}
