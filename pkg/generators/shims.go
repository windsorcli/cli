package generators

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

// The shims package is a system call abstraction layer for the generators package
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

// =============================================================================
// Types
// =============================================================================

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	WriteFile     func(name string, data []byte, perm os.FileMode) error
	ReadFile      func(name string) ([]byte, error)
	MkdirAll      func(path string, perm os.FileMode) error
	Stat          func(name string) (os.FileInfo, error)
	MarshalYAML   func(v any) ([]byte, error)
	TempDir       func(dir, pattern string) (string, error)
	RemoveAll     func(path string) error
	Chdir         func(dir string) error
	ReadDir       func(name string) ([]os.DirEntry, error)
	Setenv        func(key, value string) error
	YamlUnmarshal func(data []byte, v any) error
	JsonMarshal   func(v any) ([]byte, error)
	JsonUnmarshal func(data []byte, v any) error
	FilepathRel   func(basepath, targpath string) (string, error)
}

// =============================================================================
// Helpers
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		WriteFile:     os.WriteFile,
		ReadFile:      os.ReadFile,
		MkdirAll:      os.MkdirAll,
		Stat:          os.Stat,
		MarshalYAML:   yaml.Marshal,
		TempDir:       os.MkdirTemp,
		RemoveAll:     os.RemoveAll,
		Chdir:         os.Chdir,
		ReadDir:       os.ReadDir,
		Setenv:        os.Setenv,
		YamlUnmarshal: yaml.Unmarshal,
		JsonMarshal:   json.Marshal,
		JsonUnmarshal: json.Unmarshal,
		FilepathRel:   filepath.Rel,
	}
}
