package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
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

		// Resolve all environment printers using the controller
		envPrinters, err := controller.ResolveAllEnvPrinters()
		if err != nil {
			return fmt.Errorf("Error resolving environments: %w", err)
		}

		// Collect and initialize environment variables from all environments
		envVars := make(map[string]string)
		for _, envPrinter := range envPrinters {
			if err := envPrinter.Initialize(); err != nil {
				return fmt.Errorf("Error initializing environment: %w", err)
			}
			vars, err := envPrinter.GetEnvVars()
			if err != nil {
				return fmt.Errorf("Error getting environment variables: %w", err)
			}
			for k, v := range vars {
				envVars[k] = v
			}
		}

		// Set environment variables for the command
		for k, v := range envVars {
			if err := osSetenv(k, v); err != nil {
				return fmt.Errorf("Error setting environment variable %s: %w", k, err)
			}
		}

		// Resolve the shell instance using the controller
		shellInstance, err := controller.ResolveShell()
		if err != nil {
			return fmt.Errorf("Error resolving shell instance: %w", err)
		}

		// Initialize the shell instance
		if err := shellInstance.Initialize(); err != nil {
			return fmt.Errorf("Error initializing shell: %w", err)
		}

		// Execute the command using the resolved shell instance
		output, err := shellInstance.Exec(false, "", args[0], args[1:]...)
		if err != nil {
			return fmt.Errorf("command execution failed: %w", err)
		}

		// Print the command output
		fmt.Println(output)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
}
