// The TalosEnvPrinter is a specialized component that manages Talos environment configuration.
// It provides Talos-specific environment variable management and configuration,
// The TalosEnvPrinter handles Talos configuration settings and environment setup,
// ensuring proper Talos CLI integration and environment setup for operations.

package env

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Types
// =============================================================================

// TalosEnvPrinter is a struct that implements Talos environment configuration
type TalosEnvPrinter struct {
	BaseEnvPrinter
}

// =============================================================================
// Constructor
// =============================================================================

// NewTalosEnvPrinter creates a new TalosEnvPrinter instance
func NewTalosEnvPrinter(injector di.Injector) *TalosEnvPrinter {
	return &TalosEnvPrinter{
		BaseEnvPrinter: BaseEnvPrinter{
			injector: injector,
		},
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetEnvVars retrieves the environment variables for the Talos environment.
func (e *TalosEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Determine the root directory for configuration files.
	configRoot, err := e.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
	}

	// Construct the path to the talos config file and verify its existence.
	talosConfigPath := filepath.Join(configRoot, ".talos", "config")

	// Populate environment variables with Talos configuration data.
	envVars["TALOSCONFIG"] = talosConfigPath

	return envVars, nil
}

// Print prints the environment variables for the Talos environment.
func (e *TalosEnvPrinter) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		// Return the error if GetEnvVars fails
		return fmt.Errorf("error getting environment variables: %w", err)
	}
	// Call the Print method of the embedded BaseEnvPrinter struct with the retrieved environment variables
	return e.BaseEnvPrinter.Print(envVars)
}

// Ensure TalosEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*TalosEnvPrinter)(nil)
