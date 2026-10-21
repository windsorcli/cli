// The Provisioner runs the infrastructure lifecycle of a blueprint for the Windsor CLI.
// It provides Up and Apply for terraform, the terraform state methods, and Notify.
// It creates the terraform stack, the flux stack, the kubernetes manager, and the cluster client.
// It recovers half-migrated components and warns about orphaned local terraform state.

package provisioner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
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
)

// =============================================================================
// Errors
// =============================================================================

// ErrKustomizationsNotReady marks an Install or ApplyKustomize error from a kustomization that did not become Ready.
var ErrKustomizationsNotReady = errors.New("kustomizations not ready")

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
	shims         *Shims

	// secretPollInterval overrides how often PlaceSecrets re-checks for a pending secret's namespace;
	// zero uses constants.DefaultKustomizationWaitPollInterval. Set small in tests to avoid real waits.
	secretPollInterval time.Duration

	TerraformStack       terraforminfra.Stack
	FluxStack            fluxinfra.Stack
	Notifier             fluxinfra.Notifier
	onTerraformApply     []func(ctx context.Context, id string) (bool, error)
	onTerraformPostApply []func(ctx context.Context, id string) error
	KubernetesManager    kubernetes.KubernetesManager
	KubernetesClient     k8sclient.KubernetesClient
	ClusterClient        cluster.ClusterClient
	blueprintHandler     blueprint.BlueprintHandler
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
		shims:            NewShims(),
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
		provisioner.KubernetesClient = k8sclient.NewDynamicKubernetesClient(rt.Shell)
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
func (i *Provisioner) OnTerraformApply(fn func(ctx context.Context, id string) (bool, error)) {
	if fn != nil {
		i.onTerraformApply = append(i.onTerraformApply, fn)
	}
}

// OnTerraformPostApply registers a hook to run after each Terraform component's Done line is printed.
// Use this for operations that must not run inside the progress spinner (e.g. interactive sudo prompts).
func (i *Provisioner) OnTerraformPostApply(fn func(ctx context.Context, id string) error) {
	if fn != nil {
		i.onTerraformPostApply = append(i.onTerraformPostApply, fn)
	}
}

// Up orchestrates the high-level infrastructure deployment process. When the blueprint
// declares a backend it pivots through local state first, then the configured backend
// (applyWithBackendPivot) — unconfirmed, same as any other apply through this method,
// including from cmd/apply.go and cmd/up.go; confirmation is a cmd/bootstrap.go concern, not
// this method's. Without a declared backend it applies directly. A kubernetes backend with
// no declared backend and no kubeconfig yet is refused with an actionable error instead of a
// raw terraform connection failure. On a remote backend, a declared Backend that names no real
// component is refused rather than silently treated as unset — see resolveBackendComponents.
// Returns (halted, err); halted=true means a hook stopped after a component. The apply runs as
// one "Applying terraform" operation, so each component shows as a step under it.
func (i *Provisioner) Up(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
	if blueprint == nil {
		return false, fmt.Errorf("blueprint not provided")
	}
	if err := i.checkOrphanedLocalState(blueprint); err != nil {
		return false, err
	}
	var halted bool
	err := presenter.Track(ctx, "terraform", "Applying terraform", func(ctx context.Context) error {
		var err error
		halted, err = i.up(ctx, blueprint, onApply...)
		return err
	})
	return halted, err
}

// Apply runs terraform init, plan, and apply for a single component identified by componentID.
// Returns an error if terraform is disabled, the stack cannot be initialized, the component is
// not found, or any terraform operation fails.
func (i *Provisioner) Apply(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return fmt.Errorf("terraform is disabled")
	}
	if err := i.checkOrphanedLocalState(blueprint); err != nil {
		return err
	}
	if err := i.TerraformStack.Apply(ctx, blueprint, componentID); err != nil {
		return werror.Wrap(err, "failed to run terraform apply for %s", componentID)
	}
	return nil
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
func (i *Provisioner) MigrateState(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) ([]string, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return nil, err
	}
	if i.TerraformStack == nil {
		return nil, nil
	}
	skipped, err := i.TerraformStack.MigrateState(ctx, blueprint)
	if err != nil {
		return skipped, werror.Wrap(err, "failed to migrate terraform state")
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
		return werror.Wrap(err, "failed to migrate terraform state for %s", componentID)
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

// ListLocalStateComponentIDs returns every componentID with local Terraform state on disk,
// whether or not it is still declared in the active blueprint.
func (i *Provisioner) ListLocalStateComponentIDs() ([]string, error) {
	if err := i.ensureTerraformStack(); err != nil {
		return nil, err
	}
	if i.TerraformStack == nil {
		return nil, fmt.Errorf("terraform is disabled")
	}
	return i.TerraformStack.ListLocalStateComponentIDs()
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

// up applies the blueprint directly, or through the backend pivot when the blueprint declares a
// backend on a remote backend type. A kubernetes backend type with no declared backend and no
// kubeconfig returns an error with a remediation hint.
func (i *Provisioner) up(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {

	backendType := i.configHandler.GetTerraformBackendType()
	applyFlat := func() (bool, error) {
		if backendType == "kubernetes" && hasEnabledTerraformComponent(blueprint) && !i.kubeconfigPresent() {
			return false, werror.New("context has no kubeconfig (cluster not yet created) and the blueprint declares no backend for the kubernetes backend type", "Add backend: <component-id> to the blueprint to name the component that creates the cluster, or set terraform.backend.type to local until the cluster exists.", nil)
		}
		return i.applyDirect(ctx, blueprint, onApply...)
	}
	if backendType == "" || backendType == "local" {
		return applyFlat()
	}

	backendComponents, err := resolveBackendComponents(blueprint)
	if err != nil {
		return false, err
	}
	if len(backendComponents) == 0 {
		return applyFlat()
	}
	return i.applyWithBackendPivot(ctx, blueprint, backendComponents, onApply...)
}

// applyDirect runs terraform apply directly against the configured backend, with no backend
// detection or pivot. Used by Up when no backend applies, and by applyWithBackendPivot's Stage
// 1/3 sub-applies, which must not re-derive a backend from an already-sliced blueprint.
func (i *Provisioner) applyDirect(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
	if err := i.ensureTerraformStack(); err != nil {
		return false, err
	}
	if i.TerraformStack == nil {
		return false, nil
	}
	if err := i.recoverHalfMigratedComponents(ctx, blueprint); err != nil {
		return false, err
	}
	hooks := append([]func(ctx context.Context, id string) (bool, error){}, i.onTerraformApply...)
	hooks = append(hooks, onApply...)
	if len(i.onTerraformPostApply) > 0 {
		i.TerraformStack.PostApply(i.onTerraformPostApply...)
	}
	halted, err := i.TerraformStack.Up(ctx, blueprint, hooks...)
	if err != nil {
		return false, werror.Wrap(err, "failed to run terraform up")
	}
	return halted, nil
}

// recoverHalfMigratedComponents migrates leftover local state to the configured remote backend.
// It targets each component with local state and no remote state, such as after an interrupted bootstrap.
// It does nothing on a local backend. A disabled component still counts as declared for checkOrphanedLocalState.
// A failed probe of the remote state returns an error with a remediation hint, because the migration
// uses -force-copy and can overwrite valid remote state with a stale local file.
func (i *Provisioner) recoverHalfMigratedComponents(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) error {
	backendType := i.configHandler.GetTerraformBackendType()
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
			return werror.Wrap(err, "error inspecting local state for %s during recovery sweep", componentID)
		}
		if !hasLocal {
			continue
		}

		hasRemote, err := i.HasRemoteState(blueprint, componentID)
		if err != nil {
			return werror.New(fmt.Sprintf("recovery sweep aborted: could not probe configured backend for %q: %v", componentID, err), "Check the credentials, the network connection, and the backend storage, then run the command again. Windsor stops because a state migration can overwrite valid remote state.", err)
		}
		if hasRemote {
			continue
		}

		message := fmt.Sprintf("Migrating leftover local state for %s → %s", componentID, backendType)
		if err := presenter.Track(ctx, "terraform:"+componentID, message, func(ctx context.Context) error {
			if err := i.withBackendOverride("local-recovery-init", func() error {
				return i.InitComponent(blueprint, componentID)
			}); err != nil {
				return werror.Wrap(err, "error resetting backend pointer")
			}

			if err := i.MigrateComponentState(blueprint, componentID); err != nil {
				return werror.Wrap(err, "error migrating local state")
			}

			if err := i.RemoveLocalState(componentID); err != nil {
				presenter.Warn(ctx, fmt.Sprintf("failed to remove local state file for %q after recovery migration: %v", componentID, err))
			}
			return nil
		}); err != nil {
			return werror.Wrap(err, "recovery sweep failed for %s", componentID)
		}
	}

	return nil
}

// checkOrphanedLocalState warns about local Terraform state under a componentID no longer in
// the blueprint. A rename is the usual cause.
//
// The check does not depend on the configured backend type. A local backend can strand state
// under an old componentID just as a remote one can.
//
// Call it once per operation, with the full, unsliced blueprint. It ignores any
// terraform.backend.type override already in effect. Callers do not need to sequence it around
// one. A disabled terraform stack has no local state to check, so this is a no-op then, not an
// error.
func (i *Provisioner) checkOrphanedLocalState(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return err
	}
	if i.TerraformStack == nil {
		return nil
	}

	currentIDs := make(map[string]bool, len(blueprint.TerraformComponents))
	for _, c := range blueprint.TerraformComponents {
		currentIDs[c.GetID()] = true
	}
	return i.warnAboutOrphanedLocalState(currentIDs)
}

// warnAboutOrphanedLocalState finds componentIDs with local Terraform state on disk that are no
// longer in the blueprint. A rename is the usual cause.
//
// It only detects and warns. It never migrates or removes anything. An orphaned ID is just as
// consistent with a proper destroy-then-remove as with a rename. The two look the same from an
// on-disk ID alone. Force-copying into the shared remote backend is not safe to run unattended
// for either case: a decommissioned component's stale local state would get published into the
// shared backend forever, under an ID nothing will ever reference again.
//
// A failure to inspect local state is reported as a warning, not a returned error. This check is
// advisory, so it must not block the apply or destroy it runs alongside.
func (i *Provisioner) warnAboutOrphanedLocalState(currentIDs map[string]bool) error {
	localIDs, err := i.ListLocalStateComponentIDs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not check for orphaned local terraform state: %v\n", err)
		return nil
	}

	for _, componentID := range localIDs {
		if currentIDs[componentID] {
			continue
		}

		hasLocal, err := i.HasLocalStateWithResources(componentID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not inspect local terraform state for %q: %v\n", componentID, err)
			continue
		}
		if !hasLocal {
			continue
		}

		fmt.Fprintf(os.Stderr, "warning: found local terraform state for %q, which is no longer in the blueprint; state was not migrated automatically — if this component was renamed, re-add it under this ID to migrate its state, or reconcile it manually\n", componentID)
	}
	return nil
}

// pollInterval returns the readiness poll interval: the configured secretPollInterval, or the
// default kustomization wait poll interval when none is set.
func (i *Provisioner) pollInterval() time.Duration {
	if i.secretPollInterval > 0 {
		return i.secretPollInterval
	}
	return constants.DefaultKustomizationWaitPollInterval
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
	_, err := i.shims.Stat(filepath.Join(i.configRoot, ".kube", "config"))
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		return false
	}
	return true
}

// =============================================================================
// Helpers
// =============================================================================

// hasEnabledTerraformComponent reports whether the blueprint has at least one terraform
// component that isn't explicitly disabled. Guards against refusing an apply for a
// kubernetes backend when there is nothing to apply in the first place.
func hasEnabledTerraformComponent(blueprint *blueprintv1alpha1.Blueprint) bool {
	for _, c := range blueprint.TerraformComponents {
		if c.Enabled == nil || c.Enabled.IsEnabled() {
			return true
		}
	}
	return false
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
