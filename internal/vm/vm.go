package vm

import (
	"encoding/json"
	"io"
	"os"
	"runtime"

	"github.com/goccy/go-yaml"
	"github.com/shirou/gopsutil/mem"
)

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

// YAMLEncoder is an interface for encoding YAML data.
type YAMLEncoder interface {
	Encode(v interface{}) error
	Close() error
}

// newYAMLEncoder is a function that returns a new YAML encoder.
var newYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
	return yaml.NewEncoder(w, opts...)
}

// VMInfo holds the information about the virtual machine
type VMInfo struct {
	Address string
	Arch    string
	CPUs    int
	Disk    float64
	Memory  float64
	Name    string
	Runtime string
	Status  string
}

// VMInterface defines methods for VM operations
type VMInterface interface {
	Up(verbose ...bool) error
	Down(verbose ...bool) error
	Delete(verbose ...bool) error
	Info() (interface{}, error)
}