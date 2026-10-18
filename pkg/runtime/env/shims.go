// The Shims are the system call abstractions of the env package.
// It provides the shared shims of internal/shims and the env-specific process shims.
// Tests replace the shims to simulate file system, environment, and lookup results.
// NewShims returns the shims with the real implementations.

package env

import (
	"os"
	"os/exec"

	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives and the env-specific process shims.
type Shims struct {
	shims.Shims
	LookPath func(string) (string, error)
	Environ  func() []string
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its standard library implementation.
func NewShims() *Shims {
	return &Shims{
		Shims:    *shims.NewShims(),
		LookPath: exec.LookPath,
		Environ:  os.Environ,
	}
}
