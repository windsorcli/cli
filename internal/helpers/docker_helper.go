package helpers

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// DockerHelper is a helper struct that provides Docker-specific utility functions
type DockerHelper struct {
	ConfigHandler config.ConfigHandler
	Context       context.ContextInterface
	Helpers       []Helper
}

const registryImage = "registry:2.8.3"

// NewDockerHelper is a constructor for DockerHelper
func NewDockerHelper(di *di.DIContainer) (*DockerHelper, error) {
	cliConfigHandler, err := di.Resolve("cliConfigHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving cliConfigHandler: %w", err)
	}

	resolvedContext, err := di.Resolve("context")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}

	helpers, err := di.ResolveAll((*Helper)(nil))
	if err != nil {
		return nil, fmt.Errorf("error resolving helpers: %w", err)
	}

	// Register DockerHelper as a Helper
	dockerHelper := &DockerHelper{
		ConfigHandler: cliConfigHandler.(config.ConfigHandler),
		Context:       resolvedContext.(context.ContextInterface),
	}
	helperInstances := make([]Helper, len(helpers)+1) // Increase the slice size by 1
	for i, helper := range helpers {
		helperInstances[i] = helper.(Helper)
	}

	// Add DockerHelper to the list of helpers
	helperInstances[len(helpers)] = dockerHelper

	dockerHelper.Helpers = helperInstances

	return dockerHelper, nil
}

// Initialize performs any necessary initialization for the helper.
func (h *DockerHelper) Initialize() error {
	// Perform any necessary initialization here
	return nil
}

// GetEnvVars retrieves Docker-specific environment variables for the current context
func (h *DockerHelper) GetEnvVars() (map[string]string, error) {
	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving config root: %w", err)
	}

	// Check for the existence of compose.yaml or compose.yml
	var composeFilePath string
	yamlPath := filepath.Join(configRoot, "compose.yaml")
	ymlPath := filepath.Join(configRoot, "compose.yml")

	if _, err := os.Stat(yamlPath); err == nil {
		composeFilePath = yamlPath
	} else if _, err := os.Stat(ymlPath); err == nil {
		composeFilePath = ymlPath
	}

	envVars := map[string]string{
		"COMPOSE_FILE": composeFilePath,
	}

	return envVars, nil
}

// PostEnvExec runs any necessary commands after the environment variables have been set.
func (h *DockerHelper) PostEnvExec() error {
	return nil
}

// generateRegistryService creates a ServiceConfig for a Docker registry service
// with the specified name, remote URL, and local URL.
func generateRegistryService(name, remoteURL, localURL string) types.ServiceConfig {
	// Initialize the ServiceConfig with the provided name, a predefined image,
	// a restart policy, and labels indicating the role and manager.
	service := types.ServiceConfig{
		Name:    name,
		Image:   registryImage,
		Restart: "always",
		Labels: map[string]string{
			"role":       "registry",
			"managed_by": "windsor",
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
	return service
}

// GetComposeConfig returns a list of container data for docker-compose.
func (h *DockerHelper) GetComposeConfig() (*types.Config, error) {
	var services []types.ServiceConfig

	// Retrieve the context configuration using GetConfig
	contextConfig, err := h.ConfigHandler.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context configuration: %w", err)
	}

	// Retrieve the list of registries from the context configuration
	registries := contextConfig.Docker.Registries

	// Convert registries to service definitions
	for _, registry := range registries {
		services = append(services, generateRegistryService(registry.Name, registry.Remote, registry.Local))
	}

	return &types.Config{Services: services}, nil
}

// WriteConfig writes any vendor specific configuration files that are needed for the helper.
func (h *DockerHelper) WriteConfig() error {
	// Retrieve the context configuration using GetConfig
	contextConfig, err := h.ConfigHandler.GetConfig()
	if err != nil {
		return fmt.Errorf("error retrieving context configuration: %w", err)
	}

	// Check if Docker is defined in the windsor config
	if contextConfig.Docker == nil {
		return nil
	}

	var combinedServices []types.ServiceConfig
	var combinedVolumes map[string]types.VolumeConfig
	var combinedNetworks map[string]types.NetworkConfig

	combinedVolumes = make(map[string]types.VolumeConfig)
	combinedNetworks = make(map[string]types.NetworkConfig)

	// Iterate through each helper and collect container configs
	for _, helper := range h.Helpers {
		if helperInstance, ok := helper.(Helper); ok {
			helperName := fmt.Sprintf("%T", helperInstance)
			containerConfigs, err := helperInstance.GetComposeConfig()
			if err != nil {
				return fmt.Errorf("error getting container config from helper %s: %w", helperName, err)
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
	contextName, err := h.Context.GetContext()
	if err != nil {
		return fmt.Errorf("error retrieving context: %w", err)
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
			return fmt.Errorf("error parsing network CIDR: %w", err)
		}

		// Skip the network address
		ip = incrementIP(ip)

		// Skip the first IP address
		ip = incrementIP(ip)

		for i := range combinedServices {
			combinedServices[i].Networks = map[string]*types.ServiceNetworkConfig{
				networkName: {
					Ipv4Address: ip.String(),
				},
			}
			ip = incrementIP(ip)
			if !ipNet.Contains(ip) {
				return fmt.Errorf("not enough IP addresses in the CIDR range")
			}
		}
	}

	// Create a Project using compose-go
	project := &types.Project{
		Services: combinedServices,
		Volumes:  combinedVolumes,
		Networks: combinedNetworks,
	}

	// Serialize the docker-compose config to YAML
	yamlData, err := yamlMarshal(project)
	if err != nil {
		return fmt.Errorf("error marshaling docker-compose config to YAML: %w", err)
	}

	// Get the config root and construct the file path
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error retrieving config root: %w", err)
	}
	composeFilePath := filepath.Join(configRoot, "compose.yaml")

	// Ensure the parent context folder exists
	if err := mkdirAll(filepath.Dir(composeFilePath), 0755); err != nil {
		return fmt.Errorf("error creating parent context folder: %w", err)
	}

	// Write the YAML data to the specified file
	err = writeFile(composeFilePath, yamlData, 0644)
	if err != nil {
		return fmt.Errorf("error writing docker-compose file: %w", err)
	}

	return nil
}

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

// Ensure DockerHelper implements Helper interface
var _ Helper = (*DockerHelper)(nil)
