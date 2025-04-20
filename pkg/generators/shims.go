// The shims package is a system call abstraction layer for the generators package
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

package generators

import (
	"os"

	"github.com/goccy/go-yaml"
)

// =============================================================================
// Types
// =============================================================================

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	WriteFile   func(name string, data []byte, perm os.FileMode) error
	ReadFile    func(name string) ([]byte, error)
	MkdirAll    func(path string, perm os.FileMode) error
	Stat        func(name string) (os.FileInfo, error)
	MarshalYAML func(v any) ([]byte, error)
}

// =============================================================================
// Helpers
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		WriteFile:   os.WriteFile,
		ReadFile:    os.ReadFile,
		MkdirAll:    os.MkdirAll,
		Stat:        os.Stat,
		MarshalYAML: yaml.Marshal,
	}
}
