package stack

import "os"

// osStat is a shim for os.Stat that allows us to mock the function in tests
var osStat = os.Stat

// osChdir is a shim for os.Chdir that allows us to mock the function in tests
var osChdir = os.Chdir

// osGetwd is a shim for os.Getwd that allows us to mock the function in tests
var osGetwd = os.Getwd

// osSetenv is a shim for os.Setenv that allows us to mock the function in tests
var osSetenv = os.Setenv

// osRemove is a shim for os.Remove that allows us to mock the function in tests
var osRemove = os.Remove
