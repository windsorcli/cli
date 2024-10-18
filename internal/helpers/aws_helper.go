package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// isLocal checks if the context is "local" or has a "local-" prefix
func isLocal(context string) bool {
	return context == "local" || strings.HasPrefix(context, "local-")
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

	// Retrieve AWS-specific configuration values from the context
	awsProfile, err := h.ConfigHandler.GetString(fmt.Sprintf("contexts.%s.aws.aws_profile", currentContext))
	if err != nil {
		awsProfile = "default"
	}

	// Retrieve AWS endpoint URL if set
	awsEndpointURL, err := h.ConfigHandler.GetString(fmt.Sprintf("contexts.%s.aws.aws_endpoint_url", currentContext))
	if err != nil || awsEndpointURL == "" {
		if isLocal(currentContext) {
			awsEndpointURL = "http://aws.test:4566"
		} else {
			awsEndpointURL = ""
		}
	}

	// Retrieve custom S3 hostname if set, otherwise set default for local context
	s3Hostname, err := h.ConfigHandler.GetString(fmt.Sprintf("contexts.%s.aws.s3_hostname", currentContext))
	if err != nil || s3Hostname == "" {
		if isLocal(currentContext) {
			s3Hostname = "http://s3.local.aws.test:4566"
		}
	}

	// Retrieve MWAA endpoint if set, otherwise set default for local context
	mwaaEndpoint, err := h.ConfigHandler.GetString(fmt.Sprintf("contexts.%s.aws.mwaa_endpoint", currentContext))
	if err != nil || mwaaEndpoint == "" {
		if isLocal(currentContext) {
			mwaaEndpoint = "http://mwaa.local.aws.test:4566"
		}
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

// GetContainerConfig returns a list of container data for docker-compose.
func (h *AwsHelper) GetContainerConfig() ([]types.ServiceConfig, error) {
	// Stub implementation
	return nil, nil
}

// WriteConfig writes any vendor specific configuration files that are needed for the helper.
func (h *AwsHelper) WriteConfig() error {
	return nil
}

// Ensure AwsHelper implements Helper interface
var _ Helper = (*AwsHelper)(nil)
