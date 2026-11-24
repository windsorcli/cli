// The DockerVirt is the ContainerRuntime for platform docker.
// It provides the cleanup of the containers, volumes, and network of the context.
// It works with or without a VM, because it talks only to the Docker daemon.
// Terraform or compose start the containers, so Up and WriteConfig do nothing.

package virt

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Constants
// =============================================================================

// WindsorNetworkPrefix is the prefix of the Docker network of a context, for example windsor-local.
const WindsorNetworkPrefix = "windsor-"

// DockerComposeProjectPrefix is the prefix of the com.docker.compose.project label of a context.
const DockerComposeProjectPrefix = "workstation-windsor-"

// =============================================================================
// Types
// =============================================================================

// DockerVirt is the ContainerRuntime for platform docker.
type DockerVirt struct {
	*BaseVirt
}

// =============================================================================
// Constructor
// =============================================================================

// NewDockerVirt returns a DockerVirt for rt. It panics when rt is nil.
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

// Up checks that the Docker daemon answers, because Terraform or compose start the containers.
// It returns one error that names the endpoint when the daemon does not answer.
func (v *DockerVirt) Up(ctx context.Context) error {
	if _, err := v.shell.ExecSilent("docker", "version", "--format", "{{.Server.Version}}"); err != nil {
		return fmt.Errorf("Docker isn't running%s. Start Docker and run windsor up again", v.daemonEndpointHint())
	}
	return nil
}

// WriteConfig does nothing.
func (v *DockerVirt) WriteConfig() error {
	return nil
}

// AnnotateApplyError adds the fix for a leftover network to err when the apply of workstation/docker failed.
// It adds the text only when err names that component and the network windsor-<context> exists.
// Terraform may own that network, so the text says what to do if a failed run left it, and removes nothing.
// It returns err unchanged in all other cases.
func (v *DockerVirt) AnnotateApplyError(err error) error {
	if err == nil || !strings.Contains(err.Error(), "terraform apply for workstation/docker") {
		return err
	}
	netName := WindsorNetworkPrefix + v.configHandler.GetContext()
	out, listErr := v.shell.ExecSilent("docker", "network", "ls", "--format", "{{.Name}}")
	if listErr != nil {
		return err
	}
	for _, name := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(name) == netName {
			return fmt.Errorf("%w\nThe Docker network %s exists. If a failed run left it behind, run windsor down, then run windsor up again", err, netName)
		}
	}
	return err
}

// Down removes the containers, the named volumes, and the network of the context, and nothing else.
// It finds the containers and volumes by the label com.docker.compose.project=workstation-windsor-<context>.
// It does nothing when DOCKER_HOST names a unix socket that does not exist, because the VM is gone.
// It shows each failure as a warning and always returns nil.
func (v *DockerVirt) Down(ctx context.Context) error {
	if v.daemonSocketMissing() {
		return nil
	}
	return presenter.Track(ctx, "runtime:docker", "Cleaning up Docker resources", func(ctx context.Context) error {
		contextName := v.configHandler.GetContext()
		projectLabelValue := DockerComposeProjectPrefix + contextName
		netName := WindsorNetworkPrefix + contextName

		v.cleanProjectContainers(ctx, projectLabelValue)
		v.removeVolumes(ctx, projectLabelValue)
		v.removeNetworkIfExists(ctx, netName)
		return nil
	})
}

// =============================================================================
// Private Methods
// =============================================================================

// daemonSocketMissing reports whether DOCKER_HOST names a unix socket that does not exist.
// Any other endpoint (unset, npipe, tcp, ssh) or any other Stat error returns false, so cleanup proceeds.
func (v *DockerVirt) daemonSocketMissing() bool {
	path, ok := strings.CutPrefix(v.shims.Getenv("DOCKER_HOST"), "unix://")
	if !ok || path == "" {
		return false
	}
	_, err := v.shims.Stat(path)
	return errors.Is(err, fs.ErrNotExist)
}

// daemonEndpointHint returns the Docker endpoint in parentheses with a leading space.
// It reads DOCKER_HOST first, then the endpoint of the current docker context.
// It returns an empty string when neither names an endpoint.
func (v *DockerVirt) daemonEndpointHint() string {
	host := strings.TrimSpace(v.shims.Getenv("DOCKER_HOST"))
	if host == "" {
		out, err := v.shell.ExecSilent("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}")
		if err == nil {
			host = strings.TrimSpace(out)
		}
	}
	if host == "" {
		return ""
	}
	return " (" + host + ")"
}

// cleanProjectContainers stops and removes each container with the compose project label, also a container that never started.
// It warns and returns when it cannot list the containers.
func (v *DockerVirt) cleanProjectContainers(ctx context.Context, projectLabelValue string) {
	out, err := v.shell.ExecSilent("docker", "ps", "-a", "-q", "--filter", "label=com.docker.compose.project="+projectLabelValue)
	if err != nil {
		presenter.Warn(ctx, fmt.Sprintf("could not list containers for project %s: %v", projectLabelValue, err))
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

// removeVolumes removes each Docker volume with the compose project label.
// It warns and returns when it cannot list the volumes.
func (v *DockerVirt) removeVolumes(ctx context.Context, projectLabelValue string) {
	out, err := v.shell.ExecSilent("docker", "volume", "ls", "-q", "--filter", "label=com.docker.compose.project="+projectLabelValue)
	if err != nil {
		presenter.Warn(ctx, fmt.Sprintf("could not list volumes for project %s: %v", projectLabelValue, err))
		return
	}
	for _, name := range strings.Fields(strings.TrimSpace(out)) {
		if name == "" {
			continue
		}
		_, _ = v.shell.ExecSilent("docker", "volume", "rm", name)
	}
}

// removeNetworkIfExists removes the Docker network netName when it exists.
// It warns and returns when it cannot list the networks.
func (v *DockerVirt) removeNetworkIfExists(ctx context.Context, netName string) {
	out, err := v.shell.ExecSilent("docker", "network", "ls", "--format", "{{.Name}}")
	if err != nil {
		presenter.Warn(ctx, fmt.Sprintf("could not list Docker networks: %v", err))
		return
	}
	for _, name := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(name) == netName {
			_, _ = v.shell.ExecSilent("docker", "network", "rm", netName)
			return
		}
	}
}

// =============================================================================
// Interface Compliance
// =============================================================================

// DockerVirt implements ContainerRuntime.
var _ ContainerRuntime = (*DockerVirt)(nil)
