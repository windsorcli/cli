package env

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/di"
)

// TalosEnvPrinter is a struct that simulates a Kubernetes environment for testing purposes.
type TalosEnvPrinter struct {
	BaseEnvPrinter
}

// NewTalosEnvPrinter initializes a new TalosEnvPrinter instance using the provided dependency injector.
func NewTalosEnvPrinter(injector di.Injector) *TalosEnvPrinter {
	talosEnvPrinter := &TalosEnvPrinter{}
	talosEnvPrinter.BaseEnvPrinter = BaseEnvPrinter{
		injector:   injector,
		EnvPrinter: talosEnvPrinter,
	}
	return talosEnvPrinter
}

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

// Ensure TalosEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*TalosEnvPrinter)(nil)
