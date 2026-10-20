// The Shims are the system call abstractions of the terraform stack.
// It provides the shared shims of internal/shims and the directory and sleep shims of the stack.
// Tests replace the shims to simulate file system state and to skip the destroy retry delay.
// NewShims returns the shims with the real implementations.

package terraform

import (
	"os"
	"time"

	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives and the terraform stack shims.
type Shims struct {
	shims.Shims
	Chdir     func(dir string) error
	TimeSleep func(d time.Duration)
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its real implementation.
func NewShims() *Shims {
	return &Shims{
		Shims:     *shims.NewShims(),
		Chdir:     os.Chdir,
		TimeSleep: time.Sleep,
	}
}
