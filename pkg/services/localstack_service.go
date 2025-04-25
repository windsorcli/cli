package services

import (
	"os"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
)

// The LocalstackService is a service component that manages AWS Localstack integration
// It provides local AWS service emulation with configurable service endpoints
// The LocalstackService enables local development with AWS services
// supporting both free and pro versions with authentication and persistence

// =============================================================================
// Types
// =============================================================================

// LocalstackService is a service struct that provides Localstack-specific utility functions
type LocalstackService struct {
	BaseService
}

// =============================================================================
// Constructor
// =============================================================================

// NewLocalstackService is a constructor for LocalstackService
func NewLocalstackService(injector di.Injector) *LocalstackService {
	return &LocalstackService{
		BaseService: *NewBaseService(injector),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

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

	// Get the service name and container name
	serviceName := s.GetName()
	containerName := s.GetContainerName()

	// Create the service config
	services := []types.ServiceConfig{
		{
			Name:          serviceName,
			ContainerName: containerName,
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

	// If the localstack auth token is set, add it to the secrets
	if localstackAuthToken != "" {
		services[0].Secrets = []types.ServiceSecretConfig{
			{
				Source: "LOCALSTACK_AUTH_TOKEN",
			},
		}
	}

	return &types.Config{Services: services}, nil
}

// SupportsWildcard returns whether the service supports wildcard DNS entries
func (s *LocalstackService) SupportsWildcard() bool {
	return true
}

// Ensure LocalstackService implements Service interface
var _ Service = (*LocalstackService)(nil)
