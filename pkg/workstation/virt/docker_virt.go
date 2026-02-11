// The DockerVirt is a container runtime implementation
// It provides Docker container management capabilities through the Docker Compose interface
// It serves as the primary container orchestration layer for the Windsor CLI
// It handles container lifecycle, configuration, and networking for Docker-based services

package virt

import (
	"fmt"
	"maps"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/workstation/services"
)

// =============================================================================
// Types
// =============================================================================

// DockerVirt implements the ContainerInterface for Docker
type DockerVirt struct {
	BaseVirt
	services       []services.Service
	composeCommand string
}

// =============================================================================
// Constructor
// =============================================================================

// NewDockerVirt creates a new instance of DockerVirt
func NewDockerVirt(rt *runtime.Runtime, serviceList []services.Service) *DockerVirt {
	if rt == nil {
		panic("runtime is required")
	}

	var serviceSlice []services.Service
	if serviceList != nil {
		// Filter out nil services and copy non-nil ones
		for _, service := range serviceList {
			if service != nil {
				serviceSlice = append(serviceSlice, service)
			}
		}
		sort.Slice(serviceSlice, func(i, j int) bool {
			return serviceSlice[i].GetName() < serviceSlice[j].GetName()
		})
	}

	return &DockerVirt{
		BaseVirt: *NewBaseVirt(rt),
		services: serviceSlice,
	}
}

// Up starts Docker Compose in detached mode with retry logic. It checks if Docker is enabled,
// verifies the Docker daemon is running, regenerates the Docker Compose configuration when running
// in a Colima VM to ensure network and driver options are compatible with Colima's requirements,
// sets the COMPOSE_FILE environment variable, and attempts to start services with up to 3 retries.
// Returns an error if all attempts fail or if prerequisites are not met.
func (v *DockerVirt) Up() error {
	if v.configHandler.GetBool("docker.enabled") {
		if err := v.checkDockerDaemon(); err != nil {
			return fmt.Errorf("Docker daemon is not running: %w", err)
		}

		if err := v.determineComposeCommand(); err != nil {
			return fmt.Errorf("failed to determine compose command: %w", err)
		}

		projectRoot := v.projectRoot
		composeFilePath := filepath.Join(projectRoot, ".windsor", "docker-compose.yaml")

		if err := v.shims.Setenv("COMPOSE_FILE", composeFilePath); err != nil {
			return fmt.Errorf("failed to set COMPOSE_FILE environment variable: %w", err)
		}

		retries := 3
		var lastErr error
		var lastOutput string
		for i := range make([]struct{}, retries) {
			args := []string{"up", "--detach", "--remove-orphans"}
			message := "📦 Running docker compose up"

			if i == 0 {
				output, err := v.execComposeCommand(message, args...)
				if err == nil {
					return nil
				}
				lastErr = err
				lastOutput = output
			} else {
				output, err := v.execComposeCommandSilent(args...)
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

// Down stops all Docker containers managed by Windsor and removes associated volumes.
// When docker.enabled is true, runs compose down when .windsor/docker-compose.yaml exists.
// Always removes any remaining workstation resources by identity: containers attached to
// the windsor-<context> network and the network itself, so cleanup runs when provider is
// docker but docker.enabled is false (e.g. Terraform-owned workstation). Idempotent when
// daemon is unavailable or network does not exist.
func (v *DockerVirt) Down() error {
	if err := v.checkDockerDaemon(); err != nil {
		return nil
	}
	if v.configHandler.GetBool("docker.enabled") {
		if err := v.determineComposeCommand(); err != nil {
			return fmt.Errorf("failed to determine compose command: %w", err)
		}
		projectRoot := v.projectRoot
		composeFilePath := filepath.Join(projectRoot, ".windsor", "docker-compose.yaml")
		if _, err := v.shims.Stat(composeFilePath); err == nil {
			if err := v.shims.Setenv("COMPOSE_FILE", composeFilePath); err != nil {
				return fmt.Errorf("error setting COMPOSE_FILE environment variable: %w", err)
			}
			output, err := v.execComposeCommand("📦 Running docker compose down", "down", "--remove-orphans", "--volumes")
			if err != nil {
				return fmt.Errorf("Error executing command %s down: %w\n%s", v.composeCommand, err, output)
			}
		}
	}
	if err := v.withProgress("📦 Tearing down Docker workstation", v.removeResources); err != nil {
		return err
	}
	return nil
}

// WriteConfig generates and writes the Docker Compose configuration file by combining
// settings from all services. It creates the necessary directory structure, retrieves
// the full compose configuration, serializes it to YAML, and writes it to the .windsor
// directory with appropriate permissions.
func (v *DockerVirt) WriteConfig() error {
	projectRoot := v.projectRoot
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

// =============================================================================
// Private Methods
// =============================================================================

// determineComposeCommand checks for available docker compose commands in order of
// preference: docker-compose, docker-cli-plugin-docker-compose, and docker compose.
// It sets the first available command for later use in Docker operations.
func (v *DockerVirt) determineComposeCommand() error {
	commands := []string{"docker-compose", "docker-cli-plugin-docker-compose", "docker compose"}
	for _, cmd := range commands {
		cmdParts := strings.Fields(cmd)
		if len(cmdParts) == 0 {
			continue
		}
		command := cmdParts[0]
		args := append(cmdParts[1:], "--version")
		if _, err := v.shell.ExecSilent(command, args...); err == nil {
			v.composeCommand = cmd
			return nil
		}
	}
	return nil
}

// execComposeCommand executes the compose command with progress indicator, handling
// commands that may contain spaces (e.g., "docker compose").
func (v *DockerVirt) execComposeCommand(message string, args ...string) (string, error) {
	cmdParts := strings.Fields(v.composeCommand)
	if len(cmdParts) == 0 {
		return "", fmt.Errorf("compose command is empty")
	}
	command := cmdParts[0]
	allArgs := append(cmdParts[1:], args...)
	return v.shell.ExecProgress(message, command, allArgs...)
}

// execComposeCommandSilent executes the compose command silently, handling
// commands that may contain spaces (e.g., "docker compose").
func (v *DockerVirt) execComposeCommandSilent(args ...string) (string, error) {
	cmdParts := strings.Fields(v.composeCommand)
	if len(cmdParts) == 0 {
		return "", fmt.Errorf("compose command is empty")
	}
	command := cmdParts[0]
	allArgs := append(cmdParts[1:], args...)
	return v.shell.ExecSilent(command, allArgs...)
}

// checkDockerDaemon verifies that the Docker daemon is running and accessible by
// executing 'docker info --format json'. The command outputs JSON even on connection errors,
// so we parse the JSON and check for ServerErrors to determine if the daemon is accessible.
// If JSON parsing fails (docker command failed, not installed, etc.), returns an error.
// If JSON parsing succeeds, only checks ServerErrors and ignores command errors.
// Returns an error if the daemon cannot be contacted. The error is simplified to avoid
// printing verbose Docker error messages.
func (v *DockerVirt) checkDockerDaemon() error {
	command := "docker"
	args := []string{"info", "--format", "json"}
	output, _ := v.shell.ExecSilent(command, args...)

	var dockerInfo struct {
		ServerErrors []string `json:"ServerErrors"`
	}
	if err := v.shims.UnmarshalJSON([]byte(output), &dockerInfo); err != nil {
		return fmt.Errorf("docker daemon not accessible")
	}

	if len(dockerInfo.ServerErrors) > 0 {
		return fmt.Errorf("docker daemon not accessible")
	}

	return nil
}

// removeResources removes all containers attached to the windsor-<context> network
// (including their anonymous volumes), then the network, then named volumes with
// label com.docker.compose.project=windsor-<context>. No-op if the network does not exist.
func (v *DockerVirt) removeResources() error {
	contextName := v.configHandler.GetContext()
	networkName := fmt.Sprintf("windsor-%s", contextName)
	projectLabel := fmt.Sprintf("com.docker.compose.project=windsor-%s", contextName)

	out, err := v.shell.ExecSilent("docker", "network", "inspect", networkName,
		"--format", "{{range $k, $v := .Containers}}{{$k}}{{\"\\n\"}}{{end}}")
	if err != nil {
		return nil
	}
	for _, id := range strings.FieldsFunc(out, func(r rune) bool { return r == '\n' }) {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		_, _ = v.shell.ExecSilent("docker", "stop", id)
		_, _ = v.shell.ExecSilent("docker", "rm", "-f", "-v", id)
	}
	_, _ = v.shell.ExecSilent("docker", "network", "rm", networkName)

	volOut, err := v.shell.ExecSilent("docker", "volume", "ls", "-q", "--filter", "label="+projectLabel)
	if err != nil {
		return nil
	}
	for _, name := range strings.FieldsFunc(volOut, func(r rune) bool { return r == '\n' }) {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		_, _ = v.shell.ExecSilent("docker", "volume", "rm", name)
	}
	return nil
}

// getFullComposeConfig assembles a Docker Compose project configuration for the current Windsor context.
// Aggregates service, volume, and network definitions from all registered services. Applies Windsor-specific
// network settings, including optional IPAM configuration based on the context's network CIDR. Ensures
// compatibility with Docker Engine v28+ by setting the bridge gateway mode to nat-unprotected when supported.
// Returns the constructed types.Project or an error if service configuration retrieval fails.
func (v *DockerVirt) getFullComposeConfig() (*types.Project, error) {
	contextName := v.configHandler.GetContext()

	if !v.configHandler.GetBool("docker.enabled") {
		return nil, fmt.Errorf("Docker configuration is not defined")
	}

	var combinedServices types.Services
	var combinedVolumes map[string]types.VolumeConfig
	var combinedNetworks map[string]types.NetworkConfig

	combinedServices = make(types.Services)
	combinedVolumes = make(map[string]types.VolumeConfig)
	combinedNetworks = make(map[string]types.NetworkConfig)

	networkName := fmt.Sprintf("windsor-%s", contextName)

	networkConfig := types.NetworkConfig{
		Driver: "bridge",
	}

	if v.configHandler.GetString("vm.driver") == "colima" && v.supportsDockerEngineV28Plus() {
		networkConfig.DriverOpts = map[string]string{
			"com.docker.network.bridge.gateway_mode_ipv4": "nat-unprotected",
		}
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
				for serviceName, containerConfig := range containerConfigs.Services {
					ipAddress := serviceInstance.GetAddress()

					containerConfig.Networks = map[string]*types.ServiceNetworkConfig{
						networkName: {},
					}

					networkCIDR := v.configHandler.GetString("network.cidr_block")
					if networkCIDR != "" && ipAddress != "" {
						containerConfig.Networks[networkName].Ipv4Address = ipAddress
					}

					combinedServices[serviceName] = containerConfig
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

// supportsDockerEngineV28Plus returns true if the Docker Engine major version is 28 or higher.
// It executes 'docker version' to retrieve the server version, parses the major version component,
// and determines compatibility with features introduced in Docker Engine v28, such as nat-unprotected gateway mode.
// Returns false if the version cannot be determined or is less than 28.
func (v *DockerVirt) supportsDockerEngineV28Plus() bool {
	output, err := v.shell.ExecSilent("docker", "version", "--format", "{{.Server.Version}}")
	if err != nil {
		return false
	}
	versionStr := strings.TrimSpace(output)
	if versionStr == "" {
		return false
	}
	parts := strings.Split(versionStr, ".")
	if len(parts) < 2 {
		return false
	}
	majorVersion := parts[0]
	return majorVersion >= "28"
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure DockerVirt implements ContainerRuntime
var _ ContainerRuntime = (*DockerVirt)(nil)
