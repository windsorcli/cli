package provisioner

import (
	"fmt"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Public Methods
// =============================================================================

// Teardown reverses Bootstrap. With an external-backend pivot it destroys non-pivot
// components against the live remote, then migrates the pivot to local and destroys
// it. With a kubernetes cluster-is-backend pivot it migrates all state out first then
// destroys everything against local. Returns the IDs of components skipped because
// their state was empty. terraformOnly skips the kustomize uninstall.
func (i *Provisioner) Teardown(blueprint *blueprintv1alpha1.Blueprint, terraformOnly bool) ([]string, error) {
	backendType := i.configHandler.GetString("terraform.backend.type", "local")
	p := pivot(blueprint, backendType)

	if p == nil {
		if terraformOnly {
			return i.DestroyAllTerraform(blueprint)
		}
		return i.DestroyAll(blueprint)
	}

	if backendType == "kubernetes" && blueprint.BackendComponentID() == "" {
		return i.runFullMigrationDestroy(blueprint, terraformOnly)
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

// runFullMigrationDestroy is the kubernetes-as-backend teardown. Every
// component's state lives inside the cluster the pivot provisions, so the
// pivot dance (destroy non-pivot first, migrate pivot last) cannot work:
// destroying any component that the cluster depends on kills the backend
// for every remaining component. Instead, pin local up front, migrate
// every component's state out of the cluster, and destroy everything in
// reverse dependency order against local. configHandler is restored on
// defer. Fails closed: if MigrateState errors, no destroy runs.
func (i *Provisioner) runFullMigrationDestroy(blueprint *blueprintv1alpha1.Blueprint, terraformOnly bool) ([]string, error) {
	var skipped []string
	err := i.withBackendOverride("destroy", func() error {
		migrationSkipped, err := i.MigrateState(blueprint)
		if err != nil {
			return err
		}
		var destroySkipped []string
		var bulkErr error
		if terraformOnly {
			destroySkipped, bulkErr = i.DestroyAllTerraform(blueprint)
		} else {
			destroySkipped, bulkErr = i.DestroyAll(blueprint)
		}
		skipped = mergeSkipped(migrationSkipped, destroySkipped)
		return bulkErr
	})
	return skipped, err
}

// mergeSkipped returns the union of two skipped-component lists in input
// order without duplicates. MigrateState and DestroyAll both report
// dir-missing components, so naive concat would double-count on the
// happy path; on the error path MigrateState's list still names
// components DestroyAll didn't reach before bailing out.
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
