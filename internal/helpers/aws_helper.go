package helpers

import (
	"fmt"
	"os"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/constants"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// AwsHelper is a helper struct that provides AWS-specific utility functions
type AwsHelper struct {
	BaseHelper
	ConfigHandler config.ConfigHandler
	Context       context.ContextInterface
}

// NewAwsHelper is a constructor for AwsHelper
func NewAwsHelper(injector di.Injector) (*AwsHelper, error) {
	configHandler, err := injector.Resolve("configHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving configHandler: %w", err)
	}

	resolvedContext, err := injector.Resolve("contextHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}

	return &AwsHelper{
		ConfigHandler: configHandler.(config.ConfigHandler),
		Context:       resolvedContext.(context.ContextInterface),
	}, nil
}

// GetComposeConfig returns the top-level compose configuration including a list of container data for docker-compose.
func (h *AwsHelper) GetComposeConfig() (*types.Config, error) {
	contextConfig := h.ConfigHandler.GetConfig()

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

// Ensure AwsHelper implements Helper interface
var _ Helper = (*AwsHelper)(nil)
