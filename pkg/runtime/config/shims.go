// The Shims are the system call abstractions of the config package.
// It provides the shared shims of internal/shims and the random byte source.
// Tests replace the shims to simulate file system, environment, and random results.
// NewShims returns the shims with the real implementations.

package config

import (
	"crypto/rand"
	"path/filepath"

	"github.com/goccy/go-yaml"

	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives, the strict YAML decoder, and the random byte source.
type Shims struct {
	shims.Shims
	YamlUnmarshalStrict func(data []byte, v any) error
	EvalSymlinks        func(path string) (string, error)
	CryptoRandRead      func([]byte) (int, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its standard library implementation.
func NewShims() *Shims {
	return &Shims{
		Shims: *shims.NewShims(),
		YamlUnmarshalStrict: func(data []byte, v any) error {
			return yaml.UnmarshalWithOptions(data, v, yaml.Strict())
		},
		EvalSymlinks:   filepath.EvalSymlinks,
		CryptoRandRead: rand.Read,
	}
}
