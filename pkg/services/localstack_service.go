package services

import (
	"os"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
)

// LocalstackService is a service struct that provides Localstack-specific utility functions
type LocalstackService struct {
	BaseService
}

// NewLocalstackService is a constructor for LocalstackService
func NewLocalstackService(injector di.Injector) *LocalstackService {
	return &LocalstackService{
		BaseService: BaseService{
			injector: injector,
			name:     "aws",
		},
	}
}

// GetComposeConfig returns the top-level compose configuration including a list of container data for docker-compose.
func (s *LocalstackService) GetComposeConfig() (*types.Config, error) {
	// Get the context configuration
	contextConfig := s.configHandler.GetConfig()

	// Get the localstack auth token
	localstackAuthToken := os.Getenv("LOCALSTACK_AUTH_TOKEN")

	// Get the image to use
	image := constants.DEFAULT_AWS_LOCALSTACK_IMAGE
	if localstackAuthToken != "" {
		image = constants.DEFAULT_AWS_LOCALSTACK_PRO_IMAGE
	}

	// Get the localstack services to enable
	servicesList := ""
	if contextConfig.AWS.Localstack.Services != nil {
		servicesList = strings.Join(contextConfig.AWS.Localstack.Services, ",")
	}

	// Get the domain from the configuration
	tld := s.configHandler.GetString("dns.domain", "test")
	fullName := s.name + "." + tld

	// Create the service config
	services := []types.ServiceConfig{
		{
			Name:          fullName,
			ContainerName: fullName,
			Image:         image,
			Restart:       "always",
			Environment: map[string]*string{
				"ENFORCE_IAM":   ptrString("1"),
				"PERSISTENCE":   ptrString("1"),
				"IAM_SOFT_MODE": ptrString("0"),
				"DEBUG":         ptrString("0"),
				"SERVICES":      ptrString(servicesList),
			},
			Labels: map[string]string{
				"role":       "localstack",
				"managed_by": "windsor",
				"wildcard":   "true",
			},
		},
	}

	// If the localstack auth token is set, add it to the environment
	if localstackAuthToken != "" {
		services[0].Environment["LOCALSTACK_AUTH_TOKEN"] = ptrString("${LOCALSTACK_AUTH_TOKEN}")
	}

	return &types.Config{Services: services}, nil
}

// Ensure LocalstackService implements Service interface
var _ Service = (*LocalstackService)(nil)
