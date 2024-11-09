package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// OmniEnvPrinter is a struct that simulates a Kubernetes environment for testing purposes.
type OmniEnvPrinter struct {
	BaseEnvPrinter
}

// NewOmniEnv initializes a new omniEnv instance using the provided dependency injector.
func NewOmniEnvPrinter(injector di.Injector) *OmniEnvPrinter {
	return &OmniEnvPrinter{
		BaseEnvPrinter: BaseEnvPrinter{
			injector: injector,
		},
	}
}

// GetEnvVars retrieves the environment variables for the Omni environment.
func (e *OmniEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Resolve necessary dependencies for context operations.
	contextHandler, err := e.injector.Resolve("contextHandler")
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
func (e *OmniEnvPrinter) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		// Return the error if GetEnvVars fails
		return fmt.Errorf("error getting environment variables: %w", err)
	}
	// Call the Print method of the embedded BaseEnvPrinter struct with the retrieved environment variables
	return e.BaseEnvPrinter.Print(envVars)
}

// Ensure OmniEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*OmniEnvPrinter)(nil)
