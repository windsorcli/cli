package services

import (
	"os"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
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
func NewLocalstackService(rt *runtime.Runtime) *LocalstackService {
	return &LocalstackService{
		BaseService: *NewBaseService(rt),
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
	image := constants.DefaultAWSLocalstackImage
	if localstackAuthToken != "" {
		image = constants.DefaultAWSLocalstackProImage
	}

	// Get the localstack services to enable
	servicesList := ""
	if contextConfig != nil && contextConfig.AWS != nil && contextConfig.AWS.Localstack != nil && contextConfig.AWS.Localstack.Services != nil {
		servicesList = strings.Join(contextConfig.AWS.Localstack.Services, ",")
	}

	// Get the service name and container name
	serviceName := s.GetName()
	containerName := s.GetContainerName()

	// Create the service config
	serviceConfig := types.ServiceConfig{
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
	}

	// If the localstack auth token is set, add it to the secrets
	if localstackAuthToken != "" {
		serviceConfig.Secrets = []types.ServiceSecretConfig{
			{
				Source: "LOCALSTACK_AUTH_TOKEN",
			},
		}
	}

	services := types.Services{
		serviceName: serviceConfig,
	}

	return &types.Config{Services: services}, nil
}

// SupportsWildcard returns whether the service supports wildcard DNS entries
func (s *LocalstackService) SupportsWildcard() bool {
	return true
}

// GetIncusConfig returns the Incus configuration for the Localstack service.
// It configures a container with environment variables for AWS Localstack services.
func (s *LocalstackService) GetIncusConfig() (*IncusConfig, error) {
	contextConfig := s.configHandler.GetConfig()
	localstackAuthToken := os.Getenv("LOCALSTACK_AUTH_TOKEN")

	image := constants.DefaultAWSLocalstackImage
	if localstackAuthToken != "" {
		image = constants.DefaultAWSLocalstackProImage
	}

	servicesList := ""
	if contextConfig != nil && contextConfig.AWS != nil && contextConfig.AWS.Localstack != nil && contextConfig.AWS.Localstack.Services != nil {
		servicesList = strings.Join(contextConfig.AWS.Localstack.Services, ",")
	}

	config := make(map[string]string)
	config["environment.ENFORCE_IAM"] = "1"
	config["environment.PERSISTENCE"] = "1"
	config["environment.IAM_SOFT_MODE"] = "0"
	config["environment.DEBUG"] = "0"
	config["environment.SERVICES"] = servicesList
	if localstackAuthToken != "" {
		config["environment.LOCALSTACK_AUTH_TOKEN"] = localstackAuthToken
	}

	return &IncusConfig{
		Type:   "container",
		Image:  "docker:" + image,
		Config: config,
	}, nil
}

// Ensure LocalstackService implements Service interface
var _ Service = (*LocalstackService)(nil)
