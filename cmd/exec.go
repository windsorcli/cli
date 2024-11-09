package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/env"
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

		// Collect environment variables from all environments
		envVars := make(map[string]string)
		for _, instance := range envInstances {
			envPrinter := instance.(env.EnvPrinter)
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

		// Execute the command using the existing shell instance
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
