package generators

import (
	"os"
)

// osWriteFile is a shim for os.WriteFile
var osWriteFile = os.WriteFile

// osMkdirAll is a shim for os.MkdirAll
var osMkdirAll = os.MkdirAll

// osStat is a shim for os.Stat
var osStat = os.Stat
