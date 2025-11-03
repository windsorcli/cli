// The AzureEnvPrinter is a specialized component that manages Azure environment configuration.
// It provides Azure-specific environment variable management and configuration,
// The AzureEnvPrinter handles Azure configuration settings and environment setup,
// ensuring proper Azure CLI integration and environment setup for operations.

package env

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/di"
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
func NewAzureEnvPrinter(injector di.Injector) *AzureEnvPrinter {
	return &AzureEnvPrinter{
		BaseEnvPrinter: *NewBaseEnvPrinter(injector),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetEnvVars retrieves the environment variables for the Azure environment.
func (e *AzureEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	configRoot, err := e.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
	}

	azureConfigDir := filepath.Join(configRoot, ".azure")

	// Get the current context configuration
	config := e.configHandler.GetConfig()
	if config != nil && config.Azure != nil {
		envVars["AZURE_CONFIG_DIR"] = filepath.ToSlash(azureConfigDir)
		envVars["AZURE_CORE_LOGIN_EXPERIENCE_V2"] = "false"

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
