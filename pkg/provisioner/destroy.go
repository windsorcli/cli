package provisioner

import (
	"fmt"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Public Methods
// =============================================================================

// Teardown reverses Bootstrap. With no backend tier it forwards to DestroyAll
// (or DestroyAllTerraform when terraformOnly). With a tier declared via
// Blueprint.Backend, Stage 1 destroys non-tier components against the
// configured backend, then Stage 2 pins local, pulls every tier member's state
// to local, and destroys the tier in reverse declaration order. Returns the
// IDs of components skipped because their state was empty.
func (i *Provisioner) Teardown(blueprint *blueprintv1alpha1.Blueprint, terraformOnly bool) ([]string, error) {
	backendType := i.configHandler.GetString("terraform.backend.type", "local")
	tier := blueprint.BackendTier()
	if backendType == "" || backendType == "local" || len(tier) == 0 {
		if terraformOnly {
			return i.DestroyAllTerraform(blueprint)
		}
		return i.DestroyAll(blueprint)
	}

	tierIDs := make([]string, 0, len(tier))
	for _, c := range tier {
		tierIDs = append(tierIDs, c.GetID())
	}

	var skipped []string
	var stage1Err error
	if terraformOnly {
		skipped, stage1Err = i.DestroyAllTerraform(blueprint, tierIDs...)
	} else {
		skipped, stage1Err = i.DestroyAll(blueprint, tierIDs...)
	}
	if stage1Err != nil {
		return skipped, stage1Err
	}

	tierBP := blueprintWithComponents(blueprint, tier)
	err := i.withBackendOverride("destroy", func() error {
		migrationSkipped, err := i.MigrateState(tierBP)
		if err != nil {
			return err
		}
		destroySkipped, destroyErr := i.DestroyAllTerraform(tierBP)
		skipped = mergeSkipped(skipped, mergeSkipped(migrationSkipped, destroySkipped))
		return destroyErr
	})
	return skipped, err
}

// TeardownComponent destroys a single terraform component. Targeting any
// backend-tier member on a non-local backend is refused: its state provides
// the backend that other components rely on, so destroying it in isolation
// would orphan their state. Use `windsor destroy` (no arguments) for the
// full-cycle teardown.
func (i *Provisioner) TeardownComponent(blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
	backendType := i.configHandler.GetString("terraform.backend.type", "local")
	if backendType != "" && backendType != "local" && blueprint.IsBackendTierMember(componentID) {
		return false, fmt.Errorf("cannot destroy backend-tier component %q in isolation: its state provides the %s backend that every other component uses, so destroying it directly would orphan their state. Run `windsor destroy` (no arguments) for the full-cycle teardown that migrates state to local first", componentID, backendType)
	}
	return i.Destroy(blueprint, componentID)
}

// =============================================================================
// Private Helpers
// =============================================================================

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
