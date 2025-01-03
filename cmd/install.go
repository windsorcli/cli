package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the blueprint's cluster-level services",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Create project components
		if err := controller.CreateProjectComponents(); err != nil {
			return fmt.Errorf("Error creating project components: %w", err)
		}

		// Resolve the blueprint handler
		blueprintHandler := controller.ResolveBlueprintHandler()
		if blueprintHandler == nil {
			return fmt.Errorf("No blueprint handler found")
		}

		// Install the blueprint
		if err := blueprintHandler.Install(); err != nil {
			return fmt.Errorf("Error installing blueprint: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
}
