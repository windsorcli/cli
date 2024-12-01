package cmd

import (
	"fmt"

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

	// Set the config handler
	var err error
	configHandler, err = controller.ResolveConfigHandler()
	if err != nil {
		return fmt.Errorf("error resolving config handler: %w", err)
	}

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
