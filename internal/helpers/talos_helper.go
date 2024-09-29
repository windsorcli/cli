package helpers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
)

// TalosHelper is a helper struct that provides Talosrnetes-specific utility functions
type TalosHelper struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Context       context.ContextInterface
}

// NewTalosHelper is a constructor for TalosHelper
func NewTalosHelper(configHandler config.ConfigHandler, shell shell.Shell, ctx context.ContextInterface) *TalosHelper {
	return &TalosHelper{
		ConfigHandler: configHandler,
		Shell:         shell,
		Context:       ctx,
	}
}

// GetEnvVars retrieves Talosrnetes-specific environment variables for the current context
func (h *TalosHelper) GetEnvVars() (map[string]string, error) {
	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving config root: %w", err)
	}

	// Construct the path to the talosconfig file
	talosConfigPath := filepath.Join(configRoot, ".talos", "config")
	if _, err := os.Stat(talosConfigPath); os.IsNotExist(err) {
		talosConfigPath = ""
	}

	envVars := map[string]string{
		"TALOSCONFIG": talosConfigPath,
	}

	return envVars, nil
}

// Ensure TalosHelper implements Helper interface
var _ Helper = (*TalosHelper)(nil)
