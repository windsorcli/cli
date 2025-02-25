package services

import (
	"fmt"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
)

// WindsorService is a service struct that provides Windsor-specific utility functions
type WindsorService struct {
	BaseService
}

// NewWindsorService is a constructor for WindsorService
func NewWindsorService(injector di.Injector) *WindsorService {
	return &WindsorService{
		BaseService: BaseService{
			injector: injector,
			name:     "windsor",
		},
	}
}

// GetComposeConfig returns the docker-compose configuration for the Windsor service
func (s *WindsorService) GetComposeConfig() (*types.Config, error) {
	fullName := s.name

	// Retrieve environment keys
	originalEnvVars := s.configHandler.GetStringMap("environment")

	var envVarList types.MappingWithEquals
	if originalEnvVars != nil {
		// Create environment variable mappings in the format KEY: ${KEY}
		envVarList = make(types.MappingWithEquals, len(originalEnvVars))
		for k := range originalEnvVars {
			value := fmt.Sprintf("${%s}", k)
			envVarList[k] = &value
		}
	}

	serviceConfig := types.ServiceConfig{
		Name:          fullName,
		ContainerName: fullName,
		Image:         constants.DEFAULT_WINDSOR_IMAGE,
		Restart:       "always",
		Labels: map[string]string{
			"role":       "windsor_exec",
			"managed_by": "windsor",
		},
		Volumes: []types.ServiceVolumeConfig{
			{
				Type:   "bind",
				Source: "${WINDSOR_PROJECT_ROOT}",
				Target: "/work",
			},
		},
		Entrypoint: []string{"tail", "-f", "/dev/null"},
	}

	if envVarList != nil {
		serviceConfig.Environment = envVarList
	}

	services := []types.ServiceConfig{serviceConfig}

	return &types.Config{Services: services}, nil
}

// Ensure WindsorService implements Service interface
var _ Service = (*WindsorService)(nil)
