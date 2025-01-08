package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/config"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

// controller is the global controller
var controller ctrl.Controller

// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(controllerInstance ctrl.Controller) error {
	// Set the controller
	controller = controllerInstance

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		return err
	}

	return nil
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "windsor",
	Short: "A command line interface to assist in a context flow development environment",
	Long:  "A command line interface to assist in a context flow development environment",
}

func init() {
	// Define the --verbose flag
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}

// initializeCommonComponents initializes the controller and creates common components
func preRunEInitializeCommonComponents(cmd *cobra.Command, args []string) error {
	// Initialize the controller
	if err := controller.Initialize(); err != nil {
		return fmt.Errorf("Error initializing controller: %w", err)
	}

	// Create common components
	if err := controller.CreateCommonComponents(); err != nil {
		return fmt.Errorf("Error creating common components: %w", err)
	}

	// Resolve the config handler
	configHandler := controller.ResolveConfigHandler()
	if configHandler == nil {
		return fmt.Errorf("No config handler found")
	}
	// Initialize the config handler
	if err := configHandler.Initialize(); err != nil {
		return fmt.Errorf("Error initializing config handler: %w", err)
	}

	contextName := configHandler.GetContext()

	// Set the verbosity
	shell := controller.ResolveShell()
	if shell != nil {
		shell.SetVerbosity(verbose)
	}

	// If the context is local or starts with "local-", set the defaults to the default local config
	if contextName == "local" || len(contextName) > 6 && contextName[:6] == "local-" {
		err := configHandler.SetDefault(config.DefaultLocalConfig)
		if err != nil {
			return fmt.Errorf("error setting default local config: %w", err)
		}
	} else {
		err := configHandler.SetDefault(config.DefaultConfig)
		if err != nil {
			return fmt.Errorf("error setting default config: %w", err)
		}
	}

	// Get the cli configuration path
	cliConfigPath, err := getCliConfigPath()
	if err != nil {
		return fmt.Errorf("Error getting cli configuration path: %w", err)
	}

	// Load the configuration
	if err := configHandler.LoadConfig(cliConfigPath); err != nil {
		return fmt.Errorf("Error loading config file: %w", err)
	}
	return nil
}

// getCliConfigPath returns the path to the cli configuration file
var getCliConfigPath = func() (string, error) {
	// Determine the cliConfig path
	if cliConfigPath := os.Getenv("WINDSORCONFIG"); cliConfigPath != "" {
		return cliConfigPath, nil
	}

	homeDir, err := osUserHomeDir()
	if err != nil {
		return "", fmt.Errorf("error retrieving home directory: %w", err)
	}
	cliConfigPath := filepath.Join(homeDir, ".config", "windsor", "config.yaml")

	return cliConfigPath, nil
}
