package terraform

// The Stack package provides infrastructure component stack management functionality.
// It provides a unified interface for initializing and managing infrastructure stacks,
// with support for dependency injection and component lifecycle management.
// The Stack acts as the primary orchestrator for infrastructure operations,
// coordinating shell operations and blueprint handling. The WindsorStack is a specialized
// implementation for Terraform-based infrastructure that handles directory management,
// terraform environment configuration, and Terraform operations.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
	envvars "github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/tui"
)

// =============================================================================
// Constants
// =============================================================================

// defaultInitFlags are passed to `terraform init` for normal apply/inspect operations.
// MigrateState uses -migrate-state + -force-copy inline instead, since moving state must
// not reinstall providers.
var defaultInitFlags = []string{"-upgrade", "-force-copy"}

// =============================================================================
// Types
// =============================================================================

// TerraformStack manages Terraform infrastructure components by initializing and applying Terraform configurations.
// It processes components in order, generating terraform arguments, running Terraform init, plan, and apply operations.
//
// warningWriter is the destination for non-blocking operator-facing warnings (e.g. the
// refresh-fallback notice in destroy paths). Defaults to os.Stderr; tests inject a buffer.
// Routing warnings through this field rather than os.Stderr directly keeps tests off the
// fragile os.Stderr-redirect-with-pipe pattern, which deadlocks on Windows when the TUI
// spinner shares the redirected stream.
type TerraformStack struct {
	runtime       *runtime.Runtime
	shims         *Shims
	terraformEnv  *envvars.TerraformEnvPrinter
	postApply     []func(id string) error
	warningWriter io.Writer
}

// TerraformComponentPlan holds the plan result for a single Terraform component.
// Add, Change, and Destroy reflect terraform's "to add / to change / to destroy" counts.
// NoChanges is true when terraform reports no changes. Err is non-nil when the
// component's init or plan step failed; subsequent layers may still be attempted.
type TerraformComponentPlan struct {
	ComponentID string
	Add         int
	Change      int
	Destroy     int
	NoChanges   bool
	Err         error
}

// tfStateModule is a node in the module tree emitted by `terraform show -json`. Windsor
// blueprints wrap resources in a `module "main"` block, so the root's own Resources is
// always empty — emptiness must be determined by walking the tree.
type tfStateModule struct {
	Resources    []json.RawMessage `json:"resources"`
	ChildModules []tfStateModule   `json:"child_modules"`
}

// tfState is the minimal shape of `terraform show -json` we need for state emptiness.
type tfState struct {
	Values *struct {
		RootModule tfStateModule `json:"root_module"`
	} `json:"values"`
}

// =============================================================================
// Interfaces
// =============================================================================

// Stack defines the interface for Terraform stack operations.
// Both the Stack struct and MockStack implement this interface.
type Stack interface {
	Up(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error
	MigrateState(blueprint *blueprintv1alpha1.Blueprint) ([]string, error)
	MigrateComponentState(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	PostApply(fns ...func(id string) error)
	DestroyAll(blueprint *blueprintv1alpha1.Blueprint, excludeIDs ...string) ([]string, error)
	Plan(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	PlanAll(blueprint *blueprintv1alpha1.Blueprint) error
	PlanJSON(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	PlanAllJSON(blueprint *blueprintv1alpha1.Blueprint) error
	Apply(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	Destroy(blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error)
	PlanSummary(blueprint *blueprintv1alpha1.Blueprint) []TerraformComponentPlan
	PlanComponentSummary(blueprint *blueprintv1alpha1.Blueprint, componentID string) TerraformComponentPlan
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
		runtime:       rt,
		shims:         NewShims(),
		warningWriter: os.Stderr,
	}

	if len(opts) > 0 && opts[0] != nil {
		overrides := opts[0]
		if overrides.terraformEnv != nil {
			stack.terraformEnv = overrides.terraformEnv
		}
		if overrides.warningWriter != nil {
			stack.warningWriter = overrides.warningWriter
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
func (s *TerraformStack) PostApply(fns ...func(id string) error) {
	s.postApply = append(s.postApply, fns...)
}

// Up runs init/plan/apply for each component in order. Backend override files are cleaned up
// after all components complete so terraform_output() calls between components keep working.
// onApply hooks run inside each spinner; PostApply hooks run after each Done line and are
// consumed (not retained across calls).
func (s *TerraformStack) Up(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	postApply := s.postApply
	s.postApply = nil

	currentDir, err := s.shims.Getwd()
	if err != nil {
		return fmt.Errorf("error getting current directory: %v", err)
	}

	defer func() {
		_ = s.shims.Chdir(currentDir)
	}()

	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		return fmt.Errorf("error getting project root: project root is empty")
	}
	components := s.resolveTerraformComponents(blueprint, projectRoot)

	var backendOverridePaths []string
	defer func() {
		for _, path := range backendOverridePaths {
			_ = s.shims.Remove(path)
		}
	}()

	for _, component := range components {
		if err := tui.WithProgress(fmt.Sprintf("Applying %s", component.Path), func() error {
			if _, err := s.shims.Stat(component.FullPath); os.IsNotExist(err) {
				return fmt.Errorf("directory %s does not exist", component.FullPath)
			}

			terraformVars, terraformArgs, err := s.setupTerraformEnvironment(component)
			backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
			if _, statErr := s.shims.Stat(backendOverridePath); statErr == nil {
				backendOverridePaths = append(backendOverridePaths, backendOverridePath)
			}
			if err != nil {
				return err
			}
			terraformVars["TF_VAR_operation"] = "apply"

			if err := s.runTerraformInit(&component, terraformVars, terraformArgs, defaultInitFlags...); err != nil {
				return err
			}

			terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
			refreshArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "refresh"}
			refreshArgs = append(refreshArgs, terraformArgs.RefreshArgs...)
			refreshEnv := selectTerraformCommandEnv(terraformVars, true)
			_, _ = s.runtime.Shell.ExecSilentWithEnv(terraformCommand, refreshEnv, refreshArgs...)

			planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan"}
			planArgs = append(planArgs, terraformArgs.PlanArgs...)
			planEnv := selectTerraformCommandEnv(terraformVars, true)
			if _, err = s.runtime.Shell.ExecSilentWithEnv(terraformCommand, planEnv, planArgs...); err != nil {
				return fmt.Errorf("error running terraform plan for %s: %w", component.Path, err)
			}

			applyArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "apply"}
			applyArgs = append(applyArgs, terraformArgs.ApplyArgs...)
			applyEnv := selectTerraformCommandEnv(terraformVars, false)
			if _, err = s.runtime.Shell.ExecProgressWithEnv(fmt.Sprintf("Applying Terraform changes in %s", component.Path), terraformCommand, applyEnv, applyArgs...); err != nil {
				return fmt.Errorf("error running terraform apply for %s: %w", component.Path, err)
			}
			_ = s.runtime.TerraformProvider.CacheOutputs(component.GetID())
			componentID := component.GetID()
			for _, fn := range onApply {
				if fn != nil {
					if err := fn(componentID); err != nil {
						return fmt.Errorf("post-apply hook %s: %w", componentID, err)
					}
				}
			}
			return nil
		}); err != nil {
			return err
		}

		componentID := component.GetID()
		for _, fn := range postApply {
			if fn != nil {
				if err := fn(componentID); err != nil {
					return fmt.Errorf("post-apply hook %s: %w", componentID, err)
				}
			}
		}
	}

	return nil
}

// MigrateState runs `terraform init -migrate-state -force-copy` per component to move state
// to the currently configured backend. Components whose directories don't exist on disk are
// skipped and their IDs returned in the skipped slice, paired with any error so callers see
// both what migrated and what didn't. Stops on the first failure; safe to retry.
func (s *TerraformStack) MigrateState(blueprint *blueprintv1alpha1.Blueprint) ([]string, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	currentDir, err := s.shims.Getwd()
	if err != nil {
		return nil, fmt.Errorf("error getting current directory: %v", err)
	}
	defer func() {
		_ = s.shims.Chdir(currentDir)
	}()

	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		return nil, fmt.Errorf("error getting project root: project root is empty")
	}
	components := s.resolveTerraformComponents(blueprint, projectRoot)

	var backendOverridePaths []string
	defer func() {
		for _, path := range backendOverridePaths {
			_ = s.shims.Remove(path)
		}
	}()

	var skipped []string
	err = tui.WithProgress("Migrating terraform state", func() error {
		for _, component := range components {
			migrated, err := s.migrateOneComponent(&component, &backendOverridePaths)
			if err != nil {
				return err
			}
			if !migrated {
				skipped = append(skipped, component.GetID())
			}
		}
		return nil
	})
	return skipped, err
}

// MigrateComponentState runs `terraform init -migrate-state -force-copy` for a single
// component identified by componentID, moving its state to the currently configured
// backend. Used by `windsor bootstrap` after applying just the backend component with a
// local backend, so only that component's state is moved to remote (the rest haven't
// been applied yet and will init directly against the configured backend on the next
// Up). Returns an error if the blueprint is nil, the component is not found, the
// component's directory does not exist, or any terraform operation fails.
func (s *TerraformStack) MigrateComponentState(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if componentID == "" {
		return fmt.Errorf("component ID not provided")
	}

	currentDir, err := s.shims.Getwd()
	if err != nil {
		return fmt.Errorf("error getting current directory: %v", err)
	}
	defer func() {
		_ = s.shims.Chdir(currentDir)
	}()

	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		return fmt.Errorf("error getting project root: project root is empty")
	}
	components := s.resolveTerraformComponents(blueprint, projectRoot)

	var component *blueprintv1alpha1.TerraformComponent
	for i := range components {
		if components[i].GetID() == componentID {
			component = &components[i]
			break
		}
	}
	if component == nil {
		return fmt.Errorf("terraform component %q not found", componentID)
	}

	var backendOverridePaths []string
	defer func() {
		for _, path := range backendOverridePaths {
			_ = s.shims.Remove(path)
		}
	}()

	// No progress UI here intentionally: MigrateComponentState runs as a hidden
	// step inside the per-component bootstrap/destroy dance for s3/azurerm. The
	// operator already saw "Applying <component>" before this fires, and adding
	// "Migrating terraform state for <component>" between two applies of the
	// same component reads as redundant noise. Bulk MigrateState (the
	// kubernetes full-cycle path) keeps its progress UI because it's the only
	// visible step covering many components at once.
	migrated, err := s.migrateOneComponent(component, &backendOverridePaths)
	if err != nil {
		return err
	}
	if !migrated {
		return fmt.Errorf("component %q directory %s does not exist", componentID, component.FullPath)
	}
	return nil
}

// DestroyAll destroys components in reverse dependency order using the idempotent flow:
// init → pre-refresh state check → refresh → post-refresh state check → destroy, skipping
// the rest when state is empty at either check. Components with Destroy=false are skipped.
// excludeIDs are skipped entirely (used by symmetric-destroy flow at the cmd layer to peel
// off the backend component from the bulk pass — it gets destroyed last, after its state
// is migrated to local). Returns IDs skipped due to empty state, paired with any error.
func (s *TerraformStack) DestroyAll(blueprint *blueprintv1alpha1.Blueprint, excludeIDs ...string) ([]string, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	currentDir, err := s.shims.Getwd()
	if err != nil {
		return nil, fmt.Errorf("error getting current directory: %v", err)
	}

	defer func() {
		_ = s.shims.Chdir(currentDir)
	}()

	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		return nil, fmt.Errorf("error getting project root: project root is empty")
	}
	components := s.resolveTerraformComponents(blueprint, projectRoot)

	excluded := make(map[string]bool, len(excludeIDs))
	for _, id := range excludeIDs {
		excluded[id] = true
	}

	var backendOverridePaths []string
	defer func() {
		for _, path := range backendOverridePaths {
			_ = s.shims.Remove(path)
		}
	}()

	var skipped []string
	for i := len(components) - 1; i >= 0; i-- {
		component := components[i]

		if excluded[component.GetID()] {
			continue
		}

		if component.Destroy != nil {
			destroy := component.Destroy.ToBool()
			if destroy != nil && !*destroy {
				continue
			}
		}

		if _, err := s.shims.Stat(component.FullPath); os.IsNotExist(err) {
			skipped = append(skipped, component.GetID())
			continue
		}

		terraformVars, terraformArgs, err := s.setupTerraformEnvironment(component)
		backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
		if _, statErr := s.shims.Stat(backendOverridePath); statErr == nil {
			backendOverridePaths = append(backendOverridePaths, backendOverridePath)
		}
		if err != nil {
			return skipped, err
		}

		componentSkipped := false
		if err := tui.WithProgress(fmt.Sprintf("Destroying %s", component.Path), func() error {
			terraformVars["TF_VAR_operation"] = "destroy"

			if err := s.runTerraformInit(&component, terraformVars, terraformArgs, defaultInitFlags...); err != nil {
				return err
			}

			// Skip refresh if state is already empty: refresh can only drop resources, never
			// add them, and downstream data sources may fail against torn-down upstreams.
			hasResourcesPre, err := s.hasStateResources(&component, terraformVars)
			if err != nil {
				return err
			}
			if !hasResourcesPre {
				componentSkipped = true
				return nil
			}

			// Tolerate refresh failures for non-empty-state components. A transient refresh
			// issue (network blip, credential rotation, provider API hiccup) must not make a
			// live component undestroyable. The pre-refresh check confirmed state is non-empty,
			// so we know there is something to destroy; fall through to `terraform destroy
			// -refresh=true` and let terraform's own refresh have a second shot. Persistent
			// refresh problems will then surface from destroy itself with a more actionable
			// error than the refresh step would have. The warning is emitted to stderr so the
			// operator can correlate a later destroy failure with the upstream refresh hiccup
			// — without it, a recurring credential or connectivity issue is invisible until
			// destroy errors out, and a successful fallback leaves no trace at all.
			refreshFailed := false
			if err := s.refreshComponentState(&component, terraformVars, terraformArgs); err != nil {
				refreshFailed = true
				fmt.Fprintf(s.warningWriter, "warning: terraform refresh failed for %s; falling through to destroy -refresh=true (terraform will retry refresh during destroy): %v\n", component.Path, err)
			}

			// Skip the post-refresh empty-state check when refresh failed — the state we would
			// be reading is the pre-refresh snapshot we already classified as non-empty, so the
			// check would be redundant; if refresh succeeded but reconciliation dropped every
			// resource, we still want the skip path.
			if !refreshFailed {
				hasResources, err := s.hasStateResources(&component, terraformVars)
				if err != nil {
					return err
				}
				if !hasResources {
					componentSkipped = true
					return nil
				}
			}

			terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()

			// -refresh=false on the happy path because refreshComponentState already ran;
			// -refresh=true on the fallback path so terraform retries refresh inside destroy.
			destroyRefreshFlag := "-refresh=false"
			if refreshFailed {
				destroyRefreshFlag = "-refresh=true"
			}
			destroyArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "destroy", destroyRefreshFlag}
			destroyArgs = append(destroyArgs, terraformArgs.DestroyArgs...)
			destroyEnv := selectTerraformCommandEnv(terraformVars, true)
			if _, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, destroyEnv, destroyArgs...); err != nil {
				return fmt.Errorf("error running terraform destroy for %s: %w", component.Path, err)
			}
			return nil
		}); err != nil {
			return skipped, err
		}
		if componentSkipped {
			skipped = append(skipped, component.GetID())
		}
	}

	return skipped, nil
}

// Plan runs terraform init and plan for a single component identified by componentID.
// It resolves the component from the blueprint, sets up the environment, and executes
// init then plan without applying any changes. Returns an error if the component is not
// found, the directory does not exist, or any terraform operation fails.
func (s *TerraformStack) Plan(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if componentID == "" {
		return fmt.Errorf("component ID not provided")
	}

	component, terraformVars, terraformArgs, cleanup, err := s.prepareComponentOp(blueprint, componentID)
	if err != nil {
		return err
	}
	defer cleanup()
	terraformVars["TF_VAR_operation"] = "apply"

	if err := s.runTerraformInit(component, terraformVars, terraformArgs, defaultInitFlags...); err != nil {
		return err
	}

	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan"}
	planArgs = append(planArgs, terraformArgs.PlanArgs...)
	planEnv := selectTerraformCommandEnv(terraformVars, true)
	planOutput, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, planEnv, planArgs...)
	if err != nil {
		return fmt.Errorf("error running terraform plan for %s: %w", component.Path, err)
	}
	if planOutput != "" {
		fmt.Fprint(os.Stdout, planOutput)
	}

	return nil
}

// PlanAll runs terraform init and plan for every enabled component in the blueprint,
// streaming output directly to stdout. Stops on the first error. Returns an error if
// blueprint is nil or any component's init or plan step fails.
func (s *TerraformStack) PlanAll(blueprint *blueprintv1alpha1.Blueprint) error {
	return s.planComponents(blueprint, false)
}

// PlanJSON runs terraform init and plan -json for a single component identified by componentID,
// streaming machine-readable JSON lines to stdout. Returns an error if the component is not
// found, the directory does not exist, or any terraform operation fails.
func (s *TerraformStack) PlanJSON(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if componentID == "" {
		return fmt.Errorf("component ID not provided")
	}

	component, terraformVars, terraformArgs, cleanup, err := s.prepareComponentOp(blueprint, componentID)
	if err != nil {
		return err
	}
	defer cleanup()
	terraformVars["TF_VAR_operation"] = "apply"

	if err := s.runTerraformInit(component, terraformVars, terraformArgs, defaultInitFlags...); err != nil {
		return err
	}

	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan", "-json", "-no-color"}
	planArgs = append(planArgs, terraformArgs.PlanArgs...)
	planEnv := selectTerraformCommandEnv(terraformVars, true)
	planOutput, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, planEnv, planArgs...)
	if err != nil {
		return fmt.Errorf("error running terraform plan for %s: %w", component.Path, err)
	}
	if planOutput != "" {
		fmt.Fprint(os.Stdout, planOutput)
	}

	return nil
}

// PlanAllJSON runs terraform init and plan -json for every enabled component in the blueprint,
// streaming machine-readable JSON lines to stdout. Stops on the first error. Returns an error
// if blueprint is nil or any component's init or plan step fails.
func (s *TerraformStack) PlanAllJSON(blueprint *blueprintv1alpha1.Blueprint) error {
	return s.planComponents(blueprint, true)
}

// Apply runs terraform init, plan, and apply for a single component identified by componentID.
// It resolves the component from the blueprint, sets up the environment, and executes
// init, plan, then apply in sequence. Returns an error if the component is not found,
// the directory does not exist, or any terraform operation fails.
func (s *TerraformStack) Apply(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if componentID == "" {
		return fmt.Errorf("component ID not provided")
	}

	component, terraformVars, terraformArgs, cleanup, err := s.prepareComponentOp(blueprint, componentID)
	if err != nil {
		return err
	}
	defer cleanup()
	terraformVars["TF_VAR_operation"] = "apply"

	return tui.WithProgress(fmt.Sprintf("Applying %s", component.Path), func() error {
		if err := s.runTerraformInit(component, terraformVars, terraformArgs, defaultInitFlags...); err != nil {
			return err
		}

		terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
		planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan"}
		planArgs = append(planArgs, terraformArgs.PlanArgs...)
		planEnv := selectTerraformCommandEnv(terraformVars, true)
		if _, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, planEnv, planArgs...); err != nil {
			return fmt.Errorf("error running terraform plan for %s: %w", component.Path, err)
		}

		applyArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "apply"}
		applyArgs = append(applyArgs, terraformArgs.ApplyArgs...)
		applyEnv := selectTerraformCommandEnv(terraformVars, false)
		if _, err := s.runtime.Shell.ExecProgressWithEnv(fmt.Sprintf("Applying Terraform changes in %s", component.Path), terraformCommand, applyEnv, applyArgs...); err != nil {
			return fmt.Errorf("error running terraform apply for %s: %w", component.Path, err)
		}
		_ = s.runtime.TerraformProvider.CacheOutputs(component.GetID())

		return nil
	})
}

// Destroy tears down a single component idempotently: init → pre-refresh state check →
// refresh → post-refresh state check → destroy, skipping the rest when state is empty at
// either check. Returns (true, nil) when skipped, (false, nil) on success, (false, err) on
// failure. Destroy-mode config (S3 force_destroy, etc.) is honored via TF_VAR_operation;
// no prep-apply is run because that could recreate resources refresh just dropped.
//
// The whole sequence is wrapped in tui.WithProgress("Destroying <path>") so the visible
// label matches the bulk DestroyAll loop body — mixing this method into the per-component
// destroy dance (which calls bulk DestroyAll for non-backend components and this method
// for backend) would otherwise produce inconsistent "Destroying X" / "Destroying terraform
// for X" lines side by side. The terraform destroy exec runs silently inside the spinner
// for the same reason: the bulk loop is silent, so single-component Destroy must be too.
func (s *TerraformStack) Destroy(blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
	if blueprint == nil {
		return false, fmt.Errorf("blueprint not provided")
	}
	if componentID == "" {
		return false, fmt.Errorf("component ID not provided")
	}

	component, terraformVars, terraformArgs, cleanup, err := s.prepareComponentOp(blueprint, componentID)
	if err != nil {
		return false, err
	}
	defer cleanup()

	terraformVars["TF_VAR_operation"] = "destroy"

	skipped := false
	if err := tui.WithProgress(fmt.Sprintf("Destroying %s", component.Path), func() error {
		if err := s.runTerraformInit(component, terraformVars, terraformArgs, defaultInitFlags...); err != nil {
			return err
		}

		hasResourcesPre, err := s.hasStateResources(component, terraformVars)
		if err != nil {
			return err
		}
		if !hasResourcesPre {
			skipped = true
			return nil
		}

		// Tolerate refresh failures for non-empty-state components — see DestroyAll for the
		// detailed rationale. Falling through to `terraform destroy -refresh=true` keeps a
		// live component destroyable when refresh hits a transient issue. The stderr warning
		// gives the operator visibility into the failure so a subsequent destroy error can
		// be correlated with the upstream refresh hiccup.
		refreshFailed := false
		if err := s.refreshComponentState(component, terraformVars, terraformArgs); err != nil {
			refreshFailed = true
			fmt.Fprintf(s.warningWriter, "warning: terraform refresh failed for %s; falling through to destroy -refresh=true (terraform will retry refresh during destroy): %v\n", component.Path, err)
		}

		if !refreshFailed {
			hasResources, err := s.hasStateResources(component, terraformVars)
			if err != nil {
				return err
			}
			if !hasResources {
				skipped = true
				return nil
			}
		}

		terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
		destroyRefreshFlag := "-refresh=false"
		if refreshFailed {
			destroyRefreshFlag = "-refresh=true"
		}
		destroyArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "destroy", destroyRefreshFlag}
		destroyArgs = append(destroyArgs, terraformArgs.DestroyArgs...)
		destroyEnv := selectTerraformCommandEnv(terraformVars, true)
		if _, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, destroyEnv, destroyArgs...); err != nil {
			return fmt.Errorf("error running terraform destroy for %s: %w", component.Path, err)
		}
		return nil
	}); err != nil {
		return false, err
	}

	return skipped, nil
}

// PlanSummary runs terraform init and plan for every enabled component in the blueprint,
// capturing output to parse add/change/destroy counts rather than printing them.
// Errors are recorded per-component; the summary continues even if a component fails,
// so callers receive partial results for independent layers. Returns nil if blueprint is nil.
func (s *TerraformStack) PlanSummary(blueprint *blueprintv1alpha1.Blueprint) []TerraformComponentPlan {
	if blueprint == nil {
		return nil
	}

	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		return nil
	}

	components := s.resolveTerraformComponents(blueprint, projectRoot)
	results := make([]TerraformComponentPlan, 0, len(components))
	for i := range components {
		results = append(results, s.planOneTerraformSummary(&components[i]))
	}
	return results
}

// PlanComponentSummary runs terraform init and plan for a single component and returns its
// structured plan result. It resolves only the requested component from the blueprint,
// so no other components are initialised or planned. If the component is not found, a
// result with a non-nil Err is returned rather than an error, consistent with PlanSummary.
func (s *TerraformStack) PlanComponentSummary(blueprint *blueprintv1alpha1.Blueprint, componentID string) TerraformComponentPlan {
	result := TerraformComponentPlan{ComponentID: componentID}

	if blueprint == nil {
		result.Err = fmt.Errorf("blueprint not provided")
		return result
	}

	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		result.Err = fmt.Errorf("error getting project root: project root is empty")
		return result
	}

	components := s.resolveTerraformComponents(blueprint, projectRoot)
	var component *blueprintv1alpha1.TerraformComponent
	for i := range components {
		if components[i].GetID() == componentID {
			component = &components[i]
			break
		}
	}
	if component == nil {
		result.Err = fmt.Errorf("terraform component %q not found in blueprint", componentID)
		return result
	}

	return s.planOneTerraformSummary(component)
}

// =============================================================================
// Private Methods
// =============================================================================

// migrateOneComponent runs `terraform init -migrate-state -force-copy` for a single
// component, registering any generated backend_override.tf for cleanup via
// backendOverridePaths. Returns (false, nil) when the component's directory does not
// exist (so callers can decide whether absence is an error or a skip), (true, nil) on
// success, and (false, err) on any other failure. Shared by MigrateState (which
// tolerates missing dirs by collecting skipped IDs) and MigrateComponentState (which
// treats a missing dir as an error because the caller is asking for a specific
// component to be migrated).
func (s *TerraformStack) migrateOneComponent(component *blueprintv1alpha1.TerraformComponent, backendOverridePaths *[]string) (bool, error) {
	if _, statErr := s.shims.Stat(component.FullPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return false, nil
		}
		return false, fmt.Errorf("error checking component directory %s: %w", component.FullPath, statErr)
	}

	terraformVars, terraformArgs, err := s.setupTerraformEnvironment(*component)
	backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
	if _, statErr := s.shims.Stat(backendOverridePath); statErr == nil {
		*backendOverridePaths = append(*backendOverridePaths, backendOverridePath)
	}
	if err != nil {
		return false, err
	}

	if err := s.runTerraformInit(component, terraformVars, terraformArgs, "-migrate-state", "-force-copy"); err != nil {
		return false, err
	}
	return true, nil
}

// hasResources reports whether this module or any descendant contains a resource in state.
func (m tfStateModule) hasResources() bool {
	if len(m.Resources) > 0 {
		return true
	}
	for _, child := range m.ChildModules {
		if child.hasResources() {
			return true
		}
	}
	return false
}

// planComponents runs terraform init and plan for every enabled component in the blueprint.
// When jsonMode is true, -json and -no-color are appended to the plan args so that output
// is machine-readable JSON lines; otherwise human-readable output is streamed. Stops on
// the first error. Returns an error if blueprint is nil or any component's init or plan fails.
func (s *TerraformStack) planComponents(blueprint *blueprintv1alpha1.Blueprint, jsonMode bool) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		return fmt.Errorf("error getting project root: project root is empty")
	}

	components := s.resolveTerraformComponents(blueprint, projectRoot)

	for i := range components {
		component := &components[i]

		fmt.Fprintf(os.Stderr, "\n%s\n", tui.SectionHeader("Terraform: "+component.Path))

		terraformVars, terraformArgs, cleanup, err := s.prepareComponentEnv(component)
		if err != nil {
			return err
		}
		terraformVars["TF_VAR_operation"] = "apply"

		if err := s.runTerraformInit(component, terraformVars, terraformArgs, defaultInitFlags...); err != nil {
			cleanup()
			return err
		}

		terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
		planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan"}
		if jsonMode {
			planArgs = append(planArgs, "-json", "-no-color")
		}
		planArgs = append(planArgs, terraformArgs.PlanArgs...)
		planEnv := selectTerraformCommandEnv(terraformVars, true)
		planOutput, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, planEnv, planArgs...)
		cleanup()
		if err != nil {
			return fmt.Errorf("error running terraform plan for %s: %w", component.Path, err)
		}
		if planOutput != "" {
			fmt.Fprint(os.Stdout, planOutput)
		}
	}

	return nil
}

// refreshComponentState runs `terraform refresh` to reconcile state with cloud reality.
// Errors are returned to the caller. Destroy callers tolerate refresh failures for non-
// empty-state components by falling through to `terraform destroy -refresh=true`; see
// Destroy / DestroyAll for the rationale.
func (s *TerraformStack) refreshComponentState(component *blueprintv1alpha1.TerraformComponent, terraformVars map[string]string, terraformArgs *envvars.TerraformArgs) error {
	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	refreshArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "refresh"}
	refreshArgs = append(refreshArgs, terraformArgs.RefreshArgs...)
	refreshEnv := selectTerraformCommandEnv(terraformVars, true)
	if _, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, refreshEnv, refreshArgs...); err != nil {
		return fmt.Errorf("error refreshing terraform state for %s: %w", component.Path, err)
	}
	return nil
}

// hasStateResources reports whether the component's state contains any resources at any
// depth in the module tree. Used to short-circuit destroy on already-destroyed components.
func (s *TerraformStack) hasStateResources(component *blueprintv1alpha1.TerraformComponent, terraformVars map[string]string) (bool, error) {
	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	stateShowArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "show", "-json"}
	stateJSON, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, selectTerraformCommandEnv(terraformVars, true), stateShowArgs...)
	if err != nil {
		return false, fmt.Errorf("error reading terraform state JSON for %s: %w", component.Path, err)
	}
	var state tfState
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return false, fmt.Errorf("error parsing terraform state JSON for %s: %w", component.Path, err)
	}
	if state.Values == nil {
		return false, nil
	}
	return state.Values.RootModule.hasResources(), nil
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

// planOneTerraformSummary runs terraform init and plan -no-color for a single component
// and returns its structured result. It is shared by PlanSummary and PlanComponentSummary
// to avoid duplicating the per-component setup, init, plan, and cleanup logic.
func (s *TerraformStack) planOneTerraformSummary(component *blueprintv1alpha1.TerraformComponent) TerraformComponentPlan {
	result := TerraformComponentPlan{ComponentID: component.GetID()}

	terraformVars, terraformArgs, cleanup, err := s.prepareComponentEnv(component)
	if err != nil {
		result.Err = err
		return result
	}
	defer cleanup()
	terraformVars["TF_VAR_operation"] = "apply"

	if err := s.runTerraformInit(component, terraformVars, terraformArgs, defaultInitFlags...); err != nil {
		result.Err = err
		return result
	}

	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	// -no-color keeps the output machine-parseable for parseTerraformPlanCounts.
	planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan", "-no-color"}
	planArgs = append(planArgs, terraformArgs.PlanArgs...)
	planEnv := selectTerraformCommandEnv(terraformVars, true)
	planOutput, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, planEnv, planArgs...)
	if err != nil {
		result.Err = fmt.Errorf("error running terraform plan for %s: %w", component.Path, err)
		return result
	}

	result.Add, result.Change, result.Destroy, result.NoChanges = parseTerraformPlanCounts(planOutput)
	return result
}

// prepareComponentEnv saves the current directory, validates the component's directory exists,
// sets up the terraform environment, and returns a cleanup func that restores the working directory
// and removes any backend_override.tf. It is the shared setup used by planComponents,
// planOneTerraformSummary, and prepareComponentOp.
func (s *TerraformStack) prepareComponentEnv(component *blueprintv1alpha1.TerraformComponent) (map[string]string, *envvars.TerraformArgs, func(), error) {
	currentDir, err := s.shims.Getwd()
	if err != nil {
		return nil, nil, func() {}, fmt.Errorf("error getting current directory: %w", err)
	}

	if _, err := s.shims.Stat(component.FullPath); os.IsNotExist(err) {
		return nil, nil, func() {}, fmt.Errorf("directory %s does not exist", component.FullPath)
	}

	removeBackendOverride := func() {
		backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
		if _, statErr := s.shims.Stat(backendOverridePath); statErr == nil {
			_ = s.shims.Remove(backendOverridePath)
		}
	}

	terraformVars, terraformArgs, err := s.setupTerraformEnvironment(*component)
	if err != nil {
		removeBackendOverride()
		return nil, nil, func() {}, err
	}

	cleanup := func() {
		_ = s.shims.Chdir(currentDir)
		removeBackendOverride()
	}

	return terraformVars, terraformArgs, cleanup, nil
}

// prepareComponentOp validates inputs, resolves the named component from the blueprint, saves/restores
// the working directory, sets up the terraform environment, and registers backend override cleanup.
// The returned cleanup func must be called via defer by the caller.
func (s *TerraformStack) prepareComponentOp(blueprint *blueprintv1alpha1.Blueprint, componentID string) (
	*blueprintv1alpha1.TerraformComponent,
	map[string]string,
	*envvars.TerraformArgs,
	func(),
	error,
) {
	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		return nil, nil, nil, func() {}, fmt.Errorf("error getting project root: project root is empty")
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
		return nil, nil, nil, func() {}, fmt.Errorf("terraform component %q not found", componentID)
	}

	terraformVars, terraformArgs, cleanup, err := s.prepareComponentEnv(component)
	if err != nil {
		return nil, nil, nil, func() {}, err
	}

	return component, terraformVars, terraformArgs, cleanup, nil
}

// runTerraformInit executes terraform init for the given component, inserting any extraFlags
// (e.g. "-migrate-state") immediately after the "init" subcommand and before the argument set
// generated by the terraform provider. Returning a wrapped error keeps command-construction in
// one place so the Up path and the MigrateState path can't drift.
func (s *TerraformStack) runTerraformInit(component *blueprintv1alpha1.TerraformComponent, terraformVars map[string]string, terraformArgs *envvars.TerraformArgs, extraFlags ...string) error {
	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	initArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "init"}
	initArgs = append(initArgs, extraFlags...)
	initArgs = append(initArgs, terraformArgs.InitArgs...)
	initEnv := selectTerraformCommandEnv(terraformVars, false)
	_, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, initEnv, initArgs...)
	if err != nil {
		return fmt.Errorf("error running terraform init for %s: %w", component.Path, err)
	}
	return nil
}

// setupTerraformEnvironment computes Terraform-specific environment values and args for a component.
func (s *TerraformStack) setupTerraformEnvironment(component blueprintv1alpha1.TerraformComponent) (map[string]string, *envvars.TerraformArgs, error) {
	terraformEnv := s.getTerraformEnv()
	if terraformEnv == nil {
		return nil, nil, fmt.Errorf("terraform environment printer not available")
	}

	terraformVars, terraformArgs, err := s.runtime.TerraformProvider.GetEnvVars(component.GetID(), false)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting terraform env vars: %w", err)
	}

	if err := terraformEnv.PostEnvHook(component.FullPath); err != nil {
		return nil, nil, fmt.Errorf("error creating backend override file for %s: %w", component.Path, err)
	}

	return terraformVars, terraformArgs, nil
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

// =============================================================================
// Helpers
// =============================================================================

// parseTerraformPlanCounts extracts add/change/destroy counts from terraform plan stdout.
// Returns noChanges=true when terraform reports no infrastructure changes.
// Unrecognised output returns all zeros with noChanges=false.
func parseTerraformPlanCounts(output string) (add, change, destroy int, noChanges bool) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "No changes.") {
			return 0, 0, 0, true
		}
		if strings.HasPrefix(line, "Plan:") {
			for _, segment := range strings.Split(line, ",") {
				segment = strings.TrimSpace(strings.TrimSuffix(segment, "."))
				var n int
				if _, err := fmt.Sscanf(segment, "Plan: %d to add", &n); err == nil {
					add = n
					continue
				}
				if _, err := fmt.Sscanf(segment, "%d to add", &n); err == nil {
					add = n
					continue
				}
				if _, err := fmt.Sscanf(segment, "%d to change", &n); err == nil {
					change = n
					continue
				}
				if _, err := fmt.Sscanf(segment, "%d to destroy", &n); err == nil {
					destroy = n
					continue
				}
			}
			return
		}
	}
	return
}

// selectTerraformCommandEnv builds per-command env overrides without mutating process-wide environment.
func selectTerraformCommandEnv(terraformVars map[string]string, includeTFVars bool) map[string]string {
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
	return selected
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure TerraformStack implements the Stack interface
var _ Stack = (*TerraformStack)(nil)
