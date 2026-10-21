// The destroy methods of the Provisioner remove the terraform components and kustomizations of a blueprint.
// It provides Down, Destroy, DestroyAll, and Teardown, and the checks that run before a destroy.
// It pivots terraform to a local backend when the cluster is gone.

package provisioner

import (
	"context"
	"fmt"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
	composerblueprint "github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/constants"
	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
)

// =============================================================================
// Constants
// =============================================================================

// KustomizeFailureID is the sentinel ID for a kustomize Uninstall failure under
// continue-on-error mode. The kustomize layer reports one aggregate failure, not
// one per Kustomization.
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
// kustomize failure occurred, or a non-backend component still needs work. Add
// fields for other destroy layers, such as Helm, to this type, not to the
// terraform-package outcome.
//
// SkippedClusterGone is separate from Skipped. It names kustomizations skipped
// for a missing kubeconfig, not empty terraform state. An empty terraform
// state may be an orphan hazard. A missing kubeconfig on a destroy re-run
// just means the cluster is already gone. See reportSkippedDestroyComponents
// and reportSkippedClusterGoneKustomizations in cmd/destroy.go.
type DestroyResult struct {
	Destroyed          []string
	Skipped            []string
	SkippedClusterGone []string
	Failed             []ComponentFailure
	TerraformDeferred  bool
}

// =============================================================================
// Public Methods
// =============================================================================

// Down destroys the "workstation" terraform component if it is present in the blueprint, then returns.
// All other terraform components are left untouched; use Destroy / DestroyAll for those.
// If terraform is disabled or the blueprint has no "workstation" component, Down is a no-op.
// Returns an error if the blueprint is nil or the destroy operation fails.
func (i *Provisioner) Down(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	hasWorkstation := false
	for _, c := range blueprint.TerraformComponents {
		if c.GetID() == "workstation" && c.Enabled.IsEnabled() {
			hasWorkstation = true
			break
		}
	}
	if !hasWorkstation {
		return nil
	}

	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return nil
	}
	if _, err := i.TerraformStack.Destroy(ctx, blueprint, "workstation"); err != nil {
		return werror.Wrap(err, "failed to destroy workstation terraform component")
	}
	return nil
}

// DestroyAllTerraform destroys all terraform components in reverse dependency order.
// Components with Destroy set to false, or listed in excludeIDs, are skipped. The
// cmd-layer symmetric-destroy flow excludes the backend component here, to migrate
// and destroy it last.
// Returns the IDs of components skipped for empty state, alongside any error, so
// callers see partial progress even when a later component fails.
func (i *Provisioner) DestroyAllTerraform(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, continueOnError bool, excludeIDs ...string) (DestroyResult, error) {
	return i.destroyAllTerraform(ctx, blueprint, continueOnError, excludeIDs...)
}

// Destroy runs terraform init and destroy for a single component identified by componentID.
// Returns (skipped, nil) when the component's state is empty (nothing to destroy), (false, nil)
// when destroy ran successfully, or (false, err) on any failure. Returns an error if the
// blueprint is nil, terraform is disabled, the stack cannot be initialized, the component is
// not found, or any terraform operation fails. TerraformStack bounds refresh and destroy
// with a timeout, so an unreachable provider fails within that time instead of hanging.
func (i *Provisioner) Destroy(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
	if blueprint == nil {
		return false, fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return false, err
	}
	if i.TerraformStack == nil {
		return false, fmt.Errorf("terraform is disabled")
	}
	skipped, err := i.TerraformStack.Destroy(ctx, blueprint, componentID)
	if err != nil {
		return false, werror.Wrap(err, "failed to run terraform destroy for %s", componentID)
	}
	return skipped, nil
}

// DestroyKustomize deletes one kustomization by name from the cluster. It does nothing when there
// is no local kubeconfig, because the cluster and its kustomizations are already gone.
// It returns an error if the blueprint is nil, the kubernetes manager is not configured, or the
// delete fails. A kustomization that is not in the blueprint returns an error with a remediation hint.
func (i *Provisioner) DestroyKustomize(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	var found *blueprintv1alpha1.Kustomization
	for _, k := range blueprint.AllKustomizations() {
		if k.Name == componentID {
			kCopy := k
			found = &kCopy
			break
		}
	}
	if found == nil {
		return werror.New(fmt.Sprintf("kustomization %q not found in blueprint", componentID), kustomizationNotFoundHint, nil)
	}

	if !i.kubeconfigPresent() {
		return nil
	}

	if err := presenter.Track(ctx, "kustomize:"+componentID, fmt.Sprintf("Destroying kustomization %s", componentID), func(ctx context.Context) error {
		return i.KubernetesManager.DeleteKustomization(ctx, componentID, i.fluxNamespace())
	}); err != nil {
		return werror.Wrap(err, "failed to delete kustomization %s", componentID)
	}

	return nil
}

// DestroyAll destroys every infrastructure component: kustomizations first, then terraform.
// Uninstall is a no-op when no local kubeconfig exists. See kubeconfigPresent. DestroyAll then
// counts eligible kustomizations as SkippedClusterGone, not Destroyed. This keeps `windsor
// destroy` idempotent after a prior run already tore the cluster down.
//
// Under continueOnError, any kustomize failure defers terraform. See blocksNextStage. Windsor
// cannot tell whether a live controller is still mid-delete of a resource terraform never
// tracked, so it never guesses.
//
// excludeIDs skips components in the terraform pass. Callers use this to destroy the backend
// component last.
func (i *Provisioner) DestroyAll(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, continueOnError bool, excludeIDs ...string) (DestroyResult, error) {
	var result DestroyResult
	if blueprint == nil {
		return result, fmt.Errorf("blueprint not provided")
	}

	if i.KubernetesManager != nil {
		clusterGone := !i.kubeconfigPresent()
		if err := i.Uninstall(ctx, blueprint); err != nil {
			if !continueOnError {
				return result, err
			}
			result.Failed = append(result.Failed, ComponentFailure{ID: KustomizeFailureID, Err: err})
			result.TerraformDeferred = true
			return result, nil
		}
		for _, k := range blueprint.AllKustomizations() {
			if !fluxinfra.KustomizationDestroyEligible(k) {
				continue
			}
			if clusterGone {
				result.SkippedClusterGone = append(result.SkippedClusterGone, k.Name)
			} else {
				result.Destroyed = append(result.Destroyed, k.Name)
			}
		}
	}

	if err := i.ensureTerraformStack(); err != nil {
		return result, err
	}
	if i.TerraformStack != nil {
		outcome, err := i.TerraformStack.DestroyAll(ctx, blueprint, continueOnError, excludeIDs...)
		result.Destroyed = append(result.Destroyed, outcome.Destroyed...)
		result.Skipped = append(result.Skipped, outcome.Skipped...)
		result.Failed = append(result.Failed, outcome.Failed...)
		if err != nil {
			return result, werror.Wrap(err, "failed to run terraform destroy")
		}
	}

	return result, nil
}

// Teardown reverses Bootstrap. With no declared backend, it calls DestroyAll, or DestroyAllTerraform
// when terraformOnly is true. With a backend, it destroys the non-backend components, then the other
// components of the backend, then the backend component. A stage runs only if the stage before it had
// no failure that blocksNextStage reports. TerraformDeferred marks a skipped stage. A kubernetes backend
// type with no declared backend returns an error with a remediation hint.
func (i *Provisioner) Teardown(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, terraformOnly bool, continueOnError bool) (DestroyResult, error) {
	var result DestroyResult
	backendType := i.configHandler.GetTerraformBackendType()
	if backendType == "kubernetes" && blueprint.Backend == "" {
		return result, werror.New("blueprint configures terraform.backend.type=kubernetes but does not declare Blueprint.Backend", "Set backend: <cluster-component-id> at the top level of the blueprint to name the terraform component that creates the cluster.", nil)
	}
	if err := i.checkOrphanedLocalState(blueprint); err != nil {
		return result, err
	}

	destroyFlat := func() (DestroyResult, error) {
		if terraformOnly {
			return i.DestroyAllTerraform(ctx, blueprint, continueOnError)
		}
		return i.DestroyAll(ctx, blueprint, continueOnError)
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
		result, stage1Err = i.DestroyAllTerraform(ctx, blueprint, continueOnError, backendComponentIDs...)
	} else {
		result, stage1Err = i.DestroyAll(ctx, blueprint, continueOnError, backendComponentIDs...)
	}
	if stage1Err != nil {
		return result, stage1Err
	}

	if blocksNextStage(result.Failed) {
		result.TerraformDeferred = true
		return result, nil
	}

	backendComponentsBP := blueprintWithComponents(blueprint, backendComponents)
	err = i.withBackendOverride("destroy", func() error {
		migrationSkipped, err := i.MigrateState(ctx, backendComponentsBP)
		if err != nil {
			return err
		}

		if len(backendComponents) > 1 {
			membersResult, destroyErr := i.destroyAllTerraform(ctx, backendComponentsBP, continueOnError, blueprint.Backend)
			result.Destroyed = append(result.Destroyed, membersResult.Destroyed...)
			result.Skipped = mergeSkipped(result.Skipped, mergeSkipped(migrationSkipped, membersResult.Skipped))
			result.Failed = append(result.Failed, membersResult.Failed...)
			if destroyErr != nil {
				return destroyErr
			}
			if blocksNextStage(membersResult.Failed) {
				result.TerraformDeferred = true
				return nil
			}

			backendBP := blueprintWithComponents(blueprint, backendComponents[len(backendComponents)-1:])
			backendResult, backendErr := i.destroyAllTerraform(ctx, backendBP, continueOnError)
			result.Destroyed = append(result.Destroyed, backendResult.Destroyed...)
			result.Skipped = mergeSkipped(result.Skipped, backendResult.Skipped)
			result.Failed = append(result.Failed, backendResult.Failed...)
			return backendErr
		}

		backendComponentsResult, destroyErr := i.destroyAllTerraform(ctx, backendComponentsBP, continueOnError)
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
func (i *Provisioner) TeardownComponent(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
	if err := i.CheckComponentDestroyable(blueprint, componentID); err != nil {
		return false, err
	}
	if err := i.checkOrphanedLocalState(blueprint); err != nil {
		return false, err
	}
	return i.Destroy(ctx, blueprint, componentID)
}

// CheckComponentDestroyable reports whether one terraform component can be destroyed on its own.
// On a remote backend, it refuses a backend component, because the other components keep their state
// in that backend. The refusal returns an error with a remediation hint. Callers run it before a destroy
// plan, so the refusal shows before terraform init fails against a cluster that can be gone.
// It also refuses a Backend that names no component, as resolveBackendComponents does.
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
			return werror.New(fmt.Sprintf("cannot destroy backend component %q in isolation: its state provides the %s backend that every other component uses, so destroying it directly would orphan their state", componentID, backendType), "Run windsor destroy with no arguments. It migrates the state to local before it destroys the backend component.", nil)
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
func (i *Provisioner) PrepareLocalTeardown(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) (bool, error) {
	backendType := i.configHandler.GetTerraformBackendType()
	if backendType != "kubernetes" {
		return false, nil
	}

	if err := i.configHandler.Set("terraform.backend.type", "local"); err != nil {
		return false, werror.Wrap(err, "failed to pivot terraform backend to local for teardown")
	}

	if _, err := i.MigrateState(ctx, blueprint); err != nil {
		if i.clusterReachableForTeardown() {
			_ = i.configHandler.Set("terraform.backend.type", backendType)
			return false, werror.Wrap(err, "failed to migrate terraform state to local before teardown")
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
		return false, werror.Wrap(err, "failed to pivot terraform backend to local for teardown")
	}
	return true, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// destroyAllTerraform is the shared implementation behind DestroyAllTerraform.
func (i *Provisioner) destroyAllTerraform(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, continueOnError bool, excludeIDs ...string) (DestroyResult, error) {
	var result DestroyResult
	if blueprint == nil {
		return result, fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return result, err
	}
	if i.TerraformStack == nil {
		return result, fmt.Errorf("terraform is disabled")
	}
	outcome, err := i.TerraformStack.DestroyAll(ctx, blueprint, continueOnError, excludeIDs...)
	result.Destroyed = outcome.Destroyed
	result.Skipped = outcome.Skipped
	result.Failed = outcome.Failed
	if err != nil {
		return result, werror.Wrap(err, "failed to run terraform destroy")
	}
	return result, nil
}

// checkKubernetesReachableForDestroy fails fast if the cluster is unreachable, so terraform's
// kubernetes/helm provider doesn't hang against broken auth. No-op when there's no kubeconfig.
func (i *Provisioner) checkKubernetesReachableForDestroy() error {
	if i.KubernetesManager == nil || !i.kubeconfigPresent() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultKubernetesReachabilityCheckTimeout)
	defer cancel()
	if err := i.KubernetesManager.WaitForKubernetesHealthy(ctx, "", nil); err != nil {
		return werror.Wrap(err, "kubernetes API is unreachable, refusing to run terraform destroy (its kubernetes/helm provider would hang against a broken cluster)")
	}
	return nil
}

// clusterReachableForTeardown reports whether the kubernetes-backend cluster is present and reachable.
func (i *Provisioner) clusterReachableForTeardown() bool {
	if !i.kubeconfigPresent() {
		return false
	}
	return i.checkKubernetesReachableForDestroy() == nil
}

// =============================================================================
// Helpers
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

// blocksNextStage reports whether any failure — kustomize or terraform — should stop the
// destroy from proceeding to its next stage. There is no reachability exception: Windsor has
// no way to tell whether a stuck kustomize teardown still has a live controller mid-delete of
// a resource terraform never tracked, so any failure is treated as blocking.
func blocksNextStage(failed []ComponentFailure) bool {
	return len(failed) > 0
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
