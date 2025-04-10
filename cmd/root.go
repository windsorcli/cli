package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

// Define a custom type for context keys
type contextKey string

const controllerKey = contextKey("controller")

// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(controllerInstance ctrl.Controller) error {
	// Create a context with the controller
	ctx := context.WithValue(context.Background(), controllerKey, controllerInstance)

	// Execute the root command with the context
	return rootCmd.ExecuteContext(ctx)
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:               "windsor",
	Short:             "A command line interface to assist your cloud native development workflow",
	Long:              "A command line interface to assist your cloud native development workflow",
	PersistentPreRunE: preRunEInitializeCommonComponents,
}

// preRunEInitializeCommonComponents initializes the controller and creates common components
func preRunEInitializeCommonComponents(cmd *cobra.Command, args []string) error {
	cmd.SetContext(cmd.Root().Context())

	// Retrieve the controller from the context
	controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

	// Initialize the controller
	if err := controller.Initialize(); err != nil {
		return fmt.Errorf("Error initializing controller: %w", err)
	}

	// Create common components
	if err := controller.CreateCommonComponents(); err != nil {
		return fmt.Errorf("Error creating common components: %w", err)
	}

	// Resolve the shell and config handler
	shell := controller.ResolveShell()
	if shell != nil {
		shell.SetVerbosity(verbose)
	}

	// Check if we're in a trusted directory, but only for needed commands
	cmdName := cmd.Name()
	if cmdName != "hook" && cmdName != "init" && (cmdName != "env" || !cmd.Flags().Changed("decrypt")) {
		if err := shell.CheckTrustedDirectory(); err != nil {
			fmt.Fprintf(os.Stderr, "\033[33mWarning: You are not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve.\033[0m\n")
		}
	}

	// Resolve the config handler
	configHandler := controller.ResolveConfigHandler()
	if configHandler == nil {
		return fmt.Errorf("No config handler found")
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

	// Load the current context
	configHandler.GetContext()

	// Load the configuration if a config path was determined
	if cliConfigPath != "" {
		if err := configHandler.LoadConfig(cliConfigPath); err != nil {
			return fmt.Errorf("Error loading config file: %w", err)
		}
	}

	// Create the secrets provider
	if err := controller.CreateSecretsProviders(); err != nil {
		return fmt.Errorf("Error creating secrets provider: %w", err)
	}

	return nil
}

func init() {
	// Define the --verbose flag
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}
