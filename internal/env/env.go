package env

import (
	"os"

	"github.com/windsor-hotel/cli/internal/di"
)

// stat is a variable that holds the os.Stat function for mocking
var stat = os.Stat

// EnvInterface defines the methods for environment-specific utility functions
type EnvInterface interface {
	Print(envVars map[string]string)
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
