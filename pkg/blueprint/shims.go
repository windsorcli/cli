package blueprint

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/goccy/go-yaml"
	"github.com/google/go-jsonnet"
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

// osReadFile is a wrapper around os.ReadFile
var osReadFile = os.ReadFile

// osStat is a wrapper around os.Stat
var osStat = os.Stat

// osMkdirAll is a wrapper around os.MkdirAll
var osMkdirAll = os.MkdirAll

// regexpMatchString is a shim for regexp.MatchString
var regexpMatchString = regexp.MatchString

// jsonMarshal is a wrapper around json.Marshal
var jsonMarshal = json.Marshal

// jsonUnmarshal is a wrapper around json.Unmarshal
var jsonUnmarshal = json.Unmarshal

// yamlJSONToYAML is a wrapper around yaml.JSONToYAML
var yamlJSONToYAML = yaml.JSONToYAML

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

// TLACode is a wrapper around jsonnet.VM.TLACode
func (vm *jsonnetVM) TLACode(key, val string) {
	vm.VM.TLACode(key, val)
}

// ExtCode is a wrapper around jsonnet.VM.ExtCode
func (vm *jsonnetVM) ExtCode(key, val string) {
	vm.VM.ExtCode(key, val)
}

// mockJsonnetVM is a mock implementation of jsonnetVMInterface for testing
type mockJsonnetVM struct{}

// TLACode is a mock implementation that does nothing
func (vm *mockJsonnetVM) TLACode(key, val string) {}

// EvaluateAnonymousSnippet is a mock implementation that returns an error
func (vm *mockJsonnetVM) EvaluateAnonymousSnippet(filename, snippet string) (string, error) {
	return "", fmt.Errorf("error evaluating snippet")
}

// ExtCode is a mock implementation that does nothing
func (vm *mockJsonnetVM) ExtCode(key, val string) {}
