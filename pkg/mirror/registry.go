package mirror

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// The Registry manages the lifecycle of a local distribution/distribution
// container used as the air-gapped OCI mirror target.
// It provides idempotent start, readiness probing, and stop operations against
// the Docker daemon via the shell shim, so callers can ensure a running registry
// without tracking container state themselves.
// The registry storage backend is bind-mounted from a host cache directory so
// mirrored content persists across invocations.

// =============================================================================
// Constants
// =============================================================================

const (
	defaultRegistryImage     = "ghcr.io/distribution/distribution:3"
	defaultRegistryContainer = "windsor-mirror"
	defaultRegistryPort      = 5000
	registryReadyTimeout     = 30 * time.Second
	registryReadyInterval    = 250 * time.Millisecond
)

// =============================================================================
// Types
// =============================================================================

// Registry encapsulates the distribution container lifecycle.
type Registry struct {
	shell         shell.Shell
	shims         *Shims
	image         string
	containerName string
	hostPort      int
	cacheDir      string
	context       string
}

// =============================================================================
// Constructor
// =============================================================================

// NewRegistry returns a Registry configured with sensible defaults and the
// provided shell, shims, host cache directory, port, and windsor context.
// cacheDir is bind-mounted into the container at /var/lib/registry so pushed
// blobs persist on disk. The windsor context is recorded as a container
// label for discoverability and ownership signalling. If hostPort is 0 the
// package default (5000) is used.
func NewRegistry(sh shell.Shell, shims *Shims, cacheDir string, hostPort int, context string) *Registry {
	if hostPort == 0 {
		hostPort = defaultRegistryPort
	}
	return &Registry{
		shell:         sh,
		shims:         shims,
		image:         defaultRegistryImage,
		containerName: defaultRegistryContainer,
		hostPort:      hostPort,
		cacheDir:      cacheDir,
		context:       context,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Endpoint returns the base URL at which the local registry is reachable.
func (r *Registry) Endpoint() string {
	return fmt.Sprintf("http://localhost:%d", r.hostPort)
}

// EnsureRunning is the idempotent lifecycle entrypoint. It inspects the Docker
// daemon for a container matching the registry's name. A running container
// bound to the requested host port is reused; a running container on a
// different port, a stopped container, or no container at all triggers a
// recreate. In all cases the method blocks until the registry's /v2/
// endpoint responds with a successful status code.
func (r *Registry) EnsureRunning() error {
	state, err := r.containerState()
	if err != nil {
		return err
	}
	switch state {
	case "running":
		if r.portMatches() {
			return r.waitReady()
		}
		if err := r.removeContainer(); err != nil {
			return err
		}
		if err := r.runContainer(); err != nil {
			return err
		}
	case "exited", "created", "paused":
		if err := r.removeContainer(); err != nil {
			return err
		}
		if err := r.runContainer(); err != nil {
			return err
		}
	case "":
		if err := r.runContainer(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("registry container %s is in unexpected state %q", r.containerName, state)
	}
	return r.waitReady()
}

// =============================================================================
// Private Methods
// =============================================================================

// containerState queries Docker for the current state of the registry
// container. It returns an empty string when no container with the configured
// name exists.
func (r *Registry) containerState() (string, error) {
	out, err := r.shell.ExecSilent("docker", "inspect", "-f", "{{.State.Status}}", r.containerName)
	if err != nil {
		if strings.Contains(err.Error(), "No such object") || strings.Contains(out, "No such object") {
			return "", nil
		}
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

// portMatches reports whether the existing container's published host port
// matches the Registry's configured port. Used to decide whether a running
// container can be reused or must be recreated after a --port change.
func (r *Registry) portMatches() bool {
	out, err := r.shell.ExecSilent("docker", "inspect", "-f", "{{range $p, $c := .NetworkSettings.Ports}}{{range $c}}{{.HostPort}} {{end}}{{end}}", r.containerName)
	if err != nil {
		return false
	}
	want := fmt.Sprintf("%d", r.hostPort)
	for _, p := range strings.Fields(out) {
		if p == want {
			return true
		}
	}
	return false
}

// removeContainer force-removes the registry container so a replacement can
// be created with updated port or volume bindings. Missing containers are
// treated as success.
func (r *Registry) removeContainer() error {
	if _, err := r.shell.ExecSilent("docker", "rm", "-f", r.containerName); err != nil {
		if strings.Contains(err.Error(), "No such container") {
			return nil
		}
		return fmt.Errorf("failed to remove existing registry container: %w", err)
	}
	return nil
}

// runContainer launches a new distribution container in the background with
// the host cache directory bind-mounted for persistence and the configured
// host port forwarded to container port 5000.
func (r *Registry) runContainer() error {
	args := []string{
		"run", "-d",
		"--name", r.containerName,
		"--restart", "unless-stopped",
		"-p", fmt.Sprintf("%d:5000", r.hostPort),
		"-v", fmt.Sprintf("%s:/var/lib/registry", r.cacheDir),
		"--label", "managed_by=windsor",
		"--label", "role=mirror",
	}
	if r.context != "" {
		args = append(args, "--label", "context="+r.context)
	}
	args = append(args, r.image)
	if out, err := r.shell.ExecSilent("docker", args...); err != nil {
		if strings.Contains(out, "address already in use") || strings.Contains(err.Error(), "address already in use") {
			return fmt.Errorf("host port %d is already in use — rerun with `windsor mirror --port <other>`; a common culprit on macOS is AirPlay Receiver (System Settings → AirDrop & Handoff)", r.hostPort)
		}
		return fmt.Errorf("failed to run registry container: %w", err)
	}
	return nil
}

// waitReady polls the registry's /v2/ endpoint until it returns a successful
// status code (200 or 401 — the spec allows either before auth) or the
// readiness timeout elapses.
func (r *Registry) waitReady() error {
	deadline := r.shims.Now().Add(registryReadyTimeout)
	url := r.Endpoint() + "/v2/"
	var lastErr error
	for r.shims.Now().Before(deadline) {
		resp, err := r.shims.HttpGet(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
				return nil
			}
			lastErr = fmt.Errorf("registry returned status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		r.shims.Sleep(registryReadyInterval)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("timeout")
	}
	return fmt.Errorf("registry at %s did not become ready: %w", url, lastErr)
}
