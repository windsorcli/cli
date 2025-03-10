package stack

import (
	"fmt"
	"os"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
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
		if _, err := osStat(component.FullPath); os.IsNotExist(err) {
			return fmt.Errorf("directory %s does not exist", component.FullPath)
		}

		// Change to the component directory
		if err := osChdir(component.FullPath); err != nil {
			return fmt.Errorf("error changing to directory %s: %v", component.FullPath, err)
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

		// Execute Terraform commands using the Windsor exec context
		if err := s.executeTerraformCommand("init", component.Path, "-migrate-state", "-force-copy", "-upgrade"); err != nil {
			return fmt.Errorf("error initializing Terraform in %s: %w", component.FullPath, err)
		}

		if err := s.executeTerraformCommand("plan", component.Path, "-input=false"); err != nil {
			return fmt.Errorf("error planning Terraform changes in %s: %w", component.FullPath, err)
		}

		if err := s.executeTerraformCommand("apply", component.Path); err != nil {
			return fmt.Errorf("error applying Terraform changes in %s: %w", component.FullPath, err)
		}
	}

	return nil
}

// executeTerraformCommand runs a Terraform command within the Windsor exec context
// This is challenging to mock, so we're not going to test it now.
func (s *WindsorStack) executeTerraformCommand(command, path string, args ...string) error {
	// Select the appropriate shell based on the execution mode
	var shellInstance shell.Shell
	if os.Getenv("WINDSOR_EXEC_MODE") == "container" {
		containerID, err := shell.GetWindsorExecContainerID()
		if err != nil || containerID == "" {
			shellInstance = s.shell
		} else {
			shellInstance = s.dockerShell
		}
	} else {
		shellInstance = s.shell
	}

	if shellInstance == nil {
		return fmt.Errorf("no shell found")
	}

	// Execute the command with a progress indicator
	message := fmt.Sprintf("🌎 Executing Terraform %s in %s", command, path)
	_, _, err := shellInstance.ExecProgress(message, "terraform", append([]string{command}, args...)...)
	return err
}
