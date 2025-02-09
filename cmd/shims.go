package cmd

import (
	"net"
	"os"
	"os/exec"
	"runtime"
)

// exitFunc is a function to exit the program
var exitFunc = os.Exit

// osUserHomeDir retrieves the user's home directory
var osUserHomeDir = os.UserHomeDir

// osStat retrieves the file information
var osStat = os.Stat

// getwd retrieves the current working directory
var getwd = os.Getwd

// verbose is a flag for verbose output
var verbose bool

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
