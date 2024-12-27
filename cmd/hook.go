package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var hookCmd = &cobra.Command{
	Use:          "hook",
	Short:        "Prints out windsor setup command for each platform.",
	Long:         "Prints out windsor setup command for each platform.",
	SilenceUsage: true,
	PreRunE:      preRunEInitializeCommonComponents,
	RunE: func(cmd *cobra.Command, args []string) error {

		// Initialize components
		if err := controller.InitializeComponents(); err != nil {
			if verbose {
				return fmt.Errorf("Error initializing components: %w", err)
			}
			return nil
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
