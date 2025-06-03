package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

var (
	cleanFlag   bool
	skipK8sFlag bool
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
			Trust:        true,
			Env:          true,
			VM:           true,
			Containers:   true,
			Network:      true,
			Blueprint:    true,
			Kubernetes:   true,
			Stack:        true,
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

		// Run blueprint cleanup before stack down
		blueprintHandler := controller.ResolveBlueprintHandler()
		if blueprintHandler == nil {
			return fmt.Errorf("No blueprint handler found")
		}
		if err := blueprintHandler.LoadConfig(); err != nil {
			return fmt.Errorf("Error loading blueprint config: %w", err)
		}
		if !skipK8sFlag {
			kubernetesManager := controller.ResolveKubernetesManager()
			if kubernetesManager == nil {
				return fmt.Errorf("No kubernetes manager found")
			}
			if err := kubernetesManager.InitializeClient(); err != nil {
				return fmt.Errorf("Error initializing kubernetes client: %w", err)
			}

			if err := blueprintHandler.Down(); err != nil {
				return fmt.Errorf("Error running blueprint down: %w", err)
			}
		} else {
			fmt.Fprintln(os.Stderr, "Skipping Kubernetes cleanup (--skip-k8s set)")
		}

		// Tear down the stack components
		stack := controller.ResolveStack()
		if stack == nil {
			return fmt.Errorf("No stack found")
		}
		if err := stack.Down(); err != nil {
			return fmt.Errorf("Error running stack Down command: %w", err)
		}

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
	downCmd.Flags().BoolVar(&skipK8sFlag, "skip-k8s", false, "Skip Kubernetes cleanup (blueprint cleanup)")
	rootCmd.AddCommand(downCmd)
}
