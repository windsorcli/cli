// The shims package is a system call abstraction layer
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

package flux

import (
	"os"
	"os/exec"
)

// =============================================================================
// Types
// =============================================================================

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	LookPath  func(file string) (string, error)
	MkdirAll  func(path string, perm os.FileMode) error
	RemoveAll func(path string) error
	ReadFile  func(name string) ([]byte, error)
	WriteFile func(name string, data []byte, perm os.FileMode) error
}

// =============================================================================
// Helpers
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		LookPath:  exec.LookPath,
		MkdirAll:  os.MkdirAll,
		RemoveAll: os.RemoveAll,
		ReadFile:  os.ReadFile,
		WriteFile: os.WriteFile,
	}
}
