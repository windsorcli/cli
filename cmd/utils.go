package cmd

import (
	"net"
	"os"
	"os/exec"

	"github.com/windsor-hotel/cli/internal/di"
)

// exitFunc is a function to exit the program
var exitFunc = os.Exit

// osUserHomeDir retrieves the user's home directory
var osUserHomeDir = os.UserHomeDir

// osStat retrieves the file information
var osStat = os.Stat

// getwd retrieves the current working directory
var getwd = os.Getwd

// injector is the dependency injector
var injector di.Injector

// verbose is a flag for verbose output
var verbose bool

// osSetenv sets an environment variable
var osSetenv = os.Setenv

// execCommand is the instance for executing commands
var execCommand = exec.Command

// netInterfaces retrieves the network interfaces
var netInterfaces = net.Interfaces

// ptrBool returns a pointer to a boolean value
func ptrBool(b bool) *bool {
	return &b
}

// ptrString returns a pointer to a string value
func ptrString(s string) *string {
	return &s
}
