package constants

import "time"

// Default git livereload settings
const (
	// renovate: datasource=docker depName=ghcr.io/windsorcli/git-livereload-server
	DEFAULT_GIT_LIVE_RELOAD_IMAGE         = "ghcr.io/windsorcli/git-livereload-server:v0.2.1"
	DEFAULT_GIT_LIVE_RELOAD_RSYNC_EXCLUDE = ".windsor,.terraform,data,.volumes,.venv"
	DEFAULT_GIT_LIVE_RELOAD_RSYNC_PROTECT = "flux-system"
	DEFAULT_GIT_LIVE_RELOAD_USERNAME      = "local"
	DEFAULT_GIT_LIVE_RELOAD_PASSWORD      = "local"
	DEFAULT_GIT_LIVE_RELOAD_WEBHOOK_URL   = "http://flux-webhook.private.test"
)

// Default Talos settings
const (
	// renovate: datasource=docker depName=ghcr.io/siderolabs/talos
	DEFAULT_TALOS_IMAGE             = "ghcr.io/siderolabs/talos:v1.9.1"
	DEFAULT_TALOS_WORKER_CPU        = 4
	DEFAULT_TALOS_WORKER_RAM        = 4
	DEFAULT_TALOS_CONTROL_PLANE_CPU = 2
	DEFAULT_TALOS_CONTROL_PLANE_RAM = 2
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
	REGISTRY_DEFAULT_IMAGE = "registry:2.8.3"
)

// Default network settings
const (
	DEFAULT_NETWORK_CIDR = "10.5.0.0/16"
)

// Minimum versions for tools
const (
	MINIMUM_VERSION_COLIMA         = "0.7.0"
	MINIMUM_VERSION_DOCKER         = "25.0.0"
	MINIMUM_VERSION_DOCKER_COMPOSE = "2.24.0"
	MINIMUM_VERSION_KUBECTL        = "1.32.0"
	MINIMUM_VERSION_LIMA           = "1.0.0"
	MINIMUM_VERSION_TALOSCTL       = "1.7.0"
	MINIMUM_VERSION_TERRAFORM      = "1.7.0"
)
