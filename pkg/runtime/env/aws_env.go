// The AwsEnvPrinter is a specialized component that manages AWS environment configuration.
// It provides AWS-specific environment variable management and configuration,
// The AwsEnvPrinter handles AWS profile, endpoint, and S3 configuration settings,
// ensuring proper AWS CLI integration and environment setup for AWS operations.

package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Types
// =============================================================================

// AwsEnvPrinter is a struct that implements AWS environment configuration
type AwsEnvPrinter struct {
	BaseEnvPrinter
}

// =============================================================================
// Constructor
// =============================================================================

// NewAwsEnvPrinter creates a new AwsEnvPrinter instance
func NewAwsEnvPrinter(shell shell.Shell, configHandler config.ConfigHandler) *AwsEnvPrinter {
	if shell == nil {
		panic("shell is required")
	}
	if configHandler == nil {
		panic("config handler is required")
	}

	return &AwsEnvPrinter{
		BaseEnvPrinter: *NewBaseEnvPrinter(shell, configHandler),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetEnvVars retrieves the environment variables for the AWS environment.
func (e *AwsEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Get the context configuration
	contextConfigData := e.configHandler.GetConfig()

	// Ensure the context configuration and AWS-specific settings are available.
	if contextConfigData == nil || contextConfigData.AWS == nil {
		return nil, fmt.Errorf("context configuration or AWS configuration is missing")
	}

	// Determine the root directory for configuration files.
	configRoot, err := e.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
	}

	// Construct the path to the AWS configuration file and verify its existence.
	awsConfigPath := filepath.Join(configRoot, ".aws", "config")
	if _, err := e.shims.Stat(awsConfigPath); os.IsNotExist(err) {
		awsConfigPath = ""
	}

	// Populate environment variables with AWS configuration data.
	if awsConfigPath != "" {
		envVars["AWS_CONFIG_FILE"] = filepath.ToSlash(awsConfigPath)
	}
	if contextConfigData.AWS.AWSProfile != nil {
		envVars["AWS_PROFILE"] = *contextConfigData.AWS.AWSProfile
	}
	if contextConfigData.AWS.AWSEndpointURL != nil {
		envVars["AWS_ENDPOINT_URL"] = *contextConfigData.AWS.AWSEndpointURL
	}
	if contextConfigData.AWS.S3Hostname != nil {
		envVars["S3_HOSTNAME"] = *contextConfigData.AWS.S3Hostname
	}
	if contextConfigData.AWS.MWAAEndpoint != nil {
		envVars["MWAA_ENDPOINT"] = *contextConfigData.AWS.MWAAEndpoint
	}

	return envVars, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure AwsEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*AwsEnvPrinter)(nil)
