// The AzureEnvPrinter is a specialized component that manages Azure environment configuration.
// It provides Azure-specific environment variable management and configuration,
// The AzureEnvPrinter handles Azure configuration settings and environment setup,
// ensuring proper Azure CLI integration and environment setup for operations.

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

// AzureEnvPrinter is a struct that implements Azure environment configuration
type AzureEnvPrinter struct {
	BaseEnvPrinter
}

// =============================================================================
// Constructor
// =============================================================================

// NewAzureEnvPrinter creates a new AzureEnvPrinter instance
func NewAzureEnvPrinter(shell shell.Shell, configHandler config.ConfigHandler) *AzureEnvPrinter {
	if shell == nil {
		panic("shell is required")
	}
	if configHandler == nil {
		panic("config handler is required")
	}

	return &AzureEnvPrinter{
		BaseEnvPrinter: *NewBaseEnvPrinter(shell, configHandler),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetEnvVars retrieves the environment variables for the Azure environment.
// In global mode (no windsor.yaml in the project tree) AZURE_CONFIG_DIR is not
// emitted so the az CLI defers to the operator's ambient ~/.azure config; the
// project-level identifiers (ARM_SUBSCRIPTION_ID, ARM_TENANT_ID, ARM_ENVIRONMENT)
// are still emitted because they describe which Azure account/tenant the
// context targets, not whose credentials are used.
func (e *AzureEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)
	global := e.shell.IsGlobal()

	// Get the current context configuration
	config := e.configHandler.GetConfig()
	if config != nil && config.Azure != nil {
		if !global {
			configRoot, err := e.configHandler.GetConfigRoot()
			if err != nil {
				return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
			}
			azureConfigDir := filepath.Join(configRoot, ".azure")
			envVars["AZURE_CONFIG_DIR"] = filepath.ToSlash(azureConfigDir)
			envVars["AZURE_CORE_LOGIN_EXPERIENCE_V2"] = "false"
		}

		if config.Azure.SubscriptionID != nil {
			envVars["ARM_SUBSCRIPTION_ID"] = *config.Azure.SubscriptionID
		}
		if config.Azure.TenantID != nil {
			envVars["ARM_TENANT_ID"] = *config.Azure.TenantID
		}
		if config.Azure.Environment != nil {
			envVars["ARM_ENVIRONMENT"] = *config.Azure.Environment
		}
	}

	return envVars, nil
}
