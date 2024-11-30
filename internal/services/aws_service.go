package services

import (
	"os"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/constants"
	"github.com/windsor-hotel/cli/internal/di"
)

// AwsService is a service struct that provides AWS-specific utility functions
type AwsService struct {
	BaseService
}

// NewAwsService is a constructor for AwsService
func NewAwsService(injector di.Injector) *AwsService {
	return &AwsService{
		BaseService: BaseService{
			injector: injector,
		},
	}
}

// GetComposeConfig returns the top-level compose configuration including a list of container data for docker-compose.
func (s *AwsService) GetComposeConfig() (*types.Config, error) {
	contextConfig := s.configHandler.GetConfig()

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
