// The shims package is a system call abstraction layer
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

package evaluator

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/google/go-jsonnet"
)

// =============================================================================
// Types
// =============================================================================

// JsonnetVM provides an interface for Jsonnet virtual machine operations.
// It abstracts the underlying Jsonnet VM implementation to enable testing
// and dependency injection. The interface supports setting external code,
// configuring importers, and evaluating Jsonnet snippets.
type JsonnetVM interface {
	ExtCode(key, val string)
	Importer(importer jsonnet.Importer)
	EvaluateAnonymousSnippet(filename, snippet string) (string, error)
}

// realJsonnetVM is the concrete implementation of JsonnetVM that wraps
// the actual Jsonnet VM from the go-jsonnet library. It provides the
// real functionality for Jsonnet evaluation in production code.
type realJsonnetVM struct {
	vm *jsonnet.VM
}

// ExtCode sets external code variables for the Jsonnet VM.
// This allows passing external data into Jsonnet evaluation contexts,
// enabling Jsonnet code to access runtime values and configuration.
func (j *realJsonnetVM) ExtCode(key, val string) {
	j.vm.ExtCode(key, val)
}

// Importer configures the file importer for the Jsonnet VM.
// This determines how Jsonnet resolves import statements and file paths,
// enabling support for relative imports and custom import resolution.
func (j *realJsonnetVM) Importer(importer jsonnet.Importer) {
	j.vm.Importer(importer)
}

// EvaluateAnonymousSnippet evaluates a Jsonnet code snippet as a string.
// The filename parameter is used for error reporting and import resolution.
// Returns the evaluated JSON output as a string, or an error if evaluation fails.
func (j *realJsonnetVM) EvaluateAnonymousSnippet(filename, snippet string) (string, error) {
	return j.vm.EvaluateAnonymousSnippet(filename, snippet)
}

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	ReadFile      func(string) ([]byte, error)
	JsonMarshal   func(any) ([]byte, error)
	JsonUnmarshal func([]byte, any) error
	YamlMarshal   func(any) ([]byte, error)
	FilepathBase  func(string) string
	NewJsonnetVM  func() JsonnetVM
}

// =============================================================================
// Helpers
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		ReadFile:      os.ReadFile,
		JsonMarshal:   json.Marshal,
		JsonUnmarshal: json.Unmarshal,
		YamlMarshal:   yaml.Marshal,
		FilepathBase:  filepath.Base,
		NewJsonnetVM: func() JsonnetVM {
			return &realJsonnetVM{vm: jsonnet.MakeVM()}
		},
	}
}

