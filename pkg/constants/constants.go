// The constants package holds the default values of the Windsor CLI.
// It provides the default settings, timeouts, versions and resource values.
// It also holds the build values that the release build sets with ldflags.
// Helpers select a default value from a config value or a source type.

package constants

import "time"

// =============================================================================
// Variables
// =============================================================================

// Version is the CLI version. The build sets it with ldflags.
var Version = "dev"

// CommitSHA is the git commit SHA. The build sets it with ldflags.
var CommitSHA = "none"

// BuildDate is the build time in RFC 3339 format. goreleaser sets it with ldflags.
// It keeps its default value after `go build` or `go install`.
var BuildDate = "unknown"

// PinnedBlueprintURL is the blueprint URL of a release build. goreleaser sets it with ldflags.
var PinnedBlueprintURL = ""

// =============================================================================
// Constants
// =============================================================================

const DefaultControlPlaneCPUSchedulable = 4

const DefaultControlPlaneMemorySchedulable = 12

const DefaultControlPlaneCPUDedicated = 4

const DefaultControlPlaneMemoryDedicated = 8

const DefaultWorkerCPU = 4

const DefaultWorkerMemory = 8

const DefaultGitopsNamespace = "system-gitops"

// DefaultFluxKustomizationInterval is the backstop reconciliation interval for a Kustomization of a vendor source.
const DefaultFluxKustomizationInterval = 1 * time.Hour

const DefaultFluxKustomizationRetryInterval = 2 * time.Minute

const DefaultFluxKustomizationWait = true

const DefaultFluxKustomizationForce = false

const DefaultFluxKustomizationTimeout = 5 * time.Minute

// DefaultFluxKustomizationInstallTimeout is the timeout of an install tier when the facet sets none.
const DefaultFluxKustomizationInstallTimeout = 10 * time.Minute

// DefaultConvergeTimeout is the time limit to reconcile stalled kustomizations and HelmReleases during an install.
const DefaultConvergeTimeout = 5 * time.Minute

// DefaultFluxSourceInterval is the backstop reconciliation interval for a vendor source.
const DefaultFluxSourceInterval = 1 * time.Hour

// DefaultFluxPrimaryRepositoryInterval is the poll interval for the repository of the blueprint.
const DefaultFluxPrimaryRepositoryInterval = 1 * time.Minute

// DefaultFluxPrimaryKustomizationInterval is the backstop reconciliation interval for a Kustomization of the blueprint repository.
const DefaultFluxPrimaryKustomizationInterval = 5 * time.Minute

const DefaultFluxSourceTimeout = 2 * time.Minute

// DefaultKustomizationWaitTotalTimeout is the time limit of one CLI wait for all kustomizations.
const DefaultKustomizationWaitTotalTimeout = 10 * time.Minute

// DefaultKustomizationWaitPollInterval is the poll interval of the CLI kustomization wait.
const DefaultKustomizationWaitPollInterval = 5 * time.Second

// DefaultKustomizationWaitMaxFailures is the number of consecutive failures before the CLI wait stops.
const DefaultKustomizationWaitMaxFailures = 5

// DefaultLoadBalancerTeardownTimeout is the time limit for a cloud controller to remove a LoadBalancer Service finalizer.
const DefaultLoadBalancerTeardownTimeout = 15 * time.Minute

// DefaultTerraformDestroyIdleTimeout is the time limit without output from `terraform destroy`.
const DefaultTerraformDestroyIdleTimeout = 5 * time.Minute

// DefaultTerraformDestroyAbsoluteTimeout is the total time limit of one `terraform destroy`.
const DefaultTerraformDestroyAbsoluteTimeout = 2 * time.Hour

// DefaultTerraformRefreshTimeout is the time limit of the terraform refresh before a destroy.
const DefaultTerraformRefreshTimeout = 2 * time.Minute

// DefaultTerraformDestroyRetryAttempts is the number of destroy attempts for one component.
// Only a failure before the timeout gets a retry.
const DefaultTerraformDestroyRetryAttempts = 2

// DefaultTerraformDestroyRetryBackoff is the wait between two destroy attempts.
const DefaultTerraformDestroyRetryBackoff = 30 * time.Second

// DefaultTerraformDestroyGracePeriod is the wait between the interrupt and the kill of a timed-out destroy.
const DefaultTerraformDestroyGracePeriod = 2 * time.Minute

// DefaultKubernetesReachabilityCheckTimeout is the time limit of the Kubernetes check before a destroy.
const DefaultKubernetesReachabilityCheckTimeout = 30 * time.Second

const DefaultNetworkCIDR = "10.5.0.0/16"

const KubernetesShortTimeout = 200 * time.Millisecond

const MinimumVersionColima = "0.9.0"

const MinimumVersionDocker = "23.0.0"

const MinimumVersionLima = "1.0.0"

// MinimumVersionLimaIncus is the minimum limactl version for colima with the incus platform.
const MinimumVersionLimaIncus = "2.0.3"

const MinimumVersionTerraform = "1.12.2"

// MinimumVersionFlux is the first flux release with the --kustomization-file flag.
const MinimumVersionFlux = "2.3.0"

// MinimumVersionFluxOperator is the first flux-operator release with the build instance command.
const MinimumVersionFluxOperator = "0.20.0"

const MinimumVersion1Password = "2.15.0"

const MinimumVersionKubelogin = "0.1.7"

const MinimumVersionSOPS = "3.10.0"

const MinimumVersionAWS = "2.0.0"

const MinimumVersionAzure = "2.50.0"

const MinimumVersionGCP = "400.0.0"

const DefaultNodeHealthCheckTimeout = 5 * time.Minute

const DefaultNodeHealthCheckPollInterval = 10 * time.Second

const DefaultNodeUpgradeTimeout = 10 * time.Minute

const DefaultNodeOfflineTimeout = 3 * time.Minute

// DefaultAPIServerReadyTimeout is the time limit for the kube-apiserver of a control-plane node after a reboot.
const DefaultAPIServerReadyTimeout = 5 * time.Minute

const DefaultAPIServerPort = 6443

const DefaultOCIBlueprintURL = "oci://ghcr.io/windsorcli/core:latest"

// =============================================================================
// Types
// =============================================================================

// GitopsMode is the gitops mode of a context. Pull and push use the same reconciliation intervals.
type GitopsMode string

const (
	GitopsModePull GitopsMode = "pull"
	GitopsModePush GitopsMode = "push"
)

// =============================================================================
// Helpers
// =============================================================================

// ParseGitopsMode returns the GitopsMode for a config value.
// It returns "pull" for an empty or unknown value, so a typo gives the safe mode and not an error.
func ParseGitopsMode(s string) GitopsMode {
	if GitopsMode(s) == GitopsModePush {
		return GitopsModePush
	}
	return GitopsModePull
}

// FluxKustomizationInterval returns the default reconciliation interval for a Kustomization.
// isPrimarySource is true when the Kustomization uses the repository of the blueprint, not a vendor source.
// A short interval causes DependencyNotReady retries, so this interval is longer than the source interval.
func FluxKustomizationInterval(isPrimarySource bool) time.Duration {
	if isPrimarySource {
		return DefaultFluxPrimaryKustomizationInterval
	}
	return DefaultFluxKustomizationInterval
}

// FluxSourceInterval returns the default reconciliation interval for a GitRepository or OCIRepository.
// isPrimary is true for the repository in the top-level "repository:" field of the blueprint.
// A push to the blueprint repository can occur without a windsor command, so its poll interval is short.
func FluxSourceInterval(isPrimary bool) time.Duration {
	if isPrimary {
		return DefaultFluxPrimaryRepositoryInterval
	}
	return DefaultFluxSourceInterval
}

// GetEffectiveBlueprintURL returns PinnedBlueprintURL when the build sets it, or DefaultOCIBlueprintURL.
func GetEffectiveBlueprintURL() string {
	if PinnedBlueprintURL != "" {
		return PinnedBlueprintURL
	}
	return DefaultOCIBlueprintURL
}
