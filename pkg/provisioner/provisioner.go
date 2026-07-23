package provisioner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner/cluster"
	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	k8sclient "github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/tui"
)

// The Provisioner package provides high-level infrastructure provisioning functionality
// for terraform operations, kubernetes cluster interactions, and cluster API operations.
// It consolidates the creation and management of terraform stacks, kubernetes managers,
// and cluster clients, providing a unified interface for infrastructure lifecycle operations
// across the Windsor CLI.

// =============================================================================
// Types
// =============================================================================

// Provisioner manages the lifecycle of all infrastructure components (terraform, kubernetes, clusters).
// It provides a unified interface for creating, initializing, and managing these infrastructure components
// with proper dependency injection and error handling.
type Provisioner struct {
	configHandler config.ConfigHandler
	shell         shell.Shell
	evaluator     evaluator.ExpressionEvaluator
	contextName   string
	projectRoot   string
	configRoot    string
	runtime       *runtime.Runtime

	// secretPollInterval overrides how often PlaceSecrets re-checks for a pending secret's namespace;
	// zero uses constants.DefaultKustomizationWaitPollInterval. Set small in tests to avoid real waits.
	secretPollInterval time.Duration

	TerraformStack       terraforminfra.Stack
	FluxStack            fluxinfra.Stack
	Notifier             fluxinfra.Notifier
	onTerraformApply     []func(id string) (bool, error)
	onTerraformPostApply []func(id string) error
	KubernetesManager    kubernetes.KubernetesManager
	KubernetesClient     k8sclient.KubernetesClient
	ClusterClient        cluster.ClusterClient
	blueprintHandler     blueprint.BlueprintHandler
}

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

// VersionGate describes how the blueprint a command is about to apply relates to the version marker
// recorded in the cluster. It is the input to apply's version-equality seam: apply may reconcile in
// place only when a settled marker matches the blueprint it would apply. Any other state — a version
// mismatch or an in-flight transition — belongs to upgrade. The caller decides policy (proceed,
// refuse, or honor --force); this struct only reports the relation.
type VersionGate struct {
	MarkerFound  bool // a marker exists for this context (false = pre-bootstrap, no cluster, or legacy)
	InFlight     bool // the marker phase is not idle (an upgrade is in flight)
	VersionMatch bool // the blueprint's resolved source-ref set equals the applied set
}

// NodeHealthCheckOptions contains options for node health checking.
type NodeHealthCheckOptions struct {
	Nodes               []string
	Timeout             time.Duration
	Version             string
	K8SEndpoint         string
	K8SEndpointProvided bool
	CheckNodeReady      bool
	SkipServices        []string
	WaitForReboot       bool
	OfflineTimeout      time.Duration
}

// =============================================================================
// Constructor
// =============================================================================

// NewProvisioner creates a new Provisioner instance with the provided runtime and blueprint handler.
// It sets up kubernetes manager and kubernetes client. Terraform stack and cluster client
// are initialized lazily when needed by the Up(), Down(), and WaitForHealth() methods.
// Panics if runtime or blueprintHandler are nil.
func NewProvisioner(rt *runtime.Runtime, blueprintHandler blueprint.BlueprintHandler, opts ...*Provisioner) *Provisioner {
	if rt == nil {
		panic("runtime is required")
	}
	if rt.ConfigHandler == nil {
		panic("config handler is required on runtime")
	}
	if rt.Shell == nil {
		panic("shell is required on runtime")
	}
	if rt.Evaluator == nil {
		panic("evaluator is required on runtime")
	}
	if blueprintHandler == nil {
		panic("blueprint handler is required")
	}

	provisioner := &Provisioner{
		configHandler:    rt.ConfigHandler,
		shell:            rt.Shell,
		evaluator:        rt.Evaluator,
		contextName:      rt.ContextName,
		projectRoot:      rt.ProjectRoot,
		configRoot:       rt.ConfigRoot,
		runtime:          rt,
		blueprintHandler: blueprintHandler,
	}

	if len(opts) > 0 && opts[0] != nil {
		overrides := opts[0]
		if overrides.TerraformStack != nil {
			provisioner.TerraformStack = overrides.TerraformStack
		}
		if overrides.FluxStack != nil {
			provisioner.FluxStack = overrides.FluxStack
		}
		if overrides.Notifier != nil {
			provisioner.Notifier = overrides.Notifier
		}
		if overrides.KubernetesManager != nil {
			provisioner.KubernetesManager = overrides.KubernetesManager
		}
		if overrides.KubernetesClient != nil {
			provisioner.KubernetesClient = overrides.KubernetesClient
		}
		if overrides.ClusterClient != nil {
			provisioner.ClusterClient = overrides.ClusterClient
		}
	}

	if provisioner.KubernetesClient == nil {
		provisioner.KubernetesClient = k8sclient.NewDynamicKubernetesClient()
	}

	if provisioner.KubernetesManager == nil {
		provisioner.KubernetesManager = kubernetes.NewKubernetesManager(provisioner.KubernetesClient, rt.ConfigHandler)
	}

	return provisioner
}

// =============================================================================
// Public Methods
// =============================================================================

// OnTerraformApply registers a hook to run after each Terraform component apply, inside the
// progress spinner. The hook returns (haltAfter, err); haltAfter=true signals the component
// apply succeeded but subsequent components must not be applied (e.g. cluster reachability
// needs host configuration the operator hasn't done yet). Errors are real failures.
func (i *Provisioner) OnTerraformApply(fn func(id string) (bool, error)) {
	if fn != nil {
		i.onTerraformApply = append(i.onTerraformApply, fn)
	}
}

// OnTerraformPostApply registers a hook to run after each Terraform component's Done line is printed.
// Use this for operations that must not run inside the progress spinner (e.g. interactive sudo prompts).
func (i *Provisioner) OnTerraformPostApply(fn func(id string) error) {
	if fn != nil {
		i.onTerraformPostApply = append(i.onTerraformPostApply, fn)
	}
}

// Up orchestrates the high-level infrastructure deployment process. It runs Terraform apply
// when terraform.enabled and the stack exists, invoking the given onApply hooks after each
// component apply (after any hooks registered via OnTerraformApply). The blueprint parameter
// is required.
//
// Returns (halted bool, err error). halted=true means a hook signaled a clean stop after a
// component apply — the apply succeeded, but subsequent components were intentionally
// skipped. err remains the path for real failures.
func (i *Provisioner) Up(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) (bool, error)) (bool, error) {
	if blueprint == nil {
		return false, fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return false, err
	}
	if i.TerraformStack == nil {
		return false, nil
	}
	if err := i.recoverHalfMigratedComponents(blueprint); err != nil {
		return false, err
	}
	hooks := append([]func(id string) (bool, error){}, i.onTerraformApply...)
	hooks = append(hooks, onApply...)
	if len(i.onTerraformPostApply) > 0 {
		i.TerraformStack.PostApply(i.onTerraformPostApply...)
	}
	halted, err := i.TerraformStack.Up(blueprint, hooks...)
	if err != nil {
		return false, fmt.Errorf("failed to run terraform up: %w", err)
	}
	return halted, nil
}

// MigrateState reinitializes every Terraform component's backend against the currently configured
// backend, migrating state as needed. Used by `windsor bootstrap` after its local-first apply
// pass to move state to the configured remote backend once that backend's underlying infrastructure
// (e.g. the kubernetes cluster hosting the k8s backend) has been provisioned. Safe to invoke
// directly for users who change backend config and want existing state migrated in place. The
// blueprint parameter is required. Returns the IDs of components whose directories were missing
// and therefore skipped; callers decide whether that is an error condition — bootstrap treats
// any skip as anomalous (Up should have materialized every dir); pre-destroy migration discards
// the list because un-applied components are a normal condition there.
//
// The skipped slice is returned alongside any error (not only on success), mirroring the
// Stack.MigrateState contract. Dropping it on the error path would strand bootstrap without
// the context it needs to emit "A was skipped, then B failed" in a single diagnostic — the
// exact signal the operator needs to investigate what removed A's directory between Up and
// MigrateState.
func (i *Provisioner) MigrateState(blueprint *blueprintv1alpha1.Blueprint) ([]string, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return nil, err
	}
	if i.TerraformStack == nil {
		return nil, nil
	}
	skipped, err := i.TerraformStack.MigrateState(blueprint)
	if err != nil {
		return skipped, fmt.Errorf("failed to migrate terraform state: %w", err)
	}
	return skipped, nil
}

// MigrateComponentState reinitializes a single Terraform component's backend against the
// currently configured backend, migrating state as needed. Used by `windsor bootstrap` to
// move only the backend component's state to remote (e.g. S3) immediately after the
// backend infrastructure is applied with a local backend; subsequent components then init
// directly against the configured remote backend on the next Up. Returns an error if the
// blueprint is nil, the component is not found, terraform is disabled, or any terraform
// operation fails.
func (i *Provisioner) MigrateComponentState(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return fmt.Errorf("terraform is disabled")
	}
	if err := i.TerraformStack.MigrateComponentState(blueprint, componentID); err != nil {
		return fmt.Errorf("failed to migrate terraform state for %s: %w", componentID, err)
	}
	return nil
}

// HasRemoteState reports whether the component has non-empty state in the
// currently-configured backend. Call before any backend override so the
// probe targets the configured remote.
func (i *Provisioner) HasRemoteState(blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
	if blueprint == nil {
		return false, fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return false, err
	}
	if i.TerraformStack == nil {
		return false, fmt.Errorf("terraform is disabled")
	}
	return i.TerraformStack.HasRemoteState(blueprint, componentID)
}

// InitComponent runs `terraform init` for one component using the currently-
// configured backend; no -migrate-state, no plan, no apply.
func (i *Provisioner) InitComponent(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return fmt.Errorf("terraform is disabled")
	}
	return i.TerraformStack.InitComponent(blueprint, componentID)
}

// HasLocalStateWithResources reports whether the component's local state
// file exists and contains at least one resource entry.
func (i *Provisioner) HasLocalStateWithResources(componentID string) (bool, error) {
	if err := i.ensureTerraformStack(); err != nil {
		return false, err
	}
	if i.TerraformStack == nil {
		return false, fmt.Errorf("terraform is disabled")
	}
	return i.TerraformStack.HasLocalStateWithResources(componentID)
}

// RemoveLocalState removes the per-component local terraform state file.
// Missing files are tolerated.
func (i *Provisioner) RemoveLocalState(componentID string) error {
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return fmt.Errorf("terraform is disabled")
	}
	return i.TerraformStack.RemoveLocalState(componentID)
}

// Down destroys the "workstation" terraform component if it is present in the blueprint, then returns.
// All other terraform components are left untouched; use Destroy / DestroyAll for those.
// If terraform is disabled or the blueprint has no "workstation" component, Down is a no-op.
// Returns an error if the blueprint is nil or the destroy operation fails.
func (i *Provisioner) Down(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	hasWorkstation := false
	for _, c := range blueprint.TerraformComponents {
		if c.GetID() == "workstation" && c.Enabled.IsEnabled() {
			hasWorkstation = true
			break
		}
	}
	if !hasWorkstation {
		return nil
	}

	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return nil
	}
	// Down ignores the skipped flag from Destroy: an empty-state workstation component is
	// effectively a successful tear-down for the workstation flow's purposes (nothing to
	// destroy = nothing left). The cmd-level destroy paths surface skip status; Down does
	// not need to.
	if _, err := i.TerraformStack.Destroy(blueprint, "workstation"); err != nil {
		return fmt.Errorf("failed to destroy workstation terraform component: %w", err)
	}
	return nil
}

// DestroyAllTerraform destroys all terraform components in the stack in reverse dependency order.
// Components with Destroy set to false are skipped. excludeIDs are skipped entirely (used by the
// cmd-layer symmetric-destroy flow to peel the backend component off the bulk pass and migrate
// it before destroying it last). If terraform is disabled, returns an error. Returns the IDs of
// components that were skipped because their state was empty alongside any error, mirroring the
// MigrateState contract — the slice is paired with the error so callers see partial progress
// even when a later component fails. Skipped components had nothing in state to destroy (never
// applied, fully torn down already, or upstream destroy collapsed their cloud objects out from
// under them); cmd-layer callers surface them in the user-facing summary so an operator can see
// "these were no-ops" alongside "these were destroyed". Runs checkKubernetesReachableForDestroy
// once terraform is confirmed enabled.
func (i *Provisioner) DestroyAllTerraform(blueprint *blueprintv1alpha1.Blueprint, continueOnError bool, excludeIDs ...string) (DestroyResult, error) {
	return i.destroyAllTerraform(blueprint, continueOnError, true, excludeIDs...)
}

// destroyAllTerraform is the shared implementation behind DestroyAllTerraform. checkReachability
// is false only for Teardown's Stage 2 tier destroy: by the time Stage 2 runs, Stage 1 has
// already destroyed the cluster by design, so an unreachable Kubernetes API is the expected state
// rather than a signal of broken auth, and the backend tier never has a kubernetes/helm provider
// dependency for the check to protect.
func (i *Provisioner) destroyAllTerraform(blueprint *blueprintv1alpha1.Blueprint, continueOnError bool, checkReachability bool, excludeIDs ...string) (DestroyResult, error) {
	var result DestroyResult
	if blueprint == nil {
		return result, fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return result, err
	}
	if i.TerraformStack == nil {
		return result, fmt.Errorf("terraform is disabled")
	}
	if checkReachability {
		if err := i.checkKubernetesReachableForDestroy(); err != nil {
			return result, err
		}
	}
	outcome, err := i.TerraformStack.DestroyAll(blueprint, continueOnError, excludeIDs...)
	result.Destroyed = outcome.Destroyed
	result.Skipped = outcome.Skipped
	result.Failed = outcome.Failed
	if err != nil {
		return result, fmt.Errorf("failed to run terraform destroy: %w", err)
	}
	return result, nil
}

// Apply runs terraform init, plan, and apply for a single component identified by componentID.
// Returns an error if terraform is disabled, the stack cannot be initialized, the component is
// not found, or any terraform operation fails.
func (i *Provisioner) Apply(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return fmt.Errorf("terraform is disabled")
	}
	if err := i.TerraformStack.Apply(blueprint, componentID); err != nil {
		return fmt.Errorf("failed to run terraform apply for %s: %w", componentID, err)
	}
	return nil
}

// Destroy runs terraform init and destroy for a single component identified by componentID.
// Returns (skipped, nil) when the component's state is empty (nothing to destroy), (false, nil)
// when destroy ran successfully, or (false, err) on any failure. Returns an error if the
// blueprint is nil, terraform is disabled, the stack cannot be initialized, the component is
// not found, or any terraform operation fails. Runs checkKubernetesReachableForDestroy once
// terraform is confirmed enabled.
func (i *Provisioner) Destroy(blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
	if blueprint == nil {
		return false, fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return false, err
	}
	if i.TerraformStack == nil {
		return false, fmt.Errorf("terraform is disabled")
	}
	if err := i.checkKubernetesReachableForDestroy(); err != nil {
		return false, err
	}
	skipped, err := i.TerraformStack.Destroy(blueprint, componentID)
	if err != nil {
		return false, fmt.Errorf("failed to run terraform destroy for %s: %w", componentID, err)
	}
	return skipped, nil
}

// DestroyKustomize deletes a single kustomization by name from the cluster.
// Returns an error if the blueprint is nil, the kubernetes manager is not configured,
// the kustomization is not found in the blueprint, or the delete operation fails.
func (i *Provisioner) DestroyKustomize(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	var found *blueprintv1alpha1.Kustomization
	for _, k := range blueprint.AllKustomizations() {
		if k.Name == componentID {
			kCopy := k
			found = &kCopy
			break
		}
	}
	if found == nil {
		return fmt.Errorf("kustomization %q not found in blueprint", componentID)
	}

	if err := tui.WithProgress(fmt.Sprintf("Destroying kustomization %s", componentID), func() error {
		return i.KubernetesManager.DeleteKustomization(componentID, i.fluxNamespace())
	}); err != nil {
		return fmt.Errorf("failed to delete kustomization %s: %w", componentID, err)
	}

	return nil
}

// DestroyAll destroys all infrastructure components: first uninstalls all kustomizations,
// then destroys all terraform components. The kustomization uninstall step is skipped
// when no kubeconfig exists at the context-scoped path — the cluster is gone (or was
// never bootstrapped past terraform), so trying to talk to its API would fail with a
// stat error and abort the whole destroy. Skipping idempotently lets `windsor destroy`
// run cleanly after the cluster's already been torn down by a prior partial destroy or
// out-of-band action. excludeIDs are forwarded to the terraform destroy pass so
// cmd-layer callers can peel off the backend component for the symmetric-destroy flow
// (destroy non-backend against live remote state, then migrate-and-destroy backend
// last). Returns the IDs of terraform components that were skipped because their state
// was empty (never applied, already torn down) alongside any error from either step —
// paired with the error so callers see what was no-op'd even when a later step fails.
// Returns an error if either step fails. checkKubernetesReachableForDestroy runs after the
// kustomize step (so it never fires if kustomize already hard-failed) and before terraform.
func (i *Provisioner) DestroyAll(blueprint *blueprintv1alpha1.Blueprint, continueOnError bool, excludeIDs ...string) (DestroyResult, error) {
	var result DestroyResult
	if blueprint == nil {
		return result, fmt.Errorf("blueprint not provided")
	}

	if i.KubernetesManager != nil && i.kubeconfigPresent() {
		if err := i.Uninstall(blueprint); err != nil {
			if !continueOnError {
				return result, err
			}
			result.Failed = append(result.Failed, ComponentFailure{ID: KustomizeFailureID, Err: err})
		} else {
			for _, k := range blueprint.AllKustomizations() {
				if fluxinfra.KustomizationDestroyEligible(k) {
					result.Destroyed = append(result.Destroyed, k.Name)
				}
			}
		}
	}

	if err := i.ensureTerraformStack(); err != nil {
		return result, err
	}
	if i.TerraformStack != nil {
		if err := i.checkKubernetesReachableForDestroy(); err != nil {
			return result, err
		}
		outcome, err := i.TerraformStack.DestroyAll(blueprint, continueOnError, excludeIDs...)
		result.Destroyed = append(result.Destroyed, outcome.Destroyed...)
		result.Skipped = append(result.Skipped, outcome.Skipped...)
		result.Failed = append(result.Failed, outcome.Failed...)
		if err != nil {
			return result, fmt.Errorf("failed to run terraform destroy: %w", err)
		}
	}

	return result, nil
}

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
// Flux kustomization by querying live cluster inventory. Returns an error if
// the cluster is unreachable — destroy itself cannot proceed without it, so a
// blueprint-derived fallback would mislead. DestroyOnly hooks and destroy=
// false pinned kustomizations are filtered to match DeleteBlueprint. The
// blueprint is passed through withCrdLayer so FluxSystem tiers and synthesized
// CRD layers are flattened into Kustomizations — matching Uninstall's teardown
// set exactly, so the plan lists the same kustomizations the destroy removes.
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
// layers. Initialises each stack as needed and aggregates the results into
// a single DestroyPlanSummary for rendering. A cluster failure on the flux
// side aborts the whole plan — destroy needs the cluster, so a partial plan
// would be misleading.
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

// ApplyKustomize applies a single kustomization identified by componentID to the cluster and places its
// declared secrets. It finds the named kustomization in the blueprint, filters the blueprint to that one
// kustomization (preserving sources and repository for correct source creation), resolves that
// kustomization's secrets, applies it via the kubernetes manager, then places the resolved secrets into
// the namespace it creates — scoped to the one kustomization so placement never blocks on a namespace
// another kustomization would create. Secret pruning is off, since a single-kustomization apply is
// additive. Returns an error if the blueprint is nil, the kubernetes manager is not configured, the
// kustomization is not found, the kustomization is marked destroyOnly, or resolution, apply, or placement
// fails.
func (i *Provisioner) ApplyKustomize(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	blueprint = withCrdLayer(blueprint)

	var found *blueprintv1alpha1.Kustomization
	for _, k := range blueprint.Kustomizations {
		if k.Name == componentID {
			kCopy := k
			found = &kCopy
			break
		}
	}
	if found == nil {
		return fmt.Errorf("kustomization %q not found in blueprint", componentID)
	}
	if found.DestroyOnly != nil && *found.DestroyOnly {
		return fmt.Errorf("kustomization %q is destroy-only and cannot be applied", componentID)
	}

	filtered := *blueprint
	filtered.Kustomizations = []blueprintv1alpha1.Kustomization{*found}

	resolvedSecrets, err := i.ResolveSecrets(&filtered)
	if err != nil {
		return fmt.Errorf("error resolving secrets: %w", err)
	}

	if err := tui.WithProgress(fmt.Sprintf("Applying kustomization %s", componentID), func() error {
		if err := i.KubernetesManager.ApplyBlueprint(&filtered, i.fluxNamespace()); err != nil {
			return err
		}
		_ = i.Notify(ctx, &filtered)
		return nil
	}); err != nil {
		return fmt.Errorf("failed to apply kustomization %s: %w", componentID, err)
	}

	if err := i.PlaceSecrets(ctx, resolvedSecrets, &filtered, false); err != nil {
		return fmt.Errorf("error placing secrets: %w", err)
	}

	return nil
}

// ApplyKustomizeAll applies all non-destroyOnly kustomizations in the blueprint to the cluster and places
// their declared secrets. Delegates to Install with pruning off, since applying the full set is additive
// here — kustomization and secret pruning are separate, explicitly-flagged steps. Returns an error if the
// blueprint is nil, the kubernetes manager is not configured, or the apply or placement fails.
func (i *Provisioner) ApplyKustomizeAll(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) error {
	return i.Install(ctx, blueprint, false)
}

// Install applies the blueprint's kustomization layer and places its declared secrets as one unit, so
// every command that installs kustomizations also materializes their secrets rather than re-wiring that
// sequence itself. It first resolves the blueprint's declared Secrets to plaintext, failing before any
// cluster mutation on a misconfigured secret; then applies all blueprint resources in order — namespace,
// source repositories, and each kustomization — firing a best-effort flux webhook notification inside the
// same progress scope so flux reconciles immediately instead of at the next interval (notification
// failures never abort the install); then places the resolved secrets into the namespaces their owning
// kustomizations create, gating each on namespace creation rather than kustomization readiness so a
// consumer whose readiness depends on its secret cannot deadlock placement. prune reclaims CLI-placed
// secrets this context no longer declares, mirroring kustomization prune (on for upgrade and apply
// --prune, off otherwise). ctx is threaded into Notify and placement so a cancelled parent context
// (e.g. Ctrl+C) tears down promptly. The blueprint must be provided. Returns an error if resolution,
// apply, or placement fails.
func (i *Provisioner) Install(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, prune bool) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	resolvedSecrets, err := i.ResolveSecrets(blueprint)
	if err != nil {
		return fmt.Errorf("error resolving secrets: %w", err)
	}

	applied := withCrdLayer(blueprint)

	if err := tui.WithProgress("Installing blueprint resources", func() error {
		if err := i.KubernetesManager.ApplyBlueprint(applied, i.fluxNamespace()); err != nil {
			return err
		}
		_ = i.Notify(ctx, applied)
		return nil
	}); err != nil {
		return fmt.Errorf("failed to apply blueprint: %w", err)
	}

	if err := i.PlaceSecrets(ctx, resolvedSecrets, applied, prune); err != nil {
		return fmt.Errorf("error placing secrets: %w", err)
	}

	// Actively drive the just-applied kustomizations toward Ready — nudging any that are stuck and forcing
	// HelmReleases that stalled (e.g. one that failed to install before its secret was placed) — for a
	// bounded window, returning early once all are Ready. Best-effort: --wait paths still gate via Wait.
	_ = i.Converge(ctx, applied, constants.DefaultConvergeTimeout)

	return nil
}

// Wait waits for kustomizations from the blueprint to be ready. It initializes the kubernetes manager
// if needed and polls the status of all kustomizations until they are ready or a timeout occurs.
// The timeout is calculated from the longest dependency chain in the blueprint. The wait honors ctx,
// so a cancelled context (caller SIGTERM/Ctrl+C or command deadline) ends it promptly. Returns an
// error if the kubernetes manager is not configured, initialization fails, or waiting times out.
func (i *Provisioner) Wait(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	blueprint = withCrdLayer(blueprint)

	if err := i.KubernetesManager.WaitForKustomizations(ctx, "Waiting for kustomizations to be ready", blueprint); err != nil {
		return fmt.Errorf("failed waiting for kustomizations: %w", err)
	}

	return nil
}

// ResolvedSecrets maps an owning kustomization name to the Secrets ready to place in the namespace(s)
// it creates: Secret name -> ResolvedSecret (target namespaces plus resolved plaintext data). It holds
// resolved secret material in memory only (never serialized), produced by ResolveSecrets before Install
// and consumed by PlaceSecrets after.
type ResolvedSecrets map[string]map[string]ResolvedSecret

// ResolvedSecret is one Secret ready to place: its resolved plaintext data and the namespaces it targets.
// Namespaces is empty when the authored entry named none, in which case placement auto-resolves the single
// namespace the owning kustomization creates.
type ResolvedSecret struct {
	Namespaces []string
	Data       map[string]string
}

// ResolveSecrets resolves every flux system's declared Secrets to plaintext ahead of Install, so a
// misconfigured secret fails the command before anything is applied to the cluster. For each compiled
// kustomization carrying Secrets it evaluates each data reference (the evaluator registers resolved
// values with the shell scrubber). A reference that resolves to nothing — nil (absent from configuration
// or a secret() lookup that failed to resolve) or an empty string — fails closed unless it carries a "??"
// default, since a required key that resolves away is misconfiguration and silently dropping it strands a
// downstream consumer with a missing key. Adding a ?? default marks the key optional and omits it. A
// secret whose keys all resolve away is not created at all, so an optional secret leaves no empty Secret
// behind. The result is keyed by owning kustomization
// for PlaceSecrets to materialize post-Install; it is empty when no kustomization declares Secrets.
func (i *Provisioner) ResolveSecrets(blueprint *blueprintv1alpha1.Blueprint) (ResolvedSecrets, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	resolved := make(ResolvedSecrets)
	bp := withCrdLayer(blueprint)
	for _, k := range bp.Kustomizations {
		if len(k.Secrets) == 0 {
			continue
		}
		for secretName, entry := range k.Secrets {
			stringData := make(map[string]string, len(entry.Data))
			for key, ref := range entry.Data {
				value, err := i.evaluator.Evaluate(ref, "", nil, true)
				if err != nil {
					return nil, fmt.Errorf("resolving secret %q key %q: %w", secretName, key, err)
				}
				s := ""
				if value != nil {
					s = fmt.Sprint(value)
				}
				if s == "" {
					if strings.Contains(ref, "??") {
						continue
					}
					return nil, fmt.Errorf("resolving secret %q key %q: reference %q resolved to empty; add a ?? default (e.g. %q) to make the key optional", secretName, key, ref, "${... ?? ''}")
				}
				// Mask the materialized value in command output regardless of how it was sourced —
				// env(), a config reference, or secret() — matching the redaction secret() already
				// gets at resolution time.
				i.shell.RegisterSecret(s)
				stringData[key] = s
			}
			if len(stringData) > 0 {
				if resolved[k.Name] == nil {
					resolved[k.Name] = make(map[string]ResolvedSecret)
				}
				resolved[k.Name][secretName] = ResolvedSecret{
					Namespaces: slices.Clone(entry.Namespaces),
					Data:       stringData,
				}
			}
		}
	}
	return resolved, nil
}

// pendingPlacement is one secret awaiting its target namespace during PlaceSecrets.
type pendingPlacement struct {
	kustomization string
	secretName    string
	secret        ResolvedSecret
}

// PlaceSecrets materializes secrets that ResolveSecrets produced into their target namespace(s), and —
// when prune is set — reconciles by deleting the CLI-placed secrets this context no longer wants. It runs
// after Install. Placement is not serialized: on each round it places every secret whose target namespace
// already exists and defers the rest, then polls, so a secret whose namespace is ready is never blocked
// behind one whose namespace is not. This matters because kustomizations commonly depend on a secret the
// CLI places (e.g. a cloud-controller-manager needs its token before nodes initialize and downstream
// namespaces appear); placing the ready ones first breaks that cycle rather than deadlocking on a namespace
// that only appears once an earlier secret is placed. For each secret it resolves the target namespace(s) —
// the ones the entry named (gated on those existing in the cluster) or, when it named none, the namespace
// its owning kustomization creates or deploys into (failing closed on more than one so a secret is never
// placed by guessing) — applies an Opaque Secret into each, and rolls the workloads there that consume it,
// keyed by a content digest, so a changed value reaches running pods rather than sitting stale. It surfaces
// a single progress line naming what it is placing or waiting on, and times out naming the secrets whose
// namespaces never appeared. As it places a secret it requests an immediate flux reconcile of the owning
// kustomization (the secret is what that kustomization was waiting on), and while waiting it nudges the
// dependency closure of the still-pending owners on a throttle, so a chain unblocked by a placement
// advances in seconds rather than one flux interval per hop. When prune is set it records every
// (namespace, secret) it placed and hands
// that desired set to PruneSecrets, which reclaims any CLI-placed secret not in it. Pruning is gated so
// secret reclaim mirrors Kustomization prune: on under apply's --prune and upgrade, off for the place-only
// flows. When no kubernetes manager is configured it is a no-op only if there is also nothing to place.
func (i *Provisioner) PlaceSecrets(ctx context.Context, resolved ResolvedSecrets, blueprint *blueprintv1alpha1.Blueprint, prune bool) error {
	if i.KubernetesManager == nil {
		if len(resolved) == 0 {
			return nil
		}
		return fmt.Errorf("kubernetes manager not configured")
	}
	if len(resolved) == 0 && !prune {
		return nil
	}

	return tui.WithProgress("Placing secrets", func() error {
		pending := make([]pendingPlacement, 0)
		for kustomizationName, secrets := range resolved {
			for secretName, secret := range secrets {
				pending = append(pending, pendingPlacement{kustomizationName, secretName, secret})
			}
		}

		placed := make(map[string]map[string]bool)
		pollInterval := i.secretPollInterval
		if pollInterval == 0 {
			pollInterval = constants.DefaultKustomizationWaitPollInterval
		}
		waitCtx, cancel := context.WithTimeout(ctx, constants.DefaultFluxKustomizationInstallTimeout)
		defer cancel()

		nudged := make(map[string]struct{})
		for len(pending) > 0 {
			var stillPending []pendingPlacement
			placedOwners := make(map[string]struct{})
			for _, p := range pending {
				namespaces, ready, err := i.trySecretNamespaces(p.kustomization, p.secret.Namespaces)
				if err != nil {
					return err
				}
				if !ready {
					stillPending = append(stillPending, p)
					continue
				}
				tui.Update(fmt.Sprintf("Placing secrets: %s (%s)", p.secretName, p.kustomization))
				for _, namespace := range namespaces {
					if err := i.KubernetesManager.ApplySecret(p.secretName, namespace, p.secret.Data, p.kustomization); err != nil {
						return fmt.Errorf("applying secret %q to namespace %q: %w", p.secretName, namespace, err)
					}
					if err := i.KubernetesManager.RollWorkloadsForSecret(ctx, namespace, p.secretName, secretDigest(p.secret.Data)); err != nil {
						return fmt.Errorf("rolling workloads for secret %q in namespace %q: %w", p.secretName, namespace, err)
					}
					if placed[namespace] == nil {
						placed[namespace] = make(map[string]bool)
					}
					placed[namespace][p.secretName] = true
				}
				placedOwners[p.kustomization] = struct{}{}
			}
			pending = stillPending

			// A secret we just placed is what its owner was waiting on — reconcile that owner now so it
			// applies and re-checks readiness immediately, and force any HelmRelease it owns that had
			// stalled for lack of the secret, rather than waiting on its scheduled interval.
			if len(placedOwners) > 0 {
				owners := sortedStringKeys(placedOwners)
				i.reconcileKustomizations(ctx, owners)
				i.forceStalledHelmReleases(ctx, owners)
			}
			if len(pending) == 0 {
				break
			}
			// Drive the DAG toward creating the still-pending namespaces by nudging only the frontier —
			// not-Ready kustomizations whose dependencies are all Ready — once each. Already-Ready
			// kustomizations (e.g. crds) are never touched, and a member blocked upstream is left alone
			// until its dependency clears, so this advances the chain without churning healthy resources.
			i.nudgeFrontier(ctx, blueprint, nudged)
			tui.Update(fmt.Sprintf("Placing secrets: waiting on %s", pendingSummary(pending)))
			select {
			case <-waitCtx.Done():
				return fmt.Errorf("timed out waiting on namespaces before placing secrets: %s", pendingSummary(pending))
			case <-time.After(pollInterval):
			}
		}

		if !prune {
			return nil
		}
		if err := i.KubernetesManager.PruneSecrets(placed); err != nil {
			return fmt.Errorf("pruning orphaned secrets: %w", err)
		}
		return nil
	})
}

// secretDigest returns a stable content digest for a secret's resolved data, used to roll consuming
// workloads only when the content actually changes. Keys are sorted and each key and value is
// length-delimited by a NUL so distinct maps cannot collide, then hashed with SHA-256. The digest is
// one-way: it detects change without revealing the plaintext it summarizes.
func secretDigest(stringData map[string]string) string {
	keys := make([]string, 0, len(stringData))
	for k := range stringData {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(stringData[k]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// trySecretNamespaces reports where a secret should be placed and whether every target namespace exists
// yet, without blocking, so PlaceSecrets can place the ready ones and poll for the rest. When the entry
// names namespaces explicitly they are the targets, gated on each existing in the cluster regardless of
// which kustomization created it — the author named the target, so a secret whose namespace is created by a
// different kustomization is not blocked on the owning kustomization's inventory. When it names none the
// target is auto-resolved from the owning kustomization's Flux inventory: the namespace it creates, or —
// when it creates none — the single namespace its resources are deployed into. Auto-resolution fails closed
// when that spans more than one namespace (ambiguous — the author must name the target via `namespaces:`),
// and once the kustomization has reconciled but offers nothing to infer from (its inventory is populated
// yet names no namespace), rather than waiting forever. A (nil, false, nil) return means "not resolvable
// yet, keep polling" — the named namespaces don't exist, or the owning kustomization has not reconciled.
// Gating on the namespace existing — not on the owning kustomization being Ready — lets a secret be placed
// the moment its namespace appears, so a consumer whose readiness depends on it can never deadlock.
func (i *Provisioner) trySecretNamespaces(kustomizationName string, explicit []string) ([]string, bool, error) {
	if len(explicit) > 0 {
		missing, err := i.missingClusterNamespaces(explicit)
		if err != nil {
			return nil, false, err
		}
		if len(missing) == 0 {
			return slices.Clone(explicit), true, nil
		}
		return nil, false, nil
	}
	entries, err := i.KubernetesManager.GetKustomizationInventory(kustomizationName, i.fluxNamespace())
	if err != nil {
		return nil, false, fmt.Errorf("reading inventory for kustomization %q: %w", kustomizationName, err)
	}
	candidates := autoResolveNamespaces(entries)
	switch {
	case len(candidates) == 1:
		return candidates, true, nil
	case len(candidates) > 1:
		return nil, false, fmt.Errorf("kustomization %q spans multiple namespaces (%s); set `namespaces:` on the secret to choose where to place it", kustomizationName, strings.Join(candidates, ", "))
	case len(entries) > 0:
		return nil, false, fmt.Errorf("kustomization %q creates no namespace and deploys no namespaced resources to infer one from; set `namespaces:` on the secret to choose where to place it", kustomizationName)
	}
	return nil, false, nil
}

// pendingSummary renders the secrets still awaiting a namespace as sorted "secret (kustomization)" entries —
// naming the explicit target namespaces when the entry declared them — for the progress and timeout lines.
func pendingSummary(pending []pendingPlacement) string {
	parts := make([]string, 0, len(pending))
	for _, p := range pending {
		if len(p.secret.Namespaces) > 0 {
			parts = append(parts, fmt.Sprintf("%s (%s)→%s", p.secretName, p.kustomization, strings.Join(p.secret.Namespaces, ",")))
		} else {
			parts = append(parts, fmt.Sprintf("%s (%s)", p.secretName, p.kustomization))
		}
	}
	slices.Sort(parts)
	return strings.Join(parts, ", ")
}

// autoResolveNamespaces infers where a secret should land when its entry names no namespace, from the
// owning kustomization's Flux inventory. It prefers the Namespace object(s) the kustomization creates; when
// it creates none, it falls back to the distinct namespaces its namespaced resources are deployed into, so
// a kustomization that deploys into a namespace another kustomization created is still resolvable. The
// result is sorted so a multi-namespace outcome reads deterministically.
func autoResolveNamespaces(entries []kubernetes.InventoryEntry) []string {
	created := make(map[string]struct{})
	deployed := make(map[string]struct{})
	for _, entry := range entries {
		if entry.Kind == "Namespace" {
			created[entry.Name] = struct{}{}
		}
		if entry.Namespace != "" {
			deployed[entry.Namespace] = struct{}{}
		}
	}
	chosen := created
	if len(chosen) == 0 {
		chosen = deployed
	}
	out := make([]string, 0, len(chosen))
	for ns := range chosen {
		out = append(out, ns)
	}
	slices.Sort(out)
	return out
}

// missingClusterNamespaces returns the members of want that do not yet exist in the cluster, preserving
// want's order. It is how placement gates a secret that names its target namespace(s) explicitly: on those
// namespaces existing, regardless of which kustomization created them.
func (i *Provisioner) missingClusterNamespaces(want []string) ([]string, error) {
	var missing []string
	for _, ns := range want {
		exists, err := i.KubernetesManager.NamespaceExists(ns)
		if err != nil {
			return nil, fmt.Errorf("checking namespace %q: %w", ns, err)
		}
		if !exists {
			missing = append(missing, ns)
		}
	}
	return missing, nil
}

// reconcileKustomizations requests an immediate flux reconcile of the named Kustomizations, best-effort: it
// initializes the notifier if needed and swallows any error, so a placement round is never failed by a
// reconcile nudge. A nil or empty names slice is a no-op.
func (i *Provisioner) reconcileKustomizations(ctx context.Context, names []string) {
	if len(names) == 0 {
		return
	}
	if err := i.ensureNotifier(); err != nil {
		return
	}
	_ = i.Notifier.ReconcileKustomizations(ctx, names)
}

// helmReleaseReady reports whether a HelmRelease currently carries a Ready=True condition.
func helmReleaseReady(hr helmv2.HelmRelease) bool {
	for _, c := range hr.Status.Conditions {
		if c.Type == "Ready" {
			return c.Status == "True"
		}
	}
	return false
}

// forceStalledHelmReleases force-reconciles any HelmRelease owned by the given kustomizations that is not
// Ready. A release that failed to install or upgrade — e.g. because a secret it needs was not yet present —
// stalls and will not retry on a plain reconcile of its owning Kustomization, since the release spec is
// unchanged; forcing it (requestedAt + forceAt) makes helm-controller retry now that its inputs may be in
// place. Best-effort: read failures and the reconcile request are swallowed, and healthy releases are left
// untouched so no needless upgrade fires.
func (i *Provisioner) forceStalledHelmReleases(ctx context.Context, kustomizations []string) {
	var refs []fluxinfra.HelmReleaseRef
	seen := make(map[string]struct{})
	for _, name := range kustomizations {
		hrs, err := i.KubernetesManager.GetHelmReleasesForKustomization(name, i.fluxNamespace())
		if err != nil {
			continue
		}
		for _, hr := range hrs {
			if helmReleaseReady(hr) {
				continue
			}
			key := hr.Namespace + "/" + hr.Name
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			refs = append(refs, fluxinfra.HelmReleaseRef{Namespace: hr.Namespace, Name: hr.Name})
		}
	}
	if len(refs) == 0 {
		return
	}
	if err := i.ensureNotifier(); err != nil {
		return
	}
	_ = i.Notifier.ReconcileHelmReleases(ctx, refs, true)
}

// Converge actively drives the blueprint's kustomizations toward Ready within timeout, best-effort. Each
// round it reads readiness (without failing on a failed kustomization) and, for every kustomization not yet
// Ready, requests an immediate reconcile and forces any stalled HelmRelease it owns — so a chain that
// stalled after apply (a dependent waiting on a now-ready dependency, or a HelmRelease that failed and
// exhausted its remediation) recovers in seconds rather than at the next flux interval. Nudges are
// throttled to keep reconcile traffic modest. It returns as soon as every kustomization is Ready, or when
// timeout elapses; it is a driver, not a gate, so a still-unready cluster is not an error here — callers
// that must fail on un-readiness use Wait afterward. A nil blueprint or missing kubernetes manager is a
// no-op.
func (i *Provisioner) Converge(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, timeout time.Duration) error {
	if blueprint == nil || i.KubernetesManager == nil {
		return nil
	}
	bp := withCrdLayer(blueprint)
	names := convergeNames(bp)
	if len(names) == 0 {
		return nil
	}

	pollInterval := i.secretPollInterval
	if pollInterval == 0 {
		pollInterval = constants.DefaultKustomizationWaitPollInterval
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return tui.WithProgress("Reconciling resources", func() error {
		nudged := make(map[string]struct{})
		for {
			notReady := i.nudgeFrontier(ctx, bp, nudged)
			if len(notReady) == 0 {
				return nil
			}
			tui.Update(fmt.Sprintf("Reconciling resources: waiting on %s", strings.Join(notReady, ", ")))
			select {
			case <-waitCtx.Done():
				return nil
			case <-time.After(pollInterval):
			}
		}
	})
}

// convergeNames returns the names of the blueprint's non-destroyOnly kustomizations, the set Converge
// drives toward Ready.
func convergeNames(bp *blueprintv1alpha1.Blueprint) []string {
	names := make([]string, 0, len(bp.Kustomizations))
	for _, k := range bp.Kustomizations {
		if k.DestroyOnly != nil && *k.DestroyOnly {
			continue
		}
		names = append(names, k.Name)
	}
	return names
}

// nudgeFrontier requests an immediate reconcile of the blueprint's "frontier" kustomizations — those not
// yet Ready whose every dependency is already Ready — and force-reconciles any stalled HelmRelease they own.
// The frontier is the only set a nudge can actually advance: an already-Ready kustomization would just churn
// (needless reconciles that flap otherwise-healthy resources), and one still blocked on a not-ready
// dependency will not progress no matter how often it is poked. Each frontier member is nudged at most once
// while it stays not-Ready — recorded in nudged and cleared when it goes Ready — so successive calls do not
// re-poke the same resource; instead, as a layer goes Ready the next layer becomes the frontier and gets its
// single nudge, walking the DAG upward by state transition rather than a blanket timer. Best-effort: a
// readiness read error is a no-op, and nudged carries the one-shot state across calls within one operation.
func (i *Provisioner) nudgeFrontier(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, nudged map[string]struct{}) []string {
	if blueprint == nil {
		return nil
	}
	names := convergeNames(blueprint)
	if len(names) == 0 {
		return nil
	}
	readiness, err := i.KubernetesManager.GetKustomizationReadiness(names)
	if err != nil {
		return names // unknown readiness: report all not-Ready so a caller keeps waiting, but nudge nothing
	}
	deps := make(map[string][]string, len(blueprint.Kustomizations))
	for _, k := range blueprint.Kustomizations {
		deps[k.Name] = k.DependsOn
	}

	var notReady, frontier []string
	for _, name := range names {
		if readiness[name] {
			delete(nudged, name) // Ready now — permit a fresh nudge if it later regresses
			continue
		}
		notReady = append(notReady, name)
		if _, done := nudged[name]; done {
			continue // already nudged while not-Ready; wait for it to progress rather than re-poking
		}
		depsReady := true
		for _, d := range deps[name] {
			if r, known := readiness[d]; known && !r {
				depsReady = false
				break
			}
		}
		if depsReady {
			frontier = append(frontier, name)
		}
	}
	if len(frontier) > 0 {
		slices.Sort(frontier)
		for _, n := range frontier {
			nudged[n] = struct{}{}
		}
		i.reconcileKustomizations(ctx, frontier)
		i.forceStalledHelmReleases(ctx, frontier)
	}
	slices.Sort(notReady)
	return notReady
}

// sortedStringKeys returns the keys of a set as a sorted slice.
func sortedStringKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// WriteVersionMarker records the blueprint version applied to this context as a marker ConfigMap
// in the gitops namespace, capturing the resolved reference of each applied source. Only bootstrap
// and upgrade write the marker; apply and plan only read it, so the marker stays an authoritative
// record of what was deliberately rolled out rather than drifting with every reconcile.
func (i *Provisioner) WriteVersionMarker(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	marker, err := kubernetes.BuildVersionMarker(blueprint)
	if err != nil {
		return fmt.Errorf("failed to build version marker: %w", err)
	}
	if err := i.KubernetesManager.ApplyVersionMarker(i.fluxNamespace(), marker); err != nil {
		return fmt.Errorf("failed to write version marker: %w", err)
	}

	return nil
}

// BeginVersionTransition writes the in-flight marker for an upgrade toward blueprint: it preserves
// the applied source set from the existing marker (empty for a legacy context) and records the
// blueprint's set as the target under the upgrading phase. apply's version gate refuses while the
// marker is non-idle; WriteVersionMarker settles it to idle on success.
func (i *Provisioner) BeginVersionTransition(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	current, found, err := i.GetVersionMarker()
	if err != nil {
		return fmt.Errorf("failed to read current version marker: %w", err)
	}
	var applied map[string]kubernetes.SourceRef
	if found {
		applied = current.AppliedSources
	}

	marker, err := kubernetes.BuildTransitionMarker(applied, blueprint)
	if err != nil {
		return fmt.Errorf("failed to build transition marker: %w", err)
	}
	if err := i.KubernetesManager.ApplyVersionMarker(i.fluxNamespace(), marker); err != nil {
		return fmt.Errorf("failed to write transition marker: %w", err)
	}
	return nil
}

// Prune removes Kustomizations belonging to this context that are no longer present in the
// blueprint, leaving every still-declared kustomization (platform and user) and any other
// context's kustomizations untouched. It prepares the blueprint with the synthesized CRD layers —
// matching what Install applied — so those layers are recognized as desired and not pruned.
func (i *Provisioner) Prune(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	blueprint = withCrdLayer(blueprint)

	if err := i.KubernetesManager.PruneBlueprint(blueprint, i.fluxNamespace()); err != nil {
		return fmt.Errorf("failed to prune blueprint: %w", err)
	}

	return nil
}

// PrunableKustomizations returns the names of this context's Kustomizations that the blueprint no
// longer declares — exactly what Prune would delete. It is the read-only input to plan's prune
// preview and upgrade's confirmation gate; it deletes nothing. It prepares the blueprint with the
// synthesized CRD layers so the desired set matches what Prune deletes against.
func (i *Provisioner) PrunableKustomizations(blueprint *blueprintv1alpha1.Blueprint) ([]string, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}
	if i.KubernetesManager == nil {
		return nil, fmt.Errorf("kubernetes manager not configured")
	}

	blueprint = withCrdLayer(blueprint)

	names, err := i.KubernetesManager.ListPrunableKustomizations(blueprint, i.fluxNamespace())
	if err != nil {
		return nil, fmt.Errorf("failed to list prunable kustomizations: %w", err)
	}
	return names, nil
}

// GetVersionMarker reads the applied-version marker for this context's gitops namespace, reporting
// false when no marker exists (a pre-bootstrap, no-cluster, or legacy context). apply and plan read
// the marker to gate on the blueprint version; only bootstrap and upgrade write it. Returns false
// without consulting the cluster when no context-scoped kubeconfig is present — there is no cluster,
// so nothing is applied and there is no version to read.
func (i *Provisioner) GetVersionMarker() (kubernetes.VersionMarker, bool, error) {
	if i.KubernetesManager == nil {
		return kubernetes.VersionMarker{}, false, fmt.Errorf("kubernetes manager not configured")
	}
	if !i.kubeconfigPresent() {
		return kubernetes.VersionMarker{}, false, nil
	}
	return i.KubernetesManager.GetVersionMarker(i.fluxNamespace())
}

// CheckVersionGate reads the version marker and reports how the given blueprint relates to it,
// without deciding policy. A missing marker (pre-bootstrap, no cluster, or legacy) yields
// MarkerFound=false so callers treat it as "nothing applied yet — proceed". When a marker exists,
// InFlight reflects a non-idle phase and VersionMatch compares the blueprint's resolved source-ref
// set against the applied set. Returns an error only on a real read/decode failure or when the
// blueprint's sources cannot be reduced to an unambiguous set; callers may treat a read failure as
// best-effort (cluster unreachable) rather than a hard stop.
func (i *Provisioner) CheckVersionGate(blueprint *blueprintv1alpha1.Blueprint) (VersionGate, error) {
	if blueprint == nil {
		return VersionGate{}, fmt.Errorf("blueprint not provided")
	}

	marker, found, err := i.GetVersionMarker()
	if err != nil {
		return VersionGate{}, err
	}
	if !found {
		return VersionGate{MarkerFound: false}, nil
	}

	target, err := kubernetes.BuildVersionMarker(blueprint)
	if err != nil {
		return VersionGate{}, fmt.Errorf("failed to derive blueprint version: %w", err)
	}

	return VersionGate{
		MarkerFound:  true,
		InFlight:     marker.Phase != "" && marker.Phase != kubernetes.VersionMarkerPhaseIdle,
		VersionMatch: kubernetes.SourcesEqual(marker.AppliedSources, target.AppliedSources),
	}, nil
}

// Uninstall orchestrates the high-level kustomization teardown process from the blueprint.
// It initializes the kubernetes manager and deletes all blueprint kustomizations, including the
// synthesized CRD layer (withCrdLayer) so its Flux Kustomization objects are removed symmetrically
// with apply; the layer keeps Prune disabled, so the vendored CRDs themselves are retained.
// DeleteBlueprint emits its own per-Kustomization progress (Start/Done/Fail spinners),
// so this method does not wrap the call in WithProgress — doing so would suppress the
// inner per-Kustomization output and produce a single opaque "Removing blueprint
// resources" line that hides the long per-Kustomization waits inherent to
// WaitForTermination-driven teardown.
func (i *Provisioner) Uninstall(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	blueprint = withCrdLayer(blueprint)

	if err := i.KubernetesManager.DeleteBlueprint(blueprint, i.fluxNamespace()); err != nil {
		return fmt.Errorf("failed to delete blueprint: %w", err)
	}

	return nil
}

// UpgradeNode performs a complete per-node upgrade: sends the upgrade gRPC request (wait=false),
// waits for the node to go offline via version polling (offlineTimeout caps this phase),
// waits for the node to come back healthy, then performs a final service health check.
// outputFunc receives status messages during the wait phases. Returns an error if any
// step fails or times out.
func (i *Provisioner) UpgradeNode(ctx context.Context, node string, image string, offlineTimeout time.Duration, outputFunc func(string)) error {
	if err := i.ensureClusterClient(); err != nil {
		return err
	}
	defer i.ClusterClient.Close()

	nodes := []string{node}

	if outputFunc != nil {
		outputFunc(fmt.Sprintf("Sending upgrade request to node %s...", node))
	}
	if err := i.ClusterClient.UpgradeNodes(ctx, nodes, image); err != nil {
		return fmt.Errorf("upgrade request failed: %w", err)
	}

	if outputFunc != nil {
		outputFunc(fmt.Sprintf("Upgrade request sent to %s. Waiting for reboot...", node))
	}
	if err := i.ClusterClient.WaitForNodesReboot(ctx, nodes, versionFromImage(image), nil, offlineTimeout); err != nil {
		return fmt.Errorf("node reboot wait failed: %w", err)
	}

	if outputFunc != nil {
		outputFunc(fmt.Sprintf("Verifying kube-apiserver readiness on %s (skipped for workers)...", node))
	}
	if err := i.ClusterClient.WaitForControlPlaneAPIReady(ctx, node, outputFunc); err != nil {
		return fmt.Errorf("kube-apiserver readiness check failed: %w", err)
	}

	if outputFunc != nil {
		outputFunc(fmt.Sprintf("Node %s upgraded successfully.", node))
	}
	return nil
}

// UpgradeNodes sends an upgrade request to specified cluster nodes.
// It initializes the cluster client based on config, then calls UpgradeNodes on it.
// The caller is responsible for subsequently monitoring reboot status. Returns an error
// if the cluster client cannot be initialized or if any node upgrade request fails.
func (i *Provisioner) UpgradeNodes(ctx context.Context, nodes []string, image string) error {
	if err := i.ensureClusterClient(); err != nil {
		return err
	}
	defer i.ClusterClient.Close()

	return i.ClusterClient.UpgradeNodes(ctx, nodes, image)
}

// CheckNodeHealth performs health checks for cluster nodes and Kubernetes endpoints.
// It supports checking node health via cluster client (for Talos/Omni clusters) and/or
// Kubernetes API health checks. The method handles timeout configuration, version checking,
// and node readiness verification. Returns an error if any health check fails.
func (i *Provisioner) CheckNodeHealth(ctx context.Context, options NodeHealthCheckOptions, outputFunc func(string)) error {
	hasNodeCheck := len(options.Nodes) > 0
	hasK8sCheck := options.K8SEndpointProvided

	if !hasNodeCheck && !hasK8sCheck {
		return fmt.Errorf("no health checks specified. Use --nodes and/or --k8s-endpoint flags to specify health checks to perform")
	}

	if hasNodeCheck {
		_ = i.ensureClusterClient() // best-effort; nil client is tolerated when hasK8sCheck

		if i.ClusterClient == nil {
			if !hasK8sCheck {
				return fmt.Errorf("no health checks specified. Use --nodes and/or --k8s-endpoint flags to specify health checks to perform")
			}
			// If we have k8s check, we can continue without cluster client
		}

		if i.ClusterClient != nil {
			defer i.ClusterClient.Close()

			var checkCtx context.Context
			var cancel context.CancelFunc
			if options.Timeout > 0 {
				checkCtx, cancel = context.WithTimeout(ctx, options.Timeout)
			} else {
				checkCtx, cancel = context.WithCancel(ctx)
			}
			defer cancel()

			var clusterErr error
			if options.WaitForReboot {
				clusterErr = i.ClusterClient.WaitForNodesReboot(checkCtx, options.Nodes, options.Version, options.SkipServices, options.OfflineTimeout)
			} else {
				clusterErr = i.ClusterClient.WaitForNodesHealthy(checkCtx, options.Nodes, options.Version, options.SkipServices)
			}
			if err := clusterErr; err != nil {
				if hasK8sCheck {
					if outputFunc != nil {
						outputFunc(fmt.Sprintf("Warning: Cluster client failed (%v), continuing with Kubernetes checks\n", err))
					}
				} else {
					return fmt.Errorf("nodes failed health check: %w", err)
				}
			} else {
				if outputFunc != nil {
					message := fmt.Sprintf("All %d nodes are healthy", len(options.Nodes))
					if options.Version != "" {
						message += fmt.Sprintf(" and running version %s", options.Version)
					}
					outputFunc(message)
				}
			}
		}
	}

	if hasK8sCheck {
		if i.KubernetesManager == nil {
			return fmt.Errorf("no kubernetes manager found")
		}

		k8sEndpointStr := options.K8SEndpoint
		if k8sEndpointStr == "true" {
			k8sEndpointStr = ""
		}

		var nodeNames []string
		if options.CheckNodeReady {
			if hasNodeCheck {
				nodeNames = options.Nodes
			} else {
				return fmt.Errorf("--ready flag requires --nodes to be specified")
			}
		}

		if len(nodeNames) > 0 && outputFunc != nil {
			outputFunc(fmt.Sprintf("Waiting for %d nodes to be Ready...", len(nodeNames)))
		}

		if err := i.KubernetesManager.WaitForKubernetesHealthy(ctx, k8sEndpointStr, outputFunc, nodeNames...); err != nil {
			return fmt.Errorf("kubernetes health check failed: %w", err)
		}

		if outputFunc != nil {
			if len(nodeNames) > 0 {
				readyStatus, err := i.KubernetesManager.GetNodeReadyStatus(ctx, nodeNames)
				allFoundAndReady := err == nil && len(readyStatus) == len(nodeNames)
				for _, ready := range readyStatus {
					if !ready {
						allFoundAndReady = false
						break
					}
				}

				if allFoundAndReady {
					if k8sEndpointStr != "" {
						outputFunc(fmt.Sprintf("Kubernetes API endpoint %s is healthy and all nodes are Ready", k8sEndpointStr))
					} else {
						outputFunc("Kubernetes API endpoint (kubeconfig default) is healthy and all nodes are Ready")
					}
				} else {
					if k8sEndpointStr != "" {
						outputFunc(fmt.Sprintf("Kubernetes API endpoint %s is healthy", k8sEndpointStr))
					} else {
						outputFunc("Kubernetes API endpoint (kubeconfig default) is healthy")
					}
				}
			} else {
				if k8sEndpointStr != "" {
					outputFunc(fmt.Sprintf("Kubernetes API endpoint %s is healthy", k8sEndpointStr))
				} else {
					outputFunc("Kubernetes API endpoint (kubeconfig default) is healthy")
				}
			}
		}
	}

	return nil
}

// Notify forwards to the flux webhook Notifier and is intended as the final
// step of bootstrap/up/apply so flux reconciles the blueprint's sources
// immediately instead of waiting for its next scheduled interval. The call
// is best-effort: every failure path inside the Notifier is converted to
// nil with a warning, so callers can invoke Notify unconditionally without
// risking command failure on clusters that have no webhook configured.
func (i *Provisioner) Notify(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) error {
	if err := i.ensureNotifier(); err != nil {
		return err
	}
	return i.Notifier.Notify(ctx, blueprint)
}

// Close releases resources held by provisioner components.
// It closes cluster client connections if present. This method should be called when the
// provisioner instance is no longer needed to clean up resources.
func (i *Provisioner) Close() {
	if i.ClusterClient != nil {
		i.ClusterClient.Close()
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// recoverHalfMigratedComponents migrates leftover local state to the
// configured remote backend for components with local state but no remote
// state — typical residue from an interrupted bootstrap. Per affected
// component: init under local override (resets the pointer to local), exit
// override, migrate local→remote, remove the local file. Local-backend
// contexts short-circuit. Probe failures abort with the underlying error
// rather than fall through; -force-copy would otherwise overwrite good
// remote state with stale local content.
func (i *Provisioner) recoverHalfMigratedComponents(blueprint *blueprintv1alpha1.Blueprint) error {
	backendType := i.configHandler.GetString("terraform.backend.type", "local")
	if backendType == "" || backendType == "local" {
		return nil
	}

	for _, c := range blueprint.TerraformComponents {
		if c.Enabled != nil && !c.Enabled.IsEnabled() {
			continue
		}
		componentID := c.GetID()

		hasLocal, err := i.HasLocalStateWithResources(componentID)
		if err != nil {
			return fmt.Errorf("error inspecting local state for %s during recovery sweep: %w", componentID, err)
		}
		if !hasLocal {
			continue
		}

		hasRemote, err := i.HasRemoteState(blueprint, componentID)
		if err != nil {
			return fmt.Errorf("recovery sweep aborted: could not probe configured backend for %q: %w. The reset-and-migrate path uses terraform init -migrate-state -force-copy which would unconditionally overwrite the destination, so a transient probe failure (auth, network, missing backend storage) must not be assumed-equivalent to \"no remote state\" — that assumption could silently replace valid remote state with the local file. Resolve the underlying probe failure (check credentials, connectivity, and backend storage availability) and retry", componentID, err)
		}
		if hasRemote {
			continue
		}

		message := fmt.Sprintf("Migrating leftover local state for %s → %s", componentID, backendType)
		if err := tui.WithProgress(message, func() error {
			if err := i.withBackendOverride("local-recovery-init", func() error {
				return i.InitComponent(blueprint, componentID)
			}); err != nil {
				return fmt.Errorf("error resetting backend pointer: %w", err)
			}

			if err := i.MigrateComponentState(blueprint, componentID); err != nil {
				return fmt.Errorf("error migrating local state: %w", err)
			}

			if err := i.RemoveLocalState(componentID); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to remove local state file for %q after recovery migration: %v\n", componentID, err)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("recovery sweep failed for %s: %w", componentID, err)
		}
	}
	return nil
}

// ensureClusterClient initializes ClusterClient from config if it is not already set.
// It reads cluster.driver from the config handler and creates the appropriate client.
// Returns an error if no supported driver is configured.
func (i *Provisioner) ensureClusterClient() error {
	if i.ClusterClient != nil {
		return nil
	}
	clusterDriver := i.configHandler.GetString("cluster.driver", "")
	if values, err := i.configHandler.GetContextValues(); err == nil && values != nil {
		if clusterMap, ok := values["cluster"].(map[string]any); ok {
			if driver, ok := clusterMap["driver"].(string); ok {
				clusterDriver = driver
			}
		}
	}
	if clusterDriver == "talos" {
		i.ClusterClient = cluster.NewTalosClusterClient()
	}
	if i.ClusterClient == nil {
		return fmt.Errorf("no cluster client found; ensure cluster.driver is configured")
	}
	return nil
}

// withCrdLayer returns a copy of the blueprint with synthesized CRD kustomizations prepended ahead of
// the stack. Pruning is disabled (pruning a CRD deletes every custom resource of that kind cluster-wide)
// and wait is enabled so dependents block until the CRDs are Established. Returns the blueprint unchanged
// when it implies no CRD layers.
func withCrdLayer(bp *blueprintv1alpha1.Blueprint) *blueprintv1alpha1.Blueprint {
	if bp == nil {
		return bp
	}
	layers := blueprint.CrdLayers(bp)
	if len(layers) == 0 {
		return bp
	}
	prune := false
	wait := true
	crds := make([]blueprintv1alpha1.Kustomization, 0, len(layers))
	for _, layer := range layers {
		crds = append(crds, blueprintv1alpha1.Kustomization{
			Name:       blueprintv1alpha1.CrdKustomizationName(layer.Source),
			Path:       blueprintv1alpha1.CrdLayerName,
			Source:     layer.Source,
			Components: slices.Clone(layer.Refs),
			Prune:      &prune,
			Wait:       &wait,
		})
	}
	out := *bp
	out.Kustomizations = append(crds, bp.AllKustomizations()...)
	out.FluxSystems = nil
	return &out
}

// versionFromImage extracts the Talos version from an image URI tag.
// It returns the tag with the leading "v" stripped to match the format returned
// by the Talos Version API. Returns an empty string if no tag is present or if
// the reference uses a digest instead of a tag.
func versionFromImage(image string) string {
	if strings.Contains(image, "@") {
		return ""
	}
	idx := strings.LastIndex(image, ":")
	if idx < 0 {
		return ""
	}
	tag := image[idx+1:]
	if strings.Contains(tag, "/") {
		return ""
	}
	return strings.TrimPrefix(tag, "v")
}

// ensureTerraformStack initializes the TerraformStack if terraform is enabled and the stack is not already initialized.
// Returns an error if initialization fails, or nil if terraform is disabled or already initialized.
func (i *Provisioner) ensureTerraformStack() error {
	if i.TerraformStack != nil {
		return nil
	}
	if i.configHandler.GetBool("terraform.enabled", true) {
		i.TerraformStack = terraforminfra.NewStack(i.runtime)
	}
	return nil
}

// ensureFluxStack initializes the FluxStack if it is not already initialized.
func (i *Provisioner) ensureFluxStack() error {
	if i.FluxStack != nil {
		return nil
	}
	i.FluxStack = fluxinfra.NewStack(i.runtime, i.KubernetesManager)
	return nil
}

// ensureNotifier initializes the flux webhook Notifier if it is not already
// initialized. Mirrors ensureFluxStack — the Notifier is only constructed when
// something actually calls Notify, which keeps Provisioner construction cheap
// for commands that do not need webhook notification. The runtime and
// KubernetesClient guards convert struct-literal misuse (a *Provisioner
// constructed outside NewProvisioner in a test or future caller) from a
// panic-inside-spinner into a graceful error the caller can log or swallow.
func (i *Provisioner) ensureNotifier() error {
	if i.Notifier != nil {
		return nil
	}
	if i.runtime == nil {
		return fmt.Errorf("runtime not initialized")
	}
	if i.KubernetesClient == nil {
		return fmt.Errorf("kubernetes client not initialized")
	}
	i.Notifier = fluxinfra.NewNotifier(i.runtime, i.KubernetesClient)
	return nil
}

// fluxNamespace returns the configured gitops namespace, defaulting to DefaultGitopsNamespace.
func (i *Provisioner) fluxNamespace() string {
	return i.configHandler.GetString("gitops.namespace", constants.DefaultGitopsNamespace)
}

// kubeconfigPresent reports whether the context-scoped kubeconfig file exists on
// disk. Used by destroy paths to decide whether to attempt kustomization deletion:
// the cluster is gone (or was never bootstrapped past terraform) when the file is
// missing, so trying to talk to its API would fail with a stat error and abort the
// whole destroy. Treating it as a clean signal lets `windsor destroy` be idempotent
// across partial-tear-down recoveries. Returns false when configRoot is empty —
// without a config root we have no path to probe and the conservative answer is
// "no cluster," which is consistent with the same scenarios that produce empty
// configRoot (no context selected, no init).
//
// Only fs.ErrNotExist returns false. Permission errors and other stat failures
// fall through as "present" so the subsequent Uninstall surfaces a clear error
// rather than silently skipping kustomization cleanup and leaking finalizers
// when terraform later yanks the cluster's underlying infra.
func (i *Provisioner) kubeconfigPresent() bool {
	if i.configRoot == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(i.configRoot, ".kube", "config"))
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		return false
	}
	return true
}

// checkKubernetesReachableForDestroy fails fast if the cluster is unreachable, so terraform's
// kubernetes/helm provider doesn't hang against broken auth. No-op when there's no kubeconfig.
func (i *Provisioner) checkKubernetesReachableForDestroy() error {
	if i.KubernetesManager == nil || !i.kubeconfigPresent() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultKubernetesReachabilityCheckTimeout)
	defer cancel()
	if err := i.KubernetesManager.WaitForKubernetesHealthy(ctx, "", nil); err != nil {
		return fmt.Errorf("kubernetes API is unreachable, refusing to run terraform destroy (its kubernetes/helm provider would hang against a broken cluster): %w", err)
	}
	return nil
}
