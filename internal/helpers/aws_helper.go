package helpers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
)

// AwsHelper is a helper struct that provides AWS-specific utility functions
type AwsHelper struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Context       context.ContextInterface
}

// NewAwsHelper is a constructor for AwsHelper
func NewAwsHelper(configHandler config.ConfigHandler, shell shell.Shell, ctx context.ContextInterface) *AwsHelper {
	return &AwsHelper{
		ConfigHandler: configHandler,
		Shell:         shell,
		Context:       ctx,
	}
}

// GetEnvVars retrieves AWS-specific environment variables for the current context
func (h *AwsHelper) GetEnvVars() (map[string]string, error) {
	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving config root: %w", err)
	}

	// Construct the path to the AWS config file
	awsConfigPath := filepath.Join(configRoot, ".aws", "config")
	if _, err := os.Stat(awsConfigPath); os.IsNotExist(err) {
		awsConfigPath = ""
	}

	// Retrieve the current context
	currentContext, err := h.Context.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving current context: %w", err)
	}

	// Retrieve AWS-specific configuration values
	awsProfile, err := h.ConfigHandler.GetConfigValue("aws_profile")
	if err != nil {
		awsProfile = "default"
	}

	// Retrieve AWS endpoint URL if set
	awsEndpointURL, err := h.ConfigHandler.GetConfigValue("aws_endpoint_url")
	if err != nil || awsEndpointURL == "" {
		if currentContext == "local" {
			awsEndpointURL = os.Getenv("AWS_ENDPOINT_URL")
		} else {
			awsEndpointURL = ""
		}
	}

	// Set the environment variables
	envVars := map[string]string{
		"AWS_CONFIG_FILE":  awsConfigPath,
		"AWS_PROFILE":      awsProfile,
		"AWS_ENDPOINT_URL": awsEndpointURL,
	}

	// Retrieve custom S3 hostname if set
	s3Hostname, err := h.ConfigHandler.GetConfigValue("s3_hostname")
	if err == nil && s3Hostname != "" {
		envVars["S3_HOSTNAME"] = s3Hostname
	}

	// Retrieve MWAA endpoint if set
	mwaaEndpoint, err := h.ConfigHandler.GetConfigValue("mwaa_endpoint")
	if err == nil && mwaaEndpoint != "" {
		envVars["MWAA_ENDPOINT"] = mwaaEndpoint
	}

	return envVars, nil
}

// PostEnvExec runs any necessary commands after the environment variables have been set.
func (h *AwsHelper) PostEnvExec() error {
	return nil
}

// SetConfig sets new values for aws_endpoint_url and aws_profile
func (h *AwsHelper) SetConfig(awsEndpointURL, awsProfile string) error {
	if awsEndpointURL != "" {
		if err := h.ConfigHandler.SetConfigValue("aws_endpoint_url", awsEndpointURL); err != nil {
			return fmt.Errorf("error setting aws_endpoint_url: %w", err)
		}
	}
	if awsProfile != "" {
		if err := h.ConfigHandler.SetConfigValue("aws_profile", awsProfile); err != nil {
			return fmt.Errorf("error setting aws_profile: %w", err)
		}
	}
	if err := h.ConfigHandler.SaveConfig(""); err != nil {
		return fmt.Errorf("error saving config: %w", err)
	}
	return nil
}

// Ensure AwsHelper implements Helper interface
var _ Helper = (*AwsHelper)(nil)
