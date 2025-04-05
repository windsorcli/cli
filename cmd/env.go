package cmd

import (
	"fmt"
	"os"
	"strings"

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
		if err := shell.CheckTrustedDirectory(); err != nil {
			if verbose {
				return fmt.Errorf("Error checking trusted directory: %w", err)
			}
			return nil
		}

		// Get the initial session token
		initialSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN")

		// Save initial managed env/alias values
		initialManagedEnv := os.Getenv("WINDSOR_MANAGED_ENV")
		initialManagedAlias := os.Getenv("WINDSOR_MANAGED_ALIAS")

		// Get managed env vars and aliases once
		var envVars, aliases []string
		if initialManagedEnv != "" {
			envVars = strings.Split(initialManagedEnv, ":")
		}
		if initialManagedAlias != "" {
			aliases = strings.Split(initialManagedAlias, ":")
		}

		// Resolve all environment printers early to allow for clearing in non-project folders
		envPrinters := controller.ResolveAllEnvPrinters()

		// Check if we got any environment printers
		if len(envPrinters) == 0 && verbose {
			return fmt.Errorf("Error resolving environment printers: no printers returned")
		}

		// Check if we're in a Windsor project folder
		projectRoot, err := shell.GetProjectRoot()
		if err != nil || projectRoot == "" {
			// Not in a Windsor project folder, clear environment variables and exit
			if len(envPrinters) > 0 {
				if err := envPrinters[0].Clear(envVars, aliases); err != nil && verbose {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: not in a Windsor project, clearing environment: %v\n", err)
				}
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

		// Clear environment variables if we're in a new session (no session token)
		if initialSessionToken == "" && len(envPrinters) > 0 {
			if err := envPrinters[0].Clear(envVars, aliases); err != nil && verbose {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to clear previous environment variables: %v\n", err)
			}
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

		// Iterate through all environment printers and run their PrintAlias function
		for _, envPrinter := range envPrinters {
			if err := envPrinter.PrintAlias(); err != nil {
				if verbose {
					return fmt.Errorf("Error executing PrintAlias: %w", err)
				}
				continue
			}
		}

		// Check if session token has changed after running environment printers
		currentSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		if initialSessionToken != "" && currentSessionToken != initialSessionToken && len(envPrinters) > 0 {
			// Session token has changed, restore the initial managed env/alias values
			if initialManagedEnv != "" {
				os.Setenv("WINDSOR_MANAGED_ENV", initialManagedEnv)
			}
			if initialManagedAlias != "" {
				os.Setenv("WINDSOR_MANAGED_ALIAS", initialManagedAlias)
			}

			// Clear environment variables with original values
			if err := envPrinters[0].Clear(envVars, aliases); err != nil && verbose {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to clear previous environment variables: %v\n", err)
			}
		}

		return nil
	},
}

func init() {
	envCmd.Flags().Bool("decrypt", false, "Decrypt secrets before setting environment variables")
	rootCmd.AddCommand(envCmd)
}
