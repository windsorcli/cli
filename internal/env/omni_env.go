package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// OmniEnv is a struct that simulates a Kubernetes environment for testing purposes.
type OmniEnv struct {
	Env
}

// NewTalosEnv initializes a new TalosEnv instance using the provided dependency injection container.
func NewOmniEnv(diContainer di.ContainerInterface) *OmniEnv {
	return &OmniEnv{
		Env: Env{
			diContainer: diContainer,
		},
	}
}

// GetEnvVars retrieves the environment variables for the Omni environment.
func (e *OmniEnv) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Resolve necessary dependencies for context operations.
	contextHandler, err := e.diContainer.Resolve("contextHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving contextHandler: %w", err)
	}
	context, ok := contextHandler.(context.ContextInterface)
	if !ok {
		return nil, fmt.Errorf("failed to cast contextHandler to context.ContextInterface")
	}

	// Determine the root directory for configuration files.
	configRoot, err := context.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
	}

	// Construct the path to the omni config file and verify its existence.
	omniConfigPath := filepath.Join(configRoot, ".omni", "config")
	if _, err := stat(omniConfigPath); os.IsNotExist(err) {
		omniConfigPath = ""
	}

	// Populate environment variables with Omni configuration data.
	envVars["OMNICONFIG"] = omniConfigPath

	return envVars, nil
}

// Print prints the environment variables for the Omni environment.
func (e *OmniEnv) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		// Return the error if GetEnvVars fails
		return fmt.Errorf("error getting environment variables: %w", err)
	}
	// Call the Print method of the embedded Env struct with the retrieved environment variables
	return e.Env.Print(envVars)
}

// Ensure OmniEnv implements the EnvPrinter interface
var _ EnvPrinter = (*OmniEnv)(nil)
