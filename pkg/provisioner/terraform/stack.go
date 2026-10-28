// The TerraformStack runs terraform for the components of a blueprint.
// It provides Up, Apply, and InitComponent, and the Stack interface that the provisioner uses.
// It prepares the terraform environment of each component and runs terraform init.
// It recovers from a stale provider lock and from a stale backend pointer.

package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/debug"
	"github.com/windsorcli/cli/pkg/runtime"
	envvars "github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/tui"
)

// =============================================================================
// Constants
// =============================================================================

// defaultInitFlags are passed to `terraform init` for normal apply/inspect operations.
// -force-copy is intentionally absent: it suppresses terraform's "yes, copy the state"
// safety prompt, which is only desired when the operator has explicitly asked for state
// migration. Apply/Up/Plan/Destroy paths inherit no migration intent, so leaving the
// prompt available means an unintended migration (e.g. a backend pointer mismatch)
// surfaces as a clear refusal rather than silently auto-copying state. MigrateState and
// MigrateComponentState pass `-migrate-state -force-copy` explicitly when migration is
// the requested operation; see migrateOneComponent.
//
// -input=false runs init non-interactively. ExecSilentWithEnv never sets cmd.Stdin, so a
// terraform prompt (e.g. the backend-migration confirmation triggered on the next init after
// destroy clears state) would read EOF from a closed stdin and abort with an opaque "Error
// asking for confirmation: EOF". -input=false makes that surface as a clear, fail-fast error
// instead of a stdin read against nothing.
var defaultInitFlags = []string{"-upgrade", "-input=false"}

// componentNotFoundHint is the remediation for a component ID that names no enabled terraform component.
const componentNotFoundHint = "Use the ID of an enabled terraform component. Run windsor show blueprint to see the components."

// =============================================================================
// Types
// =============================================================================

// TerraformStack manages Terraform infrastructure components by initializing and applying Terraform configurations.
// It processes components in order, generating terraform arguments, running Terraform init, plan, and apply operations.
// Progress and non-blocking warnings go to the presenter in the context of each operation.
type TerraformStack struct {
	runtime                *runtime.Runtime
	shims                  *Shims
	terraformEnv           *envvars.TerraformEnvPrinter
	postApply              []func(ctx context.Context, id string) error
	initCache              map[initCacheKey]struct{}
	initCacheMu            sync.Mutex
	destroyRetryAttempts   int
	destroyRetryBackoff    time.Duration
	destroyIdleTimeout     time.Duration
	destroyAbsoluteTimeout time.Duration
	destroyGracePeriod     time.Duration
}

// initCacheKey identifies a previously-completed `terraform init`. Four
// dimensions distinguish whether re-init is a no-op: component, configured
// backend, kubernetes backend namespace, migrate-state flag.
type initCacheKey struct {
	componentID  string
	backendType  string
	namespace    string
	migrateState bool
}

// ComponentFailure is a per-component error captured during continue-on-error
// destroy. ID is the terraform component's blueprint identifier; Err is the
// terraform destroy error that the operator can use to diagnose.
type ComponentFailure struct {
	ID  string
	Err error
}

// =============================================================================
// Interfaces
// =============================================================================

// Stack defines the interface for Terraform stack operations.
// Both the Stack struct and MockStack implement this interface.
//
// onApply hooks return (haltAfter, err). A non-nil err is a real failure and aborts the loop
// the same as before. haltAfter=true means the component apply succeeded but subsequent
// components should NOT be applied — used to defer privileged host configuration to an
// explicit 'windsor configure network' invocation by the operator. Stack.Up returns
// (halted bool, err error); halted=true indicates a clean stop, not a failure.
type Stack interface {
	Up(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error)
	MigrateState(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) ([]string, error)
	MigrateComponentState(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	HasRemoteState(blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error)
	HasLocalStateWithResources(componentID string) (bool, error)
	InitComponent(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	RemoveLocalState(componentID string) error
	ListLocalStateComponentIDs() ([]string, error)
	PostApply(fns ...func(ctx context.Context, id string) error)
	DestroyAll(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, continueOnError bool, excludeIDs ...string) (DestroyOutcome, error)
	Plan(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	PlanAll(blueprint *blueprintv1alpha1.Blueprint) error
	PlanJSON(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	PlanAllJSON(blueprint *blueprintv1alpha1.Blueprint) error
	PlanResourceChangesJSON(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	PlanAllResourceChangesJSON(blueprint *blueprintv1alpha1.Blueprint) error
	Apply(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	Destroy(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error)
	PlanSummary(blueprint *blueprintv1alpha1.Blueprint) []TerraformComponentPlan
	PlanComponentSummary(blueprint *blueprintv1alpha1.Blueprint, componentID string) TerraformComponentPlan
	PlanDestroySummary(blueprint *blueprintv1alpha1.Blueprint) []TerraformComponentPlan
	PlanDestroyComponentSummary(blueprint *blueprintv1alpha1.Blueprint, componentID string) TerraformComponentPlan
}

// =============================================================================
// Constructor
// =============================================================================

// NewStack creates a new stack of components.
func NewStack(rt *runtime.Runtime, opts ...*TerraformStack) Stack {
	if rt == nil {
		panic("runtime is required")
	}

	stack := &TerraformStack{
		runtime:                rt,
		shims:                  NewShims(),
		destroyRetryAttempts:   constants.DefaultTerraformDestroyRetryAttempts,
		destroyRetryBackoff:    constants.DefaultTerraformDestroyRetryBackoff,
		destroyIdleTimeout:     constants.DefaultTerraformDestroyIdleTimeout,
		destroyAbsoluteTimeout: constants.DefaultTerraformDestroyAbsoluteTimeout,
		destroyGracePeriod:     constants.DefaultTerraformDestroyGracePeriod,
	}

	if len(opts) > 0 && opts[0] != nil {
		overrides := opts[0]
		if overrides.terraformEnv != nil {
			stack.terraformEnv = overrides.terraformEnv
		}
	}

	if stack.terraformEnv == nil && rt.EnvPrinters.TerraformEnv != nil {
		if terraformEnv, ok := rt.EnvPrinters.TerraformEnv.(*envvars.TerraformEnvPrinter); ok {
			stack.terraformEnv = terraformEnv
		}
	}

	return stack
}

// =============================================================================
// Public Methods
// =============================================================================

// PostApply registers hooks to run after each component's WithProgress block completes (i.e. after Done is
// printed). Hooks are consumed and cleared at the start of the next Up call so they are not retained.
func (s *TerraformStack) PostApply(fns ...func(ctx context.Context, id string) error) {
	s.postApply = append(s.postApply, fns...)
}

// Up runs terraform init, plan, and apply for each component in order. It removes the backend
// override files after all components finish, so terraform_output() works between components.
// The onApply hooks run inside each spinner. The PostApply hooks run after each Done line, and Up
// clears them. A hook can return halted=true to stop the apply after its component with no error.
// A missing component directory returns an error with a remediation hint.
func (s *TerraformStack) Up(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
	if blueprint == nil {
		return false, fmt.Errorf("blueprint not provided")
	}

	postApply := s.postApply
	s.postApply = nil

	currentDir, err := s.shims.Getwd()
	if err != nil {
		return false, werror.Wrap(err, "error getting current directory")
	}

	defer func() {
		_ = s.shims.Chdir(currentDir)
	}()

	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		return false, fmt.Errorf("error getting project root: project root is empty")
	}
	components := s.resolveTerraformComponents(blueprint, projectRoot)

	var backendOverridePaths []string
	defer func() {
		for _, path := range backendOverridePaths {
			_ = s.shims.Remove(path)
		}
	}()

	for _, component := range components {
		var componentHalted bool
		if err := presenter.Track(ctx, "terraform:"+component.GetID(), fmt.Sprintf("Applying %s", component.Path), func(ctx context.Context) error {
			if _, err := s.shims.Stat(component.FullPath); os.IsNotExist(err) {
				return missingComponentDirError(&component)
			}

			terraformVars, scopedKeys, terraformArgs, err := s.setupTerraformEnvironment(component)
			backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
			if _, statErr := s.shims.Stat(backendOverridePath); statErr == nil {
				backendOverridePaths = append(backendOverridePaths, backendOverridePath)
			}
			if err != nil {
				return err
			}
			terraformVars["TF_VAR_operation"] = "apply"

			if err := s.runTerraformInit(&component, terraformVars, scopedKeys, terraformArgs, defaultInitFlags...); err != nil {
				return err
			}

			if err := s.refreshIfStateNonEmpty(ctx, &component, terraformVars, scopedKeys, terraformArgs); err != nil {
				return err
			}

			terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
			planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan", "-refresh=false"}
			planArgs = append(planArgs, terraformArgs.PlanArgs...)
			planEnv := selectTerraformCommandEnv(terraformVars, true, scopedKeys)
			debug.Log("terraform plan for %s: TF_VAR_kubelogin_mode=%q (effective)", component.Path, s.effectiveTerraformEnvValue(planEnv, "TF_VAR_kubelogin_mode"))
			if _, err = s.runtime.Shell.ExecSilentWithEnv(terraformCommand, planEnv, planArgs...); err != nil {
				return werror.Wrap(err, "error running terraform plan for %s", component.Path)
			}

			applyArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "apply"}
			applyArgs = append(applyArgs, noColorArgs()...)
			applyArgs = append(applyArgs, terraformArgs.ApplyArgs...)
			applyEnv := selectTerraformCommandEnv(terraformVars, false, scopedKeys)
			debug.Log("terraform apply for %s: TF_VAR_kubelogin_mode=%q (effective)", component.Path, s.effectiveTerraformEnvValue(applyEnv, "TF_VAR_kubelogin_mode"))
			if _, err = s.runtime.Shell.ExecProgressWithEnv(fmt.Sprintf("Applying Terraform changes in %s", component.Path), terraformCommand, applyEnv, applyArgs...); err != nil {
				return werror.Wrap(err, "error running terraform apply for %s", component.Path)
			}
			_ = s.runtime.TerraformProvider.CacheOutputs(component.GetID())
			componentID := component.GetID()
			for _, fn := range onApply {
				if fn == nil {
					continue
				}
				haltAfter, err := fn(ctx, componentID)
				if err != nil {
					return werror.Wrap(err, "post-apply hook %s", componentID)
				}
				if haltAfter {
					componentHalted = true
				}
			}
			return nil
		}); err != nil {
			return false, err
		}

		componentID := component.GetID()
		for _, fn := range postApply {
			if fn != nil {
				if err := fn(ctx, componentID); err != nil {
					return false, werror.Wrap(err, "post-apply hook %s", componentID)
				}
			}
		}

		// A halt signaled by this component's onApply hooks stops further apply iterations
		// cleanly: the component itself is Done in the TUI (the hook returned nil), the
		// caller receives (true, nil), and the cmd layer renders the deferred-work summary.
		if componentHalted {
			return true, nil
		}
	}

	return false, nil
}

// Apply runs terraform init, plan, and apply for the component with the given ID.
// It returns an error if a terraform command fails. A component that is not in the blueprint,
// or a component directory that does not exist, returns an error with a remediation hint.
func (s *TerraformStack) Apply(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if componentID == "" {
		return fmt.Errorf("component ID not provided")
	}

	component, terraformVars, scopedKeys, terraformArgs, cleanup, err := s.prepareComponentOp(blueprint, componentID)
	if err != nil {
		return err
	}
	defer cleanup()
	terraformVars["TF_VAR_operation"] = "apply"

	return presenter.Track(ctx, "terraform:"+component.GetID(), fmt.Sprintf("Applying %s", component.Path), func(ctx context.Context) error {
		if err := s.runTerraformInit(component, terraformVars, scopedKeys, terraformArgs, defaultInitFlags...); err != nil {
			return err
		}

		if err := s.refreshIfStateNonEmpty(ctx, component, terraformVars, scopedKeys, terraformArgs); err != nil {
			return err
		}

		terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
		planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan", "-refresh=false"}
		planArgs = append(planArgs, terraformArgs.PlanArgs...)
		planEnv := selectTerraformCommandEnv(terraformVars, true, scopedKeys)
		if _, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, planEnv, planArgs...); err != nil {
			return werror.Wrap(err, "error running terraform plan for %s", component.Path)
		}

		applyArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "apply"}
		applyArgs = append(applyArgs, noColorArgs()...)
		applyArgs = append(applyArgs, terraformArgs.ApplyArgs...)
		applyEnv := selectTerraformCommandEnv(terraformVars, false, scopedKeys)
		if _, err := s.runtime.Shell.ExecProgressWithEnv(fmt.Sprintf("Applying Terraform changes in %s", component.Path), terraformCommand, applyEnv, applyArgs...); err != nil {
			return werror.Wrap(err, "error running terraform apply for %s", component.Path)
		}
		_ = s.runtime.TerraformProvider.CacheOutputs(component.GetID())

		return nil
	})
}

// InitComponent runs `terraform init` for one component using the currently-
// configured backend; no -migrate-state, no plan, no apply.
func (s *TerraformStack) InitComponent(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	component, terraformVars, scopedKeys, terraformArgs, cleanup, err := s.prepareComponentOp(blueprint, componentID)
	if err != nil {
		return err
	}
	defer cleanup()
	return s.runTerraformInit(component, terraformVars, scopedKeys, terraformArgs, defaultInitFlags...)
}

// HasRemoteState reports whether the named component has non-empty terraform
// state in the currently-configured backend. Runs init + `terraform show
// -json`; callers must invoke before any in-memory backend override.
func (s *TerraformStack) HasRemoteState(blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
	component, terraformVars, scopedKeys, terraformArgs, cleanup, err := s.prepareComponentOp(blueprint, componentID)
	if err != nil {
		return false, err
	}
	defer cleanup()

	if err := s.runTerraformInit(component, terraformVars, scopedKeys, terraformArgs, defaultInitFlags...); err != nil {
		return false, err
	}
	return s.hasStateResources(component, terraformVars, scopedKeys)
}

// =============================================================================
// Private Methods
// =============================================================================

// printComponentHeader prints a section header naming the component to stderr, ahead of
// streaming its terraform output. Shared by every per-component loop over all components.
func (s *TerraformStack) printComponentHeader(componentPath string) {
	fmt.Fprintf(os.Stderr, "\n%s\n", tui.SectionHeader("Terraform: "+componentPath))
}

// refreshProviderLock runs `terraform init -upgrade -backend=false` for component. This
// brings a stale .terraform.lock.hcl current with the module's present provider
// constraints. -backend=false skips backend configuration. The call never touches
// state and never contends for a state lock. It builds its own init args rather than
// reusing runTerraformInit, since terraformArgs.InitArgs always carries -backend=true.
func (s *TerraformStack) refreshProviderLock(component *blueprintv1alpha1.TerraformComponent, terraformVars map[string]string, scopedKeys []string) error {
	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	initArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "init", "-upgrade", "-backend=false", "-input=false"}
	initArgs = append(initArgs, noColorArgs()...)
	initEnv := selectTerraformCommandEnv(terraformVars, false, scopedKeys)
	if _, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, initEnv, initArgs...); err != nil {
		return werror.Wrap(err, "error refreshing provider lock for %s", component.Path)
	}
	return nil
}

// backendPointerPath returns the per-component backend pointer file path — the
// JSON stub terraform writes to $TF_DATA_DIR/terraform.tfstate after init to
// record which backend is currently in use. Returns empty when TF_DATA_DIR is
// unset, signalling callers should skip pointer-related operations entirely.
// Centralising the path here keeps clearBackendPointer and clearStaleBackendPointer
// from drifting if terraform changes the pointer's filename or location.
func (s *TerraformStack) backendPointerPath(terraformVars map[string]string) string {
	tfDataDir := terraformVars["TF_DATA_DIR"]
	if tfDataDir == "" {
		return ""
	}
	return filepath.Join(tfDataDir, "terraform.tfstate")
}

// clearBackendPointer removes the per-component backend pointer file (the file
// terraform writes to TF_DATA_DIR after init to record which backend is
// currently in use). After destroy completes, the remote backend resource it
// points at may itself have been torn down, leaving the pointer stale —
// subsequent init then complains "backend changed" against a backend that no
// longer exists. Removing only the pointer (terraform.tfstate inside
// TF_DATA_DIR) leaves provider plugins, module cache, and lockfiles in place
// so the next init is fast. Errors are ignored: cleanup is best-effort and a
// failure here must not undo a successful destroy.
func (s *TerraformStack) clearBackendPointer(terraformVars map[string]string) {
	pointerPath := s.backendPointerPath(terraformVars)
	if pointerPath == "" {
		return
	}
	_ = s.shims.Remove(pointerPath)
}

// resolveTerraformComponents resolves terraform components from the blueprint by resolving sources and paths.
// Components with Enabled set to false are excluded from the returned list.
func (s *TerraformStack) resolveTerraformComponents(blueprint *blueprintv1alpha1.Blueprint, projectRoot string) []blueprintv1alpha1.TerraformComponent {
	blueprintCopy := *blueprint
	s.resolveComponentSources(&blueprintCopy)
	s.resolveComponentPaths(&blueprintCopy, projectRoot)
	out := make([]blueprintv1alpha1.TerraformComponent, 0, len(blueprintCopy.TerraformComponents))
	for _, c := range blueprintCopy.TerraformComponents {
		if c.Enabled != nil && !c.Enabled.IsEnabled() {
			continue
		}
		out = append(out, c)
	}
	return out
}

// resolveComponentSources resolves component source names to full URLs using blueprint sources.
func (s *TerraformStack) resolveComponentSources(blueprint *blueprintv1alpha1.Blueprint) {
	resolvedComponents := make([]blueprintv1alpha1.TerraformComponent, len(blueprint.TerraformComponents))
	copy(resolvedComponents, blueprint.TerraformComponents)

	for i, component := range resolvedComponents {
		for _, source := range blueprint.Sources {
			if component.Source == source.Name {
				pathPrefix := source.PathPrefix
				if pathPrefix == "" {
					pathPrefix = "terraform"
				}

				ref := source.Ref.Commit
				if ref == "" {
					ref = source.Ref.SemVer
				}
				if ref == "" {
					ref = source.Ref.Tag
				}
				if ref == "" {
					ref = source.Ref.Branch
				}

				if strings.HasPrefix(source.Url, "oci://") {
					baseURL := source.Url
					if ref != "" && !strings.Contains(baseURL, ":") {
						baseURL = baseURL + ":" + ref
					}
					resolvedComponents[i].Source = baseURL + "//" + pathPrefix + "/" + component.Path
				} else {
					resolvedComponents[i].Source = source.Url + "//" + pathPrefix + "/" + component.Path + "?ref=" + ref
				}
				break
			}
		}
	}

	blueprint.TerraformComponents = resolvedComponents
}

// resolveComponentPaths determines the full filesystem path for each Terraform component.
func (s *TerraformStack) resolveComponentPaths(blueprint *blueprintv1alpha1.Blueprint, projectRoot string) {
	resolvedComponents := make([]blueprintv1alpha1.TerraformComponent, len(blueprint.TerraformComponents))
	copy(resolvedComponents, blueprint.TerraformComponents)

	for i, component := range resolvedComponents {
		componentCopy := component
		componentID := componentCopy.GetID()

		useScratchPath := componentCopy.Name != "" || componentCopy.Source != ""
		if useScratchPath {
			componentCopy.FullPath = filepath.Join(projectRoot, ".windsor", "contexts", s.runtime.ContextName, "terraform", componentID)
		} else {
			componentCopy.FullPath = filepath.Join(projectRoot, "terraform", componentID)
		}

		componentCopy.FullPath = filepath.FromSlash(componentCopy.FullPath)

		resolvedComponents[i] = componentCopy
	}

	blueprint.TerraformComponents = resolvedComponents
}

// prepareComponentEnv saves the current directory and sets up the terraform environment of a component.
// It returns a cleanup func that restores the directory and removes backend_override.tf.
// scopedKeys names the contexts/<context>/terraform/.env keys that selectTerraformCommandEnv passes.
// planComponents, planOneTerraformSummary, and prepareComponentOp use it. A component directory
// that does not exist returns an error with a remediation hint.
func (s *TerraformStack) prepareComponentEnv(component *blueprintv1alpha1.TerraformComponent) (map[string]string, []string, *envvars.TerraformArgs, func(), error) {
	currentDir, err := s.shims.Getwd()
	if err != nil {
		return nil, nil, nil, func() {}, werror.Wrap(err, "error getting current directory")
	}

	if _, err := s.shims.Stat(component.FullPath); os.IsNotExist(err) {
		return nil, nil, nil, func() {}, missingComponentDirError(component)
	}

	removeBackendOverride := func() {
		backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
		if _, statErr := s.shims.Stat(backendOverridePath); statErr == nil {
			_ = s.shims.Remove(backendOverridePath)
		}
	}

	terraformVars, scopedKeys, terraformArgs, err := s.setupTerraformEnvironment(*component)
	if err != nil {
		removeBackendOverride()
		return nil, nil, nil, func() {}, err
	}

	cleanup := func() {
		_ = s.shims.Chdir(currentDir)
		removeBackendOverride()
	}

	return terraformVars, scopedKeys, terraformArgs, cleanup, nil
}

// prepareComponentOp finds the component with the given ID and prepares its terraform environment.
// It returns a cleanup func that the caller must defer. A component that is not in the blueprint
// returns an error with a remediation hint.
func (s *TerraformStack) prepareComponentOp(blueprint *blueprintv1alpha1.Blueprint, componentID string) (
	*blueprintv1alpha1.TerraformComponent,
	map[string]string,
	[]string,
	*envvars.TerraformArgs,
	func(),
	error,
) {
	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		return nil, nil, nil, nil, func() {}, fmt.Errorf("error getting project root: project root is empty")
	}

	components := s.resolveTerraformComponents(blueprint, projectRoot)
	var component *blueprintv1alpha1.TerraformComponent
	for i, c := range components {
		if c.GetID() == componentID {
			component = &components[i]
			break
		}
	}
	if component == nil {
		return nil, nil, nil, nil, func() {}, werror.New(fmt.Sprintf("terraform component %q not found", componentID), componentNotFoundHint, nil)
	}

	terraformVars, scopedKeys, terraformArgs, cleanup, err := s.prepareComponentEnv(component)
	if err != nil {
		return nil, nil, nil, nil, func() {}, err
	}

	return component, terraformVars, scopedKeys, terraformArgs, cleanup, nil
}

// runTerraformInit runs terraform init for a component before an operation that can write state.
// When the kubernetes backend namespace changed, it adds -migrate-state -force-copy, so terraform
// copies the state into the configured namespace. It prints one line for each move.
func (s *TerraformStack) runTerraformInit(component *blueprintv1alpha1.TerraformComponent, terraformVars map[string]string, scopedKeys []string, terraformArgs *envvars.TerraformArgs, extraFlags ...string) error {
	return s.initComponent(component, terraformVars, scopedKeys, terraformArgs, false, extraFlags...)
}

// runTerraformInitReadOnly runs terraform init for a component before a plan.
// When the kubernetes backend namespace changed, it adds -reconfigure and keeps the namespace in the
// backend pointer, so the plan reads the current state and nothing moves.
func (s *TerraformStack) runTerraformInitReadOnly(component *blueprintv1alpha1.TerraformComponent, terraformVars map[string]string, scopedKeys []string, terraformArgs *envvars.TerraformArgs, extraFlags ...string) error {
	return s.initComponent(component, terraformVars, scopedKeys, terraformArgs, true, extraFlags...)
}

// initComponent runs terraform init for a component, with extraFlags after "init".
// It caches a successful init per component, backend, namespace, and migrate-state value for the process.
// It does not cache a failed init. The cache expects callers to run one at a time. Before an init
// without -migrate-state, it removes a stale backend pointer. A stale provider lock error returns
// an error with the remediation from staleProviderLockHint.
func (s *TerraformStack) initComponent(component *blueprintv1alpha1.TerraformComponent, terraformVars map[string]string, scopedKeys []string, terraformArgs *envvars.TerraformArgs, readOnly bool, extraFlags ...string) error {
	backendArgs := terraformArgs.InitArgs
	namespace := terraformArgs.KubernetesNamespace
	migrateState := slices.Contains(extraFlags, "-migrate-state")
	previous, namespaceChanged := s.kubernetesNamespaceChange(terraformVars, terraformArgs)
	movingState := false
	if namespaceChanged && readOnly {
		extraFlags = append(slices.Clone(extraFlags), "-reconfigure")
		backendArgs = withBackendNamespace(backendArgs, previous)
		namespace = previous
	} else if namespaceChanged && !migrateState {
		extraFlags = append(slices.Clone(extraFlags), "-migrate-state", "-force-copy")
		migrateState = true
		movingState = true
	}

	key := initCacheKey{
		componentID:  component.GetID(),
		backendType:  s.runtime.ConfigHandler.GetTerraformBackendType(),
		namespace:    namespace,
		migrateState: migrateState,
	}
	s.initCacheMu.Lock()
	_, hit := s.initCache[key]
	s.initCacheMu.Unlock()
	if hit {
		return nil
	}

	if !migrateState {
		s.clearStaleBackendPointer(terraformVars)
	}
	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	initArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "init"}
	initArgs = append(initArgs, extraFlags...)
	initArgs = append(initArgs, noColorArgs()...)
	initArgs = append(initArgs, backendArgs...)
	initEnv := selectTerraformCommandEnv(terraformVars, false, scopedKeys)
	_, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, initEnv, initArgs...)
	if err != nil {
		if isStaleProviderLockError(err) {
			return werror.New(fmt.Sprintf("error running terraform init for %s: %v", component.Path, err), staleProviderLockHint(component), err)
		}
		return werror.Wrap(err, "error running terraform init for %s", component.Path)
	}
	if movingState {
		presenter.Message(presenter.Background(), fmt.Sprintf("Moved terraform state of %s from namespace %s to %s", component.Path, previous, namespace))
	}

	s.initCacheMu.Lock()
	if s.initCache == nil {
		s.initCache = make(map[initCacheKey]struct{})
	}
	s.initCache[key] = struct{}{}
	s.initCacheMu.Unlock()
	return nil
}

// clearStaleBackendPointer removes the per-component backend pointer file when it records a
// backend type different from the one currently configured for windsor. The pointer is the
// JSON file terraform writes to TF_DATA_DIR after init describing which backend is in use;
// when it disagrees with the configured backend, terraform's next init treats the disagreement
// as a migration request and demands `-migrate-state` or `-reconfigure`, which fails when the
// new backend's resource doesn't exist yet (the chicken-and-egg windsor's bootstrap solves
// via a separate path).
//
// Triggers for staleness:
//   - The repo carries a committed backend_override.tf for `terraform test` against local
//     state, then a fresh `terraform init` writes a "local" pointer that windsor sees on its
//     next run with an s3-configured backend.
//   - The operator changed terraform.backend.type in values.yaml between windsor runs.
//
// All paths boil down to "the pointer disagrees with what windsor will configure now," and
// the right answer is to discard the pointer and let init set up the configured backend
// fresh. State migration is owned by the explicit `-migrate-state` path (used by
// MigrateState) — bypassed here because that caller passes the flag through extraFlags.
//
// Errors are best-effort: if the pointer can't be read or removed, init runs anyway and
// surfaces the underlying issue. Unparseable pointer files are removed as well, since they
// can only block init.
func (s *TerraformStack) clearStaleBackendPointer(terraformVars map[string]string) {
	pointerPath := s.backendPointerPath(terraformVars)
	if pointerPath == "" {
		return
	}
	data, err := s.shims.ReadFile(pointerPath)
	if err != nil {
		return
	}
	var pointer struct {
		Backend struct {
			Type string `json:"type"`
		} `json:"backend"`
	}
	if err := json.Unmarshal(data, &pointer); err != nil {
		_ = s.shims.Remove(pointerPath)
		return
	}
	configuredBackend := s.runtime.ConfigHandler.GetTerraformBackendType()
	if pointer.Backend.Type != "" && pointer.Backend.Type != configuredBackend {
		_ = s.shims.Remove(pointerPath)
	}
}

// setupTerraformEnvironment computes Terraform-specific environment values and args for a component.
// setupTerraformEnvironment computes Terraform-specific environment values and args for a
// component, plus the contexts/<context>/terraform/.env key names (scopedKeys) that
// selectTerraformCommandEnv must pass through unconditionally regardless of includeTFVars.
func (s *TerraformStack) setupTerraformEnvironment(component blueprintv1alpha1.TerraformComponent) (map[string]string, []string, *envvars.TerraformArgs, error) {
	terraformEnv := s.getTerraformEnv()
	if terraformEnv == nil {
		return nil, nil, nil, fmt.Errorf("terraform environment printer not available")
	}

	terraformVars, scopedKeys, terraformArgs, err := s.runtime.TerraformProvider.GetEnvVars(component.GetID(), false)
	if err != nil {
		return nil, nil, nil, werror.Wrap(err, "error getting terraform env vars")
	}

	if err := terraformEnv.PostEnvHook(component.FullPath); err != nil {
		return nil, nil, nil, werror.Wrap(err, "error creating backend override file for %s", component.Path)
	}

	return terraformVars, scopedKeys, terraformArgs, nil
}

// getTerraformEnv returns the terraform environment printer, checking the runtime if not set on the stack.
func (s *TerraformStack) getTerraformEnv() *envvars.TerraformEnvPrinter {
	if s.terraformEnv != nil {
		return s.terraformEnv
	}
	if s.runtime.EnvPrinters.TerraformEnv != nil {
		if terraformEnv, ok := s.runtime.EnvPrinters.TerraformEnv.(*envvars.TerraformEnvPrinter); ok {
			s.terraformEnv = terraformEnv
			return terraformEnv
		}
	}
	return nil
}

// effectiveTerraformEnvValue returns the value of key that the terraform subprocess sees.
// It returns the value in commandEnv when the key is present. Otherwise it returns the value
// in the process environment, because mergeEnvVars uses that value for a key that commandEnv
// does not set.
func (s *TerraformStack) effectiveTerraformEnvValue(commandEnv map[string]string, key string) string {
	if value, exists := commandEnv[key]; exists {
		return value
	}
	return s.shims.Getenv(key)
}

// =============================================================================
// Helpers
// =============================================================================

// isScratchComponent reports whether a component's module content lives in windsor's own
// OCI/git-sourced scratch cache (.windsor/contexts/<context>/terraform), rather than a plain local
// directory the operator authors and version-controls directly. Mirrors the useScratchPath
// condition in resolveComponentPaths, which is where this same distinction determines FullPath.
func isScratchComponent(component *blueprintv1alpha1.TerraformComponent) bool {
	return component.Name != "" || component.Source != ""
}

// missingComponentDirError returns the error for a component whose directory does not exist.
// The hint for a scratch component asks to regenerate the modules with windsor init. The hint
// for a local component asks to match the component path to a directory under terraform/.
func missingComponentDirError(component *blueprintv1alpha1.TerraformComponent) error {
	message := fmt.Sprintf("component %q directory %s does not exist", component.GetID(), component.FullPath)
	if isScratchComponent(component) {
		return werror.New(message, "Run windsor init to regenerate the terraform modules, then run the command again.", nil)
	}
	return werror.New(message, "Make sure the component path in the blueprint matches a directory under terraform/.", nil)
}

// staleProviderLockMarkers are substrings that together identify Terraform's "locked provider ...
// does not match configured version constraint" failure — the signature of a leftover local
// .terraform.lock.hcl from a previous windsor run whose module source has since changed. All
// markers must be present since Terraform's wording could otherwise coincidentally partial-match
// an unrelated error.
var staleProviderLockMarkers = []string{
	"does not match configured version constraint",
	"terraform init -upgrade",
}

// isStaleProviderLockError reports whether err is Terraform's stale-provider-lock failure, the
// one -upgrade actually resolves. See staleProviderLockMarkers. Terraform's own diagnostic
// renderer word-wraps its message at a fixed column width, so a marker can straddle a line
// break in the captured text; whitespace (including newlines) is collapsed before matching so
// wrap position never affects detection.
func isStaleProviderLockError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.Join(strings.Fields(err.Error()), " ")
	for _, marker := range staleProviderLockMarkers {
		if !strings.Contains(msg, marker) {
			return false
		}
	}
	return true
}

// staleProviderLockHint returns the remediation for a stale provider lock error. Windsor does not
// retry with -upgrade, because MigrateState must not change providers during a state move.
// For a scratch component, the hint names the cache directory to remove. For a local component,
// the hint asks to run terraform init -upgrade, because the lock file can be under version control.
func staleProviderLockHint(component *blueprintv1alpha1.TerraformComponent) string {
	if isScratchComponent(component) {
		return fmt.Sprintf("The local terraform cache for this component is stale because its module source changed. Remove %s and run the command again.", component.FullPath)
	}
	return fmt.Sprintf("The .terraform.lock.hcl in %s pins a provider version outside the current constraint. Run terraform init -upgrade in that directory, then run the command again.", component.FullPath)
}

// noColorArgs returns []string{"-no-color"} unconditionally. Windsor's own TUI
// already owns presentation of apply/destroy/init progress, and terraform's TTY
// auto-detection doesn't reliably disable color under piped subprocess capture,
// so raw ANSI codes leak into logs and CI artifacts unless the flag is passed
// explicitly regardless of the caller's environment.
func noColorArgs() []string {
	return []string{"-no-color"}
}

// selectTerraformCommandEnv builds per-command env overrides without mutating process-wide
// environment. TF_CLI_ARGS_* always reset to blank; TF_DATA_DIR and, when includeTFVars is
// true, TF_VAR_* pass through from terraformVars. scopedKeys names the additional keys to pass
// through unconditionally (contexts/<context>/terraform/.env content — see
// TerraformProvider.TerraformScopedEnvKeys): provider credentials are needed on every terraform
// invocation regardless of includeTFVars. Any other key in terraformVars is dropped, preserving
// isolation from unrelated process state.
func selectTerraformCommandEnv(terraformVars map[string]string, includeTFVars bool, scopedKeys []string) map[string]string {
	selected := make(map[string]string)
	for _, key := range []string{
		"TF_CLI_ARGS",
		"TF_CLI_ARGS_init",
		"TF_CLI_ARGS_plan",
		"TF_CLI_ARGS_apply",
		"TF_CLI_ARGS_destroy",
		"TF_CLI_ARGS_import",
		"TF_CLI_ARGS_refresh",
	} {
		selected[key] = ""
	}
	for key, value := range terraformVars {
		if key == "TF_DATA_DIR" {
			selected[key] = value
		}
		if includeTFVars && strings.HasPrefix(key, "TF_VAR_") {
			selected[key] = value
		}
	}
	for _, key := range scopedKeys {
		if value, exists := terraformVars[key]; exists {
			selected[key] = value
		}
	}
	return selected
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure TerraformStack implements the Stack interface
var _ Stack = (*TerraformStack)(nil)
