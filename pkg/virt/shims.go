package virt

import (
	"encoding/json"
	"io"
	"os"
	"runtime"

	"github.com/goccy/go-yaml"
	"github.com/shirou/gopsutil/mem"
)

// osSetenv is a variable that holds the os.Setenv function to set an environment variable.
var osSetenv = os.Setenv

// jsonUnmarshal is a variable that holds the json.Unmarshal function for decoding JSON data.
var jsonUnmarshal = json.Unmarshal

// userHomeDir is a variable that holds the os.UserHomeDir function to get the current user's home directory.
var userHomeDir = os.UserHomeDir

// mkdirAll is a variable that holds the os.MkdirAll function to create a directory and all necessary parents.
var mkdirAll = os.MkdirAll

// writeFile is a variable that holds the os.WriteFile function to write data to a file.
var writeFile = os.WriteFile

// rename is a variable that holds the os.Rename function to rename a file or directory.
var rename = os.Rename

// goArch is a variable that holds the runtime.GOARCH function to get the architecture of the current runtime.
var goArch = runtime.GOARCH

// numCPU is a variable that holds the runtime.NumCPU function to get the number of logical CPUs available to the current process.
var numCPU = runtime.NumCPU

// Mockable function for mem.VirtualMemory
var virtualMemory = mem.VirtualMemory

// ptrString is a function that creates a pointer to a string.
func ptrString(s string) *string {
	return &s
}

// ptrInt is a function that creates a pointer to an int.
func ptrInt(i int) *int {
	return &i
}

// ptrBool is a function that creates a pointer to a bool.
func ptrBool(b bool) *bool {
	return &b
}

// YAMLEncoder is an interface for encoding YAML data.
type YAMLEncoder interface {
	Encode(v interface{}) error
	Close() error
}

// yamlMarshal is a variable that holds the yaml.Marshal function to marshal a value to YAML.
var yamlMarshal = yaml.Marshal

// newYAMLEncoder is a function that returns a new YAML encoder.
var newYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
	return yaml.NewEncoder(w, opts...)
}
