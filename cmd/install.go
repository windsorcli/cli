package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the blueprint's cluster-level services",
	RunE: func(cmd *cobra.Command, args []string) error {
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		// // New snippet: Ensure projectName is set
		// configHandler := controller.ResolveConfigHandler()
		// projectName := configHandler.GetString("projectName")
		// if projectName == "" {
		// 	fmt.Println("Cannot install blueprint. Please run `windsor init` to set up your project first.")
		// 	return nil
		// }

		// Unlock the SecretProvider
		secretsProvider := controller.ResolveSecretsProvider()
		if secretsProvider != nil {
			if err := secretsProvider.LoadSecrets(); err != nil {
				return fmt.Errorf("Error loading secrets: %w", err)
			}
		}

		// Create project components
		if err := controller.CreateProjectComponents(); err != nil {
			return fmt.Errorf("Error creating project components: %w", err)
		}

		// Create service components
		if err := controller.CreateServiceComponents(); err != nil {
			return fmt.Errorf("Error creating service components: %w", err)
		}

		// Create virtualization components
		if err := controller.CreateVirtualizationComponents(); err != nil {
			return fmt.Errorf("Error creating virtualization components: %w", err)
		}

		// Initialize all components
		if err := controller.InitializeComponents(); err != nil {
			return fmt.Errorf("Error initializing components: %w", err)
		}

		// Resolve the blueprint handler
		blueprintHandler := controller.ResolveBlueprintHandler()
		if blueprintHandler == nil {
			return fmt.Errorf("No blueprint handler found")
		}

		// Install the blueprint
		if err := blueprintHandler.Install(); err != nil {
			return fmt.Errorf("Error installing blueprint: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
}
