package cmd

import (
	"fmt"
	"maps"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

var execCmd = &cobra.Command{
	Use:          "exec -- [command]",
	Short:        "Execute a shell command with environment variables",
	Long:         "Execute a shell command with environment variables set for the application.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("no command provided")
		}

		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		// Initialize with requirements
		if err := controller.InitializeWithRequirements(ctrl.Requirements{
			ConfigLoaded: true,
			Env:          true,
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

		// Load secrets
		secretsProviders := controller.ResolveAllSecretsProviders()
		if len(secretsProviders) > 0 {
			for _, secretsProvider := range secretsProviders {
				if err := secretsProvider.LoadSecrets(); err != nil {
					return fmt.Errorf("Error loading secrets: %w", err)
				}
			}
		}

		// Resolve all environment printers using the controller
		envPrinters := controller.ResolveAllEnvPrinters()

		// Collect environment variables from all printers
		envVars := make(map[string]string)
		for _, envPrinter := range envPrinters {
			vars, err := envPrinter.GetEnvVars()
			if err != nil {
				return fmt.Errorf("Error getting environment variables: %w", err)
			}
			maps.Copy(envVars, vars)
			if err := envPrinter.PostEnvHook(); err != nil {
				return fmt.Errorf("Error executing PostEnvHook: %w", err)
			}
		}

		// Set environment variables for the command
		for k, v := range envVars {
			if err := shims.Setenv(k, v); err != nil {
				return fmt.Errorf("Error setting environment variable %s: %w", k, err)
			}
		}

		// Resolve the shell instance using the controller
		shellInstance := controller.ResolveShell()
		if shellInstance == nil {
			return fmt.Errorf("No shell found")
		}

		// Execute the command using the resolved shell instance
		_, err := shellInstance.Exec(args[0], args[1:]...)
		if err != nil {
			return fmt.Errorf("command execution failed: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
}
