package env

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Types
// =============================================================================

// TalosEnvPrinter manages Talos environment configuration, providing Talos-specific
// environment variable management and configuration for CLI integration and environment setup.
type TalosEnvPrinter struct {
	BaseEnvPrinter
}

// =============================================================================
// Constructor
// =============================================================================

// NewTalosEnvPrinter creates and returns a new TalosEnvPrinter instance using the provided injector.
func NewTalosEnvPrinter(injector di.Injector) *TalosEnvPrinter {
	return &TalosEnvPrinter{
		BaseEnvPrinter: *NewBaseEnvPrinter(injector),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetEnvVars returns a map of environment variables for the Talos environment.
// It sets the TALOSCONFIG variable and, if the cluster driver is "omni", also sets OMNICONFIG.
// Returns an error if the configuration root cannot be determined.
func (e *TalosEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)
	configRoot, err := e.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
	}
	talosConfigPath := filepath.Join(configRoot, ".talos", "config")
	envVars["TALOSCONFIG"] = talosConfigPath
	provider := e.configHandler.GetString("provider", "")
	if provider == "omni" {
		omniConfigPath := filepath.Join(configRoot, ".omni", "config")
		envVars["OMNICONFIG"] = omniConfigPath
	}
	return envVars, nil
}

// Print outputs the environment variables for the Talos environment using the embedded BaseEnvPrinter.
// Returns an error if environment variable retrieval fails or printing fails.
func (e *TalosEnvPrinter) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		return fmt.Errorf("error getting environment variables: %w", err)
	}
	return e.BaseEnvPrinter.Print(envVars)
}

// TalosEnvPrinter implements the EnvPrinter interface.
var _ EnvPrinter = (*TalosEnvPrinter)(nil)
