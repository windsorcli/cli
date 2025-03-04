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

// Initialize sets up DockerVirt by resolving services, sorting them, checking Docker config,
// and determining the compose command.
func (v *DockerVirt) Initialize() error {
	if err := v.BaseVirt.Initialize(); err != nil {
		return fmt.Errorf("error initializing base: %w", err)
	}

	resolvedServices, err := v.injector.ResolveAll((*services.Service)(nil))
	if err != nil {
		return fmt.Errorf("error resolving services: %w", err)
	}

	serviceSlice := make([]services.Service, len(resolvedServices))
	for i, service := range resolvedServices {
		if s, _ := service.(services.Service); s != nil {
			serviceSlice[i] = s
		}
	}

	sort.Slice(serviceSlice, func(i, j int) bool {
		return fmt.Sprintf("%T", serviceSlice[i]) < fmt.Sprintf("%T", serviceSlice[j])
	})

	if !v.configHandler.GetBool("docker.enabled") {
		return fmt.Errorf("Docker configuration is not defined")
	}

	v.services = serviceSlice

	if err := v.determineComposeCommand(); err != nil {
		return fmt.Errorf("error determining docker compose command: %w", err)
	}

	return nil
}

// determineComposeCommand checks for available docker compose commands. If a docker-compose
// command is not available, none is set.
func (v *DockerVirt) determineComposeCommand() error {
	commands := []string{"docker-compose", "docker-cli-plugin-docker-compose", "docker compose"}
	for _, cmd := range commands {
		if _, _, err := v.shell.ExecSilent(cmd, "--version"); err == nil {
			v.composeCommand = cmd
			return nil
		}
	}
	return nil
}

// Up initializes and starts Docker Compose in detached mode. It first checks if Docker is enabled
// and ensures the Docker daemon is running. It sets the COMPOSE_FILE environment variable to the
// path of the docker-compose.yaml file. The function attempts to run "docker compose up" with
// retries, using progress display for the first attempt and silent execution for subsequent ones.
func (v *DockerVirt) Up() error {
	if v.configHandler.GetBool("docker.enabled") {
		if err := v.checkDockerDaemon(); err != nil {
			return fmt.Errorf("Docker daemon is not running: %w", err)
		}

		projectRoot, err := v.shell.GetProjectRoot()
		if err != nil {
			return fmt.Errorf("error retrieving project root: %w", err)
		}
		composeFilePath := filepath.Join(projectRoot, ".windsor", "docker-compose.yaml")

		if err := osSetenv("COMPOSE_FILE", composeFilePath); err != nil {
			return fmt.Errorf("failed to set COMPOSE_FILE environment variable: %w", err)
		}

		retries := 3
		var lastErr error
		var lastOutput string
		for i := 0; i < retries; i++ {
			args := []string{"up", "--detach", "--remove-orphans"}
			message := "📦 Running docker compose up"

			if i == 0 {
				output, _, err := v.shell.ExecProgress(message, v.composeCommand, args...)
				if err == nil {
					return nil
				}
				lastErr = err
				lastOutput = output
			} else {
				output, _, err := v.shell.ExecSilent(v.composeCommand, args...)
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

// Down stops Docker containers if enabled, ensuring the daemon is running, and executes "docker compose down".
func (v *DockerVirt) Down() error {
	if v.configHandler.GetBool("docker.enabled") {
		if err := v.checkDockerDaemon(); err != nil {
			return fmt.Errorf("Docker daemon is not running: %w", err)
		}

		projectRoot, err := v.shell.GetProjectRoot()
		if err != nil {
			return fmt.Errorf("error retrieving project root: %w", err)
		}
		composeFilePath := filepath.Join(projectRoot, ".windsor", "docker-compose.yaml")

		if err := osSetenv("COMPOSE_FILE", composeFilePath); err != nil {
			return fmt.Errorf("error setting COMPOSE_FILE environment variable: %w", err)
		}

		output, _, err := v.shell.ExecProgress("📦 Running docker compose down", v.composeCommand, "down", "--remove-orphans", "--volumes")
		if err != nil {
			return fmt.Errorf("Error executing command %s down: %w\n%s", v.composeCommand, err, output)
		}
	}
	return nil
}

// WriteConfig generates and writes the Docker compose YAML file.
func (v *DockerVirt) WriteConfig() error {
	projectRoot, err := v.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error retrieving project root: %w", err)
	}
	composeFilePath := filepath.Join(projectRoot, ".windsor", "docker-compose.yaml")

	if err := mkdirAll(filepath.Dir(composeFilePath), 0755); err != nil {
		return fmt.Errorf("error creating parent context folder: %w", err)
	}

	project, err := v.getFullComposeConfig()
	if err != nil {
		return fmt.Errorf("error getting full compose config: %w", err)
	}

	yamlData, err := yamlMarshal(project)
	if err != nil {
		return fmt.Errorf("error marshaling docker compose config to YAML: %w", err)
	}

	err = writeFile(composeFilePath, yamlData, 0644)
	if err != nil {
		return fmt.Errorf("error writing docker compose file: %w", err)
	}

	return nil
}

// GetContainerInfo retrieves information about Docker containers managed by Windsor, filtered by context and optionally by service name.
// It returns a list of ContainerInfo, which includes the container's name, IP address, and labels.
func (v *DockerVirt) GetContainerInfo(name ...string) ([]ContainerInfo, error) {
	contextName := v.configHandler.GetContext()

	command := "docker"
	args := []string{"ps", "--filter", "label=managed_by=windsor", "--filter", fmt.Sprintf("label=context=%s", contextName), "--format", "{{.ID}}"}
	out, _, err := v.shell.ExecSilent(command, args...)
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
		inspectOut, _, err := v.shell.ExecSilent(command, inspectArgs...)
		if err != nil {
			return nil, err
		}

		var labels map[string]string
		if err := jsonUnmarshal([]byte(inspectOut), &labels); err != nil {
			return nil, err
		}

		serviceName, _ := labels["com.docker.compose.service"]

		if len(name) > 0 && serviceName != name[0] {
			continue
		}

		networkInspectArgs := []string{"inspect", containerID, "--format", "{{json .NetworkSettings.Networks}}"}
		networkInspectOut, _, err := v.shell.ExecSilent(command, networkInspectArgs...)
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
	_, _, err := v.shell.ExecSilent(command, args...)
	return err
}

// getFullComposeConfig builds a Docker Compose configuration for DockerVirt. It retrieves the
// context name and configuration, checks if Docker is defined, and returns nil if not. It sets up
// combined configurations for services, volumes, and networks, defining a network with IPAM if a
// NetworkCIDR is specified. It iterates over services, gathering their configurations and IPs,
// and returns a Project with these combined settings.
func (v *DockerVirt) getFullComposeConfig() (*types.Project, error) {
	contextName := v.configHandler.GetContext()

	if v.configHandler.GetBool("docker.enabled") == false {
		return nil, nil
	}

	var combinedServices []types.ServiceConfig
	var combinedVolumes map[string]types.VolumeConfig
	var combinedNetworks map[string]types.NetworkConfig

	combinedVolumes = make(map[string]types.VolumeConfig)
	combinedNetworks = make(map[string]types.NetworkConfig)

	// Configure the network
	networkName := fmt.Sprintf("windsor-%s", contextName)

	networkConfig := types.NetworkConfig{
		Driver: "bridge",
	}

	networkCIDR := v.configHandler.GetString("network.cidr_block")
	if networkCIDR != "" {
		networkConfig.Ipam = types.IPAMConfig{
			Driver: "default",
			Config: []*types.IPAMPool{
				{
					Subnet: networkCIDR,
				},
			},
		}
	}

	combinedNetworks[networkName] = networkConfig

	// Iterate over each service and collect container configs
	for _, service := range v.services {
		if serviceInstance, ok := service.(interface {
			GetComposeConfig() (*types.Config, error)
			GetAddress() string
		}); ok {
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
					ipAddress := serviceInstance.GetAddress()

					containerConfig.Networks = map[string]*types.ServiceNetworkConfig{
						networkName: {},
					}

					if networkCIDR != "" && ipAddress != "" {
						containerConfig.Networks[networkName].Ipv4Address = ipAddress
					}

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

	project := &types.Project{
		Services: combinedServices,
		Volumes:  combinedVolumes,
		Networks: combinedNetworks,
	}

	return project, nil
}
