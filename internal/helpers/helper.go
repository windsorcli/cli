package helpers

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/goccy/go-yaml"
)

// Helper is an interface that defines methods for retrieving environment variables
// and can be implemented for individual providers.
type Helper interface {
	// Initialize performs any necessary initialization for the helper.
	Initialize() error

	// GetComposeConfig returns the top-level compose configuration including a list of container data for docker-compose.
	GetComposeConfig() (*types.Config, error)

	// WriteConfig writes any vendor specific configuration files that are needed for the helper.
	WriteConfig() error

	// Up executes necessary commands to instantiate the tool or environment.
	Up(verbose ...bool) error

	// Info returns information about the helper.
	Info() (interface{}, error)
}

// Define a variable for os.Getwd() for easier testing
var getwd = os.Getwd

// Define a variable for filepath.Glob for easier testing
var glob = filepath.Glob

// Wrapper function for os.WriteFile
var writeFile = os.WriteFile

// Override variable for os.Stat
var stat = os.Stat

// Override variable for os.Mkdir
var mkdir = os.Mkdir

// Override variable for os.MkdirAll
var mkdirAll = os.MkdirAll

// Wrapper function for os.Rename
var rename = os.Rename

// Override variable for yaml.Marshal
var yamlMarshal = yaml.Marshal

// Override variable for yaml.Unmarshal
var yamlUnmarshal = yaml.Unmarshal

// Override variable for json.Unmarshal
var jsonUnmarshal = json.Unmarshal

// Mockable function for os.UserHomeDir
var userHomeDir = os.UserHomeDir

// Helper functions to create pointers for basic types
func ptrString(s string) *string {
	return &s
}

func ptrBool(b bool) *bool {
	return &b
}

func ptrInt(i int) *int {
	return &i
}
