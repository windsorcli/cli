package constants

import "time"

// Version is the CLI version, set at build time via ldflags
var Version = "dev"

// CommitSHA is the git commit SHA, set at build time via ldflags
var CommitSHA = "none"

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

// DefaultGitLiveReloadWebhookNodePort is the NodePort for the Flux webhook receiver (docker-desktop mode).
const DefaultGitLiveReloadWebhookNodePort = 30292

// DefaultGitLiveReloadWebhookLBPort is the LoadBalancer port for the Flux webhook receiver.
const DefaultGitLiveReloadWebhookLBPort = 9292

// DefaultGitLiveReloadWebhookPath is the webhook path with token.
// Hook path corresponds to the webhook token "abcdef123456".
// see: https://fluxcd.io/flux/components/notification/receivers/
const DefaultGitLiveReloadWebhookPath = "/hook/5dc88e45e809fb0872b749c0969067e2c1fd142e17aed07573fad20553cc0c59"

// renovate: datasource=github-releases depName=siderolabs/talos
const DefaultTalosImage = "ghcr.io/siderolabs/talos:v1.12.3"

const DefaultTalosWorkerCPU = 4

const DefaultTalosWorkerRAM = 6

const DefaultTalosControlPlaneCPU = 4

const DefaultTalosControlPlaneRAM = 4

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

const DefaultCleanupNamespace = "system-cleanup"

const DefaultCleanupSemaphoreName = "cleanup-authorized"

// Used for aggregate CLI wait (not per-resource)
const DefaultKustomizationWaitTotalTimeout = 10 * time.Minute

// Poll interval for CLI WaitForKustomizations
const DefaultKustomizationWaitPollInterval = 5 * time.Second

// Maximum number of consecutive failures before giving up
const DefaultKustomizationWaitMaxFailures = 5

// renovate: datasource=docker depName=localstack/localstack
const DefaultAWSLocalstackImage = "localstack/localstack:4.13.1"

// renovate: datasource=docker depName=localstack/localstack-pro
const DefaultAWSLocalstackProImage = "localstack/localstack-pro:4.13.1"

// renovate: datasource=docker depName=coredns/coredns
const DefaultDNSImage = "coredns/coredns:1.14.1"

// renovate: datasource=docker depName=registry
const RegistryDefaultImage = "registry:3.0.0"

const RegistryDefaultHostPort = 5001

const DefaultNetworkCIDR = "10.5.0.0/16"

const KubernetesShortTimeout = 200 * time.Millisecond

const MinimumVersionColima = "0.9.0"

const MinimumVersionDocker = "23.0.0"

const MinimumVersionDockerCompose = "2.20.0"

const MinimumVersionKubectl = "1.27.0"

const MinimumVersionLima = "1.0.0"

const MinimumVersionTalosctl = "1.7.0"

const MinimumVersionTerraform = "1.7.0"

const MinimumVersion1Password = "2.15.0"

const MinimumVersionAWSCLI = "2.15.0"

const MinimumVersionKubelogin = "0.1.7"

const MinimumVersionSOPS = "3.10.0"

// DefaultAKSOIDCServerID is the standard Azure AKS OIDC server ID (application ID of the
// Microsoft-managed enterprise application "Azure Kubernetes Service AAD Server").
// This is the same for all AKS clusters with AKS-managed Azure AD enabled.
const DefaultAKSOIDCServerID = "6dae42f8-4368-4678-94ff-3960e28e3630"

// DefaultAKSOIDCClientID is the standard Azure AKS OIDC client ID used for all AKS clusters.
const DefaultAKSOIDCClientID = "80faf920-1908-4b52-b5ef-a8e7bedfc67a"

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
