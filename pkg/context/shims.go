package context

import "os"

// osStat is a shim for os.Stat
var osStat = os.Stat

// osRemoveAll is a shim for os.RemoveAll
var osRemoveAll = os.RemoveAll
