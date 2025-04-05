package env

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
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

// Print retrieves and prints the environment variables for the Docker environment.
func (e *TalosEnvPrinter) Print(customVars ...map[string]string) error {
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

// Ensure TalosEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*TalosEnvPrinter)(nil)
