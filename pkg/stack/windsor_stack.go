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

// Up creates a new stack of components.
func (s *WindsorStack) Up() error {
	// Store the current directory
	currentDir, err := s.shims.Getwd()
	if err != nil {
		return fmt.Errorf("error getting current directory: %v", err)
	}

	// Ensure we change back to the original directory once the function completes
	defer func() {
		_ = s.shims.Chdir(currentDir)
	}()

	// Get the Terraform components from the blueprint
	components := s.blueprintHandler.GetTerraformComponents()

	// Iterate over the components
	for _, component := range components {
		// Ensure the directory exists
		if _, err := s.shims.Stat(component.FullPath); os.IsNotExist(err) {
			return fmt.Errorf("directory %s does not exist", component.FullPath)
		}

		// Change to the component directory
		if err := s.shims.Chdir(component.FullPath); err != nil {
			return fmt.Errorf("error changing to directory %s: %v", component.FullPath, err)
		}

		// Iterate over all envPrinters and load the environment variables
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
			// Run the post environment hook
			if err := envPrinter.PostEnvHook(); err != nil {
				return fmt.Errorf("error running post environment hook: %v", err)
			}
		}

		// Execute 'terraform init' in the dirPath
		_, err = s.shell.ExecProgress(fmt.Sprintf("🌎 Initializing Terraform in %s", component.Path), "terraform", "init", "-migrate-state", "-upgrade")
		if err != nil {
			return fmt.Errorf("error initializing Terraform in %s: %w", component.FullPath, err)
		}

		// Execute 'terraform plan' in the dirPath
		_, err = s.shell.ExecProgress(fmt.Sprintf("🌎 Planning Terraform changes in %s", component.Path), "terraform", "plan")
		if err != nil {
			return fmt.Errorf("error planning Terraform changes in %s: %w", component.FullPath, err)
		}

		// Execute 'terraform apply' in the dirPath
		_, err = s.shell.ExecProgress(fmt.Sprintf("🌎 Applying Terraform changes in %s", component.Path), "terraform", "apply")
		if err != nil {
			return fmt.Errorf("error applying Terraform changes in %s: %w", component.FullPath, err)
		}

		// Attempt to clean up 'backend_override.tf' if it exists
		backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
		if _, err := s.shims.Stat(backendOverridePath); err == nil {
			if err := s.shims.Remove(backendOverridePath); err != nil {
				return fmt.Errorf("error removing backend_override.tf in %s: %v", component.FullPath, err)
			}
		}
	}

	return nil
}
