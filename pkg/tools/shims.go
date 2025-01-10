package tools

import "os"

// osStat is a variable that can be overridden for testing purposes, acting as a shim for os.Stat.
var osStat = os.Stat
