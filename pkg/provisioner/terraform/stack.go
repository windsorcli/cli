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
	"regexp"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime"
	envvars "github.com/windsorcli/cli/pkg/runtime/env"
)

// =============================================================================
// Interfaces
// =============================================================================

// Stack is an interface that represents a stack of components.
type Stack interface {
	Up(blueprint *blueprintv1alpha1.Blueprint) error
	Down(blueprint *blueprintv1alpha1.Blueprint) error
}

// =============================================================================
// Types
// =============================================================================

// BaseStack is a struct that implements the Stack interface.
type BaseStack struct {
	runtime          *runtime.Runtime
	blueprintHandler blueprint.BlueprintHandler
	shims            *Shims
}

// WindsorStack is a struct that implements the Stack interface.
type WindsorStack struct {
	BaseStack
	terraformEnv *envvars.TerraformEnvPrinter
}

// =============================================================================
// Constructors
// =============================================================================

// NewBaseStack creates a new base stack of components.
func NewBaseStack(rt *runtime.Runtime, blueprintHandler blueprint.BlueprintHandler) *BaseStack {
	return &BaseStack{
		runtime:          rt,
		blueprintHandler: blueprintHandler,
		shims:            NewShims(),
	}
}

// NewWindsorStack creates a new WindsorStack.
func NewWindsorStack(rt *runtime.Runtime, blueprintHandler blueprint.BlueprintHandler, opts ...*WindsorStack) *WindsorStack {
	stack := &WindsorStack{
		BaseStack: BaseStack{
			runtime:          rt,
			blueprintHandler: blueprintHandler,
			shims:            NewShims(),
		},
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

// Up creates a new stack of components.
func (s *BaseStack) Up(blueprint *blueprintv1alpha1.Blueprint) error {
	return nil
}

// Down destroys a stack of components.
func (s *BaseStack) Down(blueprint *blueprintv1alpha1.Blueprint) error {
	return nil
}

// Up creates a new stack of components by initializing and applying Terraform configurations.
// It processes components in order, generating terraform arguments, running Terraform init,
// plan, and apply operations, and cleaning up backend override files.
// The method ensures proper directory management and terraform argument setup for each component.
// The blueprint parameter is required to resolve terraform components.
func (s *WindsorStack) Up(blueprint *blueprintv1alpha1.Blueprint) error {
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

	for _, component := range components {
		if _, err := s.shims.Stat(component.FullPath); os.IsNotExist(err) {
			return fmt.Errorf("directory %s does not exist", component.FullPath)
		}

		terraformArgs, err := s.terraformEnv.GenerateTerraformArgs(component.Path, component.FullPath)
		if err != nil {
			return fmt.Errorf("error generating terraform args for %s: %w", component.Path, err)
		}

		tfCliArgsVars := []string{"TF_CLI_ARGS_init", "TF_CLI_ARGS_plan", "TF_CLI_ARGS_apply", "TF_CLI_ARGS_destroy", "TF_CLI_ARGS_import"}
		for _, envVar := range tfCliArgsVars {
			if err := s.shims.Unsetenv(envVar); err != nil {
				return fmt.Errorf("error unsetting %s: %w", envVar, err)
			}
		}

		for key, value := range terraformArgs.TerraformVars {
			if key == "TF_DATA_DIR" || strings.HasPrefix(key, "TF_VAR_") {
				if err := s.shims.Setenv(key, value); err != nil {
					return fmt.Errorf("error setting %s: %w", key, err)
				}
			}
		}

		if err := s.terraformEnv.PostEnvHook(component.FullPath); err != nil {
			return fmt.Errorf("error creating backend override file for %s: %w", component.Path, err)
		}

		initArgs := []string{fmt.Sprintf("-chdir=%s", terraformArgs.ModulePath), "init"}
		initArgs = append(initArgs, terraformArgs.InitArgs...)
		_, err = s.runtime.Shell.ExecProgress(fmt.Sprintf("🌎 Initializing Terraform in %s", component.Path), "terraform", initArgs...)
		if err != nil {
			return fmt.Errorf("error running terraform init for %s: %w", component.Path, err)
		}

		refreshArgs := []string{fmt.Sprintf("-chdir=%s", terraformArgs.ModulePath), "refresh"}
		refreshArgs = append(refreshArgs, terraformArgs.RefreshArgs...)
		_, _ = s.runtime.Shell.ExecProgress(fmt.Sprintf("🔄 Refreshing Terraform state in %s", component.Path), "terraform", refreshArgs...)

		planArgs := []string{fmt.Sprintf("-chdir=%s", terraformArgs.ModulePath), "plan"}
		planArgs = append(planArgs, terraformArgs.PlanArgs...)
		_, err = s.runtime.Shell.ExecProgress(fmt.Sprintf("🌎 Planning Terraform changes in %s", component.Path), "terraform", planArgs...)
		if err != nil {
			return fmt.Errorf("error running terraform plan for %s: %w", component.Path, err)
		}

		applyArgs := []string{fmt.Sprintf("-chdir=%s", terraformArgs.ModulePath), "apply"}
		applyArgs = append(applyArgs, terraformArgs.ApplyArgs...)
		_, err = s.runtime.Shell.ExecProgress(fmt.Sprintf("🌎 Applying Terraform changes in %s", component.Path), "terraform", applyArgs...)
		if err != nil {
			return fmt.Errorf("error running terraform apply for %s: %w", component.Path, err)
		}

		backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
		if _, err := s.shims.Stat(backendOverridePath); err == nil {
			if err := s.shims.Remove(backendOverridePath); err != nil {
				return fmt.Errorf("error removing backend override file for %s: %w", component.Path, err)
			}
		}
	}

	return nil
}

// Down destroys all Terraform components in the stack by executing Terraform destroy operations in reverse dependency order.
// For each component, Down generates Terraform arguments, sets required environment variables, unsets conflicting TF_CLI_ARGS_* variables,
// creates backend override files, runs Terraform refresh, plan (with destroy flag), and destroy commands, and removes backend override files.
// Components with Destroy set to false are skipped. Directory state is restored after execution. Errors are returned on any operation failure.
// The blueprint parameter is required to resolve terraform components.
func (s *WindsorStack) Down(blueprint *blueprintv1alpha1.Blueprint) error {
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

	for i := len(components) - 1; i >= 0; i-- {
		component := components[i]

		if component.Destroy != nil && !*component.Destroy {
			continue
		}

		if _, err := s.shims.Stat(component.FullPath); os.IsNotExist(err) {
			continue
		}

		terraformArgs, err := s.terraformEnv.GenerateTerraformArgs(component.Path, component.FullPath)
		if err != nil {
			return fmt.Errorf("error generating terraform args for %s: %w", component.Path, err)
		}

		tfCliArgsVars := []string{"TF_CLI_ARGS_init", "TF_CLI_ARGS_plan", "TF_CLI_ARGS_apply", "TF_CLI_ARGS_destroy", "TF_CLI_ARGS_import"}
		for _, envVar := range tfCliArgsVars {
			if err := s.shims.Unsetenv(envVar); err != nil {
				return fmt.Errorf("error unsetting %s: %w", envVar, err)
			}
		}

		for key, value := range terraformArgs.TerraformVars {
			if key == "TF_DATA_DIR" || strings.HasPrefix(key, "TF_VAR_") {
				if err := s.shims.Setenv(key, value); err != nil {
					return fmt.Errorf("error setting %s: %w", key, err)
				}
			}
		}

		if err := s.terraformEnv.PostEnvHook(component.FullPath); err != nil {
			return fmt.Errorf("error creating backend override file for %s: %w", component.Path, err)
		}

		refreshArgs := []string{fmt.Sprintf("-chdir=%s", terraformArgs.ModulePath), "refresh"}
		refreshArgs = append(refreshArgs, terraformArgs.RefreshArgs...)
		_, _ = s.runtime.Shell.ExecProgress(fmt.Sprintf("🔄 Refreshing Terraform state in %s", component.Path), "terraform", refreshArgs...)

		planArgs := []string{fmt.Sprintf("-chdir=%s", terraformArgs.ModulePath), "plan"}
		planArgs = append(planArgs, terraformArgs.PlanDestroyArgs...)
		if _, err := s.runtime.Shell.ExecProgress(fmt.Sprintf("🗑️  Planning terraform destroy for %s", component.Path), "terraform", planArgs...); err != nil {
			return fmt.Errorf("error running terraform plan destroy for %s: %w", component.Path, err)
		}

		destroyArgs := []string{fmt.Sprintf("-chdir=%s", terraformArgs.ModulePath), "destroy"}
		destroyArgs = append(destroyArgs, terraformArgs.DestroyArgs...)
		if _, err := s.runtime.Shell.ExecProgress(fmt.Sprintf("🗑️  Destroying terraform for %s", component.Path), "terraform", destroyArgs...); err != nil {
			return fmt.Errorf("error running terraform destroy for %s: %w", component.Path, err)
		}

		if err := s.shims.Remove(filepath.Join(component.FullPath, "backend_override.tf")); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("error removing backend_override.tf from %s: %w", component.Path, err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// resolveTerraformComponents resolves terraform components from the blueprint by resolving sources and paths.
func (s *WindsorStack) resolveTerraformComponents(blueprint *blueprintv1alpha1.Blueprint, projectRoot string) []blueprintv1alpha1.TerraformComponent {
	blueprintCopy := *blueprint
	s.resolveComponentSources(&blueprintCopy)
	s.resolveComponentPaths(&blueprintCopy, projectRoot)
	return blueprintCopy.TerraformComponents
}

// resolveComponentSources resolves component source names to full URLs using blueprint sources.
func (s *WindsorStack) resolveComponentSources(blueprint *blueprintv1alpha1.Blueprint) {
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
func (s *WindsorStack) resolveComponentPaths(blueprint *blueprintv1alpha1.Blueprint, projectRoot string) {
	resolvedComponents := make([]blueprintv1alpha1.TerraformComponent, len(blueprint.TerraformComponents))
	copy(resolvedComponents, blueprint.TerraformComponents)

	for i, component := range resolvedComponents {
		componentCopy := component

		if s.isValidTerraformRemoteSource(componentCopy.Source) || s.isOCISource(componentCopy.Source, blueprint) {
			componentCopy.FullPath = filepath.Join(projectRoot, ".windsor", ".tf_modules", componentCopy.Path)
		} else {
			componentCopy.FullPath = filepath.Join(projectRoot, "terraform", componentCopy.Path)
		}

		componentCopy.FullPath = filepath.FromSlash(componentCopy.FullPath)

		resolvedComponents[i] = componentCopy
	}

	blueprint.TerraformComponents = resolvedComponents
}

// isValidTerraformRemoteSource checks if the source is a valid Terraform module reference.
func (s *WindsorStack) isValidTerraformRemoteSource(source string) bool {
	patterns := []string{
		`^git::https://[^/]+/.*\.git(?:@.*)?$`,
		`^git@[^:]+:.*\.git(?:@.*)?$`,
		`^https?://[^/]+/.*\.git(?:@.*)?$`,
		`^https?://[^/]+/.*\.zip(?:@.*)?$`,
		`^https?://[^/]+/.*//.*(?:@.*)?$`,
		`^registry\.terraform\.io/.*`,
		`^[^/]+\.com/.*`,
	}

	for _, pattern := range patterns {
		matched, err := regexp.MatchString(pattern, source)
		if err != nil {
			return false
		}
		if matched {
			return true
		}
	}

	return false
}

// isOCISource returns true if the provided source is an OCI repository reference.
func (s *WindsorStack) isOCISource(sourceNameOrURL string, blueprint *blueprintv1alpha1.Blueprint) bool {
	if strings.HasPrefix(sourceNameOrURL, "oci://") {
		return true
	}
	if sourceNameOrURL == blueprint.Metadata.Name && strings.HasPrefix(blueprint.Repository.Url, "oci://") {
		return true
	}
	for _, source := range blueprint.Sources {
		if source.Name == sourceNameOrURL && strings.HasPrefix(source.Url, "oci://") {
			return true
		}
	}
	return false
}

// Ensure BaseStack implements Stack
var _ Stack = (*BaseStack)(nil)
