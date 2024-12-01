package cmd

import (
	"github.com/spf13/cobra"
	ctrl "github.com/windsor-hotel/cli/internal/controller"
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
