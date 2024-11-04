package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// AwsEnv is a struct that simulates an AWS environment for testing purposes.
type AwsEnv struct {
	EnvInterface
	diContainer di.ContainerInterface
}

// NewAwsEnv initializes a new AwsEnv instance using the provided dependency injection container.
func NewAwsEnv(diContainer di.ContainerInterface) *AwsEnv {
	return &AwsEnv{
		diContainer: diContainer,
	}
}

// Print displays the provided environment variables to the console.
func (a *AwsEnv) Print(envVars map[string]string) error {
	// Resolve necessary dependencies for configuration, context, and shell operations.
	contextConfig, err := a.diContainer.Resolve("cliConfigHandler")
	if err != nil {
		return fmt.Errorf("error resolving cliConfigHandler: %w", err)
	}
	configHandler, ok := contextConfig.(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("failed to cast cliConfigHandler to config.ConfigHandler")
	}

	contextHandler, err := a.diContainer.Resolve("contextHandler")
	if err != nil {
		return fmt.Errorf("error resolving contextHandler: %w", err)
	}
	context, ok := contextHandler.(context.ContextInterface)
	if !ok {
		return fmt.Errorf("failed to cast contextHandler to context.ContextInterface")
	}

	shellInstance, err := a.diContainer.Resolve("shell")
	if err != nil {
		return fmt.Errorf("error resolving shell: %w", err)
	}
	shell, ok := shellInstance.(shell.Shell)
	if !ok {
		return fmt.Errorf("failed to cast shell to shell.Shell")
	}

	// Access AWS-specific settings from the context configuration.
	contextConfigData, err := configHandler.GetConfig()
	if err != nil {
		return fmt.Errorf("error retrieving context configuration: %w", err)
	}

	// Ensure the context configuration and AWS-specific settings are available.
	if contextConfigData == nil || contextConfigData.AWS == nil {
		return fmt.Errorf("context configuration or AWS configuration is missing")
	}

	// Determine the root directory for configuration files.
	configRoot, err := context.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error retrieving configuration root directory: %w", err)
	}

	// Construct the path to the AWS configuration file and verify its existence.
	awsConfigPath := filepath.Join(configRoot, ".aws", "config")
	if _, err := stat(awsConfigPath); os.IsNotExist(err) {
		awsConfigPath = ""
	}

	// Populate environment variables with AWS configuration data.
	if awsConfigPath != "" {
		envVars["AWS_CONFIG_FILE"] = awsConfigPath
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

	// Display the environment variables using the Shell's PrintEnvVars method.
	return shell.PrintEnvVars(envVars)
}

// Ensure AwsEnv implements the EnvInterface
var _ EnvInterface = (*AwsEnv)(nil)
