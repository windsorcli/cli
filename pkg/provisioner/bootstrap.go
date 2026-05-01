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

// BootstrapSummary describes the operator-visible intent of a bootstrap call:
// which context, which backend type, which terraform components and
// kustomizations are about to be applied. It is built directly from the
// blueprint and config — no terraform invocation, no state probing, no diff
// — so reruns and multi-machine invocations show the same content. The cmd
// layer renders this for the operator's confirmation prompt.
type BootstrapSummary struct {
	ContextName string
	BackendType string
	Terraform   []BootstrapTerraformEntry
	Kustomize   []string
}

// BootstrapTerraformEntry is a single row in the Terraform section of the
// bootstrap summary. Path is the blueprint Path (e.g. "cluster/aws-eks") when
// set; ComponentID is the unique identifier (Name when set, else Path) and is
// used as the fallback when Path is empty.
type BootstrapTerraformEntry struct {
	ComponentID string
	Path        string
}

// BootstrapConfirmFn receives the bootstrap summary and returns true to
// proceed with apply or false to abort cleanly. Passing nil to Bootstrap
// runs the apply unconditionally (no summary generated, no prompt).
type BootstrapConfirmFn func(*BootstrapSummary) bool

// =============================================================================
// Public Methods
// =============================================================================

// Bootstrap brings up a context's infrastructure end-to-end. The path it takes
// depends on what the blueprint and config declare, with no probing or state
// detection of any kind:
//
//   - When the blueprint declares a "backend" terraform component, Bootstrap
//     pins terraform.backend.type=local in-memory for one Up pass that applies
//     all components against local state, then migrates state to the
//     configured remote backend via MigrateState. This is the path that
//     resolves the chicken-and-egg of "the remote state store lives in
//     infrastructure terraform must create first."
//
//   - When the blueprint has no backend component, Bootstrap calls Up
//     directly against whatever backend is configured. This covers two
//     scenarios with one branch: a local backend (no remote state at all)
//     and an out-of-band remote backend (the bucket/secret/container was
//     created externally and configured in windsor.yaml).
//
// Bootstrap does not try to detect whether a remote backend resource already
// exists. Re-running bootstrap on an already-bootstrapped stack will fail at
// apply time when the cloud rejects "create" against existing infra; the
// confirmBootstrapIfContextExists prompt and the bootstrap summary both warn
// before the apply happens.
//
// When confirm is non-nil, a BootstrapSummary built from the blueprint and
// config is passed to confirm. Returning false aborts with applied=false and
// no error.
//
// Any components whose state migration was skipped are returned in the error
// so the operator sees what didn't migrate.
func (i *Provisioner) Bootstrap(blueprint *blueprintv1alpha1.Blueprint, confirm BootstrapConfirmFn, onApply ...func(id string) error) (bool, error) {
	if blueprint == nil {
		return false, fmt.Errorf("blueprint not provided")
	}

	if confirm != nil {
		summary := i.bootstrapSummary(blueprint)
		if !confirm(summary) {
			return false, nil
		}
	}

	if blueprint.BackendComponentID() == "" {
		if err := i.Up(blueprint, onApply...); err != nil {
			return false, err
		}
		return true, nil
	}

	if err := i.withBackendOverride("bootstrap", func() error {
		return i.Up(blueprint, onApply...)
	}); err != nil {
		return false, err
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

// bootstrapSummary builds the operator-visible intent description from the
// blueprint and config — no terraform invocation, no state probing. ContextName
// comes from the runtime, BackendType from terraform.backend.type config (with
// "local" as the default when unset), Terraform entries from the blueprint's
// terraform components in declaration order (skipping disabled ones),
// Kustomize entries from the blueprint's Kustomizations.
func (i *Provisioner) bootstrapSummary(blueprint *blueprintv1alpha1.Blueprint) *BootstrapSummary {
	summary := &BootstrapSummary{
		BackendType: i.configHandler.GetString("terraform.backend.type", "local"),
	}
	if i.runtime != nil {
		summary.ContextName = i.runtime.ContextName
	}
	for _, c := range blueprint.TerraformComponents {
		if c.Enabled != nil && !c.Enabled.IsEnabled() {
			continue
		}
		summary.Terraform = append(summary.Terraform, BootstrapTerraformEntry{
			ComponentID: c.GetID(),
			Path:        c.Path,
		})
	}
	for _, k := range blueprint.Kustomizations {
		summary.Kustomize = append(summary.Kustomize, k.Name)
	}
	return summary
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
