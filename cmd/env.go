package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

var envCmd = &cobra.Command{
	Use:          "env",
	Short:        "Output commands to set environment variables",
	Long:         "Output commands to set environment variables for the application.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		// Check if current directory is in the trusted list
		shell := controller.ResolveShell()

		// Reset the shell to ensure it's in a clean state
		if err := shell.Reset(); err != nil {
			if verbose {
				return fmt.Errorf("Error executing Reset: %w", err)
			}
			return nil
		}

		// Check if the directory is trusted and has a windsor.yaml in its ancestry
		if err := shell.CheckTrustedDirectory(); err != nil {
			if verbose {
				return fmt.Errorf("Error checking trusted directory: %w", err)
			}
			return nil
		}

		// Get the project root
		projectRoot, err := shell.GetProjectRoot()
		if err != nil || projectRoot == "" {
			if verbose {
				return fmt.Errorf("Error finding windsor.yaml in directory ancestry: %w", err)
			}
			return nil
		}

		// Create virtualization components
		if err := controller.CreateVirtualizationComponents(); err != nil {
			if verbose {
				return fmt.Errorf("Error creating virtualization components: %w", err)
			}
			return nil
		}

		// Create service components
		if err := controller.CreateServiceComponents(); err != nil {
			if verbose {
				return fmt.Errorf("Error creating service components: %w", err)
			}
			return nil
		}

		// Create environment components
		if err := controller.CreateEnvComponents(); err != nil {
			if verbose {
				return fmt.Errorf("Error creating environment components: %w", err)
			}
			return nil
		}

		// Initialize components
		if err := controller.InitializeComponents(); err != nil {
			if verbose {
				return fmt.Errorf("Error initializing components: %w", err)
			}
			return nil
		}

		// Check if --decrypt flag is set
		decrypt, _ := cmd.Flags().GetBool("decrypt")
		if decrypt {
			// Unlock the SecretProvider
			secretsProviders := controller.ResolveAllSecretsProviders()
			if len(secretsProviders) > 0 {
				for _, secretsProvider := range secretsProviders {
					if err := secretsProvider.LoadSecrets(); err != nil {
						if verbose {
							return fmt.Errorf("Error loading secrets provider: %w", err)
						}
						return nil
					}
				}
			}
		}

		// Resolve all environment printers using the controller
		envPrinters := controller.ResolveAllEnvPrinters()
		if len(envPrinters) == 0 && verbose {
			return fmt.Errorf("Error resolving environment printers: no printers returned")
		}

		// Iterate through all environment printers and run their Print and PostEnvHook functions
		for _, envPrinter := range envPrinters {
			if err := envPrinter.Print(); err != nil {
				if verbose {
					return fmt.Errorf("Error executing Print: %w", err)
				}
				continue
			}
			if err := envPrinter.PostEnvHook(); err != nil {
				if verbose {
					return fmt.Errorf("Error executing PostEnvHook: %w", err)
				}
				continue
			}
		}

		return nil
	},
}

func init() {
	envCmd.Flags().Bool("decrypt", false, "Decrypt secrets before setting environment variables")
	rootCmd.AddCommand(envCmd)
}
