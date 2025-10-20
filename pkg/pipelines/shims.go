package pipelines

import "os"

// Shims provides a testable interface for system operations used by pipelines.
// This struct-based approach allows for better isolation during testing by enabling
// dependency injection of mock implementations for file system and environment operations.
// Each pipeline can use its own Shims instance with customized behavior for testing scenarios.
type Shims struct {
	Stat      func(name string) (os.FileInfo, error)
	Getenv    func(key string) string
	Setenv    func(key, value string) error
	ReadDir   func(name string) ([]os.DirEntry, error)
	ReadFile  func(name string) ([]byte, error)
	RemoveAll func(path string) error
	MkdirAll  func(path string, perm os.FileMode) error
	WriteFile func(name string, data []byte, perm os.FileMode) error
}

// NewShims creates a new Shims instance with default system call implementations.
// The returned instance provides direct access to os package functions and can be
// used in production environments or as a base for creating test-specific variants.
func NewShims() *Shims {
	return &Shims{
		Stat:      os.Stat,
		Getenv:    os.Getenv,
		Setenv:    os.Setenv,
		ReadDir:   os.ReadDir,
		ReadFile:  os.ReadFile,
		RemoveAll: os.RemoveAll,
		MkdirAll:  os.MkdirAll,
		WriteFile: os.WriteFile,
	}
}
