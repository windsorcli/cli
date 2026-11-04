// The Shims are the system call abstractions of the inventory package.
// It provides the shared shims of internal/shims and the executable lookup and temporary directory shims.
// Tests replace the shims to simulate a missing CLI or a file system failure.
// NewShims returns the shims with the real implementations.

package inventory

import (
	"os"
	"os/exec"

	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives and the inventory-specific shims.
type Shims struct {
	shims.Shims
	LookPath  func(file string) (string, error)
	MkdirTemp func(dir, pattern string) (string, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its real implementation.
func NewShims() *Shims {
	return &Shims{
		Shims:     *shims.NewShims(),
		LookPath:  exec.LookPath,
		MkdirTemp: os.MkdirTemp,
	}
}
