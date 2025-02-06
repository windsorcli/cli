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

	contextName := configHandler.GetContext()

	// Set the verbosity
	shell := controller.ResolveShell()
	if shell != nil {
		shell.SetVerbosity(verbose)
	}

	// If the context is local or starts with "local-", set the defaults to the default local config
	if contextName == "local" || len(contextName) > 6 && contextName[:6] == "local-" {
		err := configHandler.SetDefault(config.DefaultConfig_Containerized)
		if err != nil {
			return fmt.Errorf("error setting default local config: %w", err)
		}
	} else {
		err := configHandler.SetDefault(config.DefaultConfig)
		if err != nil {
			return fmt.Errorf("error setting default config: %w", err)
		}
	}

	// Determine the cliConfig path
	var cliConfigPath string
	if cliConfigPath = os.Getenv("WINDSORCONFIG"); cliConfigPath == "" {
		projectRoot, err := shell.GetProjectRoot()
		if err != nil {
			return fmt.Errorf("error retrieving project root: %w", err)
		}
		yamlPath := filepath.Join(projectRoot, "windsor.yaml")
		ymlPath := filepath.Join(projectRoot, "windsor.yml")

		// Check if windsor.yaml exists
		if _, err := osStat(yamlPath); os.IsNotExist(err) {
			// Check if windsor.yml exists only if windsor.yaml does not exist
			if _, err := osStat(ymlPath); err == nil {
				// Not unit tested for now
				cliConfigPath = ymlPath
			}
		} else {
			cliConfigPath = yamlPath
		}
	}

	// Load the configuration if a config path was determined
	if cliConfigPath != "" {
		if err := configHandler.LoadConfig(cliConfigPath); err != nil {
			return fmt.Errorf("Error loading config file: %w", err)
		}
	}
	return nil
}
