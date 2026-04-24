package tools

import (
	"os"
	"os/exec"
)

// osStat is a variable that can be overridden for testing purposes, acting as a shim for os.Stat.
var osStat = os.Stat

// osReadFile is a variable that can be overridden for testing purposes, acting as a shim for os.ReadFile.
var osReadFile = os.ReadFile

// execLookPath is a variable that can be overridden for testing purposes, acting as a shim for exec.LookPath.
var execLookPath = exec.LookPath
