package provisioner

import (
	"fmt"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
)

// =============================================================================
// Constants
// =============================================================================

// KustomizeFailureID is the sentinel ID used when the kustomize Uninstall step
// fails under continue-on-error mode. The kustomize layer surfaces a single
// aggregate failure rather than per-Kustomization entries, so the tier-gate
// logic in Teardown can distinguish kustomize failures (which do not block
// the terraform backend tier) from terraform-component failures (which do).
const KustomizeFailureID = "kustomize"

// =============================================================================
// Types
// =============================================================================

// ComponentFailure is a per-component error captured during continue-on-error
// destroy. Aliased from the terraform package so callers can use a single
// type identity across the layer boundary without duplication.
type ComponentFailure = terraforminfra.ComponentFailure

// DestroyResult is the cmd-facing aggregate of a destroy pass. Destroyed,
// Skipped, and Failed roll up every component the provisioner attempted —
// kustomize plus terraform — and TierDeferred records the provisioner-layer
// decision to leave the backend tier alone when a non-tier component still
// needs work. Fields for additional destroy layers (e.g. Helm) belong on
// this type, not on the terraform-package outcome.
type DestroyResult struct {
	Destroyed    []string
	Skipped      []string
	Failed       []ComponentFailure
	TierDeferred bool
}

// =============================================================================
// Public Methods
// =============================================================================

// Teardown reverses Bootstrap. With no backend tier it forwards to DestroyAll
// (or DestroyAllTerraform when terraformOnly). With a tier declared via
// Blueprint.Backend, Stage 1 destroys non-tier components against the
// configured backend, then Stage 2 pins local, pulls every tier member's state
// to local, and destroys the tier in reverse declaration order. When
// continueOnError is true, per-component destroy errors in Stage 1 are
// collected rather than aborting the loop; the backend tier is only attempted
// when Stage 1 produced zero failures, to avoid destroying the state store
// while other components still depend on it. The tier-deferred flag on the
// result signals when Stage 2 was skipped for this reason. Stage 2's tier
// destroy skips the Kubernetes-reachability preflight: Stage 1 has, by
// design, already destroyed the cluster (it is never a tier member), so an
// unreachable API at this point is the expected state, not a broken-auth
// signal, and the backend tier never has a kubernetes/helm provider
// dependency for the check to protect.
func (i *Provisioner) Teardown(blueprint *blueprintv1alpha1.Blueprint, terraformOnly bool, continueOnError bool) (DestroyResult, error) {
	var result DestroyResult
	backendType := i.configHandler.GetString("terraform.backend.type", "local")
	if backendType == "kubernetes" && blueprint.Backend == "" {
		return result, fmt.Errorf("blueprint configures terraform.backend.type=kubernetes but does not declare Blueprint.Backend; set `backend: <cluster-component-id>` at the blueprint top level to name the terraform component that provisions the cluster")
	}
	tier := blueprint.BackendTier()
	if backendType == "" || backendType == "local" || len(tier) == 0 {
		if terraformOnly {
			return i.DestroyAllTerraform(blueprint, continueOnError)
		}
		return i.DestroyAll(blueprint, continueOnError)
	}

	tierIDs := make([]string, 0, len(tier))
	for _, c := range tier {
		tierIDs = append(tierIDs, c.GetID())
	}

	var stage1Err error
	if terraformOnly {
		result, stage1Err = i.DestroyAllTerraform(blueprint, continueOnError, tierIDs...)
	} else {
		result, stage1Err = i.DestroyAll(blueprint, continueOnError, tierIDs...)
	}
	if stage1Err != nil {
		return result, stage1Err
	}

	if hasTerraformFailure(result.Failed) {
		result.TierDeferred = true
		return result, nil
	}

	tierBP := blueprintWithComponents(blueprint, tier)
	err := i.withBackendOverride("destroy", func() error {
		migrationSkipped, err := i.MigrateState(tierBP)
		if err != nil {
			return err
		}
		tierResult, destroyErr := i.destroyAllTerraform(tierBP, continueOnError, false)
		result.Destroyed = append(result.Destroyed, tierResult.Destroyed...)
		result.Skipped = mergeSkipped(result.Skipped, mergeSkipped(migrationSkipped, tierResult.Skipped))
		result.Failed = append(result.Failed, tierResult.Failed...)
		return destroyErr
	})
	return result, err
}

// TeardownComponent destroys a single terraform component. Targeting any
// backend-tier member on a non-local backend is refused: its state provides
// the backend that other components rely on, so destroying it in isolation
// would orphan their state. Use `windsor destroy` (no arguments) for the
// full-cycle teardown.
func (i *Provisioner) TeardownComponent(blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
	if err := i.CheckComponentDestroyable(blueprint, componentID); err != nil {
		return false, err
	}
	return i.Destroy(blueprint, componentID)
}

// CheckComponentDestroyable reports whether a single terraform component may be destroyed in isolation.
// On a non-local backend a backend-tier member is refused: its state provides the backend every other
// component uses, so destroying it directly would orphan their state. Callers run this before generating a
// destroy plan so the refusal is surfaced up front, rather than as a raw terraform init error when the
// component tries to reach a kubernetes backend whose cluster may already be gone.
func (i *Provisioner) CheckComponentDestroyable(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	backendType := i.configHandler.GetString("terraform.backend.type", "local")
	if backendType != "" && backendType != "local" && blueprint.IsBackendTierMember(componentID) {
		return fmt.Errorf("cannot destroy backend-tier component %q in isolation: its state provides the %s backend that every other component uses, so destroying it directly would orphan their state. Run `windsor destroy` (no arguments) for the full-cycle teardown that migrates state to local first", componentID, backendType)
	}
	return nil
}

// PrepareLocalTeardown makes a kubernetes-backend teardown operate entirely against local state. Because
// the kubernetes backend stores state on the cluster the teardown is about to destroy, this pulls every
// component's state to local up front — while the cluster still hosts the backend — and pivots
// terraform.backend.type to local for the rest of the process. From that point the destroy plan and every
// component destroy read local state, never dialing a backend that is going away, so "the cluster is gone"
// can no longer strand the teardown. The pivot is unconditional for a kubernetes backend; reachability is
// consulted only to classify a migration failure: while the cluster is still reachable a failure is real —
// destroying now would run against empty local state and orphan resources, so it aborts — but once the
// cluster is gone (a resumed teardown) the state was already migrated on the earlier pass and is the local
// copy, so it proceeds against it. Returns whether it pivoted; a non-kubernetes backend is a no-op.
func (i *Provisioner) PrepareLocalTeardown(blueprint *blueprintv1alpha1.Blueprint) (bool, error) {
	backendType := i.configHandler.GetString("terraform.backend.type", "local")
	if backendType == "" || backendType == "local" {
		return false, nil
	}

	// The pivot must precede MigrateState — it migrates to the currently-configured backend, so the backend
	// has to read local for state to move to local. On the abort path (a real migration failure while the
	// cluster is reachable) the pivot is reverted, so a caller that continues does not read a local backend
	// with no migrated state behind it and destroy against emptiness.
	if err := i.configHandler.Set("terraform.backend.type", "local"); err != nil {
		return false, fmt.Errorf("failed to pivot terraform backend to local for teardown: %w", err)
	}

	if _, err := i.MigrateState(blueprint); err != nil {
		if i.clusterReachableForTeardown() {
			_ = i.configHandler.Set("terraform.backend.type", backendType)
			return false, fmt.Errorf("failed to migrate terraform state to local before teardown: %w", err)
		}
	}
	return true, nil
}

// PivotToLocalIfClusterGone pivots terraform.backend.type to local for the rest of the process when the
// kubernetes backend's cluster is gone (no kubeconfig) or unreachable. Unlike PrepareLocalTeardown it does
// not migrate — the state is already the local copy a prior full teardown pulled off the cluster before
// destroying it — it only redirects reads to that copy. This is the targeted-destroy counterpart: a single
// component destroy cannot migrate everything to local without stranding the cluster-up case (one component
// destroyed locally while the rest still read kubernetes would drift), so it operates on kubernetes while
// the cluster is up and on the already-migrated local state once the cluster is gone. A reachable cluster or
// non-kubernetes backend is a no-op. Returns whether it pivoted.
func (i *Provisioner) PivotToLocalIfClusterGone() (bool, error) {
	backendType := i.configHandler.GetString("terraform.backend.type", "local")
	if backendType == "" || backendType == "local" {
		return false, nil
	}
	if i.clusterReachableForTeardown() {
		return false, nil
	}
	if err := i.configHandler.Set("terraform.backend.type", "local"); err != nil {
		return false, fmt.Errorf("failed to pivot terraform backend to local for teardown: %w", err)
	}
	return true, nil
}

// clusterReachableForTeardown reports whether the cluster hosting the kubernetes backend is present and
// reachable. It is used to classify a state-migration failure during teardown preparation: a real error
// while the cluster is up versus the expected resume case where the cluster is already gone and the state
// is already the local copy.
func (i *Provisioner) clusterReachableForTeardown() bool {
	if !i.kubeconfigPresent() {
		return false
	}
	return i.checkKubernetesReachableForDestroy() == nil
}

// =============================================================================
// Private Helpers
// =============================================================================

// hasTerraformFailure reports whether the failure list contains any entry
// that belongs to a terraform component (i.e., not the kustomize-aggregate
// sentinel). Used by Teardown's tier gate: kustomize failures do not block
// the backend terraform tier because kustomize resources do not depend on
// terraform state. Without this filter, a kustomize Uninstall error would
// permanently defer the tier on every rerun, because the cluster is the
// thing kustomize most often fails against.
func hasTerraformFailure(failed []ComponentFailure) bool {
	for _, f := range failed {
		if f.ID != KustomizeFailureID {
			return true
		}
	}
	return false
}

// mergeSkipped returns the union of two skipped-component lists in input order
// without duplicates. MigrateState and DestroyAll both report dir-missing
// components, so naive concat would double-count; on the error path
// MigrateState's list still names components DestroyAll didn't reach before
// bailing out.
func mergeSkipped(a, b []string) []string {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, ids := range [][]string{a, b} {
		for _, id := range ids {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}
