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

// Bootstrap brings up a context's infrastructure end-to-end. For local or external backends
// it forwards to Up. For an in-blueprint backend tier (Blueprint.Backend set) it always
// pivots the tier: Stage 1 pins local, pulls any existing tier state from the configured
// backend to local, applies the tier against local; Stage 2 pushes tier state to the
// configured backend; Stage 3 applies non-tier components. The algorithm is idempotent —
// every bootstrap runs the same flow regardless of whether the backend already exists.
func (i *Provisioner) Bootstrap(blueprint *blueprintv1alpha1.Blueprint, confirm BootstrapConfirmFn, onApply ...func(id string) error) (bool, error) {
	if blueprint == nil {
		return false, fmt.Errorf("blueprint not provided")
	}

	backendType := i.configHandler.GetString("terraform.backend.type", "local")
	if backendType == "kubernetes" && blueprint.Backend == "" {
		return false, fmt.Errorf("blueprint configures terraform.backend.type=kubernetes but does not declare Blueprint.Backend; set `backend: <cluster-component-id>` at the blueprint top level to name the terraform component that provisions the cluster")
	}

	if confirm != nil {
		if !confirm(i.bootstrapSummary(blueprint)) {
			return false, nil
		}
	}

	tier := blueprint.BackendTier()
	if backendType == "" || backendType == "local" || len(tier) == 0 {
		if err := i.Up(blueprint, onApply...); err != nil {
			return false, err
		}
		return true, nil
	}

	tierBP := blueprintWithComponents(blueprint, tier)
	nonTierBP := blueprintWithoutComponents(blueprint, tier)

	if err := i.withBackendOverride("bootstrap", func() error {
		if _, err := i.MigrateState(tierBP); err != nil {
			return err
		}
		return i.Up(tierBP, onApply...)
	}); err != nil {
		return false, err
	}

	skipped, err := i.MigrateState(tierBP)
	if err != nil {
		return false, err
	}
	if len(skipped) > 0 {
		return false, fmt.Errorf("bootstrap migration skipped tier components after a successful local apply: %v — their directories should have been materialised by Up", skipped)
	}

	for _, c := range tier {
		if err := i.RemoveLocalState(c.GetID()); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to remove local state file for %q after migration: %v\n", c.GetID(), err)
		}
	}

	if len(nonTierBP.TerraformComponents) == 0 {
		return true, nil
	}
	if err := i.Up(nonTierBP, onApply...); err != nil {
		return false, err
	}
	return true, nil
}

// =============================================================================
// Private Helpers
// =============================================================================

// bootstrapSummary builds the operator-visible intent description from the blueprint and config.
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
