package constants

import "time"

// The Constants package provides centralized default values and configuration constants
// It provides shared constants for default settings, timeouts, versions, and resource configurations
// The Constants package serves as a single source of truth for default values across the application
// enabling consistent behavior and easy maintenance of configuration defaults

// =============================================================================
// Constants
// =============================================================================

// renovate: datasource=docker depName=ghcr.io/windsorcli/git-livereload-server
const DefaultGitLiveReloadImage = "ghcr.io/windsorcli/git-livereload:v0.2.1"

const DefaultGitLiveReloadRsyncInclude = "kustomize"

const DefaultGitLiveReloadRsyncExclude = ".windsor,.terraform,.volumes,.venv"

const DefaultGitLiveReloadRsyncProtect = "flux-system"

const DefaultGitLiveReloadUsername = "local"

const DefaultGitLiveReloadPassword = "local"

// Hook URL corresponds to the webhook token "abcdef123456".
// see: https://fluxcd.io/flux/components/notification/receivers/
const DefaultGitLiveReloadWebhookURL = "http://worker-1.test:30292/hook/5dc88e45e809fb0872b749c0969067e2c1fd142e17aed07573fad20553cc0c59"

// renovate: datasource=github-releases depName=siderolabs/talos
const DefaultTalosImage = "ghcr.io/siderolabs/talos:v1.11.3"

const DefaultTalosWorkerCPU = 4

const DefaultTalosWorkerRAM = 4

const DefaultTalosControlPlaneCPU = 2

const DefaultTalosControlPlaneRAM = 2

const DefaultTalosAPIPort = 50000

const DefaultFluxSystemNamespace = "system-gitops"

const DefaultFluxKustomizationInterval = 1 * time.Minute

const DefaultFluxKustomizationPrune = true

const DefaultFluxKustomizationRetryInterval = 2 * time.Minute

const DefaultFluxKustomizationWait = true

const DefaultFluxKustomizationForce = false

const DefaultFluxKustomizationTimeout = 5 * time.Minute

const DefaultFluxSourceInterval = 1 * time.Minute

const DefaultFluxSourceTimeout = 2 * time.Minute

const DefaultFluxCleanupTimeout = 30 * time.Minute

// Used for aggregate CLI wait (not per-resource)
const DefaultKustomizationWaitTotalTimeout = 10 * time.Minute

// Poll interval for CLI WaitForKustomizations
const DefaultKustomizationWaitPollInterval = 5 * time.Second

// Maximum number of consecutive failures before giving up
const DefaultKustomizationWaitMaxFailures = 5

// renovate: datasource=docker depName=localstack/localstack
const DefaultAWSLocalstackImage = "localstack/localstack:4.10.0"

// renovate: datasource=docker depName=localstack/localstack-pro
const DefaultAWSLocalstackProImage = "localstack/localstack-pro:3.8.1"

// renovate: datasource=docker depName=coredns/coredns
const DefaultDNSImage = "coredns/coredns:1.13.1"

// renovate: datasource=docker depName=registry
const RegistryDefaultImage = "registry:2.8.3"

const RegistryDefaultHostPort = 5001

const DefaultNetworkCIDR = "10.5.0.0/16"

const KubernetesShortTimeout = 200 * time.Millisecond

const MinimumVersionColima = "0.7.0"

const MinimumVersionDocker = "23.0.0"

const MinimumVersionDockerCompose = "2.20.0"

const MinimumVersionKubectl = "1.27.0"

const MinimumVersionLima = "1.0.0"

const MinimumVersionTalosctl = "1.7.0"

const MinimumVersionTerraform = "1.7.0"

const MinimumVersion1Password = "2.15.0"

const MinimumVersionAWSCLI = "2.15.0"

const DefaultNodeHealthCheckTimeout = 5 * time.Minute

const DefaultNodeHealthCheckPollInterval = 10 * time.Second

const DefaultOCIBlueprintURL = "oci://ghcr.io/windsorcli/core:latest"

// =============================================================================
// Variables
// =============================================================================

// Build-time variable for pinned blueprint URL (set via ldflags)
var PinnedBlueprintURL = ""

// =============================================================================
// Helpers
// =============================================================================

// GetEffectiveBlueprintURL returns the pinned blueprint URL if set at build time,
// otherwise returns the default blueprint URL. This allows for different behavior
// between development builds (using :latest) and release builds (using pinned versions).
func GetEffectiveBlueprintURL() string {
	if PinnedBlueprintURL != "" {
		return PinnedBlueprintURL
	}
	return DefaultOCIBlueprintURL
}
