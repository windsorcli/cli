// The plan methods of the Provisioner preview terraform and kustomize changes.
// It provides the Plan methods for text, JSON, and summary output, for one component or for all.
// It provides the destroy plan summaries, and the PlanSummary and DestroyPlanSummary types.

package provisioner

import (
	"fmt"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
)

// =============================================================================
// Types
// =============================================================================

// PlanSummary holds aggregated plan results across all infrastructure layers.
// Terraform contains one entry per enabled component; Kustomize contains one
// entry per non-destroyOnly kustomization. Either slice may be nil when the
// corresponding layer is absent from the blueprint or its tooling is unavailable.
// Hints contains upgrade suggestions collected when required CLI tools are absent.
type PlanSummary struct {
	Terraform []terraforminfra.TerraformComponentPlan
	Kustomize []fluxinfra.KustomizePlan
	Hints     []string
}

// DestroyPlanSummary holds aggregated destroy-plan results across all
// infrastructure layers. The shape mirrors PlanSummary but without the Hints
// field: destroy is gated on a working cluster (the kustomize layer queries
// flux's live inventory), so a tooling-missing fallback is not part of the
// destroy contract — fail fast instead. The TerraformComponentPlan and
// KustomizePlan entries here come from the destroy-side producers; renderers
// must use a destroy-aware formatter to translate IsNew correctly (apply-side
// "(new)" becomes destroy-side "(no state)" / "(not deployed)").
type DestroyPlanSummary struct {
	Terraform []terraforminfra.TerraformComponentPlan
	Kustomize []fluxinfra.KustomizePlan
}

// =============================================================================
// Public Methods
// =============================================================================

// Plan runs terraform init and plan for a single component identified by componentID.
// It does not apply any changes. Returns an error if the terraform stack cannot be initialized,
// the component is not found, or any terraform operation fails.
func (i *Provisioner) Plan(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return fmt.Errorf("terraform is disabled")
	}
	if err := i.TerraformStack.Plan(blueprint, componentID); err != nil {
		return fmt.Errorf("failed to run terraform plan for %s: %w", componentID, err)
	}
	return nil
}

// PlanTerraformAll runs terraform init and plan for every enabled component, streaming
// output directly. Returns an error if blueprint is nil, the stack cannot be initialised,
// or any component's plan fails.
func (i *Provisioner) PlanTerraformAll(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return fmt.Errorf("terraform is disabled")
	}
	return i.TerraformStack.PlanAll(blueprint)
}

// PlanTerraformAllJSON runs terraform plan -json for every enabled component, streaming
// machine-readable JSON lines output directly to stdout. Returns an error if blueprint
// is nil, the stack cannot be initialised, or any component's plan fails.
func (i *Provisioner) PlanTerraformAllJSON(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return fmt.Errorf("terraform is disabled")
	}
	return i.TerraformStack.PlanAllJSON(blueprint)
}

// PlanTerraformJSON runs terraform plan -json for a single component, streaming
// machine-readable JSON lines output directly to stdout. Returns an error if blueprint
// is nil, the stack cannot be initialised, or the plan fails.
func (i *Provisioner) PlanTerraformJSON(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return fmt.Errorf("terraform is disabled")
	}
	return i.TerraformStack.PlanJSON(blueprint, componentID)
}

// PlanTerraformResourceChangesJSON runs terraform plan and show -json for a single component,
// printing the resulting plan document directly to stdout. Returns an error if blueprint
// is nil, the stack cannot be initialised, or the plan fails.
func (i *Provisioner) PlanTerraformResourceChangesJSON(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return fmt.Errorf("terraform is disabled")
	}
	return i.TerraformStack.PlanResourceChangesJSON(blueprint, componentID)
}

// PlanTerraformAllResourceChangesJSON runs terraform plan and show -json for every enabled
// component, printing each component's plan document directly to stdout. Returns an error
// if blueprint is nil, the stack cannot be initialised, or any component's plan fails.
func (i *Provisioner) PlanTerraformAllResourceChangesJSON(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return fmt.Errorf("terraform is disabled")
	}
	return i.TerraformStack.PlanAllResourceChangesJSON(blueprint)
}

// PlanKustomizeJSON runs kustomize build for the named kustomization (or all when componentID
// is "all") and writes the rendered manifests as JSON to stdout. Returns an error if blueprint
// is nil, the stack cannot be initialised, or the build fails.
func (i *Provisioner) PlanKustomizeJSON(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureFluxStack(); err != nil {
		return err
	}
	return i.FluxStack.PlanJSON(withCrdLayer(blueprint), componentID)
}

// PlanTerraformComponentSummary plans a single Terraform component and returns its
// structured result. Returns an error only when blueprint is nil or stack initialisation fails.
func (i *Provisioner) PlanTerraformComponentSummary(blueprint *blueprintv1alpha1.Blueprint, componentID string) (terraforminfra.TerraformComponentPlan, error) {
	if blueprint == nil {
		return terraforminfra.TerraformComponentPlan{}, fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return terraforminfra.TerraformComponentPlan{}, err
	}
	if i.TerraformStack == nil {
		return terraforminfra.TerraformComponentPlan{}, fmt.Errorf("terraform is disabled")
	}
	return i.TerraformStack.PlanComponentSummary(blueprint, componentID), nil
}

// PlanKustomizeComponentSummary plans a single Flux kustomization and returns its
// structured result. Returns an error only when blueprint is nil or stack initialisation fails.
func (i *Provisioner) PlanKustomizeComponentSummary(blueprint *blueprintv1alpha1.Blueprint, name string) (fluxinfra.KustomizePlan, error) {
	if blueprint == nil {
		return fluxinfra.KustomizePlan{}, fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureFluxStack(); err != nil {
		return fluxinfra.KustomizePlan{}, err
	}
	return i.FluxStack.PlanComponentSummary(withCrdLayer(blueprint), name), nil
}

// PlanDestroyTerraformComponentSummary previews the destroy plan for a single
// Terraform component. Returns an error if blueprint is nil, stack init fails,
// or the component is pinned destroy=false (which Teardown would skip).
func (i *Provisioner) PlanDestroyTerraformComponentSummary(blueprint *blueprintv1alpha1.Blueprint, componentID string) (terraforminfra.TerraformComponentPlan, error) {
	if blueprint == nil {
		return terraforminfra.TerraformComponentPlan{}, fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return terraforminfra.TerraformComponentPlan{}, err
	}
	if i.TerraformStack == nil {
		return terraforminfra.TerraformComponentPlan{}, fmt.Errorf("terraform is disabled")
	}
	return i.TerraformStack.PlanDestroyComponentSummary(blueprint, componentID), nil
}

// PlanDestroyKustomizeComponentSummary previews the destroy plan for a single
// Flux kustomization by querying its live inventory. Returns an error if
// blueprint is nil, stack init fails, or the kustomization is destroyOnly /
// pinned destroy=false (which DeleteBlueprint would skip).
func (i *Provisioner) PlanDestroyKustomizeComponentSummary(blueprint *blueprintv1alpha1.Blueprint, name string) (fluxinfra.KustomizePlan, error) {
	if blueprint == nil {
		return fluxinfra.KustomizePlan{}, fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureFluxStack(); err != nil {
		return fluxinfra.KustomizePlan{}, err
	}
	return i.FluxStack.PlanDestroyComponentSummary(withCrdLayer(blueprint), name), nil
}

// PlanTerraformSummary runs a best-effort summary plan across every Terraform
// component in the blueprint without touching the Flux/Kustomize layer.
// Returns an error only when blueprint is nil or stack initialisation fails.
func (i *Provisioner) PlanTerraformSummary(blueprint *blueprintv1alpha1.Blueprint) (*PlanSummary, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	summary := &PlanSummary{}

	if err := i.ensureTerraformStack(); err != nil {
		return nil, err
	}
	if i.TerraformStack != nil {
		summary.Terraform = i.TerraformStack.PlanSummary(blueprint)
	}

	return summary, nil
}

// PlanKustomizeSummary runs a best-effort summary plan across every Flux
// kustomization in the blueprint without touching the Terraform layer.
// Returns an error only when blueprint is nil or stack initialisation fails.
func (i *Provisioner) PlanKustomizeSummary(blueprint *blueprintv1alpha1.Blueprint) (*PlanSummary, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	summary := &PlanSummary{}

	if err := i.ensureFluxStack(); err != nil {
		return nil, err
	}
	summary.Kustomize, summary.Hints = i.FluxStack.PlanSummary(withCrdLayer(blueprint))

	return summary, nil
}

// PlanAll runs a best-effort summary plan across every Terraform component and
// Flux kustomization in the blueprint. It initialises both stacks as needed and
// collects per-component results without aborting on individual failures, so
// callers always receive as complete a picture as possible. Returns an error only
// when blueprint is nil or stack initialisation itself fails.
func (i *Provisioner) PlanAll(blueprint *blueprintv1alpha1.Blueprint) (*PlanSummary, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	tfSummary, err := i.PlanTerraformSummary(blueprint)
	if err != nil {
		return nil, err
	}

	k8sSummary, err := i.PlanKustomizeSummary(blueprint)
	if err != nil {
		return nil, err
	}

	return &PlanSummary{
		Terraform: tfSummary.Terraform,
		Kustomize: k8sSummary.Kustomize,
		Hints:     k8sSummary.Hints,
	}, nil
}

// PlanDestroyTerraformSummary previews the destroy plan for every Terraform
// component the blueprint would actually tear down (filtering destroy=false
// pins). Mirrors PlanTerraformSummary but uses `terraform plan -destroy -json`
// per component. Returns an error only when blueprint is nil or stack init
// fails.
func (i *Provisioner) PlanDestroyTerraformSummary(blueprint *blueprintv1alpha1.Blueprint) (*DestroyPlanSummary, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	summary := &DestroyPlanSummary{}

	if err := i.ensureTerraformStack(); err != nil {
		return nil, err
	}
	if i.TerraformStack != nil {
		summary.Terraform = i.TerraformStack.PlanDestroySummary(blueprint)
	}

	return summary, nil
}

// PlanDestroyKustomizeSummary previews the destroy plan for every eligible
// Flux kustomization from live cluster inventory. A stale kubeconfig reports
// every kustomization as not-deployed; see FluxStack.PlanDestroySummary.
// DestroyOnly hooks and destroy=false pinned kustomizations are filtered to
// match DeleteBlueprint. withCrdLayer flattens FluxSystem tiers and CRD
// layers first, so the plan matches Uninstall's teardown set.
func (i *Provisioner) PlanDestroyKustomizeSummary(blueprint *blueprintv1alpha1.Blueprint) (*DestroyPlanSummary, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	summary := &DestroyPlanSummary{}

	if err := i.ensureFluxStack(); err != nil {
		return nil, err
	}
	results, err := i.FluxStack.PlanDestroySummary(withCrdLayer(blueprint))
	if err != nil {
		return nil, err
	}
	summary.Kustomize = results

	return summary, nil
}

// PlanDestroyAll previews the destroy plan across both Terraform and Flux
// layers, aggregating both into one DestroyPlanSummary. See
// PlanDestroyKustomizeSummary for the flux-side unreachable-cluster
// classification.
func (i *Provisioner) PlanDestroyAll(blueprint *blueprintv1alpha1.Blueprint) (*DestroyPlanSummary, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	tfSummary, err := i.PlanDestroyTerraformSummary(blueprint)
	if err != nil {
		return nil, err
	}

	k8sSummary, err := i.PlanDestroyKustomizeSummary(blueprint)
	if err != nil {
		return nil, err
	}

	return &DestroyPlanSummary{
		Terraform: tfSummary.Terraform,
		Kustomize: k8sSummary.Kustomize,
	}, nil
}

// PlanKustomizeAll runs flux diff for every non-destroyOnly kustomization in the blueprint.
// Returns an error if the flux CLI is not found or any diff fails.
func (i *Provisioner) PlanKustomizeAll(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureFluxStack(); err != nil {
		return err
	}
	if err := i.FluxStack.PlanAll(withCrdLayer(blueprint)); err != nil {
		return fmt.Errorf("error planning kustomize: %w", err)
	}
	return nil
}

// PlanKustomizeAllJSON runs kustomize build for every non-destroyOnly kustomization and
// writes JSON to stdout. Returns an error if the kustomize CLI is not found or any build fails.
func (i *Provisioner) PlanKustomizeAllJSON(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureFluxStack(); err != nil {
		return err
	}
	return i.FluxStack.PlanAllJSON(withCrdLayer(blueprint))
}

// PlanKustomization runs flux diff for a single kustomization identified by componentID.
// Returns an error if the flux CLI is not found, the component is not in the blueprint, or the diff fails.
func (i *Provisioner) PlanKustomization(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureFluxStack(); err != nil {
		return err
	}
	if err := i.FluxStack.Plan(withCrdLayer(blueprint), componentID); err != nil {
		return fmt.Errorf("error planning kustomize for %s: %w", componentID, err)
	}
	return nil
}
