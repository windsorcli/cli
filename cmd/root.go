package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/shell"
)

var (
	exitFunc      = os.Exit
	osUserHomeDir = os.UserHomeDir
	osStat        = os.Stat
	getwd         = os.Getwd
	container     di.ContainerInterface
	verbose       bool
	osSetenv      = os.Setenv
)

// ConfigHandler instances
var cliConfigHandler config.ConfigHandler

// shell instance
var shellInstance shell.Shell

// terraformHelper instance
var terraformHelper helpers.Helper

// awsHelper instance
var awsHelper helpers.Helper

// colimaHelper instance
var colimaHelper helpers.Helper

// dockerHelper instance
var dockerHelper helpers.Helper

// context instance
var contextInstance *context.Context

// getCLIConfigPath returns the path to the CLI configuration file
func getCLIConfigPath() string {
	cliConfigPath := os.Getenv("WINDSORCONFIG")
	if cliConfigPath == "" {
		home, err := osUserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error finding home directory, %s\n", err)
			exitFunc(1)
		}
		cliConfigPath = filepath.Join(home, ".config", "windsor", "config.yaml")
	}
	return cliConfigPath
}

// getProjectConfigPath returns the path to the project configuration file
func getProjectConfigPath() string {
	var projectConfigPath string

	// Try to get the project root first
	projectRoot, err := shellInstance.GetProjectRoot()
	if err != nil || projectRoot == "" {
		// If project root is not found, use the current working directory
		projectRoot, err = getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error getting current working directory, %s\n", err)
			exitFunc(1)
		}
	}

	// Check for windsor.yaml or windsor.yml in the project root
	windsorYaml := filepath.Join(projectRoot, "windsor.yaml")
	windsorYml := filepath.Join(projectRoot, "windsor.yml")

	if _, err := osStat(windsorYaml); err == nil {
		projectConfigPath = windsorYaml
	} else if _, err := osStat(windsorYml); err == nil {
		projectConfigPath = windsorYml
	}
	return projectConfigPath
}

// preRunLoadConfig is the function assigned to PersistentPreRunE
func preRunLoadConfig(cmd *cobra.Command, args []string) error {
	// Check if cliConfigHandler is initialized
	if cliConfigHandler == nil {
		return fmt.Errorf("cliConfigHandler is not initialized")
	}

	// Load CLI configuration
	cliConfigPath := getCLIConfigPath()
	if err := cliConfigHandler.LoadConfig(cliConfigPath); err != nil {
		return fmt.Errorf("error loading CLI config: %w", err)
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

func init() {
	// Define the --verbose flag
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
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

	resolveAndAssign := func(key string, target interface{}) {
		instance, err := container.Resolve(key)
		if err != nil || instance == nil {
			fmt.Fprintf(os.Stderr, "Error resolving %s: %v\n", key, err)
			exitFunc(1)
		}
		switch v := target.(type) {
		case *config.ConfigHandler:
			if resolved, ok := instance.(config.ConfigHandler); ok {
				*v = resolved
			} else {
				fmt.Fprintf(os.Stderr, "Error: resolved instance for %s is not of type config.ConfigHandler\n", key)
				exitFunc(1)
			}
		case *shell.Shell:
			if resolved, ok := instance.(shell.Shell); ok {
				*v = resolved
			} else {
				fmt.Fprintf(os.Stderr, "Error: resolved instance for %s is not of type shell.Shell\n", key)
				exitFunc(1)
			}
		case *helpers.Helper:
			if resolved, ok := instance.(helpers.Helper); ok {
				*v = resolved
			} else {
				fmt.Fprintf(os.Stderr, "Error: resolved instance for %s is not of type helpers.Helper\n", key)
				exitFunc(1)
			}
		}
	}

	resolveAndAssign("cliConfigHandler", &cliConfigHandler)
	resolveAndAssign("shell", &shellInstance)
	resolveAndAssign("terraformHelper", &terraformHelper)
	resolveAndAssign("awsHelper", &awsHelper)
	resolveAndAssign("colimaHelper", &colimaHelper)
	resolveAndAssign("dockerHelper", &dockerHelper)

	// Initialize contextInstance
	contextInstance = context.NewContext(cliConfigHandler, shellInstance)
}
