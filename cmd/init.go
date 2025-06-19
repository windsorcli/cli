package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/config"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

var (
	initBackend        string
	initAwsProfile     string
	initAwsEndpointURL string
	initVmDriver       string
	initCpu            int
	initDisk           int
	initMemory         int
	initArch           string
	initDocker         bool
	initGitLivereload  bool
	initBlueprint      string
	initToolsManager   string
	initPlatform       string
	initEndpoint       string
	initSetFlags       []string
	reset              bool
)

var initCmd = &cobra.Command{
	Use:          "init [context]",
	Short:        "Initialize the application",
	Long:         "Initialize the application by setting up necessary configurations and environment",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Create shims instance for this command
		shims := NewShims()

		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		// Initialize with requirements
		if err := controller.InitializeWithRequirements(ctrl.Requirements{
			CommandName: cmd.Name(),
		}); err != nil {
			return fmt.Errorf("Error initializing: %w", err)
		}

		// Add the current directory to the trusted file list
		shell := controller.ResolveShell()
		if err := shell.AddCurrentDirToTrustedFile(); err != nil {
			return fmt.Errorf("Error adding current directory to trusted file: %w", err)
		}

		// Resolve the config handler and determine the context name
		configHandler := controller.ResolveConfigHandler()
		contextName := "local"
		if len(args) == 1 {
			contextName = args[0]
		} else if currentContext := configHandler.GetContext(); currentContext != "" {
			contextName = currentContext
		}

		// Set the context value
		if err := configHandler.SetContext(contextName); err != nil {
			return fmt.Errorf("Error setting context value: %w", err)
		}

		// Write a reset token to reset the session
		if _, err := shell.WriteResetToken(); err != nil {
			return fmt.Errorf("Error writing reset token: %w", err)
		}

		// Determine the default vm driver to use if not set
		vmDriverConfig := initVmDriver
		if vmDriverConfig == "" {
			vmDriverConfig = configHandler.GetString("vm.driver")
			if vmDriverConfig == "" && (contextName == "local" || strings.HasPrefix(contextName, "local-")) {
				switch shims.Goos() {
				case "darwin", "windows":
					vmDriverConfig = "docker-desktop"
				default:
					vmDriverConfig = "docker"
				}
			}
		}

		// Set the default configuration if applicable
		defaultConfig := &config.DefaultConfig
		if vmDriverConfig == "docker-desktop" {
			defaultConfig = &config.DefaultConfig_Localhost
		} else if vmDriverConfig == "colima" || vmDriverConfig == "docker" {
			defaultConfig = &config.DefaultConfig_Full
		}
		if err := configHandler.SetDefault(*defaultConfig); err != nil {
			return fmt.Errorf("Error setting default config: %w", err)
		}

		// Create the flag to config path mapping and set the configurations
		configurations := []struct {
			flagName   string
			configPath string
			value      any
		}{
			{"aws-endpoint-url", "aws.aws_endpoint_url", initAwsEndpointURL},
			{"aws-profile", "aws.aws_profile", initAwsProfile},
			{"docker", "docker.enabled", initDocker},
			{"backend", "terraform.backend", initBackend},
			{"vm-cpu", "vm.cpu", initCpu},
			{"vm-disk", "vm.disk", initDisk},
			{"vm-memory", "vm.memory", initMemory},
			{"vm-arch", "vm.arch", initArch},
			{"tools-manager", "toolsManager", initToolsManager},
			{"git-livereload", "git.livereload.enabled", initGitLivereload},
			{"blueprint", "blueprint", initBlueprint},
			{"endpoint", "cluster.endpoint", initEndpoint},
			{"platform", "cluster.platform", initPlatform},
		}

		for _, config := range configurations {
			if cmd.Flags().Changed(config.flagName) {
				err := configHandler.SetContextValue(config.configPath, config.value)
				if err != nil {
					return fmt.Errorf("Error setting %s configuration: %w", config.flagName, err)
				}
			}
		}

		// Process all set flags after other flags
		for _, setFlag := range initSetFlags {
			parts := strings.SplitN(setFlag, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("Invalid format for --set flag. Expected key=value")
			}
			key, value := parts[0], parts[1]
			if err := configHandler.SetContextValue(key, value); err != nil {
				return fmt.Errorf("Error setting config override %s: %w", key, err)
			}
		}

		// Set platform-specific configurations
		if initPlatform != "" {
			switch initPlatform {
			case "aws":
				if err := configHandler.SetContextValue("aws.enabled", true); err != nil {
					return fmt.Errorf("Error setting aws.enabled: %w", err)
				}
				if err := configHandler.SetContextValue("cluster.driver", "eks"); err != nil {
					return fmt.Errorf("Error setting cluster.driver: %w", err)
				}
			case "azure":
				if err := configHandler.SetContextValue("azure.enabled", true); err != nil {
					return fmt.Errorf("Error setting azure.enabled: %w", err)
				}
				if err := configHandler.SetContextValue("cluster.driver", "aks"); err != nil {
					return fmt.Errorf("Error setting cluster.driver: %w", err)
				}
			case "metal":
				if err := configHandler.SetContextValue("cluster.driver", "talos"); err != nil {
					return fmt.Errorf("Error setting cluster.driver: %w", err)
				}
			case "local":
				if err := configHandler.SetContextValue("cluster.driver", "talos"); err != nil {
					return fmt.Errorf("Error setting cluster.driver: %w", err)
				}
			}
		}

		// Set the vm driver only if it's configured and not overridden by --set flag
		if vmDriverConfig != "" && configHandler.GetString("vm.driver") == "" {
			if err := configHandler.SetContextValue("vm.driver", vmDriverConfig); err != nil {
				return fmt.Errorf("Error setting vm driver: %w", err)
			}
		}

		// Determine the cli configuration path
		projectRoot, err := shell.GetProjectRoot()
		if err != nil {
			return fmt.Errorf("Error retrieving project root: %w", err)
		}
		yamlPath := filepath.Join(projectRoot, "windsor.yaml")
		ymlPath := filepath.Join(projectRoot, "windsor.yml")

		var cliConfigPath string
		if _, err := shims.Stat(yamlPath); err == nil {
			cliConfigPath = yamlPath
		} else if _, err := shims.Stat(ymlPath); err == nil {
			cliConfigPath = ymlPath
		} else {
			cliConfigPath = yamlPath
		}

		// Set the context ID
		if err := configHandler.GenerateContextID(); err != nil {
			return fmt.Errorf("failed to generate context ID: %w", err)
		}

		// Save the cli configuration
		if err := configHandler.SaveConfig(cliConfigPath); err != nil {
			return fmt.Errorf("Error saving config file: %w", err)
		}

		// Initialize with requirements
		if err := controller.InitializeWithRequirements(ctrl.Requirements{
			Env:         true,
			VM:          true,
			Containers:  true,
			Services:    true,
			Network:     true,
			Blueprint:   true,
			Cluster:     true,
			Generators:  true,
			Stack:       true,
			Reset:       reset,
			CommandName: cmd.Name(),
			Flags: map[string]bool{
				"verbose": verbose,
			},
		}); err != nil {
			return fmt.Errorf("Error initializing: %w", err)
		}

		// Set the environment variables internally in the process
		if err := controller.SetEnvironmentVariables(); err != nil {
			return fmt.Errorf("Error setting environment variables: %w", err)
		}

		// Write configurations to file
		if err := controller.WriteConfigurationFiles(); err != nil {
			return fmt.Errorf("Error writing configuration files: %w", err)
		}

		// Print the success message
		fmt.Fprintln(os.Stderr, "Initialization successful")
		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&initBackend, "backend", "", "Specify the terraform backend to use")
	initCmd.Flags().StringVar(&initAwsProfile, "aws-profile", "", "Specify the AWS profile to use")
	initCmd.Flags().StringVar(&initAwsEndpointURL, "aws-endpoint-url", "", "Specify the AWS endpoint URL to use")
	initCmd.Flags().StringVar(&initVmDriver, "vm-driver", "", "Specify the VM driver. Only Colima is supported for now.")
	initCmd.Flags().IntVar(&initCpu, "vm-cpu", 0, "Specify the number of CPUs for Colima")
	initCmd.Flags().IntVar(&initDisk, "vm-disk", 0, "Specify the disk size for Colima")
	initCmd.Flags().IntVar(&initMemory, "vm-memory", 0, "Specify the memory size for Colima")
	initCmd.Flags().StringVar(&initArch, "vm-arch", "", "Specify the architecture for Colima")
	initCmd.Flags().BoolVar(&initDocker, "docker", false, "Enable Docker")
	initCmd.Flags().BoolVar(&initGitLivereload, "git-livereload", false, "Enable Git Livereload")
	initCmd.Flags().StringVar(&initPlatform, "platform", "", "Specify the platform to use [local|metal]")
	initCmd.Flags().StringVar(&initBlueprint, "blueprint", "", "Specify the blueprint to use")
	initCmd.Flags().StringVar(&initEndpoint, "endpoint", "", "Specify the kubernetes API endpoint")
	initCmd.Flags().StringSliceVar(&initSetFlags, "set", []string{}, "Override configuration values. Example: --set dns.enabled=false --set cluster.endpoint=https://localhost:6443")
	initCmd.Flags().BoolVar(&reset, "reset", false, "Reset/overwrite existing files and clean .terraform directory")
	rootCmd.AddCommand(initCmd)
}
