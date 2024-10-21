package helpers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// TalosHelper is a helper struct that provides Talos-specific utility functions
type TalosHelper struct {
	Context context.ContextInterface
}

// NewTalosHelper is a constructor for TalosHelper
func NewTalosHelper(di *di.DIContainer) (*TalosHelper, error) {
	resolvedContext, err := di.Resolve("context")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}

	return &TalosHelper{
		Context: resolvedContext.(context.ContextInterface),
	}, nil
}

// GetEnvVars retrieves Talos-specific environment variables for the current context
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

// PostEnvExec runs any necessary commands after the environment variables have been set.
func (h *TalosHelper) PostEnvExec() error {
	return nil
}

// GetContainerConfig returns a list of container data for docker-compose.
func (h *TalosHelper) GetContainerConfig() ([]types.ServiceConfig, error) {
	// Stub implementation
	return nil, nil
}

// WriteConfig writes any vendor specific configuration files that are needed for the helper.
func (h *TalosHelper) WriteConfig() error {
	return nil
}

// Ensure TalosHelper implements Helper interface
var _ Helper = (*TalosHelper)(nil)
