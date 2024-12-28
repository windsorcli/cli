package services

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
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

// GetComposeConfig returns a compose configuration for the registry matching the current s.name value.
func (s *RegistryService) GetComposeConfig() (*types.Config, error) {
	// Retrieve the context configuration using GetConfig
	contextConfig := s.configHandler.GetConfig()

	// Retrieve the list of registries from the context configuration
	registries := contextConfig.Docker.Registries

	// Find the registry matching the current s.name value
	if registry, exists := registries[s.name]; exists {
		service := s.generateRegistryService(s.name, registry.Remote, registry.Local)
		return &types.Config{Services: []types.ServiceConfig{service}}, nil
	}

	return nil, fmt.Errorf("no registry found with name: %s", s.name)
}

// SetAddress establishes additional address information for the registry service
func (s *RegistryService) SetAddress(address string) error {
	// Call the parent SetAddress method
	s.BaseService.SetAddress(address)

	tld := s.configHandler.GetString("dns.name", "test")
	hostName := s.name + "." + tld

	// Set the hostname field in the registry configuration
	err := s.configHandler.Set(fmt.Sprintf("docker.registries.%s.hostname", s.name), hostName)
	if err != nil {
		return fmt.Errorf("failed to set hostname for registry %s: %w", s.name, err)
	}

	return nil
}

// generateRegistryService creates a ServiceConfig for a Registry service
// with the specified name, remote URL, and local URL.
func (s *RegistryService) generateRegistryService(serviceName, remoteURL, localURL string) types.ServiceConfig {
	// Retrieve the context name
	contextName := s.contextHandler.GetContext()

	// Get the TLD from the configuration
	tld := s.configHandler.GetString("dns.name", "test")
	fullName := serviceName + "." + tld

	// Initialize the ServiceConfig with the provided name, a predefined image,
	// a restart policy, and labels indicating the role and manager.
	service := types.ServiceConfig{
		Name:          fullName,
		ContainerName: fullName,
		Image:         constants.REGISTRY_DEFAULT_IMAGE,
		Restart:       "always",
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

	// Create a .docker-cache directory at the project root
	projectRoot, err := s.shell.GetProjectRoot()
	if err == nil {
		dockerCachePath := filepath.Join(projectRoot, ".docker-cache")
		if err := mkdirAll(dockerCachePath, os.ModePerm); err == nil {
			// Configure the .docker-cache as a volume mount
			service.Volumes = []types.ServiceVolumeConfig{
				{Type: "bind", Source: dockerCachePath, Target: "/var/lib/registry"},
			}
		}
	}

	// Return the configured ServiceConfig.
	return service
}

// Ensure RegistryService implements Service interface
var _ Service = (*RegistryService)(nil)
