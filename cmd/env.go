package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/helpers"
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

		// Iterate through all helpers and print environment variables
		for _, instance := range helperInstances {
			helper := instance.(helpers.Helper)
			if err := helper.PrintEnvVars(); err != nil {
				return fmt.Errorf("Error printing environment variables: %w", err)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(envCmd)
}
