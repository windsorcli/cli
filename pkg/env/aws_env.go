package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/di"
)

// AwsEnvPrinter is a struct that simulates an AWS environment for testing purposes.
type AwsEnvPrinter struct {
	BaseEnvPrinter
}

// NewAwsEnvPrinter initializes a new AwsEnvPrinter instance using the provided dependency injector.
func NewAwsEnvPrinter(injector di.Injector) *AwsEnvPrinter {
	awsEnvPrinter := &AwsEnvPrinter{}
	awsEnvPrinter.BaseEnvPrinter = BaseEnvPrinter{
		injector:   injector,
		EnvPrinter: awsEnvPrinter,
	}
	return awsEnvPrinter
}

// GetEnvVars retrieves the environment variables for the AWS environment.
func (e *AwsEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Determine the root directory for configuration files.
	configRoot, err := e.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
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

	// Get the AWS profile from the config handler
	awsProfile := e.configHandler.GetString("aws.profile", "default")
	if awsProfile != "" {
		envVars["AWS_PROFILE"] = awsProfile
	}

	// Inject standard environment variables for different endpoints based on AWSConfig
	envVars["AWS_ENDPOINT_URL"] = e.configHandler.GetString("aws.endpoint_url", "")
	envVars["AWS_ENDPOINT_URL_S3"] = e.configHandler.GetString("aws.s3_hostname", "")
	envVars["AWS_ENDPOINT_URL_MWAA"] = e.configHandler.GetString("aws.mwaa_endpoint", "")
	envVars["AWS_REGION"] = e.configHandler.GetString("aws.region", "")

	return envVars, nil
}

// Ensure awsEnv implements the EnvPrinter interface
var _ EnvPrinter = (*AwsEnvPrinter)(nil)
