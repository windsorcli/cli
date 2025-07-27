package constants

import "time"

// Default git livereload settings
const (
	// renovate: datasource=docker depName=ghcr.io/windsorcli/git-livereload-server
	DEFAULT_GIT_LIVE_RELOAD_IMAGE         = "ghcr.io/windsorcli/git-livereload:v0.2.1"
	DEFAULT_GIT_LIVE_RELOAD_RSYNC_INCLUDE = "kustomize"
	DEFAULT_GIT_LIVE_RELOAD_RSYNC_EXCLUDE = ".windsor,.terraform,.volumes,.venv"
	DEFAULT_GIT_LIVE_RELOAD_RSYNC_PROTECT = "flux-system"
	DEFAULT_GIT_LIVE_RELOAD_USERNAME      = "local"
	DEFAULT_GIT_LIVE_RELOAD_PASSWORD      = "local"
	// Hook URL corresponds to the webhook token "abcdef123456".
	// see: https://fluxcd.io/flux/components/notification/receivers/
	DEFAULT_GIT_LIVE_RELOAD_WEBHOOK_URL = "http://worker-1.test:30292/hook/5dc88e45e809fb0872b749c0969067e2c1fd142e17aed07573fad20553cc0c59"
)

// Default Talos settings
const (
	// renovate: datasource=github-releases depName=siderolabs/talos
	DEFAULT_TALOS_IMAGE             = "ghcr.io/siderolabs/talos:v1.9.5"
	DEFAULT_TALOS_WORKER_CPU        = 4
	DEFAULT_TALOS_WORKER_RAM        = 4
	DEFAULT_TALOS_CONTROL_PLANE_CPU = 2
	DEFAULT_TALOS_CONTROL_PLANE_RAM = 2
	DEFAULT_TALOS_API_PORT          = 50000
	GRPCMaxMessageSize              = 32 * 1024 * 1024 // 32MB
)

const (
	DEFAULT_FLUX_SYSTEM_NAMESPACE             = "system-gitops"
	DEFAULT_FLUX_KUSTOMIZATION_INTERVAL       = 1 * time.Minute
	DEFAULT_FLUX_KUSTOMIZATION_PRUNE          = true
	DEFAULT_FLUX_KUSTOMIZATION_RETRY_INTERVAL = 2 * time.Minute
	DEFAULT_FLUX_KUSTOMIZATION_WAIT           = true
	DEFAULT_FLUX_KUSTOMIZATION_FORCE          = false
	DEFAULT_FLUX_KUSTOMIZATION_TIMEOUT        = 5 * time.Minute
	DEFAULT_FLUX_SOURCE_INTERVAL              = 1 * time.Minute
	DEFAULT_FLUX_SOURCE_TIMEOUT               = 2 * time.Minute
	DEFAULT_FLUX_CLEANUP_TIMEOUT              = 30 * time.Minute

	// Used for aggregate CLI wait (not per-resource)
	DEFAULT_KUSTOMIZATION_WAIT_TOTAL_TIMEOUT = 10 * time.Minute
	// Poll interval for CLI WaitForKustomizations
	DEFAULT_KUSTOMIZATION_WAIT_POLL_INTERVAL = 5 * time.Second
	// Maximum number of consecutive failures before giving up
	DEFAULT_KUSTOMIZATION_WAIT_MAX_FAILURES = 5
)

// Default AWS settings
const (
	// renovate: datasource=docker depName=localstack/localstack
	DEFAULT_AWS_LOCALSTACK_IMAGE = "localstack/localstack:3.8.1"
	// renovate: datasource=docker depName=localstack/localstack-pro
	DEFAULT_AWS_LOCALSTACK_PRO_IMAGE = "localstack/localstack-pro:3.8.1"
)

// Default DNS settings
const (
	// renovate: datasource=docker depName=coredns/coredns
	DEFAULT_DNS_IMAGE = "coredns/coredns:1.11.3"
)

// Default Registry settings
const (
	// renovate: datasource=docker depName=registry
	REGISTRY_DEFAULT_IMAGE     = "registry:2.8.3"
	REGISTRY_DEFAULT_HOST_PORT = 5001
)

// Default network settings
const (
	DEFAULT_NETWORK_CIDR = "10.5.0.0/16"
)

// Kubernetes settings
const (
	KUBERNETES_SHORT_TIMEOUT = 200 * time.Millisecond
)

// Minimum versions for tools
const (
	MINIMUM_VERSION_COLIMA         = "0.7.0"
	MINIMUM_VERSION_DOCKER         = "23.0.0"
	MINIMUM_VERSION_DOCKER_COMPOSE = "2.20.0"
	MINIMUM_VERSION_KUBECTL        = "1.27.0"
	MINIMUM_VERSION_LIMA           = "1.0.0"
	MINIMUM_VERSION_TALOSCTL       = "1.7.0"
	MINIMUM_VERSION_TERRAFORM      = "1.7.0"
	MINIMUM_VERSION_1PASSWORD      = "2.15.0"
	MINIMUM_VERSION_AWS_CLI        = "2.15.0"
)

// Default node health check settings
const (
	DEFAULT_NODE_HEALTH_CHECK_TIMEOUT       = 5 * time.Minute
	DEFAULT_NODE_HEALTH_CHECK_POLL_INTERVAL = 10 * time.Second
)

// Default OCI blueprint settings
const (
	DEFAULT_OCI_BLUEPRINT_URL = "oci://ghcr.io/windsorcli/core:latest"
)

// Build-time variable for pinned blueprint URL (set via ldflags)
var PinnedBlueprintURL = ""

// GetEffectiveBlueprintURL returns the pinned blueprint URL if set at build time,
// otherwise returns the default blueprint URL. This allows for different behavior
// between development builds (using :latest) and release builds (using pinned versions).
func GetEffectiveBlueprintURL() string {
	if PinnedBlueprintURL != "" {
		return PinnedBlueprintURL
	}
	return DEFAULT_OCI_BLUEPRINT_URL
}
