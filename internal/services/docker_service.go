package services

import (
	"fmt"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/di"
)

// DockerService is a service struct that provides Docker-specific utility functions
type DockerService struct {
	BaseService
}

const registryImage = "registry:2.8.3"

// NewDockerService is a constructor for DockerService
func NewDockerService(injector di.Injector) *DockerService {
	return &DockerService{
		BaseService: BaseService{
			injector: injector,
		},
	}
}

// generateRegistryService creates a ServiceConfig for a Docker registry service
// with the specified name, remote URL, and local URL.
func (s *DockerService) generateRegistryService(name, remoteURL, localURL string) (types.ServiceConfig, error) {
	// Retrieve the context name
	contextName, err := s.contextHandler.GetContext()
	if err != nil {
		return types.ServiceConfig{}, fmt.Errorf("error retrieving context: %w", err)
	}

	// Initialize the ServiceConfig with the provided name, a predefined image,
	// a restart policy, and labels indicating the role and manager.
	service := types.ServiceConfig{
		Name:    name,
		Image:   registryImage,
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

// GetComposeConfig returns a list of container data for docker-compose.
func (s *DockerService) GetComposeConfig() (*types.Config, error) {
	var services []types.ServiceConfig

	// Retrieve the context configuration using GetConfig
	contextConfig := s.configHandler.GetConfig()

	// Retrieve the list of registries from the context configuration
	registries := contextConfig.Docker.Registries

	// Convert registries to service definitions
	for _, registry := range registries {
		service, err := s.generateRegistryService(registry.Name, registry.Remote, registry.Local)
		if err != nil {
			return nil, err
		}
		services = append(services, service)
	}

	return &types.Config{Services: services}, nil
}

// Ensure DockerService implements Service interface
var _ Service = (*DockerService)(nil)
