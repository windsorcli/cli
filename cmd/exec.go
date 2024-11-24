package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/env"
	"github.com/windsor-hotel/cli/internal/shell"
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

		// Resolve all environments from the DI injector
		envInstances, err := injector.ResolveAll((*env.EnvPrinter)(nil))
		if err != nil {
			return fmt.Errorf("Error resolving environments: %w", err)
		}

		// Collect and initialize environment variables from all environments
		envVars := make(map[string]string)
		for _, instance := range envInstances {
			envPrinter := instance.(env.EnvPrinter)
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

		// Resolve the shell instance
		instance, err := injector.Resolve("shell")
		if err != nil {
			return fmt.Errorf("Error resolving shell instance: %w", err)
		}
		shellInstance, ok := instance.(shell.Shell)
		if !ok {
			return fmt.Errorf("Resolved instance is not of type shell.Shell")
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
