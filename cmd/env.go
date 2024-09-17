package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Output commands to set environment variables",
	Long:  "Output commands to set environment variables for the application.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if container.BaseHelper == nil {
			return fmt.Errorf("BaseHelper is not initialized")
		}

		// Print environment variables
		err := container.BaseHelper.PrintEnvVars()
		if err != nil {
			return fmt.Errorf("Error printing environment variables: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(envCmd)
}
