package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/di"
)

// TalosEnvPrinter is a struct that simulates a Kubernetes environment for testing purposes.
type TalosEnvPrinter struct {
	BaseEnvPrinter
}

// NewTalosEnvPrinter initializes a new talosEnvPrinter instance using the provided dependency injector.
func NewTalosEnvPrinter(injector di.Injector) *TalosEnvPrinter {
	return &TalosEnvPrinter{
		BaseEnvPrinter: BaseEnvPrinter{
			injector: injector,
		},
	}
}

// GetEnvVars retrieves the environment variables for the Talos environment.
func (e *TalosEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Determine the root directory for configuration files.
	configRoot, err := e.contextHandler.GetConfigRoot()
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
