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

		// Initialize with requirements
		if err := controller.InitializeWithRequirements(ctrl.Requirements{
			CommandName: cmd.Name(),
		}); err != nil {
			return fmt.Errorf("Error initializing: %w", err)
		}

		shell := controller.ResolveShell()

		// Check trusted status
		isTrusted := shell.CheckTrustedDirectory() == nil

		// Check if --hook flag is set
		hook, _ := cmd.Flags().GetBool("hook")

		// Early exit if not in trusted directory, but reset first
		if !isTrusted {
			shell.Reset()
			if !hook {
				fmt.Fprintf(cmd.ErrOrStderr(), "\033[33mWarning: You are not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve.\033[0m\n")
			}
			return nil
		}

		// Check if WINDSOR_SESSION_TOKEN is set - indicates we've been in a Windsor session
		hasSessionToken := shims.Getenv("WINDSOR_SESSION_TOKEN") != ""

		// Check if a reset signal file exists for the current session
		shouldReset, err := shell.CheckResetFlags()
		if err != nil && verbose {
			return fmt.Errorf("Error checking reset signal: %w", err)
		}

		// Set shouldReset to true if session token is not present
		if !hasSessionToken {
			shouldReset = true
		}

		// Reset only when shouldReset is true (not based on trusted status)
		if shouldReset {
			shell.Reset()

			// Set NO_CACHE=true to force fresh environment variable resolution
			if err := shims.Setenv("NO_CACHE", "true"); err != nil && verbose {
				return fmt.Errorf("Error setting NO_CACHE: %w", err)
			}
		}

		// Initialize with requirements
		if err := controller.InitializeWithRequirements(ctrl.Requirements{
			Trust:        true,
			ConfigLoaded: !hook, // Don't check if config is loaded when executed by the hook
			Env:          true,
			Tools:        true,
			Secrets:      true,
			VM:           true,
			Containers:   true,
			Network:      true,
			Services:     true,
			CommandName:  cmd.Name(),
			Flags: map[string]bool{
				"verbose": verbose,
			},
		}); err != nil {
			return fmt.Errorf("Error initializing: %w", err)
		}

		envPrinters := controller.ResolveAllEnvPrinters()
		if len(envPrinters) == 0 {
			if verbose {
				return fmt.Errorf("Error resolving environment printers: no printers returned")
			}
			return nil
		}

		// Check if --decrypt flag is set
		decrypt, _ := cmd.Flags().GetBool("decrypt")

		if decrypt {
			// Unlock the SecretProvider
			secretsProviders := controller.ResolveAllSecretsProviders()

			// Check if there are any secrets providers available
			if len(secretsProviders) == 0 {
				if verbose {
					fmt.Println("Warning: No secrets providers found. If you recently changed contexts, try running the command again.")
				}
			} else {
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
	envCmd.Flags().Bool("hook", false, "Flag that indicates the command is being executed by the hook")
	rootCmd.AddCommand(envCmd)
}
