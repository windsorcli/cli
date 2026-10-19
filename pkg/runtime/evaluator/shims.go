// The Shims are the system call abstractions of the evaluator package.
// It provides the shared shims of internal/shims and a factory for the Jsonnet VM.
// Tests replace the shims to simulate file reads, marshal failures, and Jsonnet output.
// NewShims returns the shims with the real implementations.

package evaluator

import (
	"github.com/google/go-jsonnet"
	"github.com/windsorcli/cli/internal/shims"
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

// Shims holds the shared system call primitives and the Jsonnet VM factory.
type Shims struct {
	shims.Shims
	NewJsonnetVM func() JsonnetVM
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its real implementation.
func NewShims() *Shims {
	return &Shims{
		Shims: *shims.NewShims(),
		NewJsonnetVM: func() JsonnetVM {
			return &realJsonnetVM{vm: jsonnet.MakeVM()}
		},
	}
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure realJsonnetVM implements the JsonnetVM interface
var _ JsonnetVM = (*realJsonnetVM)(nil)
