package env

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
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

// NewTalosEnvPrinter creates and returns a new TalosEnvPrinter instance.
func NewTalosEnvPrinter(shell shell.Shell, configHandler config.ConfigHandler) *TalosEnvPrinter {
	if shell == nil {
		panic("shell is required")
	}
	if configHandler == nil {
		panic("config handler is required")
	}

	return &TalosEnvPrinter{
		BaseEnvPrinter: *NewBaseEnvPrinter(shell, configHandler),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetEnvVars returns a map of environment variables for the Talos and Omni environments.
// It sets TALOSCONFIG only when cluster.driver is "talos" and sets OMNICONFIG only when
// platform is "omni". Returns an error if the configuration root cannot be determined.
func (e *TalosEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)
	configRoot, err := e.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
	}
	clusterDriver := e.configHandler.GetString("cluster.driver", "")
	platform := e.configHandler.GetString("platform", "")
	if values, valuesErr := e.configHandler.GetContextValues(); valuesErr == nil && values != nil {
		if clusterMap, ok := values["cluster"].(map[string]any); ok {
			if driver, ok := clusterMap["driver"].(string); ok {
				clusterDriver = driver
			}
		}
		if platformValue, ok := values["platform"].(string); ok && platformValue != "" {
			platform = platformValue
		}
	}
	if clusterDriver == "talos" {
		talosConfigPath := filepath.Join(configRoot, ".talos", "config")
		envVars["TALOSCONFIG"] = talosConfigPath
	}
	if platform == "omni" {
		omniConfigPath := filepath.Join(configRoot, ".omni", "config")
		envVars["OMNICONFIG"] = omniConfigPath
	}
	return envVars, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// TalosEnvPrinter implements the EnvPrinter interface.
var _ EnvPrinter = (*TalosEnvPrinter)(nil)
