package blueprint

import (
	"encoding/json"
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

// jsonnetMakeVM is a wrapper around jsonnet.MakeVM
var jsonnetMakeVM = jsonnet.MakeVM

// jsonnetVM is a wrapper around jsonnet.VM
type jsonnetVM struct {
	*jsonnet.VM
}

// jsonnetVM_TLACode is a wrapper around jsonnet.VM.TLACode
func (vm *jsonnetVM) TLACode(key, val string) {
	vm.VM.TLACode(key, val)
}

// jsonnetVM_EvaluateAnonymousSnippet is a wrapper around jsonnet.VM.EvaluateAnonymousSnippet
func (vm *jsonnetVM) EvaluateAnonymousSnippet(filename, snippet string) (string, error) {
	return vm.VM.EvaluateAnonymousSnippet(filename, snippet)
}
