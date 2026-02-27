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

// Down stops and removes containers on the current context's Windsor network (windsor-<context>),
// including their anonymous volumes via rm -v, then removes that network. Best-effort: errors
// are logged to stderr but do not cause Down to return an error. Shows a progress spinner with broom emoji.
func (v *DockerVirt) Down() error {
	return v.withProgress("🧹 Cleaning residual Docker containers and networks", func() error {
		contextName := v.configHandler.GetContext()
		projectName := WindsorNetworkPrefix + contextName

		out, err := v.shell.ExecSilent("docker", "network", "ls", "--format", "{{.Name}}")
		if err != nil {
			fmt.Fprintln(os.Stderr, "Warning: could not list Docker networks:", err)
			return nil
		}
		names := strings.Split(strings.TrimSpace(out), "\n")
		var toClean []string
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if name == projectName {
				toClean = append(toClean, name)
			}
		}
		for _, netName := range toClean {
			v.cleanNetwork(netName)
		}
		return nil
	})
}

// =============================================================================
// Private Methods
// =============================================================================

func (v *DockerVirt) cleanNetwork(netName string) {
	out, err := v.shell.ExecSilent("docker", "network", "inspect", netName, "-f", "{{range .Containers}}{{.Name}} {{end}}")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not inspect network %s: %v\n", netName, err)
		return
	}
	parts := strings.Fields(strings.TrimSpace(out))
	for _, raw := range parts {
		name := strings.TrimPrefix(raw, "/")
		if name == "" {
			continue
		}
		_, _ = v.shell.ExecSilent("docker", "stop", name)
		_, _ = v.shell.ExecSilent("docker", "rm", "-f", "-v", name)
	}
	_, _ = v.shell.ExecSilent("docker", "network", "rm", netName)
}

var _ ContainerRuntime = (*DockerVirt)(nil)
