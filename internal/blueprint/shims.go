package blueprint

import (
	"os"
	"regexp"

	"github.com/goccy/go-yaml"
)

// yamlMarshalNonNull marshals the given struct into YAML data, omitting null values
var yamlMarshalNonNull = func(v interface{}) ([]byte, error) {
	return yaml.Marshal(v)
}

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
