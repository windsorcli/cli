package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/config"
)

var exitFunc = os.Exit

// ConfigHandler instance
var configHandler config.ConfigHandler

// preRunLoadConfig is the function assigned to PersistentPreRunE
func preRunLoadConfig(cmd *cobra.Command, args []string) error {
	if configHandler == nil {
		return fmt.Errorf("configHandler is not initialized")
	}
	// Load configuration
	if err := configHandler.LoadConfig(""); err != nil {
		return fmt.Errorf("Error loading config file: %w", err)
	}
	return nil
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:               "windsor",
	Short:             "A command line interface to assist in a context flow development environment",
	Long:              "A command line interface to assist in a context flow development environment",
	PersistentPreRunE: preRunLoadConfig,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitFunc(1)
	}
}

// Initialize sets the ConfigHandler for dependency injection
func Initialize(handler config.ConfigHandler) {
	configHandler = handler
}
