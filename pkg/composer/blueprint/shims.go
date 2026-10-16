package blueprint

import (
	"path/filepath"

	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives and the blueprint-specific directory walk.
type Shims struct {
	shims.Shims
	Walk func(string, filepath.WalkFunc) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its standard library implementation.
func NewShims() *Shims {
	return &Shims{
		Shims: *shims.NewShims(),
		Walk:  filepath.Walk,
	}
}
