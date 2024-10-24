package helpers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// AwsHelper is a helper struct that provides AWS-specific utility functions
type AwsHelper struct {
	ConfigHandler config.ConfigHandler
	Context       context.ContextInterface
}

// NewAwsHelper is a constructor for AwsHelper
func NewAwsHelper(di *di.DIContainer) (*AwsHelper, error) {
	cliConfigHandler, err := di.Resolve("cliConfigHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving cliConfigHandler: %w", err)
	}

	resolvedContext, err := di.Resolve("context")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}

	return &AwsHelper{
		ConfigHandler: cliConfigHandler.(config.ConfigHandler),
		Context:       resolvedContext.(context.ContextInterface),
	}, nil
}

// Initialize performs any necessary initialization for the helper.
func (h *AwsHelper) Initialize() error {
	// Perform any necessary initialization here
	return nil
}

// GetEnvVars retrieves AWS-specific environment variables for the current context
func (h *AwsHelper) GetEnvVars() (map[string]string, error) {
	// Retrieve the context configuration using GetConfig
	contextConfig, err := h.ConfigHandler.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context config: %w", err)
	}

	// If AWS is nil, return an empty map
	if contextConfig.AWS == nil {
		return map[string]string{}, nil
	}

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

	// Retrieve AWS-specific configuration values from the context
	awsProfile := ""
	awsEndpointURL := ""
	s3Hostname := ""
	mwaaEndpoint := ""

	if contextConfig.AWS.AWSProfile != nil {
		awsProfile = *contextConfig.AWS.AWSProfile
	}

	if contextConfig.AWS.AWSEndpointURL != nil {
		awsEndpointURL = *contextConfig.AWS.AWSEndpointURL
	}

	if contextConfig.AWS.S3Hostname != nil {
		s3Hostname = *contextConfig.AWS.S3Hostname
	}

	if contextConfig.AWS.MWAAEndpoint != nil {
		mwaaEndpoint = *contextConfig.AWS.MWAAEndpoint
	}

	// Set the environment variables
	envVars := map[string]string{
		"AWS_CONFIG_FILE":  awsConfigPath,
		"AWS_PROFILE":      awsProfile,
		"AWS_ENDPOINT_URL": awsEndpointURL,
		"S3_HOSTNAME":      s3Hostname,
		"MWAA_ENDPOINT":    mwaaEndpoint,
	}

	return envVars, nil
}

// PostEnvExec runs any necessary commands after the environment variables have been set.
func (h *AwsHelper) PostEnvExec() error {
	return nil
}

// GetComposeConfig returns the top-level compose configuration including a list of container data for docker-compose.
func (h *AwsHelper) GetComposeConfig() (*types.Config, error) {
	// Stub implementation
	return nil, nil
}

// WriteConfig writes any vendor specific configuration files that are needed for the helper.
func (h *AwsHelper) WriteConfig() error {
	return nil
}

// Initialize performs any necessary initialization for the helper.
func (h *AwsHelper) Initialize() error {
	// Perform any necessary initialization here
	return nil
}

// Ensure AwsHelper implements Helper interface
var _ Helper = (*AwsHelper)(nil)
