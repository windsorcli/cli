package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

var execCmd = &cobra.Command{
	Use:          "exec -- [command]",
	Short:        "Execute a shell command with environment variables",
	Long:         "Execute a shell command with environment variables set for the application.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		// New snippet: Ensure projectName is set
		configHandler := controller.ResolveConfigHandler()
		if configHandler == nil {
			return fmt.Errorf("No config handler found")
		}
		projectName := configHandler.GetString("projectName")
		if projectName == "" {
			fmt.Println("Cannot execute commands. Please run `windsor init` to set up your project first.")
			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("no command provided")
		}

		// Create environment components
		if err := controller.CreateEnvComponents(); err != nil {
			return fmt.Errorf("Error creating environment components: %w", err)
		}

		// Initialize components
		if err := controller.InitializeComponents(); err != nil {
			return fmt.Errorf("Error initializing components: %w", err)
		}

		// Unlock the SecretProvider
		secretsProvider := controller.ResolveSecretsProvider()
		if secretsProvider != nil {
			if err := secretsProvider.LoadSecrets(); err != nil {
				return fmt.Errorf("Error loading secrets: %w", err)
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
			for k, v := range vars {
				envVars[k] = v
			}
			if err := envPrinter.PostEnvHook(); err != nil {
				return fmt.Errorf("Error executing PostEnvHook: %w", err)
			}
		}

		// Set environment variables for the command
		for k, v := range envVars {
			if err := osSetenv(k, v); err != nil {
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
