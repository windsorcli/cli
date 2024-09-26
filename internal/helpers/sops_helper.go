package helpers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
)

// SopsHelper is a helper struct that provides Sopsrnetes-specific utility functions
type SopsHelper struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Context       context.ContextInterface
}

// NewSopsHelper is a constructor for SopsHelper
func NewSopsHelper(configHandler config.ConfigHandler, shell shell.Shell, ctx context.ContextInterface) *SopsHelper {
	return &SopsHelper{
		ConfigHandler: configHandler,
		Shell:         shell,
		Context:       ctx,
	}
}

// GetEnvVars retrieves Sopsrnetes-specific environment variables for the current context
func (h *SopsHelper) GetEnvVars() (map[string]string, error) {
	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving config root: %w", err)
	}

	// Construct the path to the sopsconfig file
	sopsConfigPath := filepath.Join(configRoot, ".sops", "config")
	if _, err := os.Stat(sopsConfigPath); os.IsNotExist(err) {
		sopsConfigPath = ""
	}

	envVars := map[string]string{
		"SOPSCONFIG": sopsConfigPath,
	}

	return envVars, nil
}

// Ensure SopsHelper implements Helper interface
var _ Helper = (*SopsHelper)(nil)
