package blueprint

import (
	"os"

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
