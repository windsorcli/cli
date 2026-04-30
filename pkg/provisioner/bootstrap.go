package provisioner

import (
	"fmt"
	"os"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Types
// =============================================================================

// BootstrapConfirmFn receives the plan summary that Bootstrap is about to apply
// and returns true to proceed with apply or false to skip cleanly. When nil is
// passed to Bootstrap, the apply runs unconditionally (no plan generated).
type BootstrapConfirmFn func(*PlanSummary) bool

// =============================================================================
// Public Methods
// =============================================================================

// Bootstrap branches deterministically on whether the configured remote backend
// is reachable, then runs plan + confirm + apply through one shared code path.
//
//   - Already-bootstrapped (probe succeeds): the configured remote backend exists
//     and contains real state. Bootstrap behaves like a regular up — plan and
//     apply against the remote backend, no override, no migration.
//   - Fresh (probe fails): no remote backend resource yet, or no backend
//     component in the blueprint. Bootstrap pins terraform.backend.type=local
//     for one apply pass that creates the backend infrastructure (and
//     everything else) against local state, then migrates all component state
//     to the configured remote backend.
//
// The branch signal is the cloud resource itself (probed via terraform init in
// a scratch directory), not any local-on-disk marker — so two operators on
// different machines see the same answer for the same context. Bootstrap is
// idempotent across machines and across re-runs.
//
// When confirm is non-nil, the combined Terraform + Kustomize plan summary
// passes through confirm before the apply runs. Returning false from confirm
// aborts cleanly with applied=false. Per-component plan failures are recorded
// as `(error: ...)` rows in the summary rather than aborting the whole plan.
//
// Any components whose state migration was skipped are returned in the error
// so the operator sees what didn't migrate.
func (i *Provisioner) Bootstrap(blueprint *blueprintv1alpha1.Blueprint, confirm BootstrapConfirmFn, onApply ...func(id string) error) (bool, error) {
	if blueprint == nil {
		return false, fmt.Errorf("blueprint not provided")
	}

	if blueprint.BackendComponentID() == "" || i.backendReachable(blueprint) {
		return i.runPlanThenApply(blueprint, confirm, onApply...)
	}

	var applied bool
	if err := i.withBackendOverride("bootstrap", func() error {
		ok, err := i.runPlanThenApply(blueprint, confirm, onApply...)
		if err != nil {
			return err
		}
		applied = ok
		return nil
	}); err != nil {
		return false, err
	}
	if !applied {
		return false, nil
	}

	skipped, err := i.MigrateState(blueprint)
	if err != nil {
		if len(skipped) > 0 {
			return false, fmt.Errorf("%w (skipped components before failure: %s)", err, strings.Join(skipped, ", "))
		}
		return false, err
	}
	if len(skipped) > 0 {
		return false, fmt.Errorf("bootstrap state migration skipped components whose directories were missing after apply: %s", strings.Join(skipped, ", "))
	}
	return true, nil
}

// =============================================================================
// Private Helpers
// =============================================================================

// runPlanThenApply runs an optional plan + confirm cycle followed by Up. When
// confirm is nil, Up runs unconditionally. When confirm is non-nil, the
// combined Terraform + Kustomize plan summary is generated and passed to it;
// returning false aborts cleanly with applied=false. Shared by both Bootstrap
// branches (configured-backend and local-override) so plan, confirm, and apply
// stay in lockstep.
func (i *Provisioner) runPlanThenApply(blueprint *blueprintv1alpha1.Blueprint, confirm BootstrapConfirmFn, onApply ...func(id string) error) (bool, error) {
	if confirm != nil {
		summary, err := i.PlanAll(blueprint)
		if err != nil {
			return false, err
		}
		if !confirm(summary) {
			return false, nil
		}
	}
	if err := i.Up(blueprint, onApply...); err != nil {
		return false, err
	}
	return true, nil
}

// backendReachable returns true when the configured remote backend's underlying
// resource exists and is reachable, false otherwise. Returns false safely (no
// panic) when the terraform stack cannot be initialised — Bootstrap then takes
// the fresh-install local-override path, which sets up backend infrastructure
// from scratch.
func (i *Provisioner) backendReachable(blueprint *blueprintv1alpha1.Blueprint) bool {
	if err := i.ensureTerraformStack(); err != nil {
		return false
	}
	if i.TerraformStack == nil {
		return false
	}
	return i.TerraformStack.BackendReachable(blueprint)
}

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
