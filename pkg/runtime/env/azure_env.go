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

// GetEnvVars returns Azure env vars. AZURE_CONFIG_DIR is always set so `az login`
// writes tokens into the context folder; AZURE_CORE_LOGIN_EXPERIENCE_V2=false
// suppresses the interactive subscription picker. ARM_* vars require the matching
// azure config field to be set.
func (e *AzureEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	configRoot, err := e.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
	}

	azureConfigDir := filepath.Join(configRoot, ".azure")
	envVars["AZURE_CONFIG_DIR"] = filepath.ToSlash(azureConfigDir)
	envVars["AZURE_CORE_LOGIN_EXPERIENCE_V2"] = "false"

	contextConfigData := e.configHandler.GetConfig()
	if contextConfigData != nil && contextConfigData.Azure != nil {
		if contextConfigData.Azure.SubscriptionID != nil {
			envVars["ARM_SUBSCRIPTION_ID"] = *contextConfigData.Azure.SubscriptionID
		}
		if contextConfigData.Azure.TenantID != nil {
			envVars["ARM_TENANT_ID"] = *contextConfigData.Azure.TenantID
		}
		if contextConfigData.Azure.Environment != nil {
			envVars["ARM_ENVIRONMENT"] = *contextConfigData.Azure.Environment
		}
	}

	return envVars, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure AzureEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*AzureEnvPrinter)(nil)
