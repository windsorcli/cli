package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/shell"
)

var execCmd = &cobra.Command{
	Use:          "exec -- [command]",
	Short:        "Execute a shell command with environment variables",
	Long:         "Execute a shell command with environment variables set for the application.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		// Ensure configuration is loaded
		configHandler := controller.ResolveConfigHandler()
		if !configHandler.IsLoaded() {
			fmt.Println("Cannot execute commands. Please run `windsor init` to set up your project first.")
			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("no command provided")
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
			return fmt.Errorf("Error creating environment components: %w", err)
		}

		// Initialize components
		if err := controller.InitializeComponents(); err != nil {
			return fmt.Errorf("Error initializing components: %w", err)
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

		// Determine which shell to use based on WINDSOR_EXEC_MODE
		var shellInstance shell.Shell
		if os.Getenv("WINDSOR_EXEC_MODE") == "container" {
			shellInstance = controller.ResolveShell("dockerShell")
		} else {
			shellInstance = controller.ResolveShell()
		}

		if shellInstance == nil {
			return fmt.Errorf("No shell found")
		}

		// Execute the command using the resolved shell instance
		_, exitCode, err := shellInstance.Exec(args[0], args[1:]...)
		if err != nil {
			return err
		}

		// Set the shell's exit code
		osExit(exitCode)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
}
