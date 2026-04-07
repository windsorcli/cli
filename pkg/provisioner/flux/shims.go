// The shims package is a system call abstraction layer
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

package flux

import (
	"bytes"
	"os"
	"os/exec"
)

// =============================================================================
// Types
// =============================================================================

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	LookPath    func(file string) (string, error)
	MkdirAll    func(path string, perm os.FileMode) error
	RemoveAll   func(path string) error
	ReadFile    func(name string) ([]byte, error)
	WriteFile   func(name string, data []byte, perm os.FileMode) error
	ExecCommand func(command string, args ...string) (stdout string, stderr string, err error)
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
		ExecCommand: func(command string, args ...string) (string, string, error) {
			cmd := exec.Command(command, args...) //nolint:gosec // G204: command is always "flux" or "kustomize", never user input
			cmd.Env = append(os.Environ(), "NO_COLOR=1")
			var stdoutBuf, stderrBuf bytes.Buffer
			cmd.Stdout = &stdoutBuf
			cmd.Stderr = &stderrBuf
			err := cmd.Run()
			return stdoutBuf.String(), stderrBuf.String(), err
		},
	}
}
