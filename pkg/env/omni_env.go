package env

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
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

	// Determine the root directory for configuration files.
	configRoot, err := e.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
	}

	// Construct the path to the omni config file and verify its existence.
	omniConfigPath := filepath.Join(configRoot, ".omni", "config")

	// Populate environment variables with Omni configuration data.
	envVars["OMNICONFIG"] = omniConfigPath

	return envVars, nil
}

// Print retrieves and prints the environment variables for the Docker environment.
func (e *OmniEnvPrinter) Print(customVars ...map[string]string) error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		return fmt.Errorf("error getting environment variables: %w", err)
	}

	// If customVars are provided, merge them with envVars
	if len(customVars) > 0 {
		for key, value := range customVars[0] {
			envVars[key] = strings.TrimSpace(value)
		}
	}

	return e.BaseEnvPrinter.Print(envVars)
}

// Ensure OmniEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*OmniEnvPrinter)(nil)
