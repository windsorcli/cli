package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
)

var (
	exitFunc      = os.Exit
	osUserHomeDir = os.UserHomeDir
	container     di.ContainerInterface
)

// ConfigHandler instance
var configHandler config.ConfigHandler

// preRunLoadConfig is the function assigned to PersistentPreRunE
func preRunLoadConfig(cmd *cobra.Command, args []string) error {
	if configHandler == nil {
		return fmt.Errorf("configHandler is not initialized")
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

// Initialize sets dependency injection container
func Initialize(cont di.ContainerInterface) {
	container = cont

	instance, err := container.Resolve("cliConfigHandler")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error resolving configHandler:", err)
		exitFunc(1)
	}
	if instance == nil {
		fmt.Fprintln(os.Stderr, "Error: resolved instance is nil")
		exitFunc(1)
	}
	var ok bool
	configHandler, ok = instance.(config.ConfigHandler)
	if !ok {
		fmt.Fprintln(os.Stderr, "Error: resolved instance is not of type config.ConfigHandler")
		exitFunc(1)
	}
}
