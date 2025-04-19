package blueprint

import (
	"encoding/json"
	"os"
	"regexp"

	"github.com/goccy/go-yaml"
	"github.com/google/go-jsonnet"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// =============================================================================
// Shims
// =============================================================================

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	// YAML and JSON shims
	YamlMarshalNonNull func(v any) ([]byte, error)
	YamlMarshal        func(any) ([]byte, error)
	YamlUnmarshal      func([]byte, any) error
	JsonMarshal        func(any) ([]byte, error)
	JsonUnmarshal      func([]byte, any) error
	K8sYamlUnmarshal   func([]byte, any) error

	// File system shims
	WriteFile func(string, []byte, os.FileMode) error
	MkdirAll  func(string, os.FileMode) error
	Stat      func(string) (os.FileInfo, error)
	ReadFile  func(string) ([]byte, error)

	// Utility shims
	RegexpMatchString func(pattern string, s string) (bool, error)

	// Kubernetes shims
	ClientcmdBuildConfigFromFlags func(masterUrl, kubeconfigPath string) (*rest.Config, error)
	RestInClusterConfig           func() (*rest.Config, error)
	KubernetesNewForConfig        func(*rest.Config) (*kubernetes.Clientset, error)

	// Jsonnet shims
	NewJsonnetVM func() JsonnetVM
}

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	return &Shims{
		// YAML and JSON shims
		YamlMarshalNonNull: func(v any) ([]byte, error) {
			return yaml.Marshal(v)
		},
		YamlMarshal:      yaml.Marshal,
		YamlUnmarshal:    yaml.Unmarshal,
		JsonMarshal:      json.Marshal,
		JsonUnmarshal:    json.Unmarshal,
		K8sYamlUnmarshal: yaml.Unmarshal,

		// File system shims
		WriteFile: os.WriteFile,
		MkdirAll:  os.MkdirAll,
		Stat:      os.Stat,
		ReadFile:  os.ReadFile,

		// Utility shims
		RegexpMatchString: regexp.MatchString,

		// Kubernetes shims
		ClientcmdBuildConfigFromFlags: clientcmd.BuildConfigFromFlags,
		RestInClusterConfig:           rest.InClusterConfig,
		KubernetesNewForConfig:        kubernetes.NewForConfig,

		// Jsonnet shims
		NewJsonnetVM: NewJsonnetVM,
	}
}

// =============================================================================
// Jsonnet VM Implementation
// =============================================================================

// JsonnetVM defines the interface for Jsonnet virtual machines
type JsonnetVM interface {
	// TLACode sets a top-level argument using code
	TLACode(key, val string)
	// ExtCode sets an external variable using code
	ExtCode(key, val string)
	// EvaluateAnonymousSnippet evaluates a jsonnet snippet
	EvaluateAnonymousSnippet(filename, snippet string) (string, error)
}

// realJsonnetVM implements JsonnetVM using the actual jsonnet implementation
type realJsonnetVM struct {
	vm *jsonnet.VM
}

// NewJsonnetVM creates a new JsonnetVM using the real jsonnet implementation
func NewJsonnetVM() JsonnetVM {
	return &realJsonnetVM{vm: jsonnet.MakeVM()}
}

func (j *realJsonnetVM) TLACode(key, val string) {
	j.vm.TLACode(key, val)
}

func (j *realJsonnetVM) ExtCode(key, val string) {
	j.vm.ExtCode(key, val)
}

func (j *realJsonnetVM) EvaluateAnonymousSnippet(filename, snippet string) (string, error) {
	return j.vm.EvaluateAnonymousSnippet(filename, snippet)
}

// =============================================================================
// Helper Functions
// =============================================================================

// Helper functions to create pointers for basic types
func ptrString(s string) *string {
	return &s
}

// metav1Duration is a shim for metav1.Duration
type metav1Duration = metav1.Duration
