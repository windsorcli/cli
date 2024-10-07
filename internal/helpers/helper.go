package helpers

import (
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/goccy/go-yaml"
)

// Helper is an interface that defines methods for retrieving environment variables
// and can be implemented for individual providers.
type Helper interface {
	// GetEnvVars retrieves environment variables for the current context.
	GetEnvVars() (map[string]string, error)

	// PostEnvExec runs any necessary commands after the environment variables have been set.
	PostEnvExec() error

	// SetConfig sets the configuration value for the given key.
	SetConfig(key, value string) error

	// GetContainerConfig returns a list of container data for docker-compose.
	GetContainerConfig() ([]map[string]interface{}, error)
}

// Define a variable for os.Getwd() for easier testing
var getwd = os.Getwd

// Define a variable for filepath.Glob for easier testing
var glob = filepath.Glob

// Wrapper function for os.WriteFile
var writeFile = os.WriteFile

// Override variable for os.Stat
var stat = os.Stat

// Override variable for os.MkdirAll
var mkdirAll = os.MkdirAll

// Override variable for yaml.NewEncoder
var newYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
	return yaml.NewEncoder(w, opts...)
}

// goArch is a wrapper function around runtime.GOARCH
var goArch = func() string {
	return runtime.GOARCH
}

// Wrapper function for os.Rename
var rename = os.Rename

// Override variable for yaml.Marshal
var yamlMarshal = yaml.Marshal

// Mockable function for os.UserHomeDir
var userHomeDir = os.UserHomeDir
