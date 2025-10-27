package stack

// The WindsorStack is a specialized implementation of the Stack interface for Terraform-based infrastructure.
// It provides a concrete implementation for managing Terraform components through the Windsor CLI,
// handling directory management, terraform environment configuration, and Terraform operations.
// The WindsorStack orchestrates Terraform initialization, planning, and application,
// while managing terraform arguments and backend configurations.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/environment/envvars"
)

// =============================================================================
// Types
// =============================================================================

// WindsorStack is a struct that implements the Stack interface.
type WindsorStack struct {
	BaseStack
	terraformEnv *envvars.TerraformEnvPrinter
}

// =============================================================================
// Constructor
// =============================================================================

// NewWindsorStack creates a new WindsorStack.
func NewWindsorStack(injector di.Injector) *WindsorStack {
	return &WindsorStack{
		BaseStack: BaseStack{
			injector: injector,
			shims:    NewShims(),
		},
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize initializes the WindsorStack by calling the base Initialize and resolving terraform environment.
func (s *WindsorStack) Initialize() error {
	// Call the base Initialize method
	if err := s.BaseStack.Initialize(); err != nil {
		return err
	}

	// Resolve the terraform environment printer - required for WindsorStack
	terraformEnvInterface := s.injector.Resolve("terraformEnv")
	if terraformEnvInterface == nil {
		return fmt.Errorf("terraformEnv not found in dependency injector")
	}

	terraformEnv, ok := terraformEnvInterface.(*envvars.TerraformEnvPrinter)
	if !ok {
		return fmt.Errorf("error resolving terraformEnv")
	}
	s.terraformEnv = terraformEnv

	return nil
}

// Up creates a new stack of components by initializing and applying Terraform configurations.
// It processes components in order, generating terraform arguments, running Terraform init,
// plan, and apply operations, and cleaning up backend override files.
// The method ensures proper directory management and terraform argument setup for each component.
func (s *WindsorStack) Up() error {
	currentDir, err := s.shims.Getwd()
	if err != nil {
		return fmt.Errorf("error getting current directory: %v", err)
	}

	defer func() {
		_ = s.shims.Chdir(currentDir)
	}()

	components := s.blueprintHandler.GetTerraformComponents()

	for _, component := range components {
		if _, err := s.shims.Stat(component.FullPath); os.IsNotExist(err) {
			return fmt.Errorf("directory %s does not exist", component.FullPath)
		}

		terraformArgs, err := s.terraformEnv.GenerateTerraformArgs(component.Path, component.FullPath)
		if err != nil {
			return fmt.Errorf("error generating terraform args for %s: %w", component.Path, err)
		}

		// Set terraform environment variables (TF_VAR_* and TF_DATA_DIR)
		// First, unset any existing TF_CLI_ARGS_* environment variables to avoid conflicts
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

		// Create backend_override.tf file in the component directory
		if err := s.terraformEnv.PostEnvHook(component.FullPath); err != nil {
			return fmt.Errorf("error creating backend override file for %s: %w", component.Path, err)
		}

		initArgs := []string{fmt.Sprintf("-chdir=%s", terraformArgs.ModulePath), "init"}
		initArgs = append(initArgs, terraformArgs.InitArgs...)
		_, err = s.shell.ExecProgress(fmt.Sprintf("🌎 Initializing Terraform in %s", component.Path), "terraform", initArgs...)
		if err != nil {
			return fmt.Errorf("error running terraform init for %s: %w", component.Path, err)
		}

		// Run terraform refresh to sync state with actual infrastructure
		// This is tolerant of failures for non-existent state
		refreshArgs := []string{fmt.Sprintf("-chdir=%s", terraformArgs.ModulePath), "refresh"}
		refreshArgs = append(refreshArgs, terraformArgs.RefreshArgs...)
		_, _ = s.shell.ExecProgress(fmt.Sprintf("🔄 Refreshing Terraform state in %s", component.Path), "terraform", refreshArgs...)

		planArgs := []string{fmt.Sprintf("-chdir=%s", terraformArgs.ModulePath), "plan"}
		planArgs = append(planArgs, terraformArgs.PlanArgs...)
		_, err = s.shell.ExecProgress(fmt.Sprintf("🌎 Planning Terraform changes in %s", component.Path), "terraform", planArgs...)
		if err != nil {
			return fmt.Errorf("error running terraform plan for %s: %w", component.Path, err)
		}

		applyArgs := []string{fmt.Sprintf("-chdir=%s", terraformArgs.ModulePath), "apply"}
		applyArgs = append(applyArgs, terraformArgs.ApplyArgs...)
		_, err = s.shell.ExecProgress(fmt.Sprintf("🌎 Applying Terraform changes in %s", component.Path), "terraform", applyArgs...)
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
func (s *WindsorStack) Down() error {
	currentDir, err := s.shims.Getwd()
	if err != nil {
		return fmt.Errorf("error getting current directory: %v", err)
	}

	defer func() {
		_ = s.shims.Chdir(currentDir)
	}()

	components := s.blueprintHandler.GetTerraformComponents()

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
		_, _ = s.shell.ExecProgress(fmt.Sprintf("🔄 Refreshing Terraform state in %s", component.Path), "terraform", refreshArgs...)

		planArgs := []string{fmt.Sprintf("-chdir=%s", terraformArgs.ModulePath), "plan"}
		planArgs = append(planArgs, terraformArgs.PlanDestroyArgs...)
		if _, err := s.shell.ExecProgress(fmt.Sprintf("🗑️  Planning terraform destroy for %s", component.Path), "terraform", planArgs...); err != nil {
			return fmt.Errorf("error running terraform plan destroy for %s: %w", component.Path, err)
		}

		destroyArgs := []string{fmt.Sprintf("-chdir=%s", terraformArgs.ModulePath), "destroy"}
		destroyArgs = append(destroyArgs, terraformArgs.DestroyArgs...)
		if _, err := s.shell.ExecProgress(fmt.Sprintf("🗑️  Destroying terraform for %s", component.Path), "terraform", destroyArgs...); err != nil {
			return fmt.Errorf("error running terraform destroy for %s: %w", component.Path, err)
		}

		if err := s.shims.Remove(filepath.Join(component.FullPath, "backend_override.tf")); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("error removing backend_override.tf from %s: %w", component.Path, err)
		}
	}

	return nil
}
