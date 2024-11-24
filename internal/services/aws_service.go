package services

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

// AwsService is a service struct that provides AWS-specific utility functions
type AwsService struct {
	BaseService
	ConfigHandler config.ConfigHandler
	Context       context.ContextInterface
}

// NewAwsService is a constructor for AwsService
func NewAwsService(injector di.Injector) (*AwsService, error) {
	configHandler, err := injector.Resolve("configHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving configHandler: %w", err)
	}

	resolvedContext, err := injector.Resolve("contextHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}

	return &AwsService{
		ConfigHandler: configHandler.(config.ConfigHandler),
		Context:       resolvedContext.(context.ContextInterface),
	}, nil
}

// GetComposeConfig returns the top-level compose configuration including a list of container data for docker-compose.
func (s *AwsService) GetComposeConfig() (*types.Config, error) {
	contextConfig := s.ConfigHandler.GetConfig()

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

// Ensure AwsService implements Service interface
var _ Service = (*AwsService)(nil)
