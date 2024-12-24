package virt

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/services"
)

// DockerVirt implements the ContainerInterface for Docker
type DockerVirt struct {
	BaseVirt
	services       []services.Service
	composeCommand string
}

// NewDockerVirt creates a new instance of DockerVirt using a DI injector
func NewDockerVirt(injector di.Injector) *DockerVirt {
	return &DockerVirt{
		BaseVirt: BaseVirt{
			injector: injector,
		},
	}
}

// Initialize resolves the dependencies for DockerVirt
func (v *DockerVirt) Initialize() error {
	if err := v.BaseVirt.Initialize(); err != nil {
		return fmt.Errorf("error initializing base: %w", err)
	}

	// Resolve all services
	resolvedServices, err := v.injector.ResolveAll((*services.Service)(nil))
	if err != nil {
		return fmt.Errorf("error resolving services: %w", err)
	}

	// Convert the resolved services to the correct type
	serviceSlice := make([]services.Service, len(resolvedServices))
	for i, service := range resolvedServices {
		if s, _ := service.(services.Service); s != nil {
			serviceSlice[i] = s
		}
	}

	// Alphabetize the services by their name
	sort.Slice(serviceSlice, func(i, j int) bool {
		return fmt.Sprintf("%T", serviceSlice[i]) < fmt.Sprintf("%T", serviceSlice[j])
	})

	// Get the context configuration
	contextConfig := v.configHandler.GetConfig()
	if contextConfig == nil || contextConfig.Docker == nil {
		return fmt.Errorf("Docker configuration is not defined")
	}

	// Set the services
	v.services = serviceSlice

	// Determine the correct docker compose command
	if err := v.determineComposeCommand(); err != nil {
		return fmt.Errorf("error determining docker compose command: %w", err)
	}

	return nil
}

// determineComposeCommand checks for available docker compose commands
func (v *DockerVirt) determineComposeCommand() error {
	commands := []string{"docker-compose", "docker-cli-plugin-docker-compose", "docker compose"}
	for _, cmd := range commands {
		if _, err := v.shell.ExecSilent(cmd, "--version"); err == nil {
			v.composeCommand = cmd
			return nil
		}
	}
	return fmt.Errorf("no valid docker compose command found")
}

// Up starts docker compose
func (v *DockerVirt) Up() error {
	// Get the context configuration
	contextConfig := v.configHandler.GetConfig()

	// Check if Docker is enabled and run "docker compose up" in daemon mode if necessary
	if contextConfig != nil && contextConfig.Docker != nil && *contextConfig.Docker.Enabled {
		// Ensure Docker daemon is running
		if err := v.checkDockerDaemon(); err != nil {
			return fmt.Errorf("Docker daemon is not running: %w", err)
		}

		// Get the path to the compose.yaml file
		configRoot, err := v.contextHandler.GetConfigRoot()
		if err != nil {
			return fmt.Errorf("error retrieving config root: %w", err)
		}
		composeFilePath := filepath.Join(configRoot, "compose.yaml")

		// Set the COMPOSE_FILE environment variable and handle potential error
		if err := osSetenv("COMPOSE_FILE", composeFilePath); err != nil {
			return fmt.Errorf("failed to set COMPOSE_FILE environment variable: %w", err)
		}

		// Retry logic for docker compose up with progress display
		retries := 3
		var lastErr error
		var lastOutput string
		for i := 0; i < retries; i++ {
			args := []string{"up", "--detach", "--remove-orphans"}
			message := "📦 Running docker compose up"

			// Use ExecProgress for the first attempt to show progress
			if i == 0 {
				output, err := v.shell.ExecProgress(message, v.composeCommand, args...)
				if err == nil {
					return nil
				}
				lastErr = err
				lastOutput = output
			} else {
				// Use ExecSilent for retries to avoid multiple progress messages
				output, err := v.shell.ExecSilent(v.composeCommand, args...)
				if err == nil {
					return nil
				}
				lastErr = err
				lastOutput = output
			}

			if i < retries-1 {
				time.Sleep(time.Duration(RETRY_WAIT) * time.Second)
			}
		}
		if lastErr != nil {
			return fmt.Errorf("Error executing command %s %v: %w\n%s", v.composeCommand, []string{"up", "--detach", "--remove-orphans"}, lastErr, lastOutput)
		}
	}
	return nil
}

// Down stops the Docker container
func (v *DockerVirt) Down() error {
	// Check if Docker is enabled and run "docker compose down" if necessary
	if v.configHandler.GetBool("docker.enabled") {
		// Ensure Docker daemon is running
		if err := v.checkDockerDaemon(); err != nil {
			return fmt.Errorf("Docker daemon is not running: %w", err)
		}

		// Get the path to the compose.yaml file
		configRoot, err := v.contextHandler.GetConfigRoot()
		if err != nil {
			return fmt.Errorf("error retrieving config root: %w", err)
		}
		composeFilePath := filepath.Join(configRoot, "compose.yaml")

		// Set the COMPOSE_FILE environment variable and handle potential error
		if err := osSetenv("COMPOSE_FILE", composeFilePath); err != nil {
			return fmt.Errorf("error setting COMPOSE_FILE environment variable: %w", err)
		}

		// Run docker compose down with clean flags using the Exec function from shell.go
		output, err := v.shell.ExecProgress("📦 Running docker compose down", v.composeCommand, "down", "--remove-orphans", "--volumes")
		if err != nil {
			return fmt.Errorf("Error executing command %s down: %w\n%s", v.composeCommand, err, output)
		}
	}
	return nil
}

// WriteConfig writes the Docker configuration file
func (v *DockerVirt) WriteConfig() error {
	// Get the config root and construct the file path
	configRoot, err := v.contextHandler.GetConfigRoot()
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

	// Serialize the docker compose config to YAML
	yamlData, err := yamlMarshal(project)
	if err != nil {
		return fmt.Errorf("error marshaling docker compose config to YAML: %w", err)
	}

	// Write the YAML data to the specified file
	err = writeFile(composeFilePath, yamlData, 0644)
	if err != nil {
		return fmt.Errorf("error writing docker compose file: %w", err)
	}

	return nil
}

// GetContainerInfo returns a list of information about the Docker containers, including their labels
func (v *DockerVirt) GetContainerInfo(name ...string) ([]ContainerInfo, error) {
	// Get the context name
	contextName := v.contextHandler.GetContext()

	command := "docker"
	args := []string{"ps", "--filter", "label=managed_by=windsor", "--filter", fmt.Sprintf("label=context=%s", contextName), "--format", "{{.ID}}"}
	out, err := v.shell.ExecSilent(command, args...)
	if err != nil {
		return nil, err
	}

	containerIDs := strings.Split(strings.TrimSpace(out), "\n")
	var containerInfos []ContainerInfo

	for _, containerID := range containerIDs {
		if containerID == "" {
			continue
		}
		inspectArgs := []string{"inspect", containerID, "--format", "{{json .Config.Labels}}"}
		inspectOut, err := v.shell.ExecSilent(command, inspectArgs...)
		if err != nil {
			return nil, err
		}

		var labels map[string]string
		if err := jsonUnmarshal([]byte(inspectOut), &labels); err != nil {
			return nil, err
		}

		serviceName, _ := labels["com.docker.compose.service"]

		// If a name is provided, check if it matches the current serviceName
		if len(name) > 0 && serviceName != name[0] {
			continue
		}

		networkInspectArgs := []string{"inspect", containerID, "--format", "{{json .NetworkSettings.Networks}}"}
		networkInspectOut, err := v.shell.ExecSilent(command, networkInspectArgs...)
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
		networkKey := fmt.Sprintf("windsor-%s", contextName)
		if network, exists := networks[networkKey]; exists {
			ipAddress = network.IPAddress
		}

		containerInfo := ContainerInfo{
			Name:    serviceName,
			Address: ipAddress,
			Labels:  labels,
		}

		// If a name is provided and matches, return immediately with this containerInfo
		if len(name) > 0 && serviceName == name[0] {
			return []ContainerInfo{containerInfo}, nil
		}

		containerInfos = append(containerInfos, containerInfo)
	}

	return containerInfos, nil
}

// PrintInfo prints the container information
func (v *DockerVirt) PrintInfo() error {
	containerInfos, err := v.GetContainerInfo()
	if err != nil {
		return fmt.Errorf("error retrieving container info: %w", err)
	}

	if len(containerInfos) == 0 {
		fmt.Println("No Docker containers are currently running.")
		return nil
	}

	fmt.Printf("%-30s\t%-15s\t%-10s\n", "CONTAINER NAME", "ADDRESS", "ROLE")
	for _, info := range containerInfos {
		role := info.Labels["role"]
		fmt.Printf("%-30s\t%-15s\t%-10s\n", info.Name, info.Address, role)
	}
	fmt.Println()

	return nil
}

// Ensure DockerVirt implements ContainerRuntime
var _ ContainerRuntime = (*DockerVirt)(nil)

// checkDockerDaemon checks if the Docker daemon is running
func (v *DockerVirt) checkDockerDaemon() error {
	command := "docker"
	args := []string{"info"}
	_, err := v.shell.ExecSilent(command, args...)
	return err
}

// getFullComposeConfig retrieves the full compose configuration for the DockerVirt.
func (v *DockerVirt) getFullComposeConfig() (*types.Project, error) {
	// Get the context name
	contextName := v.contextHandler.GetContext()

	// Get the context configuration
	contextConfig := v.configHandler.GetConfig()

	// Check if Docker is defined in the windsor config
	if contextConfig.Docker == nil {
		return nil, nil
	}

	// Initialize the combined services, volumes, and networks
	var combinedServices []types.ServiceConfig
	var combinedVolumes map[string]types.VolumeConfig
	var combinedNetworks map[string]types.NetworkConfig

	combinedVolumes = make(map[string]types.VolumeConfig)
	combinedNetworks = make(map[string]types.NetworkConfig)

	// Iterate through each service and collect container configs
	for _, service := range v.services {
		if serviceInstance, ok := service.(interface {
			GetComposeConfig() (*types.Config, error)
			GetAddress() string
		}); ok {
			serviceName := fmt.Sprintf("%T", serviceInstance)

			// Retrieve the compose configuration for the service
			containerConfigs, err := serviceInstance.GetComposeConfig()
			if err != nil {
				return nil, fmt.Errorf("error getting container config from service %s: %w", serviceName, err)
			}
			if containerConfigs == nil {
				continue
			}

			// Add service configurations to the combined list
			if containerConfigs.Services != nil {
				for _, containerConfig := range containerConfigs.Services {

					// Set the IP address for the service
					containerConfig.Networks = map[string]*types.ServiceNetworkConfig{
						fmt.Sprintf("windsor-%s", contextName): {
							Ipv4Address: serviceInstance.GetAddress(),
						},
					}

					// Add the service configuration to the combined list
					combinedServices = append(combinedServices, containerConfig)
				}
			}

			// Add volume configurations to the combined map
			if containerConfigs.Volumes != nil {
				for volumeName, volumeConfig := range containerConfigs.Volumes {
					combinedVolumes[volumeName] = volumeConfig
				}
			}

			// Add network configurations to the combined map
			if containerConfigs.Networks != nil {
				for networkName, networkConfig := range containerConfigs.Networks {
					combinedNetworks[networkName] = networkConfig
				}
			}
		}
	}

	// Define the network name based on the context
	networkName := fmt.Sprintf("windsor-%s", contextName)

	// Assign the CIDR to the network configuration if available
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

	// Create a Project using compose-go with the combined configurations
	project := &types.Project{
		Services: combinedServices,
		Volumes:  combinedVolumes,
		Networks: combinedNetworks,
	}

	return project, nil
}
