package generators

import (
	"os"
	"regexp"
)

// osWriteFile is a shim for os.WriteFile
var osWriteFile = os.WriteFile

// osMkdirAll is a shim for os.MkdirAll
var osMkdirAll = os.MkdirAll

// osStat is a shim for os.Stat
var osStat = os.Stat

// regexpMatchString is a shim for regexp.MatchString
var regexpMatchString = regexp.MatchString
