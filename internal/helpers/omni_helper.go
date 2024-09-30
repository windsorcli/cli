package helpers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
)

// OmniHelper is a helper struct that provides Omnirnetes-specific utility functions
type OmniHelper struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Context       context.ContextInterface
}

// NewOmniHelper is a constructor for OmniHelper
func NewOmniHelper(configHandler config.ConfigHandler, shell shell.Shell, ctx context.ContextInterface) *OmniHelper {
	return &OmniHelper{
		ConfigHandler: configHandler,
		Shell:         shell,
		Context:       ctx,
	}
}

// GetEnvVars retrieves Omnirnetes-specific environment variables for the current context
func (h *OmniHelper) GetEnvVars() (map[string]string, error) {
	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving config root: %w", err)
	}

	// Construct the path to the omniconfig file
	omniConfigPath := filepath.Join(configRoot, ".omni", "config")
	if _, err := os.Stat(omniConfigPath); os.IsNotExist(err) {
		omniConfigPath = ""
	}

	envVars := map[string]string{
		"OMNICONFIG": omniConfigPath,
	}

	return envVars, nil
}

// PostEnvExec runs any necessary commands after the environment variables have been set.
func (h *OmniHelper) PostEnvExec() error {
	return nil
}

// SetConfig sets the configuration value for the given key
func (h *OmniHelper) SetConfig(key, value string) error {
	// This is a stub implementation
	return nil
}

// Ensure OmniHelper implements Helper interface
var _ Helper = (*OmniHelper)(nil)
