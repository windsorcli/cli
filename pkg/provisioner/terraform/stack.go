package terraform

// The Stack package provides infrastructure component stack management functionality.
// It provides a unified interface for initializing and managing infrastructure stacks,
// with support for dependency injection and component lifecycle management.
// The Stack acts as the primary orchestrator for infrastructure operations,
// coordinating shell operations and blueprint handling. The WindsorStack is a specialized
// implementation for Terraform-based infrastructure that handles directory management,
// terraform environment configuration, and Terraform operations.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
	envvars "github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/tui"
)

// =============================================================================
// Interface
// =============================================================================

// Stack defines the interface for Terraform stack operations.
// Both the Stack struct and MockStack implement this interface.
type Stack interface {
	Up(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error
	PostApply(fns ...func(id string) error)
	DestroyAll(blueprint *blueprintv1alpha1.Blueprint) error
	Plan(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	PlanAll(blueprint *blueprintv1alpha1.Blueprint) error
	PlanJSON(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	PlanAllJSON(blueprint *blueprintv1alpha1.Blueprint) error
	Apply(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	Destroy(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	PlanSummary(blueprint *blueprintv1alpha1.Blueprint) []TerraformComponentPlan
	PlanComponentSummary(blueprint *blueprintv1alpha1.Blueprint, componentID string) TerraformComponentPlan
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

// =============================================================================
// Types
// =============================================================================

// TerraformStack manages Terraform infrastructure components by initializing and applying Terraform configurations.
// It processes components in order, generating terraform arguments, running Terraform init, plan, and apply operations.
type TerraformStack struct {
	runtime      *runtime.Runtime
	shims        *Shims
	terraformEnv *envvars.TerraformEnvPrinter
	postApply    []func(id string) error
}

// =============================================================================
// Constructors
// =============================================================================

// NewStack creates a new stack of components.
func NewStack(rt *runtime.Runtime, opts ...*TerraformStack) Stack {
	if rt == nil {
		panic("runtime is required")
	}

	stack := &TerraformStack{
		runtime: rt,
		shims:   NewShims(),
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

// PostApply registers hooks to run after each component's WithProgress block completes (i.e. after Done is
// printed). Hooks are consumed and cleared at the start of the next Up call so they are not retained.
func (s *TerraformStack) PostApply(fns ...func(id string) error) {
	s.postApply = append(s.postApply, fns...)
}

// Up applies every Terraform component in dependency order, adaptively choosing each
// component's backend based on the current state of the configured remote backend.
//
// For each component with a remote backend configured, Up first writes the configured
// backend_override.tf, runs `terraform init`, and runs `terraform state list`. When the
// configured remote is reachable and already holds state for the component, apply proceeds
// against that remote backend. When the remote is unreachable (e.g. the cluster hosting
// the k8s backend has not been provisioned yet) or holds no state, Up swaps to a local
// backend_override.tf, re-initializes against local state, applies, and remembers the
// component for a second pass.
//
// After every component has applied, any components that were forced to local state
// are migrated to the configured remote backend via another `terraform init` (the
// terraform provider's init args already include -force-copy). This produces one
// idempotent behavior that works for fresh environments, partial bootstraps, and
// fully-migrated environments without any mode flags.
//
// When the configured backend is local or none, the probe is skipped and no migration
// is attempted; this matches the pre-existing behavior for those backends.
//
// Optional onApply hooks run after each component apply inside the progress spinner.
// PostApply hooks run after each component's Done line is printed and are consumed
// (cleared) at the start of the next Up call so they do not leak across invocations.
func (s *TerraformStack) Up(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	// Consume and clear postApply hooks so they are not retained across calls.
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

	backendType := s.runtime.ConfigHandler.GetString("terraform.backend.type", "local")
	probeBackend := backendType != "local" && backendType != "none"

	var needMigration []blueprintv1alpha1.TerraformComponent
	for _, component := range components {
		migrateThisComponent := false
		if err := tui.WithProgress(fmt.Sprintf("Applying %s", component.Path), func() error {
			if _, err := s.shims.Stat(component.FullPath); os.IsNotExist(err) {
				return fmt.Errorf("directory %s does not exist", component.FullPath)
			}

			terraformVars, terraformArgs, err := s.setupTerraformEnvironment(component)
			if err != nil {
				return err
			}
			terraformVars["TF_VAR_operation"] = "apply"

			// Track the per-component backend_override.tf path unconditionally. setupTerraformEnvironment,
			// initComponentBackend (forced-local branch), and the Pass 2 migration loop can all write to
			// this path; a per-iteration Stat check would miss files written later in the adaptive flow.
			// The deferred cleanup tolerates missing files, so over-registration is harmless.
			backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
			backendOverridePaths = append(backendOverridePaths, backendOverridePath)

			terraformArgs, forcedLocal, err := s.initComponentBackend(&component, terraformVars, terraformArgs, probeBackend)
			if err != nil {
				return err
			}
			migrateThisComponent = forcedLocal

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

		if migrateThisComponent {
			needMigration = append(needMigration, component)
		}

		// Run post-apply hooks after Done is printed, before the next component's spinner starts.
		componentID := component.GetID()
		for _, fn := range postApply {
			if fn != nil {
				if err := fn(componentID); err != nil {
					return fmt.Errorf("post-apply hook %s: %w", componentID, err)
				}
			}
		}
	}

	for i := range needMigration {
		if err := s.migrateComponentToRemote(&needMigration[i]); err != nil {
			return err
		}
	}

	return nil
}

// DestroyAll destroys all Terraform components in the stack by executing Terraform destroy operations in reverse dependency order.
// For each component, DestroyAll generates Terraform arguments, sets required environment variables, unsets conflicting TF_CLI_ARGS_* variables,
// creates backend override files, runs Terraform refresh, plan (with destroy flag), and destroy commands. Backend override files are
// cleaned up after all components complete. Components with Destroy set to false are skipped. Directory state is restored after execution.
// Errors are returned on any operation failure. The blueprint parameter is required to resolve terraform components.
func (s *TerraformStack) DestroyAll(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
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

	var backendOverridePaths []string
	defer func() {
		for _, path := range backendOverridePaths {
			_ = s.shims.Remove(path)
		}
	}()

	for i := len(components) - 1; i >= 0; i-- {
		component := components[i]

		if component.Destroy != nil {
			destroy := component.Destroy.ToBool()
			if destroy != nil && !*destroy {
				continue
			}
		}

		if _, err := s.shims.Stat(component.FullPath); os.IsNotExist(err) {
			continue
		}

		if err := tui.WithProgress(fmt.Sprintf("Destroying %s", component.Path), func() error {
			terraformVars, terraformArgs, err := s.setupTerraformEnvironment(component)
			if err != nil {
				return err
			}
			terraformVars["TF_VAR_operation"] = "destroy"

			backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
			if _, err := s.shims.Stat(backendOverridePath); err == nil {
				backendOverridePaths = append(backendOverridePaths, backendOverridePath)
			}

			terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
			refreshArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "refresh"}
			refreshArgs = append(refreshArgs, terraformArgs.RefreshArgs...)
			refreshEnv := selectTerraformCommandEnv(terraformVars, true)
			_, _ = s.runtime.Shell.ExecSilentWithEnv(terraformCommand, refreshEnv, refreshArgs...)

			planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan"}
			planArgs = append(planArgs, terraformArgs.PlanDestroyArgs...)
			planEnv := selectTerraformCommandEnv(terraformVars, true)
			if _, err := s.runtime.Shell.ExecProgressWithEnv(fmt.Sprintf("Planning terraform destroy for %s", component.Path), terraformCommand, planEnv, planArgs...); err != nil {
				return fmt.Errorf("error running terraform plan destroy for %s: %w", component.Path, err)
			}

			destroyArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "destroy"}
			destroyArgs = append(destroyArgs, terraformArgs.DestroyArgs...)
			destroyEnv := selectTerraformCommandEnv(terraformVars, true)
			if _, err := s.runtime.Shell.ExecProgressWithEnv(fmt.Sprintf("Destroying terraform for %s", component.Path), terraformCommand, destroyEnv, destroyArgs...); err != nil {
				return fmt.Errorf("error running terraform destroy for %s: %w", component.Path, err)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
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

		if err := s.runTerraformInit(component, terraformVars, terraformArgs); err != nil {
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

// PlanAll runs terraform init and plan for every enabled component in the blueprint,
// streaming output directly to stdout. Stops on the first error. Returns an error if
// blueprint is nil or any component's init or plan step fails.
func (s *TerraformStack) PlanAll(blueprint *blueprintv1alpha1.Blueprint) error {
	return s.planComponents(blueprint, false)
}

// PlanAllJSON runs terraform init and plan -json for every enabled component in the blueprint,
// streaming machine-readable JSON lines to stdout. Stops on the first error. Returns an error
// if blueprint is nil or any component's init or plan step fails.
func (s *TerraformStack) PlanAllJSON(blueprint *blueprintv1alpha1.Blueprint) error {
	return s.planComponents(blueprint, true)
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

	if err := s.runTerraformInit(component, terraformVars, terraformArgs); err != nil {
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

	if err := s.runTerraformInit(component, terraformVars, terraformArgs); err != nil {
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
		if err := s.runTerraformInit(component, terraformVars, terraformArgs); err != nil {
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

// Destroy runs terraform init, plan -destroy, and destroy for a single component identified by componentID.
// Returns an error if the blueprint is nil, the component is not found, or any terraform operation fails.
func (s *TerraformStack) Destroy(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
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
	terraformVars["TF_VAR_operation"] = "destroy"

	if err := s.runTerraformInit(component, terraformVars, terraformArgs); err != nil {
		return err
	}

	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()

	refreshArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "refresh"}
	refreshArgs = append(refreshArgs, terraformArgs.RefreshArgs...)
	_, _ = s.runtime.Shell.ExecSilentWithEnv(terraformCommand,
		selectTerraformCommandEnv(terraformVars, true), refreshArgs...)

	planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan"}
	planArgs = append(planArgs, terraformArgs.PlanDestroyArgs...)
	if _, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand,
		selectTerraformCommandEnv(terraformVars, true), planArgs...); err != nil {
		return fmt.Errorf("error running terraform plan destroy for %s: %w", component.Path, err)
	}

	destroyArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "destroy"}
	destroyArgs = append(destroyArgs, terraformArgs.DestroyArgs...)
	if _, err := s.runtime.Shell.ExecProgressWithEnv(
		fmt.Sprintf("Destroying terraform for %s", component.Path),
		terraformCommand, selectTerraformCommandEnv(terraformVars, true), destroyArgs...); err != nil {
		return fmt.Errorf("error running terraform destroy for %s: %w", component.Path, err)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

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

	if err := s.runTerraformInit(component, terraformVars, terraformArgs); err != nil {
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

	terraformVars, terraformArgs, err := s.setupTerraformEnvironment(*component)
	if err != nil {
		return nil, nil, func() {}, err
	}

	cleanup := func() {
		_ = s.shims.Chdir(currentDir)
		backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
		if _, statErr := s.shims.Stat(backendOverridePath); statErr == nil {
			_ = s.shims.Remove(backendOverridePath)
		}
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

// runTerraformInit executes terraform init for the given component.
func (s *TerraformStack) runTerraformInit(component *blueprintv1alpha1.TerraformComponent, terraformVars map[string]string, terraformArgs *envvars.TerraformArgs) error {
	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	initArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "init"}
	initArgs = append(initArgs, terraformArgs.InitArgs...)
	initEnv := selectTerraformCommandEnv(terraformVars, false)
	_, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, initEnv, initArgs...)
	if err != nil {
		return fmt.Errorf("error running terraform init for %s: %w", component.Path, err)
	}
	return nil
}

// initComponentBackend picks the effective backend for a component, writes the matching
// backend_override.tf, and runs terraform init. When probeBackend is true, it first probes
// the configured remote backend; if the remote has populated state, init has already run
// inside the probe and the configured remote args are returned unchanged. If the remote is
// unreachable or empty, the component is pinned to local state: a local backend_override.tf
// is written, forced-local args are generated, terraform init runs again, and the returned
// forcedLocal flag signals that Pass 2 migration is required for this component. When
// probeBackend is false (backend is local or none) the probe is skipped and init runs once
// with the configured args.
func (s *TerraformStack) initComponentBackend(
	component *blueprintv1alpha1.TerraformComponent,
	terraformVars map[string]string,
	terraformArgs *envvars.TerraformArgs,
	probeBackend bool,
) (*envvars.TerraformArgs, bool, error) {
	if probeBackend {
		if s.remoteBackendHasState(component, terraformVars, terraformArgs) {
			return terraformArgs, false, nil
		}
		if err := s.runtime.TerraformProvider.GenerateLocalBackendOverride(component.FullPath); err != nil {
			return nil, false, fmt.Errorf("error writing local backend override for %s: %w", component.Path, err)
		}
		forcedArgs, err := s.runtime.TerraformProvider.GenerateTerraformArgsForcedLocal(component.GetID(), false)
		if err != nil {
			return nil, false, fmt.Errorf("error generating forced-local args for %s: %w", component.Path, err)
		}
		if err := s.runTerraformInit(component, terraformVars, forcedArgs); err != nil {
			return nil, false, err
		}
		return forcedArgs, true, nil
	}
	if err := s.runTerraformInit(component, terraformVars, terraformArgs); err != nil {
		return nil, false, err
	}
	return terraformArgs, false, nil
}

// remoteBackendHasState runs terraform init and terraform state list against the configured
// remote backend to determine whether this component already has state there. Returns true only
// when both commands succeed and state list reports at least one resource. Any init or state
// list failure, and any empty state list output, returns false; the caller interprets false as
// "force local" and the real error (if any) will surface at Pass 2 migration time.
func (s *TerraformStack) remoteBackendHasState(component *blueprintv1alpha1.TerraformComponent, terraformVars map[string]string, terraformArgs *envvars.TerraformArgs) bool {
	if err := s.runTerraformInit(component, terraformVars, terraformArgs); err != nil {
		return false
	}
	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	stateArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "state", "list"}
	stateEnv := selectTerraformCommandEnv(terraformVars, false)
	output, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, stateEnv, stateArgs...)
	if err != nil {
		return false
	}
	return strings.TrimSpace(output) != ""
}

// migrateComponentToRemote rewrites the component's backend_override.tf to the configured remote
// backend and runs terraform init. The provider's init args already include -force-copy, which
// causes terraform to migrate local state to the remote backend. Used as Pass 2 of Up after every
// component has applied locally, so remote backends that are themselves provisioned by an earlier
// component (e.g. the kubernetes cluster hosting the k8s backend) exist by the time migration runs.
// On failure the local state written in Pass 1 is retained so the user can re-run Up to retry the
// migration once the remote backend is reachable.
func (s *TerraformStack) migrateComponentToRemote(component *blueprintv1alpha1.TerraformComponent) error {
	if err := s.runtime.TerraformProvider.GenerateBackendOverride(component.FullPath); err != nil {
		return fmt.Errorf("error writing remote backend override for %s: %w", component.Path, err)
	}
	terraformVars, terraformArgs, err := s.runtime.TerraformProvider.GetEnvVars(component.GetID(), false)
	if err != nil {
		return fmt.Errorf("error getting terraform env vars for %s: %w", component.Path, err)
	}
	if err := s.runTerraformInit(component, terraformVars, terraformArgs); err != nil {
		return fmt.Errorf("state migration failed for %s (local state retained for retry): %w", component.Path, err)
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
// Interface Compliance
// =============================================================================

// Ensure TerraformStack implements the Stack interface
var _ Stack = (*TerraformStack)(nil)
