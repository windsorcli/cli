package services

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/api/v1alpha1/docker"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
)

// RegistryService is a service struct that provides Registry-specific utility functions
type RegistryService struct {
	BaseService
	HostPort int // If set, this port is routed to the registry port from the host
}

// NewRegistryService is a constructor for RegistryService
func NewRegistryService(injector di.Injector) *RegistryService {
	return &RegistryService{
		BaseService: BaseService{
			injector: injector,
		},
	}
}

// GetComposeConfig returns a Docker Compose configuration for the registry matching s.name.
// It retrieves the context configuration, finds the registry, and generates a service config.
// If no matching registry is found, it returns an error.
func (s *RegistryService) GetComposeConfig() (*types.Config, error) {
	contextConfig := s.configHandler.GetConfig()
	registries := contextConfig.Docker.Registries

	if registry, exists := registries[s.name]; exists {
		hostname := s.GetHostname()
		service, err := s.generateRegistryService(hostname, registry)
		if err != nil {
			return nil, fmt.Errorf("failed to generate registry service: %w", err)
		}
		return &types.Config{Services: []types.ServiceConfig{service}}, nil
	}

	return nil, fmt.Errorf("no registry found with name: %s", s.name)
}

// SetAddress configures the registry's address, forms a hostname, and updates the registry config.
// It selects a port by checking the registry's HostPort; if unset and on localhost, it defaults to
// REGISTRY_DEFAULT_HOST_PORT. The port's availability is verified before assignment. If the registry
// is not a proxy ("remote" is not set), and it is localhost, it attempts to set HostPort to
// the default registry port.
func (s *RegistryService) SetAddress(address string) error {
	if err := s.BaseService.SetAddress(address); err != nil {
		return fmt.Errorf("failed to set address for base service: %w", err)
	}

	defaultPort := constants.REGISTRY_DEFAULT_HOST_PORT
	hostName := s.GetHostname()

	err := s.configHandler.SetContextValue(fmt.Sprintf("docker.registries[%s].hostname", s.name), hostName)
	if err != nil {
		return fmt.Errorf("failed to set hostname for registry %s: %w", s.name, err)
	}

	registryConfig := s.configHandler.GetConfig().Docker.Registries[s.name]
	hostPort := 0

	if registryConfig.HostPort != 0 {
		hostPort = registryConfig.HostPort
	} else if registryConfig.Remote == "" && s.IsLocalhost() {
		hostPort = defaultPort
	}

	if hostPort != 0 {
		if isPortAvailable(hostPort) {
			s.HostPort = hostPort
			err := s.configHandler.SetContextValue(fmt.Sprintf("docker.registries[%s].hostport", s.name), hostPort)
			if err != nil {
				return fmt.Errorf("failed to set host port for registry %s: %w", s.name, err)
			}
		} else {
			return fmt.Errorf("port %d is not available", hostPort)
		}
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

// This function generates a ServiceConfig for a Registry service. It sets up the service's name, image,
// restart policy, and labels. It configures environment variables based on registry URLs, creates a
// cache directory, and sets volume mounts. Ports are assigned only for non-proxy registries when the
// network mode is localhost. It returns the configured ServiceConfig or an error if any step fails.
func (s *RegistryService) generateRegistryService(hostname string, registry docker.RegistryConfig) (types.ServiceConfig, error) {
	contextName := s.configHandler.GetContext()

	service := types.ServiceConfig{
		Name:          hostname,
		ContainerName: hostname,
		Image:         constants.REGISTRY_DEFAULT_IMAGE,
		Restart:       "always",
		Labels: map[string]string{
			"role":       "registry",
			"managed_by": "windsor",
			"context":    contextName,
		},
	}

	env := make(types.MappingWithEquals)

	if registry.Remote != "" {
		env["REGISTRY_PROXY_REMOTEURL"] = &registry.Remote
	}

	if registry.Local != "" {
		env["REGISTRY_PROXY_LOCALURL"] = &registry.Local
	}

	if len(env) > 0 {
		service.Environment = env
	}

	projectRoot, err := s.shell.GetProjectRoot()
	if err != nil {
		return types.ServiceConfig{}, fmt.Errorf("error retrieving project root: %w", err)
	}
	cacheDir := projectRoot + "/.windsor/.docker-cache"
	if err := mkdirAll(cacheDir, os.ModePerm); err != nil {
		return service, fmt.Errorf("error creating .docker-cache directory: %w", err)
	}

	service.Volumes = []types.ServiceVolumeConfig{
		{Type: "bind", Source: "${WINDSOR_PROJECT_ROOT}/.windsor/.docker-cache", Target: "/var/lib/registry"},
	}

	if registry.Remote == "" && s.IsLocalhost() {
		service.Ports = []types.ServicePortConfig{
			{
				Target:    5000,
				Published: fmt.Sprintf("%d", s.HostPort),
				Protocol:  "tcp",
			},
		}
	}

	return service, nil
}

// isPortAvailable checks if a port is available for use
var isPortAvailable = func(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	defer ln.Close()
	return true
}

// Ensure RegistryService implements Service interface
var _ Service = (*RegistryService)(nil)
