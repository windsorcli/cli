package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/helpers"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Output commands to set environment variables",
	Long:  "Output commands to set environment variables for the application.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Collect environment variables
		envVars, err := collectEnvVars()
		if err != nil {
			return err
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
		shellInstance.PrintEnvVars(sortedEnvVars)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(envCmd)
}

// collectEnvVars collects environment variables from all helpers and returns them as a map.
func collectEnvVars() (map[string]string, error) {
	// Resolve all helpers from the DI container
	helperInstances, err := container.ResolveAll((*helpers.Helper)(nil))
	if err != nil {
		if verbose {
			return nil, fmt.Errorf("Error resolving helpers: %w", err)
		}
		return nil, nil
	}

	envVars := make(map[string]string)
	// Iterate through all helpers and get environment variables
	for _, instance := range helperInstances {
		helper := instance.(helpers.Helper)
		helperEnvVars, err := helper.GetEnvVars()
		if err != nil {
			if verbose {
				return nil, fmt.Errorf("Error getting environment variables: %w", err)
			}
			return nil, nil
		}
		for k, v := range helperEnvVars {
			if v != "" {
				envVars[k] = v
			}
		}

		// Call PostEnvExec on the helper
		if err := helper.PostEnvExec(); err != nil {
			if verbose {
				return nil, fmt.Errorf("Error executing PostEnvExec: %w", err)
			}
			return nil, nil
		}
	}

	return envVars, nil
}
