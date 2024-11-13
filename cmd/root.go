package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/env"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/network"
	"github.com/windsor-hotel/cli/internal/shell"
	"github.com/windsor-hotel/cli/internal/ssh"
	"github.com/windsor-hotel/cli/internal/virt"
)

// ConfigHandler instances
var configHandler config.ConfigHandler

// shell instance
var shellInstance shell.Shell

// secureShell instance
var secureShellInstance shell.Shell

// awsHelper instance
var awsHelper helpers.Helper

// dockerHelper instance
var dockerHelper helpers.Helper

// dnsHelper instance
var dnsHelper helpers.Helper

// context instance
var contextHandler context.ContextInterface

// sshClient instance
var sshClient ssh.Client

// colimaVirt instance
var colimaVirt virt.VirtualMachine

// dockerVirt instance
var dockerVirt virt.ContainerRuntime

// awsEnv instance
var awsEnv env.EnvPrinter

// dockerEnv instance
var dockerEnv env.EnvPrinter

// kubeEnv instance
var kubeEnv env.EnvPrinter

// omniEnv instance
var omniEnv env.EnvPrinter

// sopsEnv instance
var sopsEnv env.EnvPrinter

// talosEnv instance
var talosEnv env.EnvPrinter

// terraformEnv instance
var terraformEnv env.EnvPrinter

// windsorEnv instance
var windsorEnv env.EnvPrinter

// colimaNetworkManager instance
var colimaNetworkManager network.NetworkManager

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
	// Check if configHandler is initialized
	if configHandler == nil {
		return fmt.Errorf("configHandler is not initialized")
	}

	// Load CLI configuration
	cliConfigPath := getCLIConfigPath()
	if err := configHandler.LoadConfig(cliConfigPath); err != nil {
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

// Initialize sets dependency injector
func Initialize(inj di.Injector) {
	injector = inj

	resolveAndAssign := func(key string, target interface{}) {
		instance, err := injector.Resolve(key)
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
				if err := resolved.Initialize(); err != nil {
					fmt.Fprintf(os.Stderr, "Error initializing shell.Shell: %v\n", err)
					exitFunc(1)
				}
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
		case *context.ContextInterface:
			if resolved, ok := instance.(context.ContextInterface); ok {
				*v = resolved
			} else {
				fmt.Fprintf(os.Stderr, "Error: resolved instance for %s is not of type context.ContextInterface\n", key)
				exitFunc(1)
			}
		case *ssh.Client:
			if resolved, ok := instance.(ssh.Client); ok {
				*v = resolved
			} else {
				fmt.Fprintf(os.Stderr, "Error: resolved instance for %s is not of type ssh.Client\n", key)
				exitFunc(1)
			}
		case *virt.Virt:
			if resolved, ok := instance.(virt.Virt); ok {
				if err := resolved.Initialize(); err != nil {
					fmt.Fprintf(os.Stderr, "Error initializing virt.Virt: %v\n", err)
					exitFunc(1)
				}
				*v = resolved
			} else {
				fmt.Fprintf(os.Stderr, "Error: resolved instance for %s is not of type virt.Virt\n", key)
				exitFunc(1)
			}
		case *virt.ContainerRuntime:
			if resolved, ok := instance.(virt.ContainerRuntime); ok {
				if err := resolved.Initialize(); err != nil {
					fmt.Fprintf(os.Stderr, "Error initializing virt.ContainerRuntime: %v\n", err)
					exitFunc(1)
				}
				*v = resolved
			} else {
				fmt.Fprintf(os.Stderr, "Error: resolved instance for %s is not of type virt.ContainerRuntime\n", key)
				exitFunc(1)
			}
		case *virt.VirtualMachine:
			if resolved, ok := instance.(virt.VirtualMachine); ok {
				if err := resolved.Initialize(); err != nil {
					fmt.Fprintf(os.Stderr, "Error initializing virt.VirtualMachine: %v\n", err)
					exitFunc(1)
				}
				*v = resolved
			} else {
				fmt.Fprintf(os.Stderr, "Error: resolved instance for %s is not of type virt.VirtualMachine\n", key)
				exitFunc(1)
			}
		case *env.EnvPrinter:
			if resolved, ok := instance.(env.EnvPrinter); ok {
				if err := resolved.Initialize(); err != nil {
					fmt.Fprintf(os.Stderr, "Error initializing env.EnvPrinter: %v\n", err)
					exitFunc(1)
				}
				*v = resolved
			} else {
				fmt.Fprintf(os.Stderr, "Error: resolved instance for %s is not of type env.EnvInterface\n", key)
				exitFunc(1)
			}
		case *network.NetworkManager:
			if resolved, ok := instance.(network.NetworkManager); ok {
				if err := resolved.Initialize(); err != nil {
					fmt.Fprintf(os.Stderr, "Error initializing network.NetworkManager: %v\n", err)
					exitFunc(1)
				}
				*v = resolved
			} else {
				fmt.Fprintf(os.Stderr, "Error: resolved instance for %s is not of type network.NetworkManager\n", key)
				exitFunc(1)
			}
		}
	}

	resolveAndAssign("configHandler", &configHandler)
	resolveAndAssign("shell", &shellInstance)
	resolveAndAssign("secureShell", &secureShellInstance)
	resolveAndAssign("awsHelper", &awsHelper)
	resolveAndAssign("dockerHelper", &dockerHelper)
	resolveAndAssign("dnsHelper", &dnsHelper)
	resolveAndAssign("contextHandler", &contextHandler)
	resolveAndAssign("sshClient", &sshClient)
	resolveAndAssign("colimaVirt", &colimaVirt)
	resolveAndAssign("dockerVirt", &dockerVirt)
	resolveAndAssign("awsEnv", &awsEnv)
	resolveAndAssign("dockerEnv", &dockerEnv)
	resolveAndAssign("kubeEnv", &kubeEnv)
	resolveAndAssign("omniEnv", &omniEnv)
	resolveAndAssign("sopsEnv", &sopsEnv)
	resolveAndAssign("talosEnv", &talosEnv)
	resolveAndAssign("terraformEnv", &terraformEnv)
	resolveAndAssign("windsorEnv", &windsorEnv)
	resolveAndAssign("colimaNetworkManager", &colimaNetworkManager)
}
