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
	"github.com/windsorcli/cli/pkg/runtime"
	envvars "github.com/windsorcli/cli/pkg/runtime/env"
)

// =============================================================================
// Interface
// =============================================================================

// Stack defines the interface for Terraform stack operations.
// Both the Stack struct and MockStack implement this interface.
type Stack interface {
	Up(blueprint *blueprintv1alpha1.Blueprint) error
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
// plan, and apply operations, and cleaning up backend override files.
// The method ensures proper directory management and terraform argument setup for each component.
// The blueprint parameter is required to resolve terraform components.
func (s *TerraformStack) Up(blueprint *blueprintv1alpha1.Blueprint) error {
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

		terraformEnv := s.getTerraformEnv()
		if terraformEnv == nil {
			return fmt.Errorf("terraform environment printer not available")
		}
		componentID := component.GetID()
		terraformArgs, err := terraformEnv.GenerateTerraformArgs(componentID, component.FullPath, false)
		if err != nil {
			return fmt.Errorf("error generating terraform args for %s: %w", componentID, err)
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

		if err := terraformEnv.PostEnvHook(component.FullPath); err != nil {
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

	for i := len(components) - 1; i >= 0; i-- {
		component := components[i]

		if component.Destroy != nil && !*component.Destroy {
			continue
		}

		if _, err := s.shims.Stat(component.FullPath); os.IsNotExist(err) {
			continue
		}

		terraformEnv := s.getTerraformEnv()
		if terraformEnv == nil {
			return fmt.Errorf("terraform environment printer not available")
		}
		componentID := component.GetID()
		terraformArgs, err := terraformEnv.GenerateTerraformArgs(componentID, component.FullPath, false)
		if err != nil {
			return fmt.Errorf("error generating terraform args for %s: %w", componentID, err)
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

		if err := terraformEnv.PostEnvHook(component.FullPath); err != nil {
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

		var dirName string
		if componentCopy.Name != "" {
			dirName = componentCopy.Name
		} else {
			dirName = componentCopy.Path
		}

		if componentCopy.Name != "" ||
			s.isValidTerraformRemoteSource(componentCopy.Source) ||
			s.isOCISource(componentCopy.Source, blueprint) ||
			strings.HasPrefix(componentCopy.Source, "file://") {
			componentCopy.FullPath = filepath.Join(projectRoot, ".windsor", "contexts", s.runtime.ContextName, "terraform", dirName)
		} else {
			componentCopy.FullPath = filepath.Join(projectRoot, "terraform", dirName)
		}

		componentCopy.FullPath = filepath.FromSlash(componentCopy.FullPath)

		resolvedComponents[i] = componentCopy
	}

	blueprint.TerraformComponents = resolvedComponents
}

// isValidTerraformRemoteSource checks if the source is a valid Terraform module reference.
func (s *TerraformStack) isValidTerraformRemoteSource(source string) bool {
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
func (s *TerraformStack) isOCISource(sourceNameOrURL string, blueprint *blueprintv1alpha1.Blueprint) bool {
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
