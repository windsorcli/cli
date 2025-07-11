package terraform

import (
	"os"
)

// The shims package is a system call abstraction layer for the terraform package
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

// =============================================================================
// Types
// =============================================================================

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	WriteFile func(filename string, data []byte, perm os.FileMode) error
	ReadFile  func(filename string) ([]byte, error)
	Stat      func(name string) (os.FileInfo, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		WriteFile: os.WriteFile,
		ReadFile:  os.ReadFile,
		Stat:      os.Stat,
	}
}
