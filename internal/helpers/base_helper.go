package helpers

import (
	"fmt"
	"strings"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/shell"
)

type BaseHelper struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
}

func NewBaseHelper(configHandler config.ConfigHandler, shell shell.Shell) *BaseHelper {
	return &BaseHelper{
		ConfigHandler: configHandler,
		Shell:         shell,
	}
}

func (h *BaseHelper) GetEnvVars() (map[string]string, error) {
	context, err := h.ConfigHandler.GetConfigValue("context")
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	envVars, err := h.ConfigHandler.GetNestedMap(fmt.Sprintf("contexts.%s.environment", context))
	if err != nil {
		envVars = make(map[string]interface{})
	}

	stringEnvVars := make(map[string]string)
	for k, v := range envVars {
		if strVal, ok := v.(string); ok {
			stringEnvVars[strings.ToUpper(k)] = strVal // Capitalize the key
		} else {
			return nil, fmt.Errorf("non-string value found in environment variables for context %s", context)
		}
	}

	// Add WINDSOR_CONTEXT to the environment variables
	stringEnvVars["WINDSOR_CONTEXT"] = context

	// Get the project root and add WINDSOR_PROJECT_ROOT to the environment variables
	projectRoot, err := h.Shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving project root: %w", err)
	}
	stringEnvVars["WINDSOR_PROJECT_ROOT"] = projectRoot

	return stringEnvVars, nil
}

// Ensure BaseHelper implements Helper interface
var _ Helper = (*BaseHelper)(nil)
