// The DockerVirt is a container runtime implementation
// It provides Docker container management capabilities through the Docker Compose interface
// It serves as the primary container orchestration layer for the Windsor CLI
// It handles container lifecycle, configuration, and networking for Docker-based services

package virt

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/services"
)

// =============================================================================
// Types
// =============================================================================

// DockerVirt implements the ContainerInterface for Docker
type DockerVirt struct {
	BaseVirt
	services       []services.Service
	composeCommand string
	composeArgs    []string
}

// =============================================================================
// Constructor
// =============================================================================

// NewDockerVirt creates a new instance of DockerVirt using a DI injector
func NewDockerVirt(injector di.Injector) *DockerVirt {
	return &DockerVirt{
		BaseVirt:    *NewBaseVirt(injector),
		composeArgs: nil,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize resolves all dependencies for DockerVirt, including services from the DI
// container, Docker configuration status, and determines the appropriate docker compose
// command to use. It alphabetizes services and verifies Docker is enabled.
func (v *DockerVirt) Initialize() error {
	if err := v.BaseVirt.Initialize(); err != nil {
		return fmt.Errorf("error initializing base: %w", err)
	}

	resolvedServices, err := v.injector.ResolveAll((*services.Service)(nil))
	if err != nil {
		return fmt.Errorf("error resolving services: %w", err)
	}

	var serviceSlice []services.Service
	for _, service := range resolvedServices {
		if s, ok := service.(services.Service); ok && s != nil {
			serviceSlice = append(serviceSlice, s)
		}
	}

	sort.Slice(serviceSlice, func(i, j int) bool {
		return serviceSlice[i].GetName() < serviceSlice[j].GetName()
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

// Up starts docker compose in detached mode with retry logic for reliability. It
// verifies Docker is enabled, checks the daemon is running, sets the compose file
// path, and attempts to start services with up to 3 retries if initial attempts fail.
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

		if err := v.shims.Setenv("COMPOSE_FILE", composeFilePath); err != nil {
			return fmt.Errorf("failed to set COMPOSE_FILE environment variable: %w", err)
		}

		retries := 3
		var lastErr error
		var lastOutput string
		for i := range make([]struct{}, retries) {
			args := append(append([]string{}, v.composeArgs...), "up", "--detach", "--remove-orphans")
			message := "📦 Running docker compose up"

			if i == 0 {
				output, err := v.shell.ExecProgress(message, v.composeCommand, args...)
				if err == nil {
					return nil
				}
				lastErr = err
				lastOutput = output
			} else {
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
			return fmt.Errorf("Error executing command %s %v: %w\n%s", v.composeCommand, append(v.composeArgs, "up", "--detach", "--remove-orphans"), lastErr, lastOutput)
		}
	}
	return nil
}

// Down stops all Docker containers managed by Windsor and removes associated volumes
// to ensure a clean shutdown. It verifies Docker is enabled, checks the daemon is
// running, and executes docker compose down with the --remove-orphans and --volumes flags.
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

		if err := v.shims.Setenv("COMPOSE_FILE", composeFilePath); err != nil {
			return fmt.Errorf("error setting COMPOSE_FILE environment variable: %w", err)
		}

		args := append(append([]string{}, v.composeArgs...), "down", "--remove-orphans", "--volumes")
		output, err := v.shell.ExecProgress("📦 Running docker compose down", v.composeCommand, args...)
		if err != nil {
			return fmt.Errorf("Error executing command %s down: %w\n%s", v.composeCommand, err, output)
		}
	}
	return nil
}

// WriteConfig generates and writes the Docker Compose configuration file by combining
// settings from all services. It creates the necessary directory structure, retrieves
// the full compose configuration, serializes it to YAML, and writes it to the .windsor
// directory with appropriate permissions.
func (v *DockerVirt) WriteConfig() error {
	projectRoot, err := v.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error retrieving project root: %w", err)
	}
	composeFilePath := filepath.Join(projectRoot, ".windsor", "docker-compose.yaml")

	if err := v.shims.MkdirAll(filepath.Dir(composeFilePath), 0755); err != nil {
		return fmt.Errorf("error creating parent context folder: %w", err)
	}

	project, err := v.getFullComposeConfig()
	if err != nil {
		return fmt.Errorf("error getting full compose config: %w", err)
	}

	yamlData, err := v.shims.MarshalYAML(project)
	if err != nil {
		return fmt.Errorf("error marshaling docker compose config to YAML: %w", err)
	}

	err = v.shims.WriteFile(composeFilePath, yamlData, 0644)
	if err != nil {
		return fmt.Errorf("error writing docker compose file: %w", err)
	}

	return nil
}

// GetContainerInfo retrieves detailed information about Docker containers managed by
// Windsor, including their names, IP addresses, and labels. It filters containers
// by Windsor-managed labels and context, and optionally by service name if provided.
// For each container, it retrieves network settings to determine IP addresses.
func (v *DockerVirt) GetContainerInfo(name ...string) ([]ContainerInfo, error) {
	contextName := v.configHandler.GetContext()

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
		if err := v.shims.UnmarshalJSON([]byte(inspectOut), &labels); err != nil {
			return nil, fmt.Errorf("error unmarshaling container labels: %w", err)
		}

		serviceName, _ := labels["com.docker.compose.service"]

		networkInspectArgs := []string{"inspect", containerID, "--format", "{{json .NetworkSettings.Networks}}"}
		networkInspectOut, err := v.shell.ExecSilent(command, networkInspectArgs...)
		if err != nil {
			return nil, fmt.Errorf("error inspecting container networks: %w", err)
		}

		var networks map[string]struct {
			IPAddress string `json:"IPAddress"`
		}
		if err := v.shims.UnmarshalJSON([]byte(networkInspectOut), &networks); err != nil {
			return nil, fmt.Errorf("error unmarshaling container networks: %w", err)
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

		if len(name) > 0 {
			if slices.Contains(name, serviceName) {
				containerInfos = append(containerInfos, containerInfo)
			}
		} else {
			containerInfos = append(containerInfos, containerInfo)
		}
	}

	return containerInfos, nil
}

// PrintInfo displays a formatted table of running Docker containers with their names,
// IP addresses, and roles. It retrieves container information using GetContainerInfo
// and presents it in a tabular format for easy reading. If no containers are running,
// it displays an appropriate message.
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

// =============================================================================
// Private Methods
// =============================================================================

// determineComposeCommand checks for available docker compose commands in order of
// preference: docker-compose, docker-cli-plugin-docker-compose, and docker compose.
// It sets the first available command for later use in Docker operations.
func (v *DockerVirt) determineComposeCommand() error {
	type candidate struct {
		command string
		args    []string
	}

	commands := []candidate{
		{command: "docker-compose"},
		{command: "docker-cli-plugin-docker-compose"},
		{command: "docker", args: []string{"compose"}},
	}

	for _, c := range commands {
		args := append(append([]string{}, c.args...), "--version")
		if _, err := v.shell.ExecSilent(c.command, args...); err == nil {
			v.composeCommand = c.command
			v.composeArgs = c.args
			return nil
		}
	}
	return nil
}

// checkDockerDaemon verifies that the Docker daemon is running and accessible by
// executing the 'docker info' command. It returns an error if the daemon cannot
// be contacted, which is used by other functions to ensure Docker is available.
func (v *DockerVirt) checkDockerDaemon() error {
	command := "docker"
	args := []string{"info"}
	_, err := v.shell.ExecSilent(command, args...)
	return err
}

// getFullComposeConfig builds a complete Docker Compose configuration by combining
// settings from all services. It creates a network configuration with optional IPAM
// settings based on the network CIDR, collects service configurations with their
// network settings and IP addresses, and aggregates volumes and networks from all
// services into a single project configuration.
func (v *DockerVirt) getFullComposeConfig() (*types.Project, error) {
	contextName := v.configHandler.GetContext()

	if !v.configHandler.GetBool("docker.enabled") {
		return nil, fmt.Errorf("Docker configuration is not defined")
	}

	var combinedServices []types.ServiceConfig
	var combinedVolumes map[string]types.VolumeConfig
	var combinedNetworks map[string]types.NetworkConfig

	combinedVolumes = make(map[string]types.VolumeConfig)
	combinedNetworks = make(map[string]types.NetworkConfig)

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

	for _, service := range v.services {
		if serviceInstance, ok := service.(interface {
			GetComposeConfig() (*types.Config, error)
			GetAddress() string
		}); ok {
			containerConfigs, err := serviceInstance.GetComposeConfig()
			if err != nil {
				return nil, fmt.Errorf("error getting container config from service: %w", err)
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

					networkCIDR := v.configHandler.GetString("network.cidr_block")
					if networkCIDR != "" && ipAddress != "" {
						containerConfig.Networks[networkName].Ipv4Address = ipAddress
					}

					combinedServices = append(combinedServices, containerConfig)
				}
			}

			if containerConfigs.Volumes != nil {
				maps.Copy(combinedVolumes, containerConfigs.Volumes)
			}

			if containerConfigs.Networks != nil {
				maps.Copy(combinedNetworks, containerConfigs.Networks)
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
