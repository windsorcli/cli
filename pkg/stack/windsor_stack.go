package stack

// The WindsorStack is a specialized implementation of the Stack interface for Terraform-based infrastructure.
// It provides a concrete implementation for managing Terraform components through the Windsor CLI,
// handling directory management, environment configuration, and Terraform operations.
// The WindsorStack orchestrates Terraform initialization, planning, and application,
// while managing environment variables and backend configurations.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Types
// =============================================================================

// WindsorStack is a struct that implements the Stack interface.
type WindsorStack struct {
	BaseStack
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

// Up creates a new stack of components by initializing and applying Terraform configurations.
// It processes components in order, setting up environment variables, running Terraform init,
// plan, and apply operations, and cleaning up backend override files.
// The method ensures proper directory management and environment setup for each component.
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

		if err := s.shims.Chdir(component.FullPath); err != nil {
			return fmt.Errorf("error changing to directory %s: %v", component.FullPath, err)
		}

		for _, envPrinter := range s.envPrinters {
			envVars, err := envPrinter.GetEnvVars()
			if err != nil {
				return fmt.Errorf("error getting environment variables: %v", err)
			}
			for key, value := range envVars {
				if err := s.shims.Setenv(key, value); err != nil {
					return fmt.Errorf("error setting environment variable %s: %v", key, err)
				}
			}
			if err := envPrinter.PostEnvHook(); err != nil {
				return fmt.Errorf("error running post environment hook: %v", err)
			}
		}

		_, err = s.shell.ExecProgress(fmt.Sprintf("🌎 Initializing Terraform in %s", component.Path), "terraform", "init", "-migrate-state", "-upgrade", "-force-copy")
		if err != nil {
			return fmt.Errorf("error initializing Terraform in %s: %w", component.FullPath, err)
		}

		_, err = s.shell.ExecProgress(fmt.Sprintf("🌎 Planning Terraform changes in %s", component.Path), "terraform", "plan")
		if err != nil {
			return fmt.Errorf("error planning Terraform changes in %s: %w", component.FullPath, err)
		}

		// Build terraform apply command with optional parallelism flag
		applyArgs := []string{"apply"}
		if component.Parallelism != nil {
			applyArgs = append(applyArgs, fmt.Sprintf("-parallelism=%d", *component.Parallelism))
		}
		_, err = s.shell.ExecProgress(fmt.Sprintf("🌎 Applying Terraform changes in %s", component.Path), "terraform", applyArgs...)
		if err != nil {
			return fmt.Errorf("error applying Terraform changes in %s: %w", component.FullPath, err)
		}

		backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
		if _, err := s.shims.Stat(backendOverridePath); err == nil {
			if err := s.shims.Remove(backendOverridePath); err != nil {
				return fmt.Errorf("error removing backend_override.tf in %s: %v", component.FullPath, err)
			}
		}
	}

	return nil
}

// Down destroys a stack of components by executing Terraform destroy operations in reverse order.
// It processes components in reverse order, skipping any marked with destroy: false.
// For each component, it sets up environment variables, runs Terraform init, plan -destroy,
// and destroy operations, and cleans up backend override files.
// The method ensures proper directory management and environment setup for each component.
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
			return fmt.Errorf("directory %s does not exist", component.FullPath)
		}

		if err := s.shims.Chdir(component.FullPath); err != nil {
			return fmt.Errorf("error changing to directory %s: %v", component.FullPath, err)
		}

		for _, envPrinter := range s.envPrinters {
			envVars, err := envPrinter.GetEnvVars()
			if err != nil {
				return fmt.Errorf("error getting environment variables: %v", err)
			}
			for key, value := range envVars {
				if err := s.shims.Setenv(key, value); err != nil {
					return fmt.Errorf("error setting environment variable %s: %v", key, err)
				}
			}
			if err := envPrinter.PostEnvHook(); err != nil {
				return fmt.Errorf("error running post environment hook: %v", err)
			}
		}

		_, err = s.shell.ExecProgress(fmt.Sprintf("🗑️  Initializing Terraform in %s", component.Path), "terraform", "init", "-migrate-state", "-upgrade", "-force-copy")
		if err != nil {
			return fmt.Errorf("error initializing Terraform in %s: %w", component.FullPath, err)
		}

		_, err = s.shell.ExecProgress(fmt.Sprintf("🗑️  Planning Terraform destruction in %s", component.Path), "terraform", "plan", "-destroy")
		if err != nil {
			return fmt.Errorf("error planning Terraform destruction in %s: %w", component.FullPath, err)
		}

		// Build terraform destroy command with optional parallelism flag
		destroyArgs := []string{"destroy", "-auto-approve"}
		if component.Parallelism != nil {
			destroyArgs = append(destroyArgs, fmt.Sprintf("-parallelism=%d", *component.Parallelism))
		}
		_, err = s.shell.ExecProgress(fmt.Sprintf("🗑️  Destroying Terraform resources in %s", component.Path), "terraform", destroyArgs...)
		if err != nil {
			return fmt.Errorf("error destroying Terraform resources in %s: %w", component.FullPath, err)
		}

		backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
		if _, err := s.shims.Stat(backendOverridePath); err == nil {
			if err := s.shims.Remove(backendOverridePath); err != nil {
				return fmt.Errorf("error removing backend_override.tf in %s: %v", component.FullPath, err)
			}
		}
	}

	return nil
}
