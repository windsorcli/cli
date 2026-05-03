package provisioner

import (
	"fmt"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Public Methods
// =============================================================================

// Teardown is the symmetric reverse of Bootstrap. It identifies the bootstrap
// pivot — the terraform module whose apply provisions the storage backing the
// configured remote backend — and runs the dance backwards:
//
//  1. DestroyAll(exclude=pivotID) against the live remote backend (the pivot,
//     and the storage it created, are still healthy).
//  2. Pin backend.type=local in-memory.
//  3. MigrateComponentState(pivotID) from the remote backend to local files.
//  4. Destroy(pivotID) against local state.
//
// When the blueprint has no pivot (local backend, or a remote backend whose
// storage was provisioned out of band), Teardown collapses to a direct
// DestroyAll. terraformOnly=true skips the kustomize uninstall. Returns the
// IDs of components skipped because their state was empty alongside any error.
func (i *Provisioner) Teardown(blueprint *blueprintv1alpha1.Blueprint, terraformOnly bool) ([]string, error) {
	backendType := i.configHandler.GetString("terraform.backend.type", "local")
	p := pivot(blueprint, backendType)

	if p == nil {
		if terraformOnly {
			return i.DestroyAllTerraform(blueprint)
		}
		return i.DestroyAll(blueprint)
	}

	return i.runPivotDestroy(blueprint, p.GetID(), terraformOnly)
}

// TeardownComponent destroys a single terraform component. Targeting the
// bootstrap pivot is refused for any non-local backend — its state hosts the
// remote backend that every other component relies on, so destroying it in
// isolation would orphan their state. Full-cycle `windsor destroy` is the
// only safe path; it migrates the pivot's state to local first.
func (i *Provisioner) TeardownComponent(blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
	backendType := i.configHandler.GetString("terraform.backend.type", "local")
	if p := pivot(blueprint, backendType); p != nil && componentID == p.GetID() {
		return false, fmt.Errorf("cannot destroy the bootstrap pivot component %q in isolation: its state provides the %s backend that every other component uses, so destroying it directly would orphan their state. Run `windsor destroy` (no arguments) for the full-cycle teardown that migrates state to local first", componentID, backendType)
	}

	return i.Destroy(blueprint, componentID)
}

// =============================================================================
// Private Helpers
// =============================================================================

// runPivotDestroy is the symmetric reverse of bootstrap's per-pivot dance:
// rest first, pivot last. Non-pivot components are destroyed against the live
// remote backend (still healthy because the pivot — the module backing it —
// has not been touched yet). Then backend.type flips to local, the pivot's
// state migrates from remote to local, and the pivot is destroyed against
// local state. The configured backend is restored on defer for any subsequent
// operations in the same process.
func (i *Provisioner) runPivotDestroy(blueprint *blueprintv1alpha1.Blueprint, pivotID string, terraformOnly bool) ([]string, error) {
	var skipped []string
	var bulkErr error
	if terraformOnly {
		skipped, bulkErr = i.DestroyAllTerraform(blueprint, pivotID)
	} else {
		skipped, bulkErr = i.DestroyAll(blueprint, pivotID)
	}
	if bulkErr != nil {
		return skipped, bulkErr
	}

	err := i.withBackendOverride("destroy", func() error {
		if err := i.MigrateComponentState(blueprint, pivotID); err != nil {
			return err
		}
		pivotSkipped, err := i.Destroy(blueprint, pivotID)
		if err != nil {
			return err
		}
		if pivotSkipped {
			skipped = append(skipped, pivotID)
		}
		return nil
	})
	return skipped, err
}
