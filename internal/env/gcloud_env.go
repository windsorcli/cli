package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/di"
)

// GCloudEnvPrinter is a struct that simulates a Google Cloud environment for testing purposes.
type GCloudEnvPrinter struct {
	BaseEnvPrinter
}

// NewGCloudEnvPrinter initializes a new GCloudEnvPrinter instance using the provided dependency injector.
func NewGCloudEnvPrinter(injector di.Injector) *GCloudEnvPrinter {
	return &GCloudEnvPrinter{
		BaseEnvPrinter: BaseEnvPrinter{
			injector: injector,
		},
	}
}

// GetEnvVars retrieves the environment variables for the Google Cloud environment.
func (e *GCloudEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Get the context configuration
	contextConfigData := e.configHandler.GetConfig()

	// Ensure the context configuration and GCloud-specific settings are available.
	// if contextConfigData == nil || contextConfigData.GCloud == nil {
	if contextConfigData == nil {
		// fmt.Println("EXITTING")
		return nil, fmt.Errorf("context configuration or GCloud configuration is missing")
	}

	// Determine the root directory for configuration files.
	configRoot, err := e.contextHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
	}

	// Construct the path to the GCloud configuration file and verify its existence.
	gcloudConfigPath := filepath.Join(configRoot, ".gcloud", "service-account-key.json")
	if _, err := stat(gcloudConfigPath); os.IsNotExist(err) {
		gcloudConfigPath = ""
	}

	// Populate environment variables with GCloud configuration data.
	if gcloudConfigPath != "" {
		envVars["GOOGLE_APPLICATION_CREDENTIALS"] = gcloudConfigPath
	}

	return envVars, nil
}

// Print prints the environment variables for the Google Cloud environment.
func (e *GCloudEnvPrinter) Print() error {
	envVars, err := e.GetEnvVars()

	// Return nil if envVars is empty
	if len(envVars) == 0 {
		return nil
	}

	if err != nil {
		// Return the error if GetEnvVars fails
		return fmt.Errorf("error getting environment variables: %w", err)
	}
	// Call the Print method of the embedded envPrinter struct with the retrieved environment variables
	return e.BaseEnvPrinter.Print(envVars)
}

// Ensure GCloudEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*GCloudEnvPrinter)(nil)
