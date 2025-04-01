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

		// Ensure configuration is loaded
		configHandler := controller.ResolveConfigHandler()
		if !configHandler.IsLoaded() {
			return fmt.Errorf("Cannot install blueprint. Please run `windsor init` to set up your project first.")
		}

		// Determine if specific virtualization or container runtime actions are required
		vmDriver := configHandler.GetString("vm.driver")

		// Unlock the SecretProvider
		secretsProviders := controller.ResolveAllSecretsProviders()
		if len(secretsProviders) > 0 {
			for _, secretsProvider := range secretsProviders {
				if err := secretsProvider.LoadSecrets(); err != nil {
					return fmt.Errorf("Error loading secrets: %w", err)
				}
			}
		}

		// Create project components
		if err := controller.CreateProjectComponents(); err != nil {
			return fmt.Errorf("Error creating project components: %w", err)
		}

		// Determine if specific virtualization or container runtime actions are required
		if vmDriver != "" {
			// Create service components
			if err := controller.CreateServiceComponents(); err != nil {
				return fmt.Errorf("Error creating service components: %w", err)
			}

			// Create virtualization components
			if err := controller.CreateVirtualizationComponents(); err != nil {
				return fmt.Errorf("Error creating virtualization components: %w", err)
			}
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
