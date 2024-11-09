package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// EnvPrinter defines the method for printing environment variables.
type EnvPrinter interface {
	Print() error
	GetEnvVars() (map[string]string, error)
	PostEnvHook() error
}

// Env is a struct that implements the EnvPrinter interface.
type Env struct {
	Injector di.Injector
}

// Print prints the environment variables to the console.
// It can optionally take a map of key:value strings and prints those.
func (e *Env) Print(customVars ...map[string]string) error {
	var envVars map[string]string

	// Use only the passed vars
	if len(customVars) > 0 {
		envVars = customVars[0]
	} else {
		envVars = make(map[string]string)
	}

	// Use the shell package to print environment variables
	shellInstance, err := e.Injector.Resolve("shell")
	if err != nil {
		return fmt.Errorf("error resolving shell: %w", err)
	}
	shell, ok := shellInstance.(shell.Shell)
	if !ok {
		return fmt.Errorf("shell is not of type Shell")
	}

	return shell.PrintEnvVars(envVars)
}

// GetEnvVars is a placeholder for retrieving environment variables.
func (e *Env) GetEnvVars() (map[string]string, error) {
	// Placeholder implementation
	return map[string]string{}, nil
}

// PostEnvHook simulates running any necessary commands after the environment variables have been set.
func (e *Env) PostEnvHook() error {
	// Placeholder for post-processing logic
	return nil
}

// stat is a variable that holds the os.Stat function for mocking
var stat = os.Stat

// Define a variable for os.Getwd() for easier testing
var getwd = os.Getwd

// Define a variable for filepath.Glob for easier testing
var glob = filepath.Glob

// Wrapper function for os.WriteFile
var writeFile = os.WriteFile

// Wrapper function for yaml.Unmarshal
var yamlUnmarshal = yaml.Unmarshal

// stringPtr returns a pointer to a string value
func stringPtr(s string) *string {
	return &s
}

// boolPtr returns a pointer to a boolean value
func boolPtr(b bool) *bool {
	return &b
}
