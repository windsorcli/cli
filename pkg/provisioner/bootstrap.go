package provisioner

import (
	"fmt"
	"io"
	"os"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Variables
// =============================================================================

// probeErrorPause is the Ctrl-C window between Bootstrap's probe-failure
// warning and the dance starting. Tests set it to zero.
var probeErrorPause = 5 * time.Second

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

// Bootstrap brings up a context's infrastructure end-to-end. With a pivot,
// probe-hit runs plain Up after a recovery sweep for half-migrated
// components; probe-miss runs the local-then-migrate dance for the pivot,
// then Up for the rest. With no pivot, Bootstrap calls Up directly.
// validatePivot enforces pivot-first ordering. When confirm is non-nil it
// runs first; returning false aborts with applied=false.
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

	hasRemote, probeErr := i.HasRemoteState(blueprint, pivotID)
	if probeErr != nil {
		fmt.Fprintf(os.Stderr, "warning: probe of configured backend for pivot %q failed: %v — proceeding with local-then-migrate dance (correct on first-time bootstrap; abort with Ctrl-C if your backend already exists and this is a transient probe failure)\n", pivotID, probeErr)
		pauseForProbeWarning(os.Stderr, probeErrorPause)
	} else if hasRemote {
		fmt.Fprintf(os.Stderr, "Probe found existing state for pivot %q in configured backend — skipping bootstrap dance, applying as a normal up.\n", pivotID)
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
		return false, err
	}
	if len(skipped) > 0 {
		return false, fmt.Errorf("bootstrap migration skipped pivot %q: component directory missing after apply", pivotID)
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

// pauseForProbeWarning emits a per-second countdown to w so the operator has
// a Ctrl-C window before terraform output drowns the warning. Pause <= 0 is
// a no-op (test mode).
func pauseForProbeWarning(w io.Writer, pause time.Duration) {
	if pause <= 0 {
		return
	}
	seconds := int(pause / time.Second)
	if seconds < 1 {
		time.Sleep(pause)
		return
	}
	for remaining := seconds; remaining > 0; remaining-- {
		fmt.Fprintf(w, "  proceeding in %ds (Ctrl-C to abort)...\n", remaining)
		time.Sleep(time.Second)
	}
}

// pivot returns the terraform component whose apply provisions the storage
// for the configured remote backend, or nil when no dance is needed.
// Detection: local/empty backend → nil; any IsBackend() component → that
// component (operator-declared wins); kubernetes with no IsBackend() →
// first enabled component (cluster IS the backend); other remote with no
// IsBackend() → nil (out-of-band setup).
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
// enabled terraform component. Earlier components would either fail (backend
// not yet provisioned) or land state in remote before the pivot applied,
// corrupting the bootstrap sequence.
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

// blueprintWithOnly returns a shallow copy of bp with only the component
// matching id in TerraformComponents.
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
// id removed from TerraformComponents. Order preserved.
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

// withBackendOverride pins terraform.backend.type to "local" for the duration
// of fn, restoring the previously-configured value via defer. In-memory only;
// values.yaml is unaffected. opLabel appears in override-failure messages.
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
