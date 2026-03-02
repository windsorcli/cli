// The DockerVirt is a container runtime implementation for platform docker.
// It provides Docker-native residual container/network cleanup and works with or without
// a VM (e.g. Colima); it only talks to the Docker daemon.

package virt

import (
	"fmt"
	"os"
	"strings"

	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Constants
// =============================================================================

// WindsorNetworkPrefix is the prefix for Windsor-managed Docker networks (e.g. windsor-local).
// Down() targets only the current context's network, windsor-<context>.
const WindsorNetworkPrefix = "windsor-"

// DockerComposeProjectPrefix is the prefix for the compose-style project label value (e.g. workstation-windsor-local).
// Containers are grouped by label com.docker.compose.project=<prefix><context> for display and cleanup.
const DockerComposeProjectPrefix = "workstation-windsor-"

// =============================================================================
// Types
// =============================================================================

// DockerVirt implements ContainerRuntime for platform docker.
// It does not start or manage a VM; Up/WriteConfig are no-ops when Terraform or compose elsewhere own the stack.
// Down() performs robust cleanup of residual containers and networks so windsor down clears local resources
// even if Terraform destroy was skipped.
type DockerVirt struct {
	*BaseVirt
}

// =============================================================================
// Constructor
// =============================================================================

// NewDockerVirt creates a new DockerVirt with the provided runtime.
func NewDockerVirt(rt *runtime.Runtime) *DockerVirt {
	if rt == nil {
		panic("runtime is required")
	}
	return &DockerVirt{
		BaseVirt: NewBaseVirt(rt),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Up is a no-op for DockerVirt; containers are started by Terraform or compose elsewhere.
func (v *DockerVirt) Up() error {
	return nil
}

// WriteConfig is a no-op for DockerVirt.
func (v *DockerVirt) WriteConfig() error {
	return nil
}

// Down stops and removes only resources for the current project/context: containers and named volumes
// with label com.docker.compose.project=workstation-windsor-<context>, and the network windsor-<context>.
// Anonymous volumes are removed with containers via rm -v. No global Docker cleanup is performed.
// Best-effort: errors are logged to stderr but do not cause Down to return an error. Shows a progress spinner with broom emoji.
func (v *DockerVirt) Down() error {
	return v.withProgress("🧹 Cleaning residual Docker containers and networks", func() error {
		contextName := v.configHandler.GetContext()
		projectLabelValue := DockerComposeProjectPrefix + contextName
		netName := WindsorNetworkPrefix + contextName

		v.cleanProjectContainers(projectLabelValue)
		v.removeVolumes(projectLabelValue)
		v.removeNetworkIfExists(netName)
		return nil
	})
}

// =============================================================================
// Private Methods
// =============================================================================

// cleanProjectContainers stops and removes only containers with the given compose project label
// (com.docker.compose.project=<projectLabelValue>), including created-but-never-started. No other containers are touched.
func (v *DockerVirt) cleanProjectContainers(projectLabelValue string) {
	out, err := v.shell.ExecSilent("docker", "ps", "-a", "-q", "--filter", "label=com.docker.compose.project="+projectLabelValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not list containers for project %s: %v\n", projectLabelValue, err)
		return
	}
	ids := strings.Fields(strings.TrimSpace(out))
	for _, id := range ids {
		if id == "" {
			continue
		}
		_, _ = v.shell.ExecSilent("docker", "stop", id)
		_, _ = v.shell.ExecSilent("docker", "rm", "-f", "-v", id)
	}
}

// removeVolumes removes only Docker volumes with the given compose project label
// (com.docker.compose.project=<projectLabelValue>). Best-effort; errors are logged.
func (v *DockerVirt) removeVolumes(projectLabelValue string) {
	out, err := v.shell.ExecSilent("docker", "volume", "ls", "-q", "--filter", "label=com.docker.compose.project="+projectLabelValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not list volumes for project %s: %v\n", projectLabelValue, err)
		return
	}
	for _, name := range strings.FieldsFunc(out, func(r rune) bool { return r == ' ' || r == '\n' || r == '\t' }) {
		if name == "" {
			continue
		}
		_, _ = v.shell.ExecSilent("docker", "volume", "rm", name)
	}
}

// removeNetworkIfExists removes only the Docker network with the given name (e.g. windsor-<context>).
// Best-effort; errors are logged. No other networks are touched.
func (v *DockerVirt) removeNetworkIfExists(netName string) {
	out, err := v.shell.ExecSilent("docker", "network", "ls", "--format", "{{.Name}}")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not list Docker networks: %v\n", err)
		return
	}
	for _, name := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(name) == netName {
			_, _ = v.shell.ExecSilent("docker", "network", "rm", netName)
			return
		}
	}
}

var _ ContainerRuntime = (*DockerVirt)(nil)
