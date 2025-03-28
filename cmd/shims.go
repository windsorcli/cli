package cmd

import (
	"net"
	"os"
	"os/exec"
	"runtime"
)

// osUserHomeDir retrieves the user's home directory
var osUserHomeDir = os.UserHomeDir

// osStat retrieves the file information
var osStat = os.Stat

// osRemoveAll removes a directory and all its contents
var osRemoveAll = os.RemoveAll

// osExit is a function to exit the program
var osExit = os.Exit

// getwd retrieves the current working directory
var getwd = os.Getwd

// osSetenv sets an environment variable
var osSetenv = os.Setenv

// execCommand is the instance for executing commands
var execCommand = exec.Command

// netInterfaces retrieves the network interfaces
var netInterfaces = net.Interfaces

// Define a variable for runtime.GOOS for easier testing
var goos = func() string {
	return runtime.GOOS
}
