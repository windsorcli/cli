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

		// Ensure configuration is loaded
		configHandler := controller.ResolveConfigHandler()
		if configHandler == nil {
			return fmt.Errorf("No config handler found")
		}
		if !configHandler.IsLoaded() {
			return fmt.Errorf("No configuration is loaded. Is there a project to tear down?")
		}

		// Determine if specific virtualization or container runtime actions are required
		vmDriver := configHandler.GetString("vm.driver")

		// Create virtualization components if the virtualization driver is configured
		if vmDriver != "" {
			if err := controller.CreateVirtualizationComponents(); err != nil {
				return fmt.Errorf("Error creating virtualization components: %w", err)
			}
		}

		// Initialize all components
		if err := controller.InitializeComponents(); err != nil {
			return fmt.Errorf("Error initializing components: %w", err)
		}

		// Set the environment variables internally in the process
		if err := controller.SetEnvironmentVariables(); err != nil {
			return fmt.Errorf("Error setting environment variables: %w", err)
		}

		// Resolve the shell
		shell := controller.ResolveShell()

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
			if err := osRemoveAll(volumesPath); err != nil {
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
