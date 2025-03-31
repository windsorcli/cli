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
	backend        string
	awsProfile     string
	awsEndpointURL string
	vmDriver       string
	cpu            int
	disk           int
	memory         int
	arch           string
	docker         bool
	gitLivereload  bool
	blueprint      string
	toolsManager   string
)

var initCmd = &cobra.Command{
	Use:          "init [context]",
	Short:        "Initialize the application",
	Long:         "Initialize the application by setting up necessary configurations and environment",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

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

		// Determine the default vm driver to use if not set
		vmDriverConfig := vmDriver
		if vmDriverConfig == "" {
			vmDriverConfig = configHandler.GetString("vm.driver")
			if vmDriverConfig == "" && (contextName == "local" || strings.HasPrefix(contextName, "local-")) {
				switch goos() {
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

		// Set the vm driver only if it's configured
		if vmDriverConfig != "" {
			if err := configHandler.SetContextValue("vm.driver", vmDriverConfig); err != nil {
				return fmt.Errorf("Error setting vm driver: %w", err)
			}
		}

		// Create the flag to config path mapping and set the configurations
		configurations := []struct {
			flagName   string
			configPath string
			value      interface{}
		}{
			{"aws-endpoint-url", "aws.aws_endpoint_url", awsEndpointURL},
			{"aws-profile", "aws.aws_profile", awsProfile},
			{"docker", "docker.enabled", docker},
			{"backend", "terraform.backend", backend},
			{"vm-cpu", "vm.cpu", cpu},
			{"vm-disk", "vm.disk", disk},
			{"vm-memory", "vm.memory", memory},
			{"vm-arch", "vm.arch", arch},
			{"tools-manager", "toolsManager", toolsManager},
			{"git-livereload", "git.livereload.enabled", gitLivereload},
		}

		for _, config := range configurations {
			if cmd.Flags().Changed(config.flagName) {
				err := configHandler.SetContextValue(config.configPath, config.value)
				if err != nil {
					return fmt.Errorf("Error setting %s configuration: %w", config.flagName, err)
				}
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
		if _, err := osStat(yamlPath); err == nil {
			cliConfigPath = yamlPath
		} else if _, err := osStat(ymlPath); err == nil {
			cliConfigPath = ymlPath
		} else {
			cliConfigPath = yamlPath
		}

		// Save the cli configuration
		if err := configHandler.SaveConfig(cliConfigPath); err != nil {
			return fmt.Errorf("Error saving config file: %w", err)
		}

		// Create and initialize components
		if err := controller.CreateProjectComponents(); err != nil {
			return fmt.Errorf("Error creating project components: %w", err)
		}
		if vmDriver != "" {
			if err := controller.CreateServiceComponents(); err != nil {
				return fmt.Errorf("Error creating service components: %w", err)
			}
			if err := controller.CreateVirtualizationComponents(); err != nil {
				return fmt.Errorf("Error creating virtualization components: %w", err)
			}
		}
		if err := controller.CreateStackComponents(); err != nil {
			return fmt.Errorf("Error creating stack components: %w", err)
		}
		if err := controller.InitializeComponents(); err != nil {
			return fmt.Errorf("Error initializing components: %w", err)
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
	initCmd.Flags().StringVar(&backend, "backend", "", "Specify the terraform backend to use")
	initCmd.Flags().StringVar(&awsProfile, "aws-profile", "", "Specify the AWS profile to use")
	initCmd.Flags().StringVar(&awsEndpointURL, "aws-endpoint-url", "", "Specify the AWS endpoint URL to use")
	initCmd.Flags().StringVar(&vmDriver, "vm-driver", "", "Specify the VM driver. Only Colima is supported for now.")
	initCmd.Flags().IntVar(&cpu, "vm-cpu", 0, "Specify the number of CPUs for Colima")
	initCmd.Flags().IntVar(&disk, "vm-disk", 0, "Specify the disk size for Colima")
	initCmd.Flags().IntVar(&memory, "vm-memory", 0, "Specify the memory size for Colima")
	initCmd.Flags().StringVar(&arch, "vm-arch", "", "Specify the architecture for Colima")
	initCmd.Flags().BoolVar(&docker, "docker", false, "Enable Docker")
	initCmd.Flags().BoolVar(&gitLivereload, "git-livereload", false, "Enable Git Livereload")
	rootCmd.AddCommand(initCmd)
}
