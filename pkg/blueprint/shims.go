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

// YAML and JSON shims
// yamlMarshalNonNull marshals the given struct into YAML data, omitting null values
var yamlMarshalNonNull = func(v interface{}) ([]byte, error) {
	return yaml.Marshal(v)
}

// yamlMarshal is a wrapper around yaml.Marshal
var yamlMarshal = yaml.Marshal

// yamlUnmarshal is a wrapper around yaml.Unmarshal
var yamlUnmarshal = yaml.Unmarshal

// jsonMarshal is a wrapper around json.Marshal
var jsonMarshal = json.Marshal

// jsonUnmarshal is a wrapper around json.Unmarshal
var jsonUnmarshal = json.Unmarshal

// k8sYamlUnmarshal is a wrapper around yaml.Unmarshal for Kubernetes YAML
var k8sYamlUnmarshal = yaml.Unmarshal

// File system shims
// osWriteFile is a wrapper around os.WriteFile
var osWriteFile = os.WriteFile

// osMkdirAll is a wrapper around os.MkdirAll
var osMkdirAll = os.MkdirAll

// osStat is a wrapper around os.Stat
var osStat = os.Stat

// osReadFile is a wrapper around os.ReadFile
var osReadFile = os.ReadFile

// Other utility shims
// regexpMatchString is a wrapper around regexp.MatchString
var regexpMatchString = regexp.MatchString

// Kubernetes client-go shims
// clientcmdBuildConfigFromFlags is a shim for clientcmd.BuildConfigFromFlags
var clientcmdBuildConfigFromFlags = clientcmd.BuildConfigFromFlags

// restInClusterConfig is a shim for rest.InClusterConfig
var restInClusterConfig = rest.InClusterConfig

// kubernetesNewForConfig is a shim for kubernetes.NewForConfig
var kubernetesNewForConfig = kubernetes.NewForConfig

// metav1Duration is a shim for metav1.Duration
type metav1Duration = metav1.Duration

// ----------------------------------------------------------------------------
// Jsonnet VM Implementation
// ----------------------------------------------------------------------------

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
var NewJsonnetVM = func() JsonnetVM {
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

// jsonnetMakeVM is the main shim used by the application to create JsonnetVMs
// By default it uses the real implementation but can be replaced in tests
var jsonnetMakeVM = NewJsonnetVM

func ptrString(s string) *string { return &s }
