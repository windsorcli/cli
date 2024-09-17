package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/di"
)

var rootCmd = &cobra.Command{
	Use:   "cli",
	Short: "CLI application",
	Long:  "CLI application for managing Windsor Hotel operations.",
}

var container *di.Container

// Initialize sets up the dependencies for the application
func Initialize(diContainer *di.Container) {
	container = diContainer
}

// preRunLoadConfig is the function assigned to PersistentPreRunE
func preRunLoadConfig(cmd *cobra.Command, args []string) error {
	if container.ConfigHandler == nil {
		return fmt.Errorf("configHandler is not initialized")
	}
	// Load configuration
	if err := container.ConfigHandler.LoadConfig(""); err != nil {
		return fmt.Errorf("Error loading config file: %w", err)
	}
	return nil
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}
