package cmd

import (
	"fmt"
	"os"

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
		shell := controller.ResolveShell()

		// Check trusted status
		isTrusted := shell.CheckTrustedDirectory() == nil

		// Check if WINDSOR_SESSION_TOKEN is set - indicates we've been in a Windsor session
		hasSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN") != ""

		// Early exit condition: Not trusted AND no session token
		if !isTrusted && !hasSessionToken {
			return nil
		}

		// Resolve config handler to check vm.driver
		configHandler := controller.ResolveConfigHandler()
		vmDriver := configHandler.GetString("vm.driver")

		// Create virtualization and service components only if vm.driver is configured
		if vmDriver != "" {
			if err := controller.CreateVirtualizationComponents(); err != nil {
				if verbose {
					return fmt.Errorf("Error creating virtualization components: %w", err)
				}
				return nil
			}

			if err := controller.CreateServiceComponents(); err != nil {
				if verbose {
					return fmt.Errorf("Error creating service components: %w", err)
				}
				return nil
			}
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

		// Resolve environment printers
		envPrinters := controller.ResolveAllEnvPrinters()
		if len(envPrinters) == 0 {
			if verbose {
				return fmt.Errorf("Error resolving environment printers: no printers returned")
			}
			return nil
		}

		// Handle untrusted case with session token - reset and exit
		if !isTrusted && hasSessionToken {
			envPrinters[0].Reset()
			return nil
		}

		// Check if --decrypt flag is set
		decrypt, _ := cmd.Flags().GetBool("decrypt")
		if decrypt {
			// Unlock the SecretProvider
			secretsProviders := controller.ResolveAllSecretsProviders()
			for _, secretsProvider := range secretsProviders {
				if err := secretsProvider.LoadSecrets(); err != nil {
					if verbose {
						return fmt.Errorf("Error loading secrets provider: %w", err)
					}
					return nil
				}
			}
		}

		// Track first error from printers
		var firstError error

		// Iterate through all environment printers and run their Print and PostEnvHook functions
		for _, envPrinter := range envPrinters {
			if err := envPrinter.Print(); err != nil && firstError == nil {
				firstError = fmt.Errorf("Error executing Print: %w", err)
			}

			if err := envPrinter.PostEnvHook(); err != nil && firstError == nil {
				firstError = fmt.Errorf("Error executing PostEnvHook: %w", err)
			}
		}

		if verbose {
			return firstError
		}
		return nil
	},
}

func init() {
	envCmd.Flags().Bool("decrypt", false, "Decrypt secrets before setting environment variables")
	rootCmd.AddCommand(envCmd)
}
