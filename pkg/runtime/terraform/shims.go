// The shims package is a system call abstraction layer
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

package terraform

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// =============================================================================
// Types
// =============================================================================

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	ReadFile       func(string) ([]byte, error)
	JsonUnmarshal  func([]byte, any) error
	JsonMarshal    func(any) ([]byte, error)
	Getenv         func(string) string
	Setenv         func(string, string) error
	Unsetenv       func(string) error
	Stat           func(string) (os.FileInfo, error)
	Remove         func(string) error
	WriteFile      func(string, []byte, os.FileMode) error
	Getwd          func() (string, error)
	YamlUnmarshal  func([]byte, any) error
	YamlMarshal    func(any) ([]byte, error)
	Glob           func(string) ([]string, error)
	HclParseConfig func([]byte, string, hcl.Pos) (*hclwrite.File, hcl.Diagnostics)
}

// =============================================================================
// Helpers
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		ReadFile:       os.ReadFile,
		JsonUnmarshal:  json.Unmarshal,
		JsonMarshal:    json.Marshal,
		Getenv:         os.Getenv,
		Setenv:         os.Setenv,
		Unsetenv:       os.Unsetenv,
		Stat:           os.Stat,
		Remove:         os.Remove,
		WriteFile:      os.WriteFile,
		Getwd:          os.Getwd,
		YamlUnmarshal:  yaml.Unmarshal,
		YamlMarshal:    yaml.Marshal,
		Glob:           filepath.Glob,
		HclParseConfig: hclwrite.ParseConfig,
	}
}
