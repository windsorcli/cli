package context

import "os"

// osStat is a shim for os.Stat
var osStat = os.Stat

// osRemoveAll is a shim for os.RemoveAll
var osRemoveAll = os.RemoveAll

// osReadFile is a shim for os.ReadFile
var osReadFile = os.ReadFile

// osWriteFile is a shim for os.WriteFile
var osWriteFile = os.WriteFile

// osMkdirAll is a shim for os.MkdirAll
var osMkdirAll = os.MkdirAll
