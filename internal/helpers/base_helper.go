package helpers

import (
	"fmt"
	"strings"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// BaseHelper is a helper struct that provides various utility functions
type BaseHelper struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Context       context.ContextInterface
}

// NewBaseHelper is a constructor for BaseHelper
func NewBaseHelper(di *di.DIContainer) (*BaseHelper, error) {
	configHandler, err := di.Resolve("configHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving configHandler: %w", err)
	}

	resolvedShell, err := di.Resolve("shell")
	if err != nil {
		return nil, fmt.Errorf("error resolving shell: %w", err)
	}

	resolvedContext, err := di.Resolve("context")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}

	return &BaseHelper{
		ConfigHandler: configHandler.(config.ConfigHandler),
		Shell:         resolvedShell.(shell.Shell),
		Context:       resolvedContext.(context.ContextInterface),
	}, nil
}

// GetEnvVars retrieves environment variables for the current context
func (h *BaseHelper) GetEnvVars() (map[string]string, error) {
	// Get the current context
	context, err := h.Context.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	// Get environment variables for the context from the config handler
	envVars, err := h.ConfigHandler.GetNestedMap(fmt.Sprintf("contexts.%s.environment", context))
	if err != nil {
		envVars = make(map[string]interface{})
	}

	// Convert environment variables to a map of strings
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

// PostEnvExec runs any necessary commands after the environment variables have been set.
func (h *BaseHelper) PostEnvExec() error {
	return nil
}

// SetConfig sets the configuration value for the given key
func (h *BaseHelper) SetConfig(key, value string) error {
	// This is a stub implementation
	return nil
}

// Ensure BaseHelper implements Helper interface
var _ Helper = (*BaseHelper)(nil)
