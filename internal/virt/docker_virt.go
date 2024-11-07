package virt

import (
	"fmt"
	"sort"
	"strings"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
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

// Up starts the Docker container
func (v *DockerVirt) Up(verbose ...bool) error {
	// Placeholder implementation
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
	// Placeholder implementation
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
