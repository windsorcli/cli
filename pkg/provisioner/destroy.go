package provisioner

import (
	"fmt"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	composerblueprint "github.com/windsorcli/cli/pkg/composer/blueprint"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
)

// =============================================================================
// Constants
// =============================================================================

// KustomizeFailureID is the sentinel ID for a kustomize Uninstall failure under
// continue-on-error mode. The kustomize layer reports one aggregate failure, not
// one per Kustomization. blocksNextStage uses this ID to tell a kustomize failure
// (blocks only while the cluster is reachable) apart from a terraform-component
// failure (always blocks).
const KustomizeFailureID = "kustomize"

// =============================================================================
// Types
// =============================================================================

// ComponentFailure is a per-component error captured during continue-on-error
// destroy. Aliased from the terraform package so callers can use a single
// type identity across the layer boundary without duplication.
type ComponentFailure = terraforminfra.ComponentFailure

// DestroyResult is the cmd-facing aggregate of a destroy pass. Destroyed, Skipped,
// and Failed roll up every component the provisioner attempted, kustomize and
// terraform. TerraformDeferred marks that the terraform stage was skipped: a
// kustomize failure left the cluster reachable, or a non-backend component still
// needs work. Add fields for other destroy layers, such as Helm, to this type,
// not to the terraform-package outcome.
type DestroyResult struct {
	Destroyed         []string
	Skipped           []string
	Failed            []ComponentFailure
	TerraformDeferred bool
}

// =============================================================================
// Public Methods
// =============================================================================

// Teardown reverses Bootstrap.
//
// Without a declared backend, Teardown forwards to DestroyAll (or
// DestroyAllTerraform when terraformOnly is true).
//
// With a backend declared via Blueprint.Backend, Teardown destroys in stages:
//  1. Destroy every non-backend component.
//  2. Destroy the backend's other components, if Stage 1 had no failures.
//  3. Destroy the backend component itself, if Stage 2 had no failures.
//
// This order keeps the state store alive until its dependents are gone. See
// blocksNextStage for the failure check between stages. TerraformDeferred
// marks a skipped stage.
//
// A Backend that names no real component is refused; see resolveBackendComponents.
func (i *Provisioner) Teardown(blueprint *blueprintv1alpha1.Blueprint, terraformOnly bool, continueOnError bool) (DestroyResult, error) {
	var result DestroyResult
	backendType := i.configHandler.GetTerraformBackendType()
	if backendType == "kubernetes" && blueprint.Backend == "" {
		return result, fmt.Errorf("blueprint configures terraform.backend.type=kubernetes but does not declare Blueprint.Backend; set `backend: <cluster-component-id>` at the blueprint top level to name the terraform component that provisions the cluster")
	}
	if err := i.checkOrphanedLocalState(blueprint); err != nil {
		return result, err
	}

	destroyFlat := func() (DestroyResult, error) {
		if terraformOnly {
			return i.DestroyAllTerraform(blueprint, continueOnError)
		}
		return i.DestroyAll(blueprint, continueOnError)
	}
	if backendType == "" || backendType == "local" {
		return destroyFlat()
	}

	backendComponents, err := resolveBackendComponents(blueprint)
	if err != nil {
		return result, err
	}
	if len(backendComponents) == 0 {
		return destroyFlat()
	}

	backendComponentIDs := make([]string, 0, len(backendComponents))
	for _, c := range backendComponents {
		backendComponentIDs = append(backendComponentIDs, c.GetID())
	}

	var stage1Err error
	if terraformOnly {
		result, stage1Err = i.DestroyAllTerraform(blueprint, continueOnError, backendComponentIDs...)
	} else {
		result, stage1Err = i.DestroyAll(blueprint, continueOnError, backendComponentIDs...)
	}
	if stage1Err != nil {
		return result, stage1Err
	}

	if i.blocksNextStage(result.Failed) {
		result.TerraformDeferred = true
		return result, nil
	}

	backendComponentsBP := blueprintWithComponents(blueprint, backendComponents)
	err = i.withBackendOverride("destroy", func() error {
		migrationSkipped, err := i.MigrateState(backendComponentsBP)
		if err != nil {
			return err
		}

		if len(backendComponents) > 1 {
			membersResult, destroyErr := i.destroyAllTerraform(backendComponentsBP, continueOnError, blueprint.Backend)
			result.Destroyed = append(result.Destroyed, membersResult.Destroyed...)
			result.Skipped = mergeSkipped(result.Skipped, mergeSkipped(migrationSkipped, membersResult.Skipped))
			result.Failed = append(result.Failed, membersResult.Failed...)
			if destroyErr != nil {
				return destroyErr
			}
			if i.blocksNextStage(membersResult.Failed) {
				result.TerraformDeferred = true
				return nil
			}

			backendBP := blueprintWithComponents(blueprint, backendComponents[len(backendComponents)-1:])
			backendResult, backendErr := i.destroyAllTerraform(backendBP, continueOnError)
			result.Destroyed = append(result.Destroyed, backendResult.Destroyed...)
			result.Skipped = mergeSkipped(result.Skipped, backendResult.Skipped)
			result.Failed = append(result.Failed, backendResult.Failed...)
			return backendErr
		}

		backendComponentsResult, destroyErr := i.destroyAllTerraform(backendComponentsBP, continueOnError)
		result.Destroyed = append(result.Destroyed, backendComponentsResult.Destroyed...)
		result.Skipped = mergeSkipped(result.Skipped, mergeSkipped(migrationSkipped, backendComponentsResult.Skipped))
		result.Failed = append(result.Failed, backendComponentsResult.Failed...)
		return destroyErr
	})
	return result, err
}

// TeardownComponent destroys a single terraform component. Targeting any backend
// component on a non-local backend is refused: its state provides the backend that
// other components rely on, so destroying it in isolation would orphan their state.
// Use `windsor destroy` (no arguments) for the full-cycle teardown.
func (i *Provisioner) TeardownComponent(blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
	if err := i.CheckComponentDestroyable(blueprint, componentID); err != nil {
		return false, err
	}
	if err := i.checkOrphanedLocalState(blueprint); err != nil {
		return false, err
	}
	return i.Destroy(blueprint, componentID)
}

// CheckComponentDestroyable reports whether a single terraform component may be destroyed in isolation.
// On a non-local backend a backend component is refused: its state provides the backend every other
// component uses, so destroying it directly would orphan their state. Callers run this before generating a
// destroy plan so the refusal is surfaced up front, rather than as a raw terraform init error when the
// component tries to reach a kubernetes backend whose cluster may already be gone. A Backend that names
// no real component also refuses outright — see resolveBackendComponents.
func (i *Provisioner) CheckComponentDestroyable(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	backendType := i.configHandler.GetTerraformBackendType()
	if backendType == "" || backendType == "local" {
		return nil
	}
	backendComponents, err := resolveBackendComponents(blueprint)
	if err != nil {
		return err
	}
	for _, c := range backendComponents {
		if c.GetID() == componentID {
			return fmt.Errorf("cannot destroy backend component %q in isolation: its state provides the %s backend that every other component uses, so destroying it directly would orphan their state. Run `windsor destroy` (no arguments) for the full-cycle teardown that migrates state to local first", componentID, backendType)
		}
	}
	return nil
}

// ValidateBackendComponents refuses a Blueprint.Backend that names no real component, before a
// destroy plan is generated.
func (i *Provisioner) ValidateBackendComponents(blueprint *blueprintv1alpha1.Blueprint) error {
	_, err := resolveBackendComponents(blueprint)
	return err
}

// PrepareLocalTeardown migrates every component's state to local and pivots terraform.backend.type
// to local, so a kubernetes-backend teardown never dials the cluster it's about to destroy. A
// migration failure aborts only while the cluster is still reachable; once it's gone, state was
// already migrated on an earlier pass. No-op for a non-kubernetes backend.
func (i *Provisioner) PrepareLocalTeardown(blueprint *blueprintv1alpha1.Blueprint) (bool, error) {
	backendType := i.configHandler.GetTerraformBackendType()
	if backendType != "kubernetes" {
		return false, nil
	}

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

// PivotToLocalIfClusterGone pivots terraform.backend.type to local, without migrating, when the
// kubernetes backend's cluster is gone or unreachable — the targeted-destroy counterpart to
// PrepareLocalTeardown, reading state a prior full teardown already migrated to local. No-op
// otherwise.
func (i *Provisioner) PivotToLocalIfClusterGone() (bool, error) {
	backendType := i.configHandler.GetTerraformBackendType()
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

// clusterReachableForTeardown reports whether the kubernetes-backend cluster is present and reachable.
func (i *Provisioner) clusterReachableForTeardown() bool {
	if !i.kubeconfigPresent() {
		return false
	}
	return i.checkKubernetesReachableForDestroy() == nil
}

// =============================================================================
// Private Helpers
// =============================================================================

// resolveBackendComponents resolves the components named by Blueprint.Backend.
// It runs ValidateComposedBlueprint first: destroy (and down/env) skip
// blueprint validation at load time, so a stale Backend name must still
// fail loud here. Apply already validates at load, so this same check is
// defense-in-depth there, not load-bearing.
func resolveBackendComponents(blueprint *blueprintv1alpha1.Blueprint) ([]*blueprintv1alpha1.TerraformComponent, error) {
	if err := composerblueprint.ValidateComposedBlueprint(blueprint); err != nil {
		return nil, err
	}
	return blueprint.BackendComponents(), nil
}

// hasTerraformFailure reports whether the failure list contains any entry
// that belongs to a terraform component (i.e., not the kustomize-aggregate
// sentinel). A terraform-component failure always blocks the next destroy
// stage; see blocksNextStage for the full gate, which also accounts for
// kustomize failures.
func hasTerraformFailure(failed []ComponentFailure) bool {
	for _, f := range failed {
		if f.ID != KustomizeFailureID {
			return true
		}
	}
	return false
}

// blocksNextStage reports whether failed should stop the next destroy stage. A terraform
// failure always blocks; a kustomize failure blocks only while the cluster is reachable, since
// a live controller may still be tearing down a resource terraform never tracked.
func (i *Provisioner) blocksNextStage(failed []ComponentFailure) bool {
	if hasTerraformFailure(failed) {
		return true
	}
	return len(failed) > 0 && i.clusterReachableForTeardown()
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
