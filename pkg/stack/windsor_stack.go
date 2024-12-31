package stack

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/di"
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
	// Get the Terraform components from the blueprint
	components := s.blueprintHandler.GetTerraformComponents()

	// Iterate over the components
	for _, component := range components {
		// Ensure the directory exists
		if _, err := osStat(component.FullPath); os.IsNotExist(err) {
			return fmt.Errorf("directory %s does not exist", component.FullPath)
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

		// Execute 'terraform init', 'terraform plan', and 'terraform apply' using the -chdir flag
		commands := [][]string{
			{"terraform", "-chdir=" + component.FullPath, "init", "-migrate-state", "-upgrade"},
			{"terraform", "-chdir=" + component.FullPath, "plan", "-lock=false"},
			{"terraform", "-chdir=" + component.FullPath, "apply"},
		}

		commandMessages := []string{
			"🌎 Initializing Terraform for %s",
			"🌎 Planning Terraform changes for %s",
			"🌎 Applying Terraform changes for %s",
		}

		for i, cmd := range commands {
			_, err := s.shell.ExecProgress(
				fmt.Sprintf(commandMessages[i], component.Path),
				cmd[0], cmd[1:]...,
			)
			if err != nil {
				return fmt.Errorf("error running Terraform command %s in %s: %v", cmd[2], component.FullPath, err)
			}
		}

		// Attempt to clean up 'backend_override.tf' if it exists
		backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
		if _, err := osStat(backendOverridePath); err == nil {
			if err := osRemove(backendOverridePath); err != nil {
				return fmt.Errorf("error removing backend_override.tf in %s: %v", component.FullPath, err)
			}
		}
	}

	return nil
}
