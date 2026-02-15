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
)

// =============================================================================
// Interface
// =============================================================================

// Stack defines the interface for Terraform stack operations.
// Both the Stack struct and MockStack implement this interface.
type Stack interface {
	Up(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error
	Down(blueprint *blueprintv1alpha1.Blueprint) error
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

// Up creates a new stack of components by initializing and applying Terraform configurations.
// It processes components in order, generating terraform arguments, running Terraform init,
// plan, and apply operations. Backend override files are cleaned up after all components complete,
// ensuring they remain available for terraform_output() calls between component executions.
// Optional onApply hooks run after each component apply, in order; they are not retained after Up returns.
func (s *TerraformStack) Up(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error {
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

	for _, component := range components {
		if _, err := s.shims.Stat(component.FullPath); os.IsNotExist(err) {
			return fmt.Errorf("directory %s does not exist", component.FullPath)
		}

		terraformArgs, err := s.setupTerraformEnvironment(component)
		if err != nil {
			return err
		}

		backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
		if _, err := s.shims.Stat(backendOverridePath); err == nil {
			backendOverridePaths = append(backendOverridePaths, backendOverridePath)
		}

		terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
		initArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "init"}
		initArgs = append(initArgs, terraformArgs.InitArgs...)
		_, err = s.runtime.Shell.ExecProgress(fmt.Sprintf("🌎 Initializing Terraform in %s", component.Path), terraformCommand, initArgs...)
		if err != nil {
			return fmt.Errorf("error running terraform init for %s: %w", component.Path, err)
		}

		refreshArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "refresh"}
		refreshArgs = append(refreshArgs, terraformArgs.RefreshArgs...)
		_, _ = s.runtime.Shell.ExecProgress(fmt.Sprintf("🔄 Refreshing Terraform state in %s", component.Path), terraformCommand, refreshArgs...)

		planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan"}
		planArgs = append(planArgs, terraformArgs.PlanArgs...)
		_, err = s.runtime.Shell.ExecProgress(fmt.Sprintf("🌎 Planning Terraform changes in %s", component.Path), terraformCommand, planArgs...)
		if err != nil {
			return fmt.Errorf("error running terraform plan for %s: %w", component.Path, err)
		}

		applyArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "apply"}
		applyArgs = append(applyArgs, terraformArgs.ApplyArgs...)
		_, err = s.runtime.Shell.ExecProgress(fmt.Sprintf("🌎 Applying Terraform changes in %s", component.Path), terraformCommand, applyArgs...)
		if err != nil {
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
	}

	return nil
}

// Down destroys all Terraform components in the stack by executing Terraform destroy operations in reverse dependency order.
// For each component, Down generates Terraform arguments, sets required environment variables, unsets conflicting TF_CLI_ARGS_* variables,
// creates backend override files, runs Terraform refresh, plan (with destroy flag), and destroy commands. Backend override files are
// cleaned up after all components complete. Components with Destroy set to false are skipped. Directory state is restored after execution.
// Errors are returned on any operation failure. The blueprint parameter is required to resolve terraform components.
func (s *TerraformStack) Down(blueprint *blueprintv1alpha1.Blueprint) error {
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

		terraformArgs, err := s.setupTerraformEnvironment(component)
		if err != nil {
			return err
		}

		backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
		if _, err := s.shims.Stat(backendOverridePath); err == nil {
			backendOverridePaths = append(backendOverridePaths, backendOverridePath)
		}

		terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
		refreshArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "refresh"}
		refreshArgs = append(refreshArgs, terraformArgs.RefreshArgs...)
		_, _ = s.runtime.Shell.ExecProgress(fmt.Sprintf("🔄 Refreshing Terraform state in %s", component.Path), terraformCommand, refreshArgs...)

		planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan"}
		planArgs = append(planArgs, terraformArgs.PlanDestroyArgs...)
		if _, err := s.runtime.Shell.ExecProgress(fmt.Sprintf("🗑️  Planning terraform destroy for %s", component.Path), terraformCommand, planArgs...); err != nil {
			return fmt.Errorf("error running terraform plan destroy for %s: %w", component.Path, err)
		}

		destroyArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "destroy"}
		destroyArgs = append(destroyArgs, terraformArgs.DestroyArgs...)
		if _, err := s.runtime.Shell.ExecProgress(fmt.Sprintf("🗑️  Destroying terraform for %s", component.Path), terraformCommand, destroyArgs...); err != nil {
			return fmt.Errorf("error running terraform destroy for %s: %w", component.Path, err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// resolveTerraformComponents resolves terraform components from the blueprint by resolving sources and paths.
func (s *TerraformStack) resolveTerraformComponents(blueprint *blueprintv1alpha1.Blueprint, projectRoot string) []blueprintv1alpha1.TerraformComponent {
	blueprintCopy := *blueprint
	s.resolveComponentSources(&blueprintCopy)
	s.resolveComponentPaths(&blueprintCopy, projectRoot)
	return blueprintCopy.TerraformComponents
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

// setupTerraformEnvironment configures the Terraform environment for a component.
// It unsets TF_CLI_ARGS_* environment variables, gets terraform env vars and args,
// sets TF_DATA_DIR and TF_VAR_* environment variables, and runs the PostEnvHook.
// Returns the terraform args needed for command construction, or an error.
func (s *TerraformStack) setupTerraformEnvironment(component blueprintv1alpha1.TerraformComponent) (*envvars.TerraformArgs, error) {
	terraformEnv := s.getTerraformEnv()
	if terraformEnv == nil {
		return nil, fmt.Errorf("terraform environment printer not available")
	}

	tfCliArgsVars := []string{"TF_CLI_ARGS_init", "TF_CLI_ARGS_plan", "TF_CLI_ARGS_apply", "TF_CLI_ARGS_destroy", "TF_CLI_ARGS_import"}
	for _, envVar := range tfCliArgsVars {
		if err := s.shims.Unsetenv(envVar); err != nil {
			return nil, fmt.Errorf("error unsetting %s: %w", envVar, err)
		}
	}

	terraformVars, terraformArgs, err := s.runtime.TerraformProvider.GetEnvVars(component.GetID(), false)
	if err != nil {
		return nil, fmt.Errorf("error getting terraform env vars: %w", err)
	}

	for key, value := range terraformVars {
		if key == "TF_DATA_DIR" || strings.HasPrefix(key, "TF_VAR_") {
			if err := s.shims.Setenv(key, value); err != nil {
				return nil, fmt.Errorf("error setting %s: %w", key, err)
			}
		}
	}

	if err := terraformEnv.PostEnvHook(component.FullPath); err != nil {
		return nil, fmt.Errorf("error creating backend override file for %s: %w", component.Path, err)
	}

	return terraformArgs, nil
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
