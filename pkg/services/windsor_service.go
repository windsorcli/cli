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

// GetComposeConfig generates the docker-compose config for Windsor service. It sets up
// environment variables, DNS settings if enabled, and service configurations.
func (s *WindsorService) GetComposeConfig() (*types.Config, error) {
	fullName := s.name

	originalEnvVars := s.configHandler.GetStringMap("environment")

	var envVarList types.MappingWithEquals
	if originalEnvVars != nil {
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

	if s.configHandler.GetBool("dns.enabled") {
		resolvedServices, err := s.injector.ResolveAll((*Service)(nil))
		if err != nil {
			return nil, fmt.Errorf("error retrieving DNS service: %w", err)
		}

		var dnsService *DNSService
		for _, svc := range resolvedServices {
			if ds, ok := svc.(*DNSService); ok {
				dnsService = ds
				break
			}
		}

		if dnsService == nil {
			return nil, fmt.Errorf("DNS service not found")
		}

		dnsAddress := dnsService.GetAddress()
		dnsDomain := s.configHandler.GetString("dns.domain", "test")

		serviceConfig.DNS = []string{dnsAddress}
		serviceConfig.DNSSearch = []string{dnsDomain}
	}

	services := []types.ServiceConfig{serviceConfig}

	return &types.Config{Services: services}, nil
}

// Ensure WindsorService implements Service interface
var _ Service = (*WindsorService)(nil)
