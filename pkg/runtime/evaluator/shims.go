// The Shims are the system call abstractions of the evaluator package.
// It provides the shared shims of internal/shims.
// Tests replace the shims to simulate file reads and marshal failures.
// NewShims returns the shims with the real implementations.

package evaluator

import (
	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives.
type Shims struct {
	shims.Shims
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its real implementation.
func NewShims() *Shims {
	return &Shims{
		Shims: *shims.NewShims(),
	}
}
