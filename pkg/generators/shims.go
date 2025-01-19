package generators

import (
	"os"

	"github.com/goccy/go-yaml"
)

// osWriteFile is a shim for os.WriteFile
var osWriteFile = os.WriteFile

// osReadFile is a shim for os.ReadFile
var osReadFile = os.ReadFile

// osMkdirAll is a shim for os.MkdirAll
var osMkdirAll = os.MkdirAll

// osStat is a shim for os.Stat
var osStat = os.Stat

// yamlMarshal is a shim for yaml.Marshal
var yamlMarshal = yaml.Marshal
