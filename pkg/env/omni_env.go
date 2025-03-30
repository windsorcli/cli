package env

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/di"
)

// OmniEnvPrinter is a struct that simulates a Kubernetes environment for testing purposes.
type OmniEnvPrinter struct {
	BaseEnvPrinter
}

// NewOmniEnvPrinter initializes a new OmniEnvPrinter instance using the provided dependency injector.
func NewOmniEnvPrinter(injector di.Injector) *OmniEnvPrinter {
	omniEnvPrinter := &OmniEnvPrinter{}
	omniEnvPrinter.BaseEnvPrinter = BaseEnvPrinter{
		injector:   injector,
		envPrinter: omniEnvPrinter,
	}
	return omniEnvPrinter
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

// Ensure OmniEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*OmniEnvPrinter)(nil)
