// The shims package is a system call abstraction layer
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

package terraform

import (
	"os"
)

// =============================================================================
// Types
// =============================================================================

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	Stat     func(string) (os.FileInfo, error)
	Chdir    func(string) error
	Getwd    func() (string, error)
	Setenv   func(string, string) error
	Unsetenv func(string) error
	Remove   func(string) error
}

// =============================================================================
// Helpers
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		Stat:     os.Stat,
		Chdir:    os.Chdir,
		Getwd:    os.Getwd,
		Setenv:   os.Setenv,
		Unsetenv: os.Unsetenv,
		Remove:   os.Remove,
	}
}
