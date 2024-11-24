package services

import (
	"fmt"
	"net"
	"sort"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

type DockerInfo struct {
	Services map[string]ServiceInfo `json:"services"`
}

type ServiceInfo struct {
	Role string `json:"role"`
	IP   string `json:"ip"`
}

// DockerService is a service struct that provides Docker-specific utility functions
type DockerService struct {
	BaseService
	ConfigHandler config.ConfigHandler
	Context       context.ContextInterface
	Injector      di.Injector
	Shell         shell.Shell
}

const registryImage = "registry:2.8.3"

// NewDockerService is a constructor for DockerService
func NewDockerService(injector di.Injector) (*DockerService, error) {
	configHandler, err := injector.Resolve("configHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving configHandler: %w", err)
	}

	resolvedContext, err := injector.Resolve("contextHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}

	resolvedShell, err := injector.Resolve("shell")
	if err != nil {
		return nil, fmt.Errorf("error resolving shell: %w", err)
	}

	return &DockerService{
		ConfigHandler: configHandler.(config.ConfigHandler),
		Context:       resolvedContext.(context.ContextInterface),
		Injector:      injector,
		Shell:         resolvedShell.(shell.Shell),
	}, nil
}

// Initialize performs any necessary initialization for the service.
func (s *DockerService) Initialize() error {
	// Perform any necessary initialization here
	return nil
}

// generateRegistryService creates a ServiceConfig for a Docker registry service
// with the specified name, remote URL, and local URL.
func (s *DockerService) generateRegistryService(name, remoteURL, localURL string) (types.ServiceConfig, error) {
	// Retrieve the context name
	contextName, err := s.Context.GetContext()
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
	contextConfig := s.ConfigHandler.GetConfig()

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

// GetFullComposeConfig retrieves the full compose configuration for the DockerService.
func (s *DockerService) GetFullComposeConfig() (*types.Project, error) {
	// Retrieve the context configuration using GetConfig
	contextConfig := s.ConfigHandler.GetConfig()

	// Check if Docker is defined in the windsor config
	if contextConfig.Docker == nil {
		return nil, nil
	}

	var combinedServices []types.ServiceConfig
	var combinedVolumes map[string]types.VolumeConfig
	var combinedNetworks map[string]types.NetworkConfig

	combinedVolumes = make(map[string]types.VolumeConfig)
	combinedNetworks = make(map[string]types.NetworkConfig)

	// Initialize services on-the-fly
	services, err := s.Injector.ResolveAll((*Service)(nil))
	if err != nil {
		return nil, fmt.Errorf("error resolving services: %w", err)
	}

	// Iterate through each service and collect container configs
	for _, service := range services {
		if serviceInstance, ok := service.(Service); ok {
			serviceName := fmt.Sprintf("%T", serviceInstance)
			containerConfigs, err := serviceInstance.GetComposeConfig()
			if err != nil {
				return nil, fmt.Errorf("error getting container config from service %s: %w", serviceName, err)
			}
			if containerConfigs == nil {
				continue
			}
			if containerConfigs.Services != nil {
				for _, containerConfig := range containerConfigs.Services {
					combinedServices = append(combinedServices, containerConfig)
				}
			}
			if containerConfigs.Volumes != nil {
				for volumeName, volumeConfig := range containerConfigs.Volumes {
					combinedVolumes[volumeName] = volumeConfig
				}
			}
			if containerConfigs.Networks != nil {
				for networkName, networkConfig := range containerConfigs.Networks {
					combinedNetworks[networkName] = networkConfig
				}
			}
		}
	}

	// Create a network called "windsor-<context-name>"
	contextName, err := s.Context.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}
	networkName := fmt.Sprintf("windsor-%s", contextName)

	// Assign the CIDR to the network configuration
	if contextConfig.Docker.NetworkCIDR != nil {
		combinedNetworks[networkName] = types.NetworkConfig{
			Driver: "bridge",
			Ipam: types.IPAMConfig{
				Driver: "default",
				Config: []*types.IPAMPool{
					{
						Subnet: *contextConfig.Docker.NetworkCIDR,
					},
				},
			},
		}
	} else {
		combinedNetworks[networkName] = types.NetworkConfig{}
	}

	// Assign IP addresses to services based on the network CIDR
	if contextConfig.Docker.NetworkCIDR != nil {
		ip, ipNet, err := net.ParseCIDR(*contextConfig.Docker.NetworkCIDR)
		if err != nil {
			return nil, fmt.Errorf("error parsing network CIDR: %w", err)
		}

		// Skip the network address
		ip = incrementIP(ip)

		// Skip the first IP address
		ip = incrementIP(ip)

		// Alphabetize the names of the services
		sort.Slice(combinedServices, func(i, j int) bool {
			return combinedServices[i].Name < combinedServices[j].Name
		})

		for i := range combinedServices {
			combinedServices[i].Networks = map[string]*types.ServiceNetworkConfig{
				networkName: {
					Ipv4Address: ip.String(),
				},
			}
			ip = incrementIP(ip)
			if !ipNet.Contains(ip) {
				return nil, fmt.Errorf("not enough IP addresses in the CIDR range")
			}
		}
	}

	// Create a Project using compose-go
	project := &types.Project{
		Services: combinedServices,
		Volumes:  combinedVolumes,
		Networks: combinedNetworks,
	}

	return project, nil
}

// Ensure DockerService implements Service interface
var _ Service = (*DockerService)(nil)

// incrementIP increments an IP address by one
func incrementIP(ip net.IP) net.IP {
	ip = ip.To4()
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
	return ip
}
