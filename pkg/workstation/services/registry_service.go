package services

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/windsorcli/cli/api/v1alpha1/docker"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
)

// The RegistryService is a service component that manages Docker registry integration
// It provides local and remote registry capabilities with configurable endpoints
// The RegistryService enables container image management and distribution
// supporting both local caching and remote registry proxying

// =============================================================================
// Types
// =============================================================================

var (
	registryNextPort = constants.RegistryDefaultHostPort + 1
	registryMu       sync.Mutex
	localRegistry    *RegistryService
)

// RegistryService is a service struct that provides Registry-specific utility functions
type RegistryService struct {
	BaseService
	hostPort int
}

// =============================================================================
// Constructor
// =============================================================================

// NewRegistryService is a constructor for RegistryService
func NewRegistryService(rt *runtime.Runtime) *RegistryService {
	return &RegistryService{
		BaseService: *NewBaseService(rt),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetComposeConfig returns a Docker Compose configuration for the registry matching s.name.
// It retrieves the context configuration, finds the registry, and generates a service config.
// If no matching registry is found, it returns an error.
func (s *RegistryService) GetComposeConfig() (*types.Config, error) {
	contextConfig := s.configHandler.GetConfig()
	registries := contextConfig.Docker.Registries

	if registry, exists := registries[s.name]; exists {
		service, err := s.generateRegistryService(registry)
		if err != nil {
			return nil, fmt.Errorf("failed to generate registry service: %w", err)
		}
		serviceName := getBasename(s.GetHostname())
		return &types.Config{Services: types.Services{serviceName: service}}, nil
	}

	return nil, fmt.Errorf("no registry found with name: %s", s.name)
}

// SetAddress configures the registry's address, forms a hostname, and updates the registry config.
// It assigns the "registry_url" and the default host port for the first non-remote registry, storing it as "localRegistry".
func (s *RegistryService) SetAddress(address string, portAllocator *PortAllocator) error {
	if err := s.BaseService.SetAddress(address, portAllocator); err != nil {
		return fmt.Errorf("failed to set address for base service: %w", err)
	}

	hostName := s.GetHostname()

	err := s.configHandler.Set(fmt.Sprintf("docker.registries[%s].hostname", s.name), hostName)
	if err != nil {
		return fmt.Errorf("failed to set hostname for registry %s: %w", s.name, err)
	}

	registryConfig := s.configHandler.GetConfig().Docker.Registries[s.name]
	hostPort := 0

	if registryConfig.HostPort != 0 {
		hostPort = registryConfig.HostPort
	} else if s.isLocalhostMode() && registryConfig.Remote == "" {
		registryMu.Lock()
		defer registryMu.Unlock()

		if localRegistry == nil {
			localRegistry = s
			hostPort = constants.RegistryDefaultHostPort
			err = s.configHandler.Set("docker.registry_url", hostName)
			if err != nil {
				return fmt.Errorf("failed to set registry URL for registry %s: %w", s.name, err)
			}
		} else {
			hostPort = registryNextPort
			registryNextPort++
		}
	}

	if hostPort != 0 {
		s.hostPort = hostPort
		err := s.configHandler.Set(fmt.Sprintf("docker.registries[%s].hostport", s.name), hostPort)
		if err != nil {
			return fmt.Errorf("failed to set host port for registry %s: %w", s.name, err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// This function generates a ServiceConfig for a Registry service. It sets up the service's name, image,
// restart policy, and labels. It configures environment variables based on registry URLs, creates a
// cache directory, and sets volume mounts. Ports are assigned only for non-proxy registries when the
// network mode is localhost. It returns the configured ServiceConfig or an error if any step fails.
func (s *RegistryService) generateRegistryService(registry docker.RegistryConfig) (types.ServiceConfig, error) {
	contextName := s.configHandler.GetContext()
	serviceName := getBasename(s.GetHostname())
	containerName := s.GetContainerName()

	service := types.ServiceConfig{
		Name:          serviceName,
		ContainerName: containerName,
		Image:         constants.RegistryDefaultImage,
		Restart:       "always",
		Labels: map[string]string{
			"role":       "registry",
			"managed_by": "windsor",
			"context":    contextName,
		},
	}

	// Initialize environment variables
	env := make(map[string]*string)

	// Set remote URL if specified
	if registry.Remote != "" {
		remoteURL := registry.Remote
		env["REGISTRY_PROXY_REMOTEURL"] = &remoteURL
	}

	// Set local URL if specified
	if registry.Local != "" {
		localURL := registry.Local
		env["REGISTRY_PROXY_LOCALURL"] = &localURL
	}

	// Always set environment, even if empty
	service.Environment = env

	projectRoot := s.runtime.ProjectRoot
	cacheDir := projectRoot + "/.windsor/.docker-cache"
	if err := s.shims.MkdirAll(cacheDir, os.ModePerm); err != nil {
		return service, fmt.Errorf("error creating .docker-cache directory: %w", err)
	}

	service.Volumes = []types.ServiceVolumeConfig{
		{Type: "bind", Source: "${WINDSOR_PROJECT_ROOT}/.windsor/.docker-cache", Target: "/var/lib/registry"},
	}

	if s.isLocalhostMode() {
		service.Ports = []types.ServicePortConfig{
			{
				Target:    5000,
				Published: fmt.Sprintf("%d", s.hostPort),
				Protocol:  "tcp",
			},
		}
	}

	return service, nil
}

// =============================================================================
// Helpers
// =============================================================================

// getBasename removes the last part of a domain name if it exists
func getBasename(name string) string {
	if parts := strings.Split(name, "."); len(parts) > 1 {
		return strings.Join(parts[:len(parts)-1], ".")
	}
	return name
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure RegistryService implements Service interface
var _ Service = (*RegistryService)(nil)
