package env

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/goccy/go-yaml"
)

// stat is a variable that holds the os.Stat function for mocking
var stat = os.Stat

// Define a variable for os.Getwd() for easier testing
var getwd = os.Getwd

// Define a variable for filepath.Glob for easier testing
var glob = filepath.Glob

// Wrapper function for os.WriteFile
var writeFile = os.WriteFile

// Wrapper function for os.ReadDir
var readDir = os.ReadDir

// Wrapper function for yaml.Unmarshal
var yamlUnmarshal = yaml.Unmarshal

// Wrapper function for yaml.Marshal
var yamlMarshal = yaml.Marshal

// intPtr returns a pointer to an int value
func intPtr(i int) *int {
	return &i
}

// stringPtr returns a pointer to a string value
func stringPtr(s string) *string {
	return &s
}

// Define a variable for runtime.GOOS for easier testing
var goos = func() string {
	return runtime.GOOS
}

// Define a variable for os.UserHomeDir for easier testing
var osUserHomeDir = os.UserHomeDir

// Define a variable for os.MkdirAll for easier testing
var mkdirAll = os.MkdirAll

// Define a variable for os.ReadFile for easier testing
var readFile = os.ReadFile

// Define a variable for exec.LookPath for easier testing
var execLookPath = exec.LookPath

// Define a variable for os.LookupEnv for easier testing
var osLookupEnv = os.LookupEnv

// Define a variable for os.Remove for easier testing
var osRemove = os.Remove
