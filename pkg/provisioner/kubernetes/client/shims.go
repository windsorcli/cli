// The Shims are the system call abstractions of the kubernetes client.
// It provides the shared shims of internal/shims.
// Tests replace the shims to simulate file system state.
// NewShims returns the shims with the real implementations.

package client

import (
	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives of the kubernetes client.
type Shims struct {
	shims.Shims
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its real implementation.
func NewShims() *Shims {
	return &Shims{Shims: *shims.NewShims()}
}
