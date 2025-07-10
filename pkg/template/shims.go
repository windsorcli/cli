package template

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/google/go-jsonnet"
)

// The shims package is a system call abstraction layer for the template package
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

// =============================================================================
// Types
// =============================================================================

// JsonnetVM provides an interface for Jsonnet virtual machine operations
type JsonnetVM interface {
	ExtCode(key, val string)
	EvaluateAnonymousSnippet(filename, snippet string) (string, error)
}

// RealJsonnetVM is the real implementation of JsonnetVM
type RealJsonnetVM struct {
	vm *jsonnet.VM
}

// ExtCode sets external code for the Jsonnet VM
func (j *RealJsonnetVM) ExtCode(key, val string) {
	j.vm.ExtCode(key, val)
}

// EvaluateAnonymousSnippet evaluates a Jsonnet snippet
func (j *RealJsonnetVM) EvaluateAnonymousSnippet(filename, snippet string) (string, error) {
	return j.vm.EvaluateAnonymousSnippet(filename, snippet)
}

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	ReadFile      func(name string) ([]byte, error)
	ReadDir       func(name string) ([]os.DirEntry, error)
	Stat          func(name string) (os.FileInfo, error)
	WriteFile     func(name string, data []byte, perm os.FileMode) error
	MkdirAll      func(path string, perm os.FileMode) error
	Getenv        func(key string) string
	YamlUnmarshal func(data []byte, v any) error
	YamlMarshal   func(v any) ([]byte, error)
	JsonMarshal   func(v any) ([]byte, error)
	JsonUnmarshal func(data []byte, v any) error
	NewJsonnetVM  func() JsonnetVM
	FilepathBase  func(path string) string
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		ReadFile:      os.ReadFile,
		ReadDir:       os.ReadDir,
		Stat:          os.Stat,
		WriteFile:     os.WriteFile,
		MkdirAll:      os.MkdirAll,
		Getenv:        os.Getenv,
		YamlUnmarshal: yaml.Unmarshal,
		YamlMarshal:   yaml.Marshal,
		JsonMarshal:   json.Marshal,
		JsonUnmarshal: json.Unmarshal,
		NewJsonnetVM: func() JsonnetVM {
			return &RealJsonnetVM{vm: jsonnet.MakeVM()}
		},
		FilepathBase: filepath.Base,
	}
}
