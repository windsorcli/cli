package services

import (
	"fmt"
	"os"
	"strconv"
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

// GetComposeConfig constructs and returns a Docker Compose configuration for the Localstack service.
// It retrieves the context configuration, checks for a Localstack authentication token, and determines
// the appropriate image to use. It also gathers the list of Localstack services to enable, constructs
// the full domain name, and sets up the service configuration with environment variables, labels, and
// port settings. If an authentication token is present, it adds it to the service secrets.
func (s *LocalstackService) GetComposeConfig() (*types.Config, error) {
	contextConfig := s.configHandler.GetConfig()
	localstackAuthToken := os.Getenv("LOCALSTACK_AUTH_TOKEN")

	image := constants.DEFAULT_AWS_LOCALSTACK_IMAGE
	if localstackAuthToken != "" {
		image = constants.DEFAULT_AWS_LOCALSTACK_PRO_IMAGE
	}

	servicesList := ""
	if contextConfig.AWS.Localstack.Services != nil {
		servicesList = strings.Join(contextConfig.AWS.Localstack.Services, ",")
	}

	tld := s.configHandler.GetString("dns.domain", "test")
	fullName := s.name + "." + tld

	port, err := strconv.ParseUint(constants.DEFAULT_AWS_LOCALSTACK_PORT, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid port format: %w", err)
	}
	port32 := uint32(port)

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
				"role":       "aws",
				"managed_by": "windsor",
			},
			Ports: []types.ServicePortConfig{
				{
					Target:    port32,
					Published: constants.DEFAULT_AWS_LOCALSTACK_PORT,
					Protocol:  "tcp",
				},
			},
		},
	}

	if localstackAuthToken != "" {
		services[0].Environment["LOCALSTACK_AUTH_TOKEN"] = ptrString("${LOCALSTACK_AUTH_TOKEN}")
	}

	return &types.Config{Services: services}, nil
}

// Ensure LocalstackService implements Service interface
var _ Service = (*LocalstackService)(nil)
