package virt

import (
	"fmt"
	"net"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/shell"
)

// DockerVirt implements the ContainerInterface for Docker
type DockerVirt struct {
	container di.ContainerInterface
}

// NewDockerVirt creates a new instance of DockerVirt using a DI container
func NewDockerVirt(container di.ContainerInterface) *DockerVirt {
	return &DockerVirt{
		container: container,
	}
}

// Up starts docker-compose
func (v *DockerVirt) Up(verbose ...bool) error {
	// Set verbose to false if not defined
	verboseFlag := false
	if len(verbose) > 0 {
		verboseFlag = verbose[0]
	}

	cliConfigHandler, err := v.container.Resolve("cliConfigHandler")
	if err != nil {
		return fmt.Errorf("error resolving config handler: %w", err)
	}
	contextConfig, err := cliConfigHandler.(config.ConfigHandler).GetConfig()
	if err != nil {
		return fmt.Errorf("error retrieving context configuration: %w", err)
	}

	resolvedShell, err := v.container.Resolve("shell")
	if err != nil {
		return fmt.Errorf("error resolving shell: %w", err)
	}
	shell := resolvedShell.(shell.Shell)

	// Check if Docker is enabled and run "docker-compose up" in daemon mode if necessary
	if contextConfig != nil && contextConfig.Docker != nil && *contextConfig.Docker.Enabled {
		// Ensure Docker daemon is running
		if err := v.checkDockerDaemon(); err != nil {
			return fmt.Errorf("Docker daemon is not running: %w", err)
		}

		// Retry logic for docker-compose up
		retries := 3
		var lastErr error
		var lastOutput string
		for i := 0; i < retries; i++ {
			command := "docker-compose"
			args := []string{"up", "-d"}
			output, err := shell.Exec(verboseFlag, "Executing docker-compose up command", command, args...)
			if err == nil {
				lastErr = nil
				break
			}

			lastErr = err
			lastOutput = output

			if i < retries-1 {
				fmt.Println("Retrying docker-compose up...")
				time.Sleep(2 * time.Second)
			}
		}

		if lastErr != nil {
			return fmt.Errorf("Error executing command %s %v: %w\n%s", "docker-compose", []string{"up", "-d"}, lastErr, lastOutput)
		}
	}

	return nil
}

// Down stops the Docker container
func (v *DockerVirt) Down(verbose ...bool) error {
	// Placeholder implementation
	return nil
}

// Delete removes the Docker container
func (v *DockerVirt) Delete(verbose ...bool) error {
	// Placeholder implementation
	return nil
}

// WriteConfig writes the Docker configuration file
func (v *DockerVirt) WriteConfig() error {
	// Get the config root and construct the file path
	resolvedContext, err := v.container.Resolve("contextHandler")
	if err != nil {
		return fmt.Errorf("error resolving context handler: %w", err)
	}
	configRoot, err := resolvedContext.(context.ContextInterface).GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error retrieving config root: %w", err)
	}
	composeFilePath := filepath.Join(configRoot, "compose.yaml")

	// Ensure the parent context folder exists
	if err := mkdirAll(filepath.Dir(composeFilePath), 0755); err != nil {
		return fmt.Errorf("error creating parent context folder: %w", err)
	}

	// Retrieve the full compose configuration
	project, err := v.getFullComposeConfig()
	if err != nil {
		return fmt.Errorf("error getting full compose config: %w", err)
	}

	// Serialize the docker-compose config to YAML
	yamlData, err := yamlMarshal(project)
	if err != nil {
		return fmt.Errorf("error marshaling docker-compose config to YAML: %w", err)
	}

	// Write the YAML data to the specified file
	err = writeFile(composeFilePath, yamlData, 0644)
	if err != nil {
		return fmt.Errorf("error writing docker-compose file: %w", err)
	}

	return nil
}

// GetContainerInfo returns a list of information about the Docker containers, including their labels
func (v *DockerVirt) GetContainerInfo() ([]ContainerInfo, error) {
	resolvedContext, err := v.container.Resolve("contextHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving context handler: %w", err)
	}
	contextName, err := resolvedContext.(context.ContextInterface).GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	shellInterface, err := v.container.Resolve("shell")
	if err != nil {
		return nil, fmt.Errorf("error resolving shell: %w", err)
	}
	shell := shellInterface.(shell.Shell)
	command := "docker"
	args := []string{"ps", "--filter", "label=managed_by=windsor", "--filter", fmt.Sprintf("label=context=%s", contextName), "--format", "{{.ID}}"}
	out, err := shell.Exec(false, "Fetching container IDs", command, args...)
	if err != nil {
		return nil, err
	}

	containerIDs := strings.Split(strings.TrimSpace(out), "\n")
	var containerInfos []ContainerInfo

	for _, containerID := range containerIDs {
		inspectArgs := []string{"inspect", containerID, "--format", "{{json .Config.Labels}}"}
		inspectOut, err := shell.Exec(false, "Inspecting container", command, inspectArgs...)
		if err != nil {
			return nil, err
		}

		var labels map[string]string
		if err := jsonUnmarshal([]byte(inspectOut), &labels); err != nil {
			return nil, err
		}

		serviceName, serviceExists := labels["com.docker.compose.service"]
		if !serviceExists {
			continue
		}

		networkInspectArgs := []string{"inspect", containerID, "--format", "{{json .NetworkSettings.Networks}}"}
		networkInspectOut, err := shell.Exec(false, "Inspecting container network settings", command, networkInspectArgs...)
		if err != nil {
			return nil, err
		}

		var networks map[string]struct {
			IPAddress string `json:"IPAddress"`
		}
		if err := jsonUnmarshal([]byte(networkInspectOut), &networks); err != nil {
			return nil, err
		}

		var ipAddress string
		for _, network := range networks {
			ipAddress = network.IPAddress
			break
		}

		containerInfos = append(containerInfos, ContainerInfo{
			Name:    serviceName,
			Address: ipAddress,
			Labels:  labels,
		})
	}

	// Sort containerInfos alphabetically by container name
	sort.Slice(containerInfos, func(i, j int) bool {
		return containerInfos[i].Name < containerInfos[j].Name
	})

	return containerInfos, nil
}

// Ensure DockerVirt implements ContainerInterface
var _ ContainerInterface = (*DockerVirt)(nil)

// checkDockerDaemon checks if the Docker daemon is running
func (v *DockerVirt) checkDockerDaemon() error {
	resolvedShell, err := v.container.Resolve("shell")
	if err != nil {
		return fmt.Errorf("error resolving shell: %w", err)
	}
	shell, ok := resolvedShell.(shell.Shell)
	if !ok {
		return fmt.Errorf("resolved shell is not of type shell.Shell")
	}

	command := "docker"
	args := []string{"info"}
	_, err = shell.Exec(false, "Checking Docker daemon", command, args...)
	return err
}

// getFullComposeConfig retrieves the full compose configuration for the DockerVirt.
func (v *DockerVirt) getFullComposeConfig() (*types.Project, error) {
	// Create a network called "windsor-<context-name>"
	resolvedContext, err := v.container.Resolve("contextHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving context handler: %w", err)
	}
	contextName, err := resolvedContext.(context.ContextInterface).GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	// Retrieve the context configuration
	resolvedConfigHandler, err := v.container.Resolve("cliConfigHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving config handler: %w", err)
	}
	configHandler := resolvedConfigHandler.(config.ConfigHandler)
	contextConfig, err := configHandler.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context configuration: %w", err)
	}

	// Check if Docker is defined in the windsor config
	if contextConfig.Docker == nil {
		return nil, nil
	}

	var combinedServices []types.ServiceConfig
	var combinedVolumes map[string]types.VolumeConfig
	var combinedNetworks map[string]types.NetworkConfig

	combinedVolumes = make(map[string]types.VolumeConfig)
	combinedNetworks = make(map[string]types.NetworkConfig)

	// Initialize helpers on-the-fly
	helpers, err := v.container.ResolveAll((*helpers.Helper)(nil))
	if err != nil {
		return nil, fmt.Errorf("error resolving helpers: %w", err)
	}

	// Iterate through each helper and collect container configs
	for _, helper := range helpers {
		if helperInstance, ok := helper.(interface{ GetComposeConfig() (*types.Config, error) }); ok {
			helperName := fmt.Sprintf("%T", helperInstance)
			containerConfigs, err := helperInstance.GetComposeConfig()
			if err != nil {
				return nil, fmt.Errorf("error getting container config from helper %s: %w", helperName, err)
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
