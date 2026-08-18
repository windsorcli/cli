package provisioner

import (
	"fmt"
	"os"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
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
// Up's doc for the backend-tier pivot. Kept as a distinct, self-documenting entry point for
// `windsor bootstrap`'s explicit-confirmation flow. Returns (halted, err); halted=true means
// an inner apply call stopped cleanly after a component (e.g. cluster reachability needs
// operator action), leaving bootstrap partially complete until the operator re-runs it.
func (i *Provisioner) Bootstrap(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) (bool, error)) (bool, error) {
	return i.Up(blueprint, onApply...)
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
// Private Helpers
// =============================================================================

// applyWithBackendPivot pins local, migrates any existing tier state to local, applies the
// tier locally, migrates it to the configured backend, then applies non-tier components
// directly against it. Idempotent regardless of whether the backend already exists.
// Sub-applies use applyDirect, not Up, to avoid re-deriving a tier and recursing.
func (i *Provisioner) applyWithBackendPivot(blueprint *blueprintv1alpha1.Blueprint, tier []*blueprintv1alpha1.TerraformComponent, onApply ...func(id string) (bool, error)) (bool, error) {
	tierBP := blueprintWithComponents(blueprint, tier)
	nonTierBP := blueprintWithoutComponents(blueprint, tier)

	var tierHalted bool
	if err := i.withBackendOverride("backend-pivot", func() error {
		if _, err := i.MigrateState(tierBP); err != nil {
			return err
		}
		halted, err := i.applyDirect(tierBP, onApply...)
		if err != nil {
			return err
		}
		tierHalted = halted
		return nil
	}); err != nil {
		return false, err
	}
	if tierHalted {
		// Halt during tier apply — don't migrate state or run non-tier components yet.
		return true, nil
	}

	skipped, err := i.MigrateState(tierBP)
	if err != nil {
		return false, err
	}
	if len(skipped) > 0 {
		return false, fmt.Errorf("backend-tier migration skipped components after a successful local apply: %v — their directories should have been materialised by the local apply", skipped)
	}

	for _, c := range tier {
		if err := i.RemoveLocalState(c.GetID()); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to remove local state file for %q after migration: %v\n", c.GetID(), err)
		}
	}

	if len(nonTierBP.TerraformComponents) == 0 {
		return false, nil
	}
	return i.applyDirect(nonTierBP, onApply...)
}

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

// withBackendOverride pins terraform.backend.type to "local" for the duration of fn,
// restoring the previously-configured value via defer.
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
