package services

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
)

// RegistryService is a service struct that provides Registry-specific utility functions
type RegistryService struct {
	BaseService
	nextPort int
}

// NewRegistryService is a constructor for RegistryService
func NewRegistryService(injector di.Injector) *RegistryService {
	return &RegistryService{
		BaseService: BaseService{
			injector: injector,
		},
		nextPort: 5000, // Initialize the next available port for localhost
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
		// Get the service hostname
		hostname := s.GetHostname()

		// Pass the hostname to generateRegistryService
		service, err := s.generateRegistryService(hostname, registry.Remote, registry.Local)
		if err != nil {
			return nil, fmt.Errorf("failed to generate registry service: %w", err)
		}
		return &types.Config{Services: []types.ServiceConfig{service}}, nil
	}

	return nil, fmt.Errorf("no registry found with name: %s", s.name)
}

// SetAddress configures the registry address, forms a hostname, updates the
// registry config, and returns an error if any step fails. It appends the domain
// to the service name to form the hostname.
func (s *RegistryService) SetAddress(address string) error {
	if err := s.BaseService.SetAddress(address); err != nil {
		return fmt.Errorf("failed to set address for base service: %w", err)
	}

	// Get the hostname using the GetHostname method
	hostName := s.GetHostname()

	err := s.configHandler.SetContextValue(fmt.Sprintf("docker.registries[%s].hostname", s.name), hostName)
	if err != nil {
		return fmt.Errorf("failed to set hostname for registry %s: %w", s.name, err)
	}

	return nil
}

// GetHostname returns the hostname of the registry service. This is constructed
// by removing the existing domain from the name and appending the configured domain.
func (s *RegistryService) GetHostname() string {
	domain := s.configHandler.GetString("dns.domain", "test")
	nameWithoutDomain := s.name
	if dotIndex := strings.LastIndex(s.name, "."); dotIndex != -1 {
		nameWithoutDomain = s.name[:dotIndex]
	}
	return nameWithoutDomain + "." + domain
}

// generateRegistryService creates a ServiceConfig for a Registry service
// with the specified name, remote URL, and local URL.
func (s *RegistryService) generateRegistryService(hostName, remoteURL, localURL string) (types.ServiceConfig, error) {
	// Retrieve the context name
	contextName := s.configHandler.GetContext()

	// Initialize the ServiceConfig with the provided name, a predefined image,
	// a restart policy, and labels indicating the role and manager.
	service := types.ServiceConfig{
		Name:          hostName,
		ContainerName: hostName,
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
	if err != nil {
		return types.ServiceConfig{}, fmt.Errorf("error retrieving project root: %w", err)
	}
	cacheDir := projectRoot + "/.windsor/.docker-cache"
	if err := mkdirAll(cacheDir, os.ModePerm); err != nil {
		return service, fmt.Errorf("error creating .docker-cache directory: %w", err)
	}

	// Use the WINDSOR_PROJECT_ROOT environment variable for the volume mount
	service.Volumes = []types.ServiceVolumeConfig{
		{Type: "bind", Source: "${WINDSOR_PROJECT_ROOT}/.windsor/.docker-cache", Target: "/var/lib/registry"},
	}

	// Check if the address is localhost and assign ports if it is
	// Only forward port 5000 if the registry is not used as a proxy
	if isLocalhost(s.address) && remoteURL == "" {
		for {
			if isPortAvailable(s.nextPort) {
				service.Ports = []types.ServicePortConfig{
					{
						Target:    5000,
						Published: fmt.Sprintf("%d", s.nextPort),
						Protocol:  "tcp",
					},
				}
				s.nextPort++
				break
			}
			s.nextPort++
		}
	}

	// Return the configured ServiceConfig and nil error.
	return service, nil
}

// isPortAvailable checks if a port is available for use
func isPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	defer ln.Close()
	return true
}

// Ensure RegistryService implements Service interface
var _ Service = (*RegistryService)(nil)
