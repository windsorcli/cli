// The bootstrap methods of the Provisioner apply a blueprint for the first time.
// It provides Bootstrap, an alias for Up, and BuildBootstrapSummary.
// It applies the backend components with a local backend and migrates their state to the configured backend.
// Then it applies the other components against the configured backend.

package provisioner

import (
	"context"
	"fmt"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Types
// =============================================================================

// BootstrapSummary describes the operator-visible intent of a bootstrap call.
type BootstrapSummary struct {
	ContextName string
	BackendType string
	Terraform   []BootstrapTerraformEntry
	Kustomize   []string
}

// BootstrapTerraformEntry is a single row in the Terraform section of the bootstrap summary.
type BootstrapTerraformEntry struct {
	ComponentID string
	Path        string
}

// BootstrapConfirmFn receives the bootstrap summary and returns true to proceed.
type BootstrapConfirmFn func(*BootstrapSummary) bool

// =============================================================================
// Public Methods
// =============================================================================

// Bootstrap brings up a context's infrastructure end-to-end; it is a thin alias for Up — see
// Up's doc for the backend pivot. Kept as a distinct, self-documenting entry point for
// `windsor bootstrap`'s explicit-confirmation flow. Returns (halted, err); halted=true means
// an inner apply call stopped cleanly after a component (e.g. cluster reachability needs
// operator action), leaving bootstrap partially complete until the operator re-runs it.
func (i *Provisioner) Bootstrap(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
	return i.Up(ctx, blueprint, onApply...)
}

// BuildBootstrapSummary constructs the operator-visible intent description for a bootstrap,
// independent of any Provisioner instance so the project layer can render the plan before
// committing to privileged work. The CRD layers are materialized via withCrdLayer so the bootstrap
// plan lists the synthesized "crds"/"crds-<source>" kustomizations the stack installs, matching what
// `windsor plan` shows rather than hiding them until apply.
func BuildBootstrapSummary(blueprint *blueprintv1alpha1.Blueprint, contextName, backendType string) *BootstrapSummary {
	summary := &BootstrapSummary{
		ContextName: contextName,
		BackendType: backendType,
	}
	if summary.BackendType == "" {
		summary.BackendType = "local"
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
	for _, k := range withCrdLayer(blueprint).Kustomizations {
		summary.Kustomize = append(summary.Kustomize, k.Name)
	}
	return summary
}

// =============================================================================
// Private Methods
// =============================================================================

// applyWithBackendPivot sets a local backend, migrates the backend components to it, and applies
// them. Then it migrates them to the configured backend and applies the other components against
// it. It gives the same result whether the backend exists or not. When a hook halts the backend
// apply, it stops before the migration. It calls applyDirect, not Up, so it does not recurse.
func (i *Provisioner) applyWithBackendPivot(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, backendComponents []*blueprintv1alpha1.TerraformComponent, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
	backendComponentsBP := blueprintWithComponents(blueprint, backendComponents)
	nonBackendBP := blueprintWithoutComponents(blueprint, backendComponents)

	var backendHalted bool
	if err := i.withBackendOverride(ctx, "backend-pivot", func() error {
		if _, err := i.MigrateState(ctx, backendComponentsBP); err != nil {
			return err
		}
		halted, err := i.applyDirect(ctx, backendComponentsBP, onApply...)
		if err != nil {
			return err
		}
		backendHalted = halted
		return nil
	}); err != nil {
		return false, err
	}
	if backendHalted {
		return true, nil
	}

	skipped, err := i.MigrateState(ctx, backendComponentsBP)
	if err != nil {
		return false, err
	}
	if len(skipped) > 0 {
		return false, fmt.Errorf("backend migration skipped components after a successful local apply: %v — their directories should have been materialised by the local apply", skipped)
	}

	for _, c := range backendComponents {
		if err := i.RemoveLocalState(c.GetID()); err != nil {
			presenter.Warn(ctx, fmt.Sprintf("failed to remove local state file for %q after migration: %v", c.GetID(), err))
		}
	}

	if len(nonBackendBP.TerraformComponents) == 0 {
		return false, nil
	}
	return i.applyDirect(ctx, nonBackendBP, onApply...)
}

// withBackendOverride sets terraform.backend.type to "local" while fn runs, then restores the
// previous value. A failure to restore the value is reported as a warning in ctx.
func (i *Provisioner) withBackendOverride(ctx context.Context, opLabel string, fn func() error) error {
	original := i.configHandler.GetTerraformBackendType()
	if err := i.configHandler.Set("terraform.backend.type", "local"); err != nil {
		return werror.Wrap(err, "failed to override backend for %s", opLabel)
	}
	defer func() {
		if err := i.configHandler.Set("terraform.backend.type", original); err != nil {
			presenter.Warn(ctx, fmt.Sprintf("failed to restore terraform.backend.type to %q after %s: %v", original, opLabel, err))
		}
	}()
	return fn()
}

// =============================================================================
// Helpers
// =============================================================================

// blueprintWithComponents returns a shallow copy of bp containing only the given
// terraform components, in their order in the slice. Non-terraform fields are shared.
func blueprintWithComponents(bp *blueprintv1alpha1.Blueprint, components []*blueprintv1alpha1.TerraformComponent) *blueprintv1alpha1.Blueprint {
	cp := *bp
	cp.TerraformComponents = make([]blueprintv1alpha1.TerraformComponent, len(components))
	for i, c := range components {
		cp.TerraformComponents[i] = *c
	}
	return &cp
}

// blueprintWithoutComponents returns a shallow copy of bp with the given terraform
// components removed, preserving declaration order of the survivors.
func blueprintWithoutComponents(bp *blueprintv1alpha1.Blueprint, components []*blueprintv1alpha1.TerraformComponent) *blueprintv1alpha1.Blueprint {
	exclude := make(map[string]bool, len(components))
	for _, c := range components {
		exclude[c.GetID()] = true
	}
	cp := *bp
	cp.TerraformComponents = make([]blueprintv1alpha1.TerraformComponent, 0, len(bp.TerraformComponents))
	for _, c := range bp.TerraformComponents {
		if !exclude[c.GetID()] {
			cp.TerraformComponents = append(cp.TerraformComponents, c)
		}
	}
	return &cp
}
