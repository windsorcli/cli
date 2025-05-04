package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

var (
	cleanFlag bool
)

var downCmd = &cobra.Command{
	Use:          "down",
	Short:        "Tear down the Windsor environment",
	Long:         "Tear down the Windsor environment by executing necessary shell commands.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		// Initialize with requirements
		if err := controller.InitializeWithRequirements(ctrl.Requirements{
			ConfigLoaded: true,
			Tools:        true,
			Trust:        true,
			Env:          true,
			VM:           true,
			Containers:   true,
			Network:      true,
			CommandName:  cmd.Name(),
			Flags: map[string]bool{
				"verbose": verbose,
			},
		}); err != nil {
			return fmt.Errorf("Error initializing: %w", err)
		}

		// Set the environment variables internally in the process
		if err := controller.SetEnvironmentVariables(); err != nil {
			return fmt.Errorf("Error setting environment variables: %w", err)
		}

		// Resolve components
		shell := controller.ResolveShell()
		configHandler := controller.ResolveConfigHandler()

		// Determine if the container runtime is enabled
		containerRuntimeEnabled := configHandler.GetBool("docker.enabled")

		// Tear down the container runtime if enabled in configuration
		if containerRuntimeEnabled {
			// Resolve container runtime
			containerRuntime := controller.ResolveContainerRuntime()
			if containerRuntime == nil {
				return fmt.Errorf("No container runtime found")
			}

			// Tear down the container runtime
			if err := containerRuntime.Down(); err != nil {
				return fmt.Errorf("Error running container runtime Down command: %w", err)
			}
		}

		// Clean up context specific artifacts if --clean flag is set
		if cleanFlag {
			if err := configHandler.Clean(); err != nil {
				return fmt.Errorf("Error cleaning up context specific artifacts: %w", err)
			}

			// Delete everything in the .volumes folder
			projectRoot, err := shell.GetProjectRoot()
			if err != nil {
				return fmt.Errorf("Error retrieving project root: %w", err)
			}
			volumesPath := filepath.Join(projectRoot, ".volumes")
			if err := shims.RemoveAll(volumesPath); err != nil {
				return fmt.Errorf("Error deleting .volumes folder: %w", err)
			}
		}

		// Print success message
		fmt.Fprintln(os.Stderr, "Windsor environment torn down successfully.")

		return nil
	},
}

func init() {
	downCmd.Flags().BoolVar(&cleanFlag, "clean", false, "Clean up context specific artifacts")
	rootCmd.AddCommand(downCmd)
}
