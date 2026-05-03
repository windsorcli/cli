package provisioner

import (
	"fmt"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Public Methods
// =============================================================================

// Teardown is Bootstrap reversed. With a pivot: destroy non-pivot components
// against the live remote, pin local, migrate the pivot's state to local,
// destroy the pivot. Without a pivot: plain DestroyAll. terraformOnly=true
// skips the kustomize uninstall. Returns IDs of components skipped because
// their state was empty alongside any error.
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
// bootstrap pivot is refused for non-local backends — its state hosts the
// remote backend other components rely on, so destroying it in isolation
// would orphan their state. Use full-cycle `windsor destroy` instead.
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

// runPivotDestroy is Bootstrap's per-pivot dance reversed: destroy non-pivot
// components against the live remote first, then pin local, migrate the
// pivot's state to local, and destroy the pivot. configHandler is restored
// on defer.
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
