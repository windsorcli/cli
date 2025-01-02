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

// yamlMarshalNonNull marshals the given struct into YAML data, omitting null values
var yamlMarshalNonNull = func(v interface{}) ([]byte, error) {
	return yaml.Marshal(v)
}

// yamlMarshal is a wrapper around yaml.Marshal
var yamlMarshal = yaml.Marshal

// yamlUnmarshal is a wrapper around yaml.Unmarshal
var yamlUnmarshal = yaml.Unmarshal

// osWriteFile is a wrapper around os.WriteFile
var osWriteFile = os.WriteFile

// osMkdirAll is a wrapper around os.MkdirAll
var osMkdirAll = os.MkdirAll

// jsonMarshal is a wrapper around json.Marshal
var jsonMarshal = json.Marshal

// jsonnetMakeVMFunc is a function type for creating a new jsonnet VM
type jsonnetMakeVMFunc func() jsonnetVMInterface

// jsonnetVMInterface defines the interface for a jsonnet VM
type jsonnetVMInterface interface {
	TLACode(key, val string)
	EvaluateAnonymousSnippet(filename, snippet string) (string, error)
	ExtCode(key, val string)
}

// jsonnetMakeVM is a variable holding the function to create a new jsonnet VM
var jsonnetMakeVM jsonnetMakeVMFunc = func() jsonnetVMInterface {
	return &jsonnetVM{VM: jsonnet.MakeVM()}
}

// jsonnetVM is a wrapper around jsonnet.VM that implements jsonnetVMInterface
type jsonnetVM struct {
	*jsonnet.VM
}

// EvaluateAnonymousSnippet is a wrapper around jsonnet.VM.EvaluateAnonymousSnippet
func (vm *jsonnetVM) EvaluateAnonymousSnippet(filename, snippet string) (string, error) {
	return vm.VM.EvaluateAnonymousSnippet(filename, snippet)
}

// ExtCode is a wrapper around jsonnet.VM.ExtCode
func (vm *jsonnetVM) ExtCode(key, val string) {
	vm.VM.ExtCode(key, val)
}

// Shim for Kubernetes client-go functions

// clientcmdBuildConfigFromFlags is a shim for clientcmd.BuildConfigFromFlags
var clientcmdBuildConfigFromFlags = clientcmd.BuildConfigFromFlags

// restInClusterConfig is a shim for rest.InClusterConfig
var restInClusterConfig = rest.InClusterConfig

// kubernetesNewForConfigFunc is a function type for creating a new Kubernetes client
type kubernetesNewForConfigFunc func(config *rest.Config) (kubernetes.Interface, error)

// metav1Duration is a shim for metav1.Duration
type metav1Duration = metav1.Duration

// Add back the missing functions from file_context_0
var (
	regexpMatchString = regexp.MatchString
	osStat            = os.Stat
	osReadFile        = os.ReadFile
	k8sYamlUnmarshal  = yaml.Unmarshal
)
