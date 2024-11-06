package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// TalosEnv is a struct that simulates a Kubernetes environment for testing purposes.
type TalosEnv struct {
	Env
}

// NewTalosEnv initializes a new TalosEnv instance using the provided dependency injection container.
func NewTalosEnv(diContainer di.ContainerInterface) *TalosEnv {
	return &TalosEnv{
		Env: Env{
			diContainer: diContainer,
		},
	}
}

// GetEnvVars retrieves the environment variables for the Talos environment.
func (e *TalosEnv) GetEnvVars() (map[string]string, error) {
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

	// Construct the path to the talos config file and verify its existence.
	talosConfigPath := filepath.Join(configRoot, ".talos", "config")
	if _, err := stat(talosConfigPath); os.IsNotExist(err) {
		talosConfigPath = ""
	}

	// Populate environment variables with Talos configuration data.
	envVars["TALOSCONFIG"] = talosConfigPath

	return envVars, nil
}

// Print prints the environment variables for the Talos environment.
func (e *TalosEnv) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		// Return the error if GetEnvVars fails
		return fmt.Errorf("error getting environment variables: %w", err)
	}
	// Call the Print method of the embedded Env struct with the retrieved environment variables
	return e.Env.Print(envVars)
}

// Ensure TalosEnv implements the EnvPrinter interface
var _ EnvPrinter = (*TalosEnv)(nil)
