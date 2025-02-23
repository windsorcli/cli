package generators

import (
	"os"

	"github.com/goccy/go-yaml"
	"gopkg.in/ini.v1"
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

// iniLoad is a shim for ini.Load used in AWSGenerator
var iniLoad = ini.Load

// iniEmpty is a shim for ini.Empty used in AWSGenerator
var iniEmpty = ini.Empty

// iniSaveTo is a shim for cfg.SaveTo used in AWSGenerator
var iniSaveTo = func(cfg *ini.File, filename string) error {
	return cfg.SaveTo(filename)
}
