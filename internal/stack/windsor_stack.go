package stack

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/internal/di"
)

// WindsorStack is a struct that implements the Stack interface.
type WindsorStack struct {
	BaseStack
}

// NewWindsorStack creates a new WindsorStack.
func NewWindsorStack(injector di.Injector) *WindsorStack {
	return &WindsorStack{
		BaseStack: BaseStack{
			injector: injector,
		},
	}
}

// Up creates a new stack of components.
func (s *WindsorStack) Up() error {
	// Store the current directory
	currentDir, err := osGetwd()
	if err != nil {
		return fmt.Errorf("error getting current directory: %v", err)
	}

	// Ensure we change back to the original directory once the function completes
	defer func() {
		_ = osChdir(currentDir)
	}()

	// Get the Terraform components from the blueprint
	components := s.blueprintHandler.GetTerraformComponents()

	// Iterate over the components
	for _, component := range components {
		// Ensure the directory exists
		if _, err := osStat(component.Path); os.IsNotExist(err) {
			return fmt.Errorf("directory %s does not exist", component.Path)
		}

		// Change to the component directory
		if err := osChdir(component.Path); err != nil {
			return fmt.Errorf("error changing to directory %s: %v", component.Path, err)
		}

		// Iterate over all envPrinters and load the environment variables
		for _, envPrinter := range s.envPrinters {
			envVars, err := envPrinter.GetEnvVars()
			if err != nil {
				return fmt.Errorf("error getting environment variables: %v", err)
			}
			for key, value := range envVars {
				if err := osSetenv(key, value); err != nil {
					return fmt.Errorf("error setting environment variable %s: %v", key, err)
				}
			}
			// Run the post environment hook
			if err := envPrinter.PostEnvHook(); err != nil {
				return fmt.Errorf("error running post environment hook: %v", err)
			}
		}

		// Execute 'terraform init' in the dirPath
		_, err = s.shell.Exec("", "terraform", "init", "-migrate-state", "-upgrade")
		if err != nil {
			return fmt.Errorf("error running 'terraform init' in %s: %v", component.Path, err)
		}

		// Execute 'terraform plan' in the dirPath
		_, err = s.shell.Exec("", "terraform", "plan", "-lock=false")
		if err != nil {
			return fmt.Errorf("error running 'terraform plan' in %s: %v", component.Path, err)
		}

		// Execute 'terraform apply' in the dirPath
		_, err = s.shell.Exec("", "terraform", "apply")
		if err != nil {
			return fmt.Errorf("error running 'terraform apply' in %s: %v", component.Path, err)
		}

		// Attempt to clean up 'backend_override.tf' if it exists
		backendOverridePath := filepath.Join(component.Path, "backend_override.tf")
		if _, err := osStat(backendOverridePath); err == nil {
			if err := osRemove(backendOverridePath); err != nil {
				return fmt.Errorf("error removing backend_override.tf in %s: %v", component.Path, err)
			}
		}
	}

	return nil
}
