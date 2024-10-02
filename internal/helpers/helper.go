package helpers

import (
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/goccy/go-yaml"
)

// FileWriter is an interface that wraps the basic Write, WriteString, and Close methods.
type FileWriter interface {
	Write(p []byte) (n int, err error)
	WriteString(s string) (n int, err error)
	Close() error
}

// Helper is an interface that defines methods for retrieving environment variables
// and can be implemented for individual providers.
type Helper interface {
	// GetEnvVars retrieves environment variables for the current context.
	GetEnvVars() (map[string]string, error)

	// PostEnvExec runs any necessary commands after the environment variables have been set.
	PostEnvExec() error

	// SetConfig sets the configuration value for the given key.
	SetConfig(key, value string) error
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

// Override variable for os.CreateTemp
var osCreateTemp = os.CreateTemp

// Override variable for os.Create
var osCreate = func(name string) (FileWriter, error) {
	return os.Create(name)
}

// Override variable for yaml.NewEncoder
var newYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
	return yaml.NewEncoder(w, opts...)
}

// goArch is a wrapper function around runtime.GOARCH
var goArch = func() string {
	return runtime.GOARCH
}
