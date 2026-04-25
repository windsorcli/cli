package provisioner

import (
	"fmt"
	"os"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Public Methods
// =============================================================================

// DestroyAllWithBackendLifecycle dispatches bulk destroy on terraform.backend.type:
//
//   - kubernetes  → full-cycle: pin backend.type=local, MigrateState pulls every
//     component's state from the cluster's Secrets to local files, then DestroyAll
//     tears everything down in reverse dependency order against the local state
//     (which is critical because the cluster itself is going away). Mirror image
//     of bootstrap's full-cycle path on the destroy side.
//
//   - s3 / azurerm → per-component dance: destroy non-backend components FIRST
//     against the live remote backend (init reads remote, destroy writes remote),
//     then pin backend.type=local, migrate the backend component's state from
//     remote to local, destroy the backend component last. Avoids the wasteful
//     "migrate everything to local first" pattern for backends where every
//     non-backend component's state is independent of the backend bucket.
//
//   - local / unset → direct DestroyAll, no migration.
//
// terraformOnly=true skips the kustomize uninstall that DestroyAll normally runs
// first. Returns the IDs of components skipped because their state was empty
// alongside any error, mirroring DestroyAll's contract.
func (i *Provisioner) DestroyAllWithBackendLifecycle(blueprint *blueprintv1alpha1.Blueprint, terraformOnly bool) ([]string, error) {
	originalBackend := i.configHandler.GetString("terraform.backend.type", "local")

	switch originalBackend {
	case "kubernetes":
		return i.runFullCycleDestroyAll(blueprint, terraformOnly, originalBackend)
	case "s3", "azurerm":
		return i.runPerComponentDestroyAll(blueprint, terraformOnly, originalBackend)
	default:
		if terraformOnly {
			return i.DestroyAllTerraform(blueprint)
		}
		return i.DestroyAll(blueprint)
	}
}

// DestroyTerraformComponentWithBackendLifecycle destroys a single terraform
// component. The kubernetes path is intentionally simple: direct destroy with
// kubeconfig already in env from a prior `windsor up` — operators destroying
// individual components don't need the full-cycle dance because the cluster is
// still alive and serving Secrets. For s3/azurerm, the backend component
// special-case applies: migrate its state to local first, then destroy. All
// other backends/components fall through to a direct destroy. Returns the same
// (skipped, err) tuple as Destroy.
func (i *Provisioner) DestroyTerraformComponentWithBackendLifecycle(blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
	originalBackend := i.configHandler.GetString("terraform.backend.type", "local")
	if originalBackend != "s3" && originalBackend != "azurerm" {
		return i.Destroy(blueprint, componentID)
	}

	backendID := findBackendComponentID(blueprint)
	if componentID != backendID {
		return i.Destroy(blueprint, componentID)
	}

	if err := i.configHandler.Set("terraform.backend.type", "local"); err != nil {
		return false, fmt.Errorf("failed to override backend for backend-component destroy: %w", err)
	}
	defer func() {
		if err := i.configHandler.Set("terraform.backend.type", originalBackend); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to restore terraform.backend.type to %q after destroy: %v\n", originalBackend, err)
		}
	}()

	if err := i.MigrateComponentState(blueprint, componentID); err != nil {
		return false, err
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
// against the local state. The deferred restore guards exit paths between the
// override and any panic; restoring twice is idempotent.
func (i *Provisioner) runFullCycleDestroyAll(blueprint *blueprintv1alpha1.Blueprint, terraformOnly bool, originalBackend string) ([]string, error) {
	if err := i.configHandler.Set("terraform.backend.type", "local"); err != nil {
		return nil, fmt.Errorf("failed to override backend for destroy: %w", err)
	}
	defer func() {
		if err := i.configHandler.Set("terraform.backend.type", originalBackend); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to restore terraform.backend.type to %q after destroy: %v\n", originalBackend, err)
		}
	}()

	if _, err := i.MigrateState(blueprint); err != nil {
		return nil, fmt.Errorf("failed to migrate state to local before destroy: %w", err)
	}

	if terraformOnly {
		return i.DestroyAllTerraform(blueprint)
	}
	return i.DestroyAll(blueprint)
}

// runPerComponentDestroyAll is the destroy mirror of bootstrap's per-component
// path. When the blueprint declares a "backend" terraform component and the
// configured backend is s3/azurerm, non-backend components are destroyed first
// against the live remote backend (their state is independent of any one
// component, the bucket still exists), then backend.type flips to local, the
// backend component's state is migrated from remote to local, and the backend
// component is destroyed last. Without a backend component the call collapses
// to a direct DestroyAll — out-of-band bucket setups stay on remote state
// through teardown.
func (i *Provisioner) runPerComponentDestroyAll(blueprint *blueprintv1alpha1.Blueprint, terraformOnly bool, originalBackend string) ([]string, error) {
	backendID := findBackendComponentID(blueprint)

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

	if err := i.configHandler.Set("terraform.backend.type", "local"); err != nil {
		return skipped, fmt.Errorf("failed to override backend for backend-component destroy: %w", err)
	}
	defer func() {
		if err := i.configHandler.Set("terraform.backend.type", originalBackend); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to restore terraform.backend.type to %q after destroy: %v\n", originalBackend, err)
		}
	}()

	if err := i.MigrateComponentState(blueprint, backendID); err != nil {
		return skipped, err
	}
	backendSkipped, err := i.Destroy(blueprint, backendID)
	if err != nil {
		return skipped, err
	}
	if backendSkipped {
		skipped = append(skipped, backendID)
	}
	return skipped, nil
}

// findBackendComponentID returns the ID of the terraform component that bootstraps the
// remote state backend, or "" if no such component is declared. The backend component
// is identified by GetID() == "backend"; this is the same convention enforced by
// blueprint.ValidateComposedBlueprint, so the lookup here matches the validator's view.
// Out-of-band buckets (no backend component in the blueprint) return "" and let callers
// collapse to a direct destroy without the migration dance.
func findBackendComponentID(blueprint *blueprintv1alpha1.Blueprint) string {
	if blueprint == nil {
		return ""
	}
	for i := range blueprint.TerraformComponents {
		c := blueprint.TerraformComponents[i]
		if c.GetID() == "backend" {
			return c.GetID()
		}
	}
	return ""
}
