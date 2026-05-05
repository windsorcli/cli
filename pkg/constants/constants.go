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
const DefaultTalosImage = "ghcr.io/siderolabs/talos:v1.13.0"

const DefaultTalosAPIPort = 50000

const DefaultControlPlaneCPUSchedulable = 8

const DefaultControlPlaneMemorySchedulable = 12

const DefaultControlPlaneCPUDedicated = 4

const DefaultControlPlaneMemoryDedicated = 4

const DefaultWorkerCPU = 4

const DefaultWorkerMemory = 8

const DefaultGitopsNamespace = "system-gitops"

const DefaultFluxKustomizationInterval = 1 * time.Minute

// DefaultFluxKustomizationIntervalPush is the reconciliation interval for
// Kustomizations when gitops.mode is "push". It is intentionally long because
// Windsor annotates sources on every install/apply to trigger reconcile; flux's
// own poll becomes a fallback rather than the primary path.
const DefaultFluxKustomizationIntervalPush = 1 * time.Hour

const DefaultFluxKustomizationPrune = true

const DefaultFluxKustomizationRetryInterval = 2 * time.Minute

const DefaultFluxKustomizationWait = true

const DefaultFluxKustomizationForce = false

const DefaultFluxKustomizationTimeout = 5 * time.Minute

const DefaultFluxSourceInterval = 1 * time.Minute

// DefaultFluxSourceIntervalPush is the reconciliation interval for flux Sources
// (GitRepository / OCIRepository) when gitops.mode is "push". Matches the
// Kustomization push interval so the two layers stay in step.
const DefaultFluxSourceIntervalPush = 1 * time.Hour

// GitopsMode controls whether Windsor or flux drives reconciliation cadence.
// In "pull" mode (default) flux polls sources on short intervals so changes
// propagate without CLI involvement. In "push" mode Windsor triggers reconcile
// via annotation during install/apply, so polling becomes a long-interval
// fallback rather than the primary path. Unknown and empty values resolve to
// "pull" via ParseGitopsMode so an unset config key keeps today's behaviour.
type GitopsMode string

const (
	GitopsModePull GitopsMode = "pull"
	GitopsModePush GitopsMode = "push"
)

// ParseGitopsMode resolves a config string to a GitopsMode, defaulting to
// "pull" for empty or unrecognised values. Keeps the config surface forgiving
// so typos fall back to safe behaviour rather than refusing to apply.
func ParseGitopsMode(s string) GitopsMode {
	if GitopsMode(s) == GitopsModePush {
		return GitopsModePush
	}
	return GitopsModePull
}

// FluxKustomizationInterval returns the default reconciliation interval for
// Kustomizations under the given mode. Blueprint-level Interval overrides take
// precedence over both defaults; this only affects Kustomizations that leave
// Interval unset.
func FluxKustomizationInterval(mode GitopsMode) time.Duration {
	if mode == GitopsModePush {
		return DefaultFluxKustomizationIntervalPush
	}
	return DefaultFluxKustomizationInterval
}

// FluxSourceInterval returns the default reconciliation interval for flux
// Sources (GitRepository / OCIRepository) under the given mode. As with
// FluxKustomizationInterval, blueprint-level overrides win when present.
func FluxSourceInterval(mode GitopsMode) time.Duration {
	if mode == GitopsModePush {
		return DefaultFluxSourceIntervalPush
	}
	return DefaultFluxSourceInterval
}

const DefaultFluxSourceTimeout = 2 * time.Minute

// Used for aggregate CLI wait (not per-resource)
const DefaultKustomizationWaitTotalTimeout = 10 * time.Minute

// Poll interval for CLI WaitForKustomizations
const DefaultKustomizationWaitPollInterval = 5 * time.Second

// Maximum number of consecutive failures before giving up
const DefaultKustomizationWaitMaxFailures = 5

// renovate: datasource=docker depName=localstack/localstack
const DefaultAWSLocalstackImage = "localstack/localstack:4.14.0"

// renovate: datasource=docker depName=localstack/localstack-pro
const DefaultAWSLocalstackProImage = "localstack/localstack-pro:4.14.0"

// renovate: datasource=docker depName=coredns/coredns
const DefaultDNSImage = "coredns/coredns:1.14.3"

// renovate: datasource=docker depName=registry
const RegistryDefaultImage = "registry:3.1.1"

const RegistryDefaultHostPort = 5001

const DefaultNetworkCIDR = "10.5.0.0/16"

const KubernetesShortTimeout = 200 * time.Millisecond

const MinimumVersionColima = "0.9.0"

const MinimumVersionDocker = "23.0.0"

const MinimumVersionKubectl = "1.27.0"

const MinimumVersionLima = "1.0.0"

// MinimumVersionLimaIncus is the minimum limactl version when using colima with platform incus.
// Lima 1.x can hang after "Terminal is not available"; 2.0.3+ is required for reliable colima+incus startup.
const MinimumVersionLimaIncus = "2.0.3"

const MinimumVersionTerraform = "1.7.0"

const MinimumVersion1Password = "2.15.0"

const MinimumVersionKubelogin = "0.1.7"

const MinimumVersionSOPS = "3.10.0"

const MinimumVersionAWS = "2.0.0"

const MinimumVersionAzure = "2.50.0"

// DefaultAKSOIDCServerID is the standard Azure AKS OIDC server ID (application ID of the
// Microsoft-managed enterprise application "Azure Kubernetes Service AAD Server").
// This is the same for all AKS clusters with AKS-managed Azure AD enabled.
const DefaultAKSOIDCServerID = "6dae42f8-4368-4678-94ff-3960e28e3630"

// DefaultAKSOIDCClientID is the standard Azure AKS OIDC client ID used for all AKS clusters.
const DefaultAKSOIDCClientID = "80faf920-1908-4b52-b5ef-a8e7bedfc67a"

const DefaultNodeHealthCheckTimeout = 5 * time.Minute

const DefaultNodeHealthCheckPollInterval = 10 * time.Second

const DefaultNodeUpgradeTimeout = 10 * time.Minute

const DefaultNodeOfflineTimeout = 3 * time.Minute

// DefaultAPIServerReadyTimeout caps how long UpgradeNode waits for the kube-apiserver
// on a control-plane node to accept connections after a reboot.
const DefaultAPIServerReadyTimeout = 5 * time.Minute

// DefaultAPIServerPort is the standard Kubernetes API server port.
const DefaultAPIServerPort = 6443

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
