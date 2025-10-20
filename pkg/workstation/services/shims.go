package services

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

// The Shims package is a system call abstraction layer for the services package
// It provides mockable wrappers around system and runtime functions
// The Shims package enables dependency injection and test isolation
// by allowing system calls to be intercepted and replaced in tests

// =============================================================================
// Types
// =============================================================================

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	Getwd         func() (string, error)
	Glob          func(pattern string) (matches []string, err error)
	WriteFile     func(filename string, data []byte, perm os.FileMode) error
	Stat          func(name string) (os.FileInfo, error)
	Mkdir         func(path string, perm os.FileMode) error
	MkdirAll      func(path string, perm os.FileMode) error
	RemoveAll     func(path string) error
	Rename        func(oldpath, newpath string) error
	YamlMarshal   func(in any) ([]byte, error)
	YamlUnmarshal func(in []byte, out any) error
	JsonUnmarshal func(data []byte, v any) error
	UserHomeDir   func() (string, error)
}

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		Getwd:         os.Getwd,
		Glob:          filepath.Glob,
		WriteFile:     os.WriteFile,
		Stat:          os.Stat,
		Mkdir:         os.Mkdir,
		MkdirAll:      os.MkdirAll,
		RemoveAll:     os.RemoveAll,
		Rename:        os.Rename,
		YamlMarshal:   yaml.Marshal,
		YamlUnmarshal: yaml.Unmarshal,
		JsonUnmarshal: json.Unmarshal,
		UserHomeDir:   os.UserHomeDir,
	}
}

// =============================================================================
// Helpers
// =============================================================================

// Helper functions to create pointers for basic types
func ptrString(s string) *string {
	return &s
}
