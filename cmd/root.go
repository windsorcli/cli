package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

var (
	verbose  bool // Enables detailed logging output for debugging purposes.
	silent   bool // Suppresses error messages and other output, useful for scripting.
	exitCode int  // Global exit code variable
)

// Define a custom type for context keys
type contextKey string

// Define a custom type for context keys
const controllerKey = contextKey("controller")

// Execute runs the root command with a controller, handling errors and exit codes.
func Execute(controllerInstance ctrl.Controller) error {
	ctx := context.WithValue(context.Background(), controllerKey, controllerInstance)

	err := rootCmd.ExecuteContext(ctx)

	if exitCode != 0 {
		if !silent {
			fmt.Fprintln(os.Stderr, err)
		}
		osExit(exitCode)
	}

	if err != nil {
		return err
	}

	return nil
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

	// Resolve the config handler
	configHandler := controller.ResolveConfigHandler()
	if configHandler == nil {
		return fmt.Errorf("No config handler found")
	}

	// Set the verbosity
	shell := controller.ResolveShell()
	if shell != nil {
		shell.SetVerbosity(verbose)
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
	// Define the --silent flag
	rootCmd.PersistentFlags().BoolVarP(&silent, "silent", "s", false, "Enable silent mode, suppressing output")
}
