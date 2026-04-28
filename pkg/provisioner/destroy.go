package provisioner

import (
	"fmt"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Public Methods
// =============================================================================

// Teardown dispatches bulk destroy on terraform.backend.type:
//
//   - kubernetes → full-cycle: pin backend.type=local, MigrateState pulls every
//     component's state from the cluster's Secrets to local files, then DestroyAll
//     tears everything down against the local state. Mirror of bootstrap's
//     full-cycle on the destroy side.
//   - s3 / azurerm → per-component dance: destroy non-backend components FIRST
//     against the live remote backend, then pin backend.type=local, migrate the
//     backend component's state from remote to local, destroy the backend last.
//   - local / unset → direct DestroyAll, no migration.
//
// terraformOnly=true skips the kustomize uninstall. Returns the IDs of components
// skipped because their state was empty alongside any error.
func (i *Provisioner) Teardown(blueprint *blueprintv1alpha1.Blueprint, terraformOnly bool) ([]string, error) {
	switch i.configHandler.GetString("terraform.backend.type", "local") {
	case "kubernetes":
		return i.runFullCycleDestroyAll(blueprint, terraformOnly)
	case "s3", "azurerm":
		return i.runPerComponentDestroyAll(blueprint, terraformOnly)
	default:
		if terraformOnly {
			return i.DestroyAllTerraform(blueprint)
		}
		return i.DestroyAll(blueprint)
	}
}

// TeardownComponent destroys a single terraform component. Targeting the backend
// component itself is refused for any remote backend (kubernetes, s3, azurerm) —
// the backend's storage hosts state for every other component, so destroying it
// in isolation would orphan their state. Full-cycle `windsor destroy` is the
// only safe path; it migrates state to local first.
func (i *Provisioner) TeardownComponent(blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
	backendType := i.configHandler.GetString("terraform.backend.type", "local")
	backendID := blueprint.BackendComponentID()

	if backendType != "local" && backendID != "" && componentID == backendID {
		return false, fmt.Errorf("cannot destroy the %s backend component %q in isolation: its storage holds terraform state for every other component, so destroying it directly would orphan their state. Run `windsor destroy` (no arguments) for the full-cycle teardown that migrates state to local first", backendType, componentID)
	}

	return i.Destroy(blueprint, componentID)
}

// =============================================================================
// Private Helpers
// =============================================================================

// runFullCycleDestroyAll is the destroy mirror of bootstrap's full-cycle path. The
// backend (kubernetes Secrets in the cluster) is going away as part of this
// teardown, so every component's state must move to local before any destroy
// runs — otherwise destroy-in-progress would race with the cluster's Secret
// store evaporating. After migration, DestroyAll iterates components in reverse
// against the local state.
func (i *Provisioner) runFullCycleDestroyAll(blueprint *blueprintv1alpha1.Blueprint, terraformOnly bool) ([]string, error) {
	var skipped []string
	err := i.withBackendOverride("destroy", func() error {
		var migErr error
		skipped, migErr = i.MigrateState(blueprint)
		if migErr != nil {
			if len(skipped) > 0 {
				return fmt.Errorf("failed to migrate state to local before destroy: %w (skipped components before failure: %s)", migErr, strings.Join(skipped, ", "))
			}
			return fmt.Errorf("failed to migrate state to local before destroy: %w", migErr)
		}
		if len(skipped) > 0 {
			return fmt.Errorf("kubernetes full-cycle destroy aborted: state migration skipped components whose directories were missing (%s); their state may still live on the cluster's Secret store, which is about to be destroyed — running destroy now would orphan the underlying cloud resources. Restore the missing component directories and re-run, or destroy them individually first", strings.Join(skipped, ", "))
		}

		var destroyErr error
		if terraformOnly {
			skipped, destroyErr = i.DestroyAllTerraform(blueprint)
		} else {
			skipped, destroyErr = i.DestroyAll(blueprint)
		}
		return destroyErr
	})
	if err != nil {
		return nil, err
	}
	return skipped, nil
}

// runPerComponentDestroyAll is the destroy mirror of bootstrap's per-component
// path. When the blueprint declares a "backend" terraform component and the
// configured backend is s3/azurerm, non-backend components are destroyed first
// against the live remote backend, then backend.type flips to local, the backend
// component's state is migrated from remote to local, and the backend component
// is destroyed last. Without a backend component the call collapses to a direct
// DestroyAll — out-of-band bucket setups stay on remote state through teardown.
func (i *Provisioner) runPerComponentDestroyAll(blueprint *blueprintv1alpha1.Blueprint, terraformOnly bool) ([]string, error) {
	backendID := blueprint.BackendComponentID()

	if backendID == "" {
		if terraformOnly {
			return i.DestroyAllTerraform(blueprint)
		}
		return i.DestroyAll(blueprint)
	}

	var skipped []string
	var bulkErr error
	if terraformOnly {
		skipped, bulkErr = i.DestroyAllTerraform(blueprint, backendID)
	} else {
		skipped, bulkErr = i.DestroyAll(blueprint, backendID)
	}
	if bulkErr != nil {
		return skipped, bulkErr
	}

	err := i.withBackendOverride("backend-component destroy", func() error {
		if err := i.MigrateComponentState(blueprint, backendID); err != nil {
			return err
		}
		backendSkipped, err := i.Destroy(blueprint, backendID)
		if err != nil {
			return err
		}
		if backendSkipped {
			skipped = append(skipped, backendID)
		}
		return nil
	})
	return skipped, err
}
