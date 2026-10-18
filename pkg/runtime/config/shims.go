// The Shims are the system call abstractions of the config package.
// It provides the shared shims of internal/shims and the random byte source.
// Tests replace the shims to simulate file system, environment, and random results.
// NewShims returns the shims with the real implementations.

package config

import (
	"crypto/rand"

	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives and the config-specific random byte source.
type Shims struct {
	shims.Shims
	CryptoRandRead func([]byte) (int, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its standard library implementation.
func NewShims() *Shims {
	return &Shims{
		Shims:          *shims.NewShims(),
		CryptoRandRead: rand.Read,
	}
}
