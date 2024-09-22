package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [context]",
	Short: "Initialize the application",
	Long:  "Initialize the application by setting up necessary configurations and environment",
	Args:  cobra.ExactArgs(1), // Ensure exactly one argument is provided
	RunE: func(cmd *cobra.Command, args []string) error {
		contextName := args[0]

		// Set the context value
		if err := cliConfigHandler.SetConfigValue("context", contextName); err != nil {
			return fmt.Errorf("Error setting config value: %w", err)
		}
		// Save the cli configuration
		if err := cliConfigHandler.SaveConfig(""); err != nil {
			return fmt.Errorf("Error saving config file: %w", err)
		}
		// Save the project configuration
		if err := projectConfigHandler.SaveConfig(""); err != nil {
			return fmt.Errorf("Error saving project config file: %w", err)
		}
		fmt.Println("Initialization successful")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
