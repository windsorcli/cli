package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/shell"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Output commands to set environment variables",
	Long:  "Output commands to set environment variables for the application.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Resolve all helpers from the DI container
		helperInstances, err := container.ResolveAll((*helpers.Helper)(nil))
		if err != nil {
			return fmt.Errorf("Error resolving helpers: %w", err)
		}

		// Resolve the shell from the DI container
		var sh shell.Shell
		shInstance, err := container.Resolve("shell")
		if err != nil {
			return fmt.Errorf("Error resolving shell: %w", err)
		}
		sh = shInstance.(shell.Shell)

		// Iterate through all helpers and get environment variables
		for _, instance := range helperInstances {
			helper := instance.(helpers.Helper)
			envVars, err := helper.GetEnvVars()
			if err != nil {
				return fmt.Errorf("Error getting environment variables: %w", err)
			}

			// Sort the environment variables by key
			keys := make([]string, 0, len(envVars))
			for k := range envVars {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			// Use the shell to print environment variables
			sortedEnvVars := make(map[string]string)
			for _, k := range keys {
				sortedEnvVars[k] = envVars[k]
			}
			sh.PrintEnvVars(sortedEnvVars)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(envCmd)
}
