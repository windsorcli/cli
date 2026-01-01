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

