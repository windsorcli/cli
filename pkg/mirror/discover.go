package mirror

import (
	"fmt"
	"strings"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// The discover unit locates an existing OCI registry container provisioned by
// the workstation terraform module for the current context, so `windsor mirror`
// can push to it by default instead of starting its own throwaway registry.
// Containers are identified by the Windsor-standard `role` and `context`
// labels; the first match is returned as a `host:port` endpoint suitable for
// Options.Target.

// =============================================================================
// Constants
// =============================================================================

// mirrorRoleLabel is the `role` label value the workstation terraform module
// sets on the container dedicated to `windsor mirror` targets. Registries
// provisioned for cluster pull-through proxying use `role=registry` and are
// intentionally excluded; a dedicated role keeps push targets unambiguous.
const mirrorRoleLabel = "mirror"

// registryInternalPort is the port distribution/distribution listens on
// inside its container. Host-side port mappings and bridge-IP probes both
// resolve to this value.
const registryInternalPort = "5000"

// =============================================================================
// Public Methods
// =============================================================================

// DiscoverTarget queries the local Docker daemon for a container labelled
// `role=mirror` and `context=<context>` and returns a pushable
// `host:port` endpoint. When no matching container is running the empty
// string is returned with a nil error so callers can fall back to the
// self-hosted local-registry path without having to special-case the
// absence of workstation tooling.
func DiscoverTarget(sh shell.Shell, context string) (string, error) {
	if sh == nil || strings.TrimSpace(context) == "" {
		return "", nil
	}

	out, err := sh.ExecSilent("docker", "ps",
		"--filter", "label=role="+mirrorRoleLabel,
		"--filter", "label=context="+context,
		"--format", "{{.ID}}",
	)
	if err != nil {
		return "", nil
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return "", nil
	}
	id := fields[0]

	if port, err := sh.ExecSilent("docker", "port", id, registryInternalPort+"/tcp"); err == nil {
		if ep := firstPublishedEndpoint(port); ep != "" {
			return ep, nil
		}
	}

	ip, err := sh.ExecSilent("docker", "inspect",
		"--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}",
		id,
	)
	if err != nil {
		return "", fmt.Errorf("inspect mirror container %s: %w", id, err)
	}
	for _, addr := range strings.Fields(ip) {
		if addr != "" {
			return addr + ":" + registryInternalPort, nil
		}
	}
	return "", nil
}

// =============================================================================
// Helpers
// =============================================================================

// firstPublishedEndpoint parses the `docker port` output (one mapping per
// line, e.g. `0.0.0.0:55000`) and returns the first usable host endpoint,
// rewriting a `0.0.0.0` bind to `localhost` so clients on the host work
// without manual translation. Empty input yields an empty result.
func firstPublishedEndpoint(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.LastIndex(line, ":")
		if idx < 0 {
			continue
		}
		host := line[:idx]
		port := line[idx+1:]
		if host == "0.0.0.0" || host == "" || host == "::" {
			host = "localhost"
		}
		return host + ":" + port
	}
	return ""
}
