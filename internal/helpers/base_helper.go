package helpers

import (
	"fmt"
	"strings"

	"github.com/windsor-hotel/cli/internal/config"
)

type BaseHelper struct {
	ConfigHandler config.ConfigHandler
}

func NewBaseHelper(configHandler config.ConfigHandler) *BaseHelper {
	return &BaseHelper{ConfigHandler: configHandler}
}

func (h *BaseHelper) GetEnvVars() (map[string]string, error) {
	context, err := h.ConfigHandler.GetConfigValue("context")
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	envVars, err := h.ConfigHandler.GetNestedMap(fmt.Sprintf("contexts.%s.environment", context))
	if err != nil {
		return map[string]string{}, nil
	}

	stringEnvVars := make(map[string]string)
	for k, v := range envVars {
		if strVal, ok := v.(string); ok {
			stringEnvVars[strings.ToUpper(k)] = strVal // Capitalize the key
		} else {
			return nil, fmt.Errorf("non-string value found in environment variables for context %s", context)
		}
	}

	// Add WINDSORCONTEXT to the environment variables
	stringEnvVars["WINDSORCONTEXT"] = context

	return stringEnvVars, nil
}

// Ensure BaseHelper implements Helper interface
var _ Helper = (*BaseHelper)(nil)
