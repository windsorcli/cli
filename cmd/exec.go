package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/helpers"
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

		// Resolve all helpers from the DI container
		helperInstances, err := container.ResolveAll((*helpers.Helper)(nil))
		if err != nil {
			return fmt.Errorf("Error resolving helpers: %w", err)
		}

		// Collect environment variables from all helpers
		envVars := make(map[string]string)
		for _, instance := range helperInstances {
			helper := instance.(helpers.Helper)
			helperEnvVars, err := helper.GetEnvVars()
			if err != nil {
				return fmt.Errorf("Error getting environment variables: %w", err)
			}
			for k, v := range helperEnvVars {
				if v != "" {
					envVars[k] = v
				}
			}
		}

		// Set environment variables for the command
		for k, v := range envVars {
			os.Setenv(k, v)
		}

		// Execute the command using the existing shell instance
		output, err := shellInstance.Exec(args[0], args[1:]...)
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
