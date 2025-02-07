package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

var checkCmd = &cobra.Command{
	Use:          "check",
	Short:        "Check the tool versions",
	Long:         "Check the tool versions required by the project",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		// Create project components
		if err := controller.CreateProjectComponents(); err != nil {
			return fmt.Errorf("Error creating project components: %w", err)
		}

		// Initialize components
		if err := controller.InitializeComponents(); err != nil {
			return fmt.Errorf("Error initializing components: %w", err)
		}

		// Resolve the tools manager and check the tools
		toolsManager := controller.ResolveToolsManager()
		if toolsManager == nil {
			return fmt.Errorf("No tools manager found")
		}
		if err := toolsManager.Check(); err != nil {
			return fmt.Errorf("Error checking tools: %w", err)
		}
		fmt.Println("All tools are up to date.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
}
