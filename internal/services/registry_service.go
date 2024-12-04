package services

import (
	"fmt"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/constants"
	"github.com/windsor-hotel/cli/internal/di"
)

// RegistryService is a service struct that provides Registry-specific utility functions
type RegistryService struct {
	BaseService
}

// NewRegistryService is a constructor for RegistryService
func NewRegistryService(injector di.Injector) *RegistryService {
	return &RegistryService{
		BaseService: BaseService{
			injector: injector,
		},
	}
}

// generateRegistryService creates a ServiceConfig for a Registry service
// with the specified name, remote URL, and local URL.
func (s *RegistryService) generateRegistryService(name, remoteURL, localURL string) (types.ServiceConfig, error) {
	// Get the top level domain from the configuration
	tld := s.configHandler.GetString("dns.name", "test")

	// Retrieve the context name
	contextName, err := s.contextHandler.GetContext()
	if err != nil {
		return types.ServiceConfig{}, fmt.Errorf("error retrieving context: %w", err)
	}

	// Initialize the ServiceConfig with the provided name, a predefined image,
	// a restart policy, and labels indicating the role and manager.
	service := types.ServiceConfig{
		Name:    fmt.Sprintf("%s.%s", name, tld),
		Image:   constants.REGISTRY_DEFAULT_IMAGE,
		Restart: "always",
		Labels: map[string]string{
			"role":       "registry",
			"managed_by": "windsor",
			"context":    contextName,
		},
	}

	// Initialize the environment variables map.
	env := make(types.MappingWithEquals)

	// Add the remote URL to the environment variables if specified.
	if remoteURL != "" {
		env["REGISTRY_PROXY_REMOTEURL"] = &remoteURL
	}

	// Add the local URL to the environment variables if specified.
	if localURL != "" {
		env["REGISTRY_PROXY_LOCALURL"] = &localURL
	}

	// If any environment variables were added, assign them to the service.
	if len(env) > 0 {
		service.Environment = env
	}

	// Return the configured ServiceConfig.
	return service, nil
}

// GetComposeConfig returns a compose configuration for the registry matching the current s.name value.
func (s *RegistryService) GetComposeConfig() (*types.Config, error) {
	// Retrieve the context configuration using GetConfig
	contextConfig := s.configHandler.GetConfig()

	// Retrieve the list of registries from the context configuration
	registries := contextConfig.Docker.Registries

	// Find the registry matching the current s.name value
	for _, registry := range registries {
		if registry.Name == s.name {
			service, err := s.generateRegistryService(registry.Name, registry.Remote, registry.Local)
			if err != nil {
				return nil, err
			}
			return &types.Config{Services: []types.ServiceConfig{service}}, nil
		}
	}

	return nil, fmt.Errorf("no registry found with name: %s", s.name)
}

// Ensure RegistryService implements Service interface
var _ Service = (*RegistryService)(nil)
