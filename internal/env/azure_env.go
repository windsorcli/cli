package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/di"
)

// AzureEnvPrinter is a struct that simulates an Azure environment for testing purposes.
type AzureEnvPrinter struct {
	BaseEnvPrinter
}

// NewAzureEnvPrinter initializes a new AzureEnvPrinter instance using the provided dependency injector.
func NewAzureEnvPrinter(injector di.Injector) *AzureEnvPrinter {
	return &AzureEnvPrinter{
		BaseEnvPrinter: BaseEnvPrinter{
			injector: injector,
		},
	}
}

// GetEnvVars retrieves the environment variables for the Azure environment.
func (e *AzureEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Get the context configuration
	contextConfigData := e.configHandler.GetConfig()

	// Ensure the context configuration and Azure-specific settings are available.
	if contextConfigData == nil || contextConfigData.Azure == nil {
		return nil, fmt.Errorf("context configuration or Azure configuration is missing")
	}

	// Determine the root directory for configuration files.
	configRoot, err := e.contextHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
	}

	// Construct the path to the Azure configuration file and verify its existence.
	azureConfigPath := filepath.Join(configRoot, ".azure", "config")
	if _, err := stat(azureConfigPath); os.IsNotExist(err) {
		azureConfigPath = ""
	}

	// Populate environment variables with Azure configuration data.
	if azureConfigPath != "" {
		envVars["AZURE_CONFIG_FILE"] = azureConfigPath
	}
	if contextConfigData.Azure.AzureProfile != nil {
		envVars["AZURE_PROFILE"] = *contextConfigData.Azure.AzureProfile
	}
	if contextConfigData.Azure.AzureEndpointURL != nil {
		envVars["AZURE_ENDPOINT_URL"] = *contextConfigData.Azure.AzureEndpointURL
	}
	if contextConfigData.Azure.StorageAccountName != nil {
		envVars["STORAGE_ACCOUNT_NAME"] = *contextConfigData.Azure.StorageAccountName
	}
	if contextConfigData.Azure.FunctionAppEndpoint != nil {
		envVars["FUNCTION_APP_ENDPOINT"] = *contextConfigData.Azure.FunctionAppEndpoint
	}

	return envVars, nil
}

// Print prints the environment variables for the Azure environment.
func (e *AzureEnvPrinter) Print() error {
	envVars, err := e.GetEnvVars()

	// Return nil if envVars is empty
	if len(envVars) == 0 {
		return fmt.Errorf("no environment variables: %w", err)
	}

	if err != nil {
		// Return the error if GetEnvVars fails
		return fmt.Errorf("error getting environment variables: %w", err)
	}

	// Call the Print method of the embedded envPrinter struct with the retrieved environment variables
	return e.BaseEnvPrinter.Print(envVars)
}

// Ensure AzureEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*AzureEnvPrinter)(nil)
