package helpers

import (
	"fmt"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
)

// ColimaHelper is a struct that provides various utility functions for working with Colima
type ColimaHelper struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Context       context.ContextInterface
}

// NewColimaHelper is a constructor for ColimaHelper
func NewColimaHelper(configHandler config.ConfigHandler, shell shell.Shell, ctx context.ContextInterface) *ColimaHelper {
	return &ColimaHelper{
		ConfigHandler: configHandler,
		Shell:         shell,
		Context:       ctx,
	}
}

// GetEnvVars retrieves the environment variables for the Colima command
func (h *ColimaHelper) GetEnvVars() (map[string]string, error) {
	// Colima does not use environment variables
	return map[string]string{}, nil
}

// PostEnvExec runs any necessary commands after the environment variables have been set.
func (h *ColimaHelper) PostEnvExec() error {
	// No post environment execution needed for Colima
	return nil
}

// SetConfig sets the configuration value for the given key
func (h *ColimaHelper) SetConfig(key, value string) error {
	if key == "vm_driver" && value == "colima" {
		context, err := h.Context.GetContext()
		if err != nil {
			return fmt.Errorf("error retrieving context: %w", err)
		}
		if err = h.ConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.vm.driver", context), value); err != nil {
			return fmt.Errorf("error setting colima config: %w", err)
		}
		return nil
	}
	return nil
}

// Ensure ColimaHelper implements Helper interface
var _ Helper = (*ColimaHelper)(nil)
