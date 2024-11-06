package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/constants"
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

	resolvedContext, err := di.Resolve("contextHandler")
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

	// If contextConfig or AWS is nil, return an empty map
	if contextConfig == nil || contextConfig.AWS == nil {
		return map[string]string{}, nil
	}

	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving config root: %w", err)
	}

	// Construct the path to the AWS config file
	awsConfigPath := filepath.Join(configRoot, ".aws", "config")
	if _, err := stat(awsConfigPath); os.IsNotExist(err) {
		awsConfigPath = ""
	}

	// Set the environment variables
	envVars := map[string]string{}

	if awsConfigPath != "" {
		envVars["AWS_CONFIG_FILE"] = awsConfigPath
	}
	if contextConfig.AWS.AWSProfile != nil {
		envVars["AWS_PROFILE"] = *contextConfig.AWS.AWSProfile
	}
	if contextConfig.AWS.AWSEndpointURL != nil {
		envVars["AWS_ENDPOINT_URL"] = *contextConfig.AWS.AWSEndpointURL
	}
	if contextConfig.AWS.S3Hostname != nil {
		envVars["S3_HOSTNAME"] = *contextConfig.AWS.S3Hostname
	}
	if contextConfig.AWS.MWAAEndpoint != nil {
		envVars["MWAA_ENDPOINT"] = *contextConfig.AWS.MWAAEndpoint
	}

	return envVars, nil
}

// GetComposeConfig returns the top-level compose configuration including a list of container data for docker-compose.
func (h *AwsHelper) GetComposeConfig() (*types.Config, error) {
	contextConfig, err := h.ConfigHandler.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context config: %w", err)
	}

	if contextConfig.AWS == nil ||
		contextConfig.AWS.Localstack == nil ||
		contextConfig.AWS.Localstack.Create == nil ||
		!*contextConfig.AWS.Localstack.Create {
		return nil, nil
	}

	localstackAuthToken := os.Getenv("LOCALSTACK_AUTH_TOKEN")

	image := constants.DEFAULT_AWS_LOCALSTACK_IMAGE
	if localstackAuthToken != "" {
		image = constants.DEFAULT_AWS_LOCALSTACK_PRO_IMAGE
	}

	servicesList := ""
	if contextConfig.AWS.Localstack.Services != nil {
		servicesList = strings.Join(contextConfig.AWS.Localstack.Services, ",")
	}

	services := []types.ServiceConfig{
		{
			Name:    "aws.test",
			Image:   image,
			Restart: "always",
			Environment: map[string]*string{
				"ENFORCE_IAM":   strPtr("1"),
				"PERSISTENCE":   strPtr("1"),
				"IAM_SOFT_MODE": strPtr("0"),
				"DEBUG":         strPtr("0"),
				"SERVICES":      strPtr(servicesList),
			},
			Labels: map[string]string{
				"role":       "localstack",
				"managed_by": "windsor",
				"wildcard":   "true",
			},
		},
	}

	if localstackAuthToken != "" {
		services[0].Secrets = []types.ServiceSecretConfig{
			{
				Source: "LOCALSTACK_AUTH_TOKEN",
			},
		}
	}

	return &types.Config{Services: services}, nil
}

// WriteConfig writes any vendor specific configuration files that are needed for the helper.
func (h *AwsHelper) WriteConfig() error {
	return nil
}

// Up executes necessary commands to instantiate the tool or environment.
func (h *AwsHelper) Up(verbose ...bool) error {
	return nil
}

// Info returns information about the helper.
func (h *AwsHelper) Info() (interface{}, error) {
	return nil, nil
}

// Ensure AwsHelper implements Helper interface
var _ Helper = (*AwsHelper)(nil)
