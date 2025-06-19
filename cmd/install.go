package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

var installCmd = &cobra.Command{
	Use:          "install",
	Short:        "Install the blueprint's cluster-level services",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		// Initialize with requirements
		if err := controller.InitializeWithRequirements(ctrl.Requirements{
			ConfigLoaded: true,
			Env:          true,
			Secrets:      true,
			VM:           true,
			Containers:   true,
			Services:     true,
			Network:      true,
			Blueprint:    true,
			Cluster:      true,
			Generators:   true,
			Stack:        true,
			CommandName:  cmd.Name(),
			Flags: map[string]bool{
				"verbose": verbose,
			},
		}); err != nil {
			return fmt.Errorf("Error initializing: %w", err)
		}

		// Unlock the SecretProvider
		secretsProviders := controller.ResolveAllSecretsProviders()
		if len(secretsProviders) > 0 {
			for _, secretsProvider := range secretsProviders {
				if err := secretsProvider.LoadSecrets(); err != nil {
					return fmt.Errorf("Error loading secrets: %w", err)
				}
			}
		}

		// Set the environment variables internally in the process
		if err := controller.SetEnvironmentVariables(); err != nil {
			return fmt.Errorf("Error setting environment variables: %w", err)
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

		// If wait flag is set, wait for kustomizations to be ready
		if waitFlag {
			if err := blueprintHandler.WaitForKustomizations("⏳ Waiting for kustomizations to be ready"); err != nil {
				return fmt.Errorf("failed waiting for kustomizations: %w", err)
			}
		}

		return nil
	},
}

func init() {
	installCmd.Flags().BoolVar(&waitFlag, "wait", false, "Wait for kustomization resources to be ready")
	rootCmd.AddCommand(installCmd)
}
