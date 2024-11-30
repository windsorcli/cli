package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/config"
	ctrl "github.com/windsor-hotel/cli/internal/controller"
)

// controller is the global controller
var controller ctrl.Controller

// configHandler is the global config handler
var configHandler config.ConfigHandler

// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(controllerInstance ctrl.Controller) error {
	// Set the controller
	controller = controllerInstance

	configHandler, err := controller.ResolveConfigHandler()
	if err != nil {
		return fmt.Errorf("error resolving configHandler: %w", err)
	}

	// Load CLI configuration
	cliConfigPath, err := getCLIConfigPath()
	if err != nil {
		return fmt.Errorf("error getting CLI config path: %w", err)
	}

	if err := configHandler.LoadConfig(cliConfigPath); err != nil {
		return fmt.Errorf("error loading CLI config: %w", err)
	}

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

// getCLIConfigPath returns the path to the CLI configuration file
var getCLIConfigPath = func() (string, error) {
	cliConfigPath := os.Getenv("WINDSORCONFIG")
	if cliConfigPath == "" {
		home, err := osUserHomeDir()
		if err != nil {
			return "", fmt.Errorf("error retrieving user home directory: %w", err)
		}
		cliConfigPath = filepath.Join(home, ".config", "windsor", "config.yaml")
	}
	return cliConfigPath, nil
}
