package provisioner

import (
	"fmt"
	"os"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Public Methods
// =============================================================================

// Bootstrap pins terraform.backend.type=local for one Up pass that applies every
// component against local state, then restores the configured backend and
// migrates all component state to it. When the blueprint declares no backend
// component, Bootstrap collapses to a plain Up. Any components whose state
// migration was skipped are returned in the error so the operator sees what
// didn't migrate.
func (i *Provisioner) Bootstrap(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if blueprint.BackendComponentID() == "" {
		return i.Up(blueprint, onApply...)
	}

	if err := i.withBackendOverride("bootstrap apply", func() error {
		return i.Up(blueprint, onApply...)
	}); err != nil {
		return err
	}

	skipped, err := i.MigrateState(blueprint)
	if err != nil {
		if len(skipped) > 0 {
			return fmt.Errorf("%w (skipped components before failure: %s)", err, strings.Join(skipped, ", "))
		}
		return err
	}
	if len(skipped) > 0 {
		return fmt.Errorf("bootstrap state migration skipped components whose directories were missing after apply: %s", strings.Join(skipped, ", "))
	}
	return nil
}

// =============================================================================
// Private Helpers
// =============================================================================

// withBackendOverride pins terraform.backend.type to "local" for the duration of
// fn, restoring the previously-configured value via defer. opLabel appears in
// the override-failed error and the stderr restore-warning. The Set is in-memory
// only — values.yaml on disk is unaffected — so a stale local override after a
// failed restore is bounded to the current process.
func (i *Provisioner) withBackendOverride(opLabel string, fn func() error) error {
	original := i.configHandler.GetString("terraform.backend.type", "local")
	if err := i.configHandler.Set("terraform.backend.type", "local"); err != nil {
		return fmt.Errorf("failed to override backend for %s: %w", opLabel, err)
	}
	defer func() {
		if err := i.configHandler.Set("terraform.backend.type", original); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to restore terraform.backend.type to %q after %s: %v\n", original, opLabel, err)
		}
	}()
	return fn()
}
