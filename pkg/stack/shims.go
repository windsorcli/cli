package stack

// The Shims provides system call shims for the stack package.
// It provides variables that can be overridden for testing purposes,
// The Shims act as replaceable system call interfaces,
// enabling test control over file system and environment operations.

import "os"

// osStat is a variable that can be overridden for testing purposes, acting as a shim for os.Stat.
var osStat = os.Stat

// osChdir is a variable that can be overridden for testing purposes, acting as a shim for os.Chdir.
var osChdir = os.Chdir

// osGetwd is a variable that can be overridden for testing purposes, acting as a shim for os.Getwd.
var osGetwd = os.Getwd

// osSetenv is a variable that can be overridden for testing purposes, acting as a shim for os.Setenv.
var osSetenv = os.Setenv

// osRemove is a variable that can be overridden for testing purposes, acting as a shim for os.Remove.
var osRemove = os.Remove
