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

// Bootstrap brings up a context's infrastructure end-to-end. It identifies a
// "pivot" component — the terraform module whose apply provisions the storage
// backing the configured remote backend — then either runs a tight three-
// phase dance or skips it based on whether the pivot already has state in the
// remote backend:
//
//  1. Probe the configured remote backend for the pivot's state.
//  2. Probe-hit (rerun): sweep non-pivot components for leftover local state
//     from a previous interrupted bootstrap; migrate any local-only state to
//     remote so the subsequent Up doesn't try to re-create cloud resources
//     it can't see in remote state. Then run plain Up against the configured
//     remote backend. This handles the common remediation pattern where a
//     prior bootstrap migrated some components but failed before reaching
//     others.
//  3. Probe-miss (first bootstrap): apply the pivot against local state
//     (backend.type pinned to "local" in-memory), migrate the pivot's state
//     to the configured remote backend, discard the now-stale local file so
//     future reruns rely on the probe rather than filesystem residue, then
//     apply every other terraform component normally against the live remote
//     backend.
//
// When the blueprint has no pivot (e.g. local backend, or a remote backend
// whose storage was provisioned out of band), Bootstrap calls Up directly.
// The pivot must be the first enabled terraform component; otherwise
// validatePivot aborts before any apply touches state.
//
// When confirm is non-nil, a BootstrapSummary built from the blueprint and
// config is passed to confirm. Returning false aborts with applied=false and
// no error. Any components whose state migration was skipped are returned in
// the error so the operator sees what didn't migrate.
func (i *Provisioner) Bootstrap(blueprint *blueprintv1alpha1.Blueprint, confirm BootstrapConfirmFn, onApply ...func(id string) error) (bool, error) {
	if blueprint == nil {
		return false, fmt.Errorf("blueprint not provided")
	}

	if confirm != nil {
		if !confirm(i.bootstrapSummary(blueprint)) {
			return false, nil
		}
	}

	backendType := i.configHandler.GetString("terraform.backend.type", "local")
	p := pivot(blueprint, backendType)

	if p == nil {
		if err := i.Up(blueprint, onApply...); err != nil {
			return false, err
		}
		return true, nil
	}

	pivotID := p.GetID()
	if err := validatePivot(blueprint, pivotID); err != nil {
		return false, err
	}

	if hasRemote, probeErr := i.HasRemoteState(blueprint, pivotID); probeErr == nil && hasRemote {
		if err := i.Up(blueprint, onApply...); err != nil {
			return false, err
		}
		return true, nil
	}

	pivotOnly := blueprintWithOnly(blueprint, pivotID)
	rest := blueprintWithout(blueprint, pivotID)

	if err := i.withBackendOverride("bootstrap", func() error {
		return i.Up(pivotOnly, onApply...)
	}); err != nil {
		return false, err
	}

	skipped, err := i.MigrateState(pivotOnly)
	if err != nil {
		if len(skipped) > 0 {
			return false, fmt.Errorf("%w (skipped components before failure: %s)", err, strings.Join(skipped, ", "))
		}
		return false, err
	}
	if len(skipped) > 0 {
		return false, fmt.Errorf("bootstrap state migration skipped components whose directories were missing after apply: %s", strings.Join(skipped, ", "))
	}

	if err := i.RemoveLocalState(pivotID); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to remove local state file for pivot %q after migration: %v\n", pivotID, err)
	}

	if len(rest.TerraformComponents) == 0 {
		return true, nil
	}
	if err := i.Up(rest, onApply...); err != nil {
		return false, err
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

// pivot returns the terraform component whose apply provisions the
// storage backing the configured remote backend, or nil when no local-then-
// migrate dance is needed. Detection rules:
//
//   - backendType == "" or "local": nil. No remote, no dance.
//   - any IsBackend() component is present: that component. Operator-declared
//     backend modules win regardless of backendType.
//   - backendType == "kubernetes" with no IsBackend() component: the first
//     enabled terraform component. The cluster module IS the backend; the
//     operator's component order declares which one.
//   - any other remote backendType with no IsBackend() component: nil. The
//     storage was created out of band and terraform can use it directly.
func pivot(bp *blueprintv1alpha1.Blueprint, backendType string) *blueprintv1alpha1.TerraformComponent {
	if bp == nil {
		return nil
	}
	if backendType == "" || backendType == "local" {
		return nil
	}
	for i := range bp.TerraformComponents {
		c := &bp.TerraformComponents[i]
		if c.Enabled != nil && !c.Enabled.IsEnabled() {
			continue
		}
		if c.IsBackend() {
			return c
		}
	}
	if backendType != "kubernetes" {
		return nil
	}
	for i := range bp.TerraformComponents {
		c := &bp.TerraformComponents[i]
		if c.Enabled != nil && !c.Enabled.IsEnabled() {
			continue
		}
		return c
	}
	return nil
}

// validatePivot returns an ordering error when the pivot is not the first
// enabled terraform component. Phase-3 Up against the configured remote
// backend depends on the pivot's state being live; any earlier component
// would either fail (backend not yet provisioned) or pollute remote state
// mid-bootstrap.
func validatePivot(bp *blueprintv1alpha1.Blueprint, pivotID string) error {
	for _, c := range bp.TerraformComponents {
		if c.Enabled != nil && !c.Enabled.IsEnabled() {
			continue
		}
		if c.GetID() == pivotID {
			return nil
		}
		return fmt.Errorf("bootstrap: pivot component %q must be the first enabled terraform component; %q is currently first — reorder your blueprint", pivotID, c.GetID())
	}
	return fmt.Errorf("bootstrap: pivot component %q not found among enabled terraform components", pivotID)
}

// blueprintWithOnly returns a shallow copy of bp whose TerraformComponents
// slice contains only the component matching id. Other blueprint fields
// (Kustomizations, Sources, Repository, Metadata) are copied as-is.
func blueprintWithOnly(bp *blueprintv1alpha1.Blueprint, id string) *blueprintv1alpha1.Blueprint {
	cp := *bp
	cp.TerraformComponents = nil
	for _, c := range bp.TerraformComponents {
		if c.GetID() == id {
			cp.TerraformComponents = []blueprintv1alpha1.TerraformComponent{c}
			break
		}
	}
	return &cp
}

// blueprintWithout returns a shallow copy of bp with the component matching
// id removed from TerraformComponents. Order is preserved. Kustomizations
// and other fields are copied as-is.
func blueprintWithout(bp *blueprintv1alpha1.Blueprint, id string) *blueprintv1alpha1.Blueprint {
	cp := *bp
	cp.TerraformComponents = make([]blueprintv1alpha1.TerraformComponent, 0, len(bp.TerraformComponents))
	for _, c := range bp.TerraformComponents {
		if c.GetID() != id {
			cp.TerraformComponents = append(cp.TerraformComponents, c)
		}
	}
	return &cp
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
