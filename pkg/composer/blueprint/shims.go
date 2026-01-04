package blueprint

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

// Shims provides testable wrappers around external dependencies for the blueprint package.
// This enables dependency injection and mocking in unit tests while maintaining
// clean separation between business logic and external system interactions.
type Shims struct {
	Stat          func(string) (os.FileInfo, error)
	ReadFile      func(string) ([]byte, error)
	ReadDir       func(string) ([]os.DirEntry, error)
	WriteFile     func(string, []byte, os.FileMode) error
	MkdirAll      func(string, os.FileMode) error
	Walk          func(string, filepath.WalkFunc) error
	YamlMarshal   func(any) ([]byte, error)
	YamlUnmarshal func([]byte, any) error
	FilepathBase  func(string) string
	TrimSpace     func(string) string
	HasPrefix     func(string, string) bool
	Contains      func(string, string) bool
	Replace       func(string, string, string, int) string
}

// NewShims creates a new Shims instance with default implementations
// that delegate to the actual system functions and libraries.
func NewShims() *Shims {
	return &Shims{
		Stat:     os.Stat,
		ReadFile: os.ReadFile,
		ReadDir:  os.ReadDir,
		WriteFile: func(path string, data []byte, perm os.FileMode) error {
			return os.WriteFile(path, data, perm)
		},
		MkdirAll:     os.MkdirAll,
		Walk:         filepath.Walk,
		FilepathBase: filepath.Base,
		TrimSpace:    strings.TrimSpace,
		HasPrefix:    strings.HasPrefix,
		Contains:     strings.Contains,
		Replace:      strings.Replace,
		YamlMarshal: func(v any) ([]byte, error) {
			return yaml.Marshal(v)
		},
		YamlUnmarshal: func(data []byte, v any) error {
			return yaml.Unmarshal(data, v)
		},
	}
}
