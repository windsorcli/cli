package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
)

// Injector is the global injector
var injector di.Injector

// configHandler is the global config handler
var configHandler config.ConfigHandler

// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(inj di.Injector) error {
	// Set the injector
	injector = inj

	configHandlerInstance, err := injector.Resolve("configHandler")
	if err != nil {
		return fmt.Errorf("error resolving configHandler: %w", err)
	}

	var ok bool
	configHandler, ok = configHandlerInstance.(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("resolved instance is not of type config.ConfigHandler")
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
