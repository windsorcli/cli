// The shims package is a system call abstraction layer
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

package cmd

import (
	"os"
	"os/exec"
	"runtime"
)

// =============================================================================
// Types
// =============================================================================

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	Exit        func(int)
	UserHomeDir func() (string, error)
	Stat        func(string) (os.FileInfo, error)
	RemoveAll   func(string) error
	Getwd       func() (string, error)
	Setenv      func(string, string) error
	Command     func(string, ...string) *exec.Cmd
	Getenv      func(string) string
	ReadFile    func(string) ([]byte, error)
}

// =============================================================================
// Helpers
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		Exit:        os.Exit,
		UserHomeDir: os.UserHomeDir,
		Stat:        os.Stat,
		RemoveAll:   os.RemoveAll,
		Getwd:       os.Getwd,
		Setenv:      os.Setenv,
		Command:     exec.Command,
		Getenv:      os.Getenv,
		ReadFile:    os.ReadFile,
	}
}

// Goos returns the operating system name
func (s *Shims) Goos() string {
	return runtime.GOOS
}
