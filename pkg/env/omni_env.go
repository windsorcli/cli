// The OmniEnvPrinter is a specialized component that manages Omni environment configuration.
// It provides Omni-specific environment variable management and configuration,
// The OmniEnvPrinter handles Omni configuration settings and environment setup,
// ensuring proper Omni CLI integration and environment setup for operations.

package env

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Types
// =============================================================================

// OmniEnvPrinter is a struct that implements Omni environment configuration
type OmniEnvPrinter struct {
	BaseEnvPrinter
}

// =============================================================================
// Constructor
// =============================================================================

// NewOmniEnvPrinter creates a new OmniEnvPrinter instance
func NewOmniEnvPrinter(injector di.Injector) *OmniEnvPrinter {
	return &OmniEnvPrinter{
		BaseEnvPrinter: BaseEnvPrinter{
			injector: injector,
		},
	}
}

// =============================================================================
// Public Methods
// =============================================================================

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
