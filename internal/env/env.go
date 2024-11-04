package env

import (
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/di"
)

// EnvInterface defines the methods for environment-specific utility functions
type EnvInterface interface {
	Print(envVars map[string]string) error
	PostEnvHook() error
}

// Env is a struct that implements the EnvInterface
type Env struct {
	diContainer di.ContainerInterface
}

// PostEnvHook simulates running any necessary commands after the environment variables have been set.
// If a custom PostEnvHookFunc is provided, it will use that function instead.
func (e *Env) PostEnvHook() error {
	// Placeholder
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

// stringPtr returns a pointer to a string value
func stringPtr(s string) *string {
	return &s
}

// boolPtr returns a pointer to a boolean value
func boolPtr(b bool) *bool {
	return &b
}
