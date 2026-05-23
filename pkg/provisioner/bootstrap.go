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
//
// Returns (halted, err). halted=true means one of the inner Up calls signaled a clean
// halt-after-component (e.g. cluster reachability needs operator action). On halt the
// caller surfaces the deferred-work summary; bootstrap is partially complete and the
// operator re-runs after addressing it. Operator confirmation is the project layer's
// responsibility — callers gate Bootstrap on the operator's decision so declining the
// plan never reaches privileged work (workstation startup, DNS, etc.).
func (i *Provisioner) Bootstrap(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) (bool, error)) (bool, error) {
	if blueprint == nil {
		return false, fmt.Errorf("blueprint not provided")
	}

	backendType := i.configHandler.GetString("terraform.backend.type", "local")
	tier := blueprint.BackendTier()
	if backendType == "" || backendType == "local" || len(tier) == 0 {
		return i.Up(blueprint, onApply...)
	}

	tierBP := blueprintWithComponents(blueprint, tier)
	nonTierBP := blueprintWithoutComponents(blueprint, tier)

	var tierHalted bool
	if err := i.withBackendOverride("bootstrap", func() error {
		if _, err := i.MigrateState(tierBP); err != nil {
			return err
		}
		halted, err := i.Up(tierBP, onApply...)
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
		return false, fmt.Errorf("bootstrap migration skipped tier components after a successful local apply: %v — their directories should have been materialised by Up", skipped)
	}

	for _, c := range tier {
		if err := i.RemoveLocalState(c.GetID()); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to remove local state file for %q after migration: %v\n", c.GetID(), err)
		}
	}

	if len(nonTierBP.TerraformComponents) == 0 {
		return false, nil
	}
	return i.Up(nonTierBP, onApply...)
}

// BuildBootstrapSummary constructs the operator-visible intent description for a bootstrap,
// independent of any Provisioner instance so the project layer can render the plan before
// committing to privileged work.
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
	for _, k := range blueprint.Kustomizations {
		summary.Kustomize = append(summary.Kustomize, k.Name)
	}
	return summary
}

// =============================================================================
// Private Helpers
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
