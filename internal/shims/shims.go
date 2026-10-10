// The Shims is a set of mockable system call primitives shared across packages.
// It provides file, environment, and marshal functions as replaceable function fields.
// Package Shims structs embed it by value and add only their package-specific fields.
// Tests replace a promoted field on the embedding struct to intercept a call.

package shims

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"github.com/goccy/go-yaml"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the system call primitives that many packages need.
type Shims struct {
	ReadFile      func(name string) ([]byte, error)
	WriteFile     func(name string, data []byte, perm os.FileMode) error
	Stat          func(name string) (os.FileInfo, error)
	MkdirAll      func(path string, perm os.FileMode) error
	Remove        func(name string) error
	RemoveAll     func(path string) error
	Rename        func(oldpath, newpath string) error
	ReadDir       func(name string) ([]os.DirEntry, error)
	Glob          func(pattern string) ([]string, error)
	Getwd         func() (string, error)
	UserHomeDir   func() (string, error)
	Getenv        func(key string) string
	Setenv        func(key, value string) error
	LookupEnv     func(key string) (string, bool)
	Goos          func() string
	YamlMarshal   func(v any) ([]byte, error)
	YamlUnmarshal func(data []byte, v any) error
	JsonMarshal   func(v any) ([]byte, error)
	JsonUnmarshal func(data []byte, v any) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its standard library or go-yaml implementation.
func NewShims() *Shims {
	return &Shims{
		ReadFile:      os.ReadFile,
		WriteFile:     os.WriteFile,
		Stat:          os.Stat,
		MkdirAll:      os.MkdirAll,
		Remove:        os.Remove,
		RemoveAll:     os.RemoveAll,
		Rename:        os.Rename,
		ReadDir:       os.ReadDir,
		Glob:          filepath.Glob,
		Getwd:         os.Getwd,
		UserHomeDir:   os.UserHomeDir,
		Getenv:        os.Getenv,
		Setenv:        os.Setenv,
		LookupEnv:     os.LookupEnv,
		Goos:          func() string { return runtime.GOOS },
		YamlMarshal:   func(v any) ([]byte, error) { return yaml.Marshal(v) },
		YamlUnmarshal: func(data []byte, v any) error { return yaml.Unmarshal(data, v) },
		JsonMarshal:   json.Marshal,
		JsonUnmarshal: json.Unmarshal,
	}
}
