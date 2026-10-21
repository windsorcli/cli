// The bootstrap methods of the Provisioner apply a blueprint for the first time.
// It provides Bootstrap, an alias for Up, and BuildBootstrapSummary.
// It applies the backend components with a local backend and migrates their state to the configured backend.
// Then it applies the other components against the configured backend.

package provisioner

import (
	"context"
	"fmt"
	"os"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
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

// applyWithBackendPivot pins local, migrates any existing backend-component state to local,
// applies those components locally, migrates them to the configured backend, then applies
// the remaining components directly against it. Idempotent regardless of whether the backend
// already exists. Sub-applies use applyDirect, not Up, to avoid re-deriving the backend
// components and recursing.
func (i *Provisioner) applyWithBackendPivot(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, backendComponents []*blueprintv1alpha1.TerraformComponent, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
	backendComponentsBP := blueprintWithComponents(blueprint, backendComponents)
	nonBackendBP := blueprintWithoutComponents(blueprint, backendComponents)

	var backendHalted bool
	if err := i.withBackendOverride("backend-pivot", func() error {
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
		// Halt during the backend components' apply — don't migrate state or run the rest yet.
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
			fmt.Fprintf(os.Stderr, "warning: failed to remove local state file for %q after migration: %v\n", c.GetID(), err)
		}
	}

	if len(nonBackendBP.TerraformComponents) == 0 {
		return false, nil
	}
	return i.applyDirect(ctx, nonBackendBP, onApply...)
}

// withBackendOverride pins terraform.backend.type to "local" for the duration of fn,
// restoring the previously-configured value via defer.
func (i *Provisioner) withBackendOverride(opLabel string, fn func() error) error {
	original := i.configHandler.GetTerraformBackendType()
	if err := i.configHandler.Set("terraform.backend.type", "local"); err != nil {
		return werror.Wrap(err, "failed to override backend for %s", opLabel)
	}
	defer func() {
		if err := i.configHandler.Set("terraform.backend.type", original); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to restore terraform.backend.type to %q after %s: %v\n", original, opLabel, err)
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
