package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/config"
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
	PreRunE:      preRunEInitializeCommonComponents,
	RunE: func(cmd *cobra.Command, args []string) error {

		// Add the current directory to the trusted file list
		shell := controller.ResolveShell()
		if err := shell.AddCurrentDirToTrustedFile(); err != nil {
			return fmt.Errorf("Error adding current directory to trusted file: %w", err)
		}

		// Resolve the config handler
		configHandler := controller.ResolveConfigHandler()

		var contextName string
		if len(args) == 1 {
			contextName = args[0]
		} else {
			contextName = configHandler.GetContext()
		}

		// Set the context value
		if contextName == "" {
			contextName = "local"
		}
		if err := configHandler.SetContext(contextName); err != nil {
			return fmt.Errorf("Error setting context value: %w", err)
		}

		// Create the flag to config path mapping
		configurations := []struct {
			flagName   string
			configPath string
			value      interface{}
		}{
			{"aws-endpoint-url", "aws.aws_endpoint_url", awsEndpointURL},
			{"aws-profile", "aws.aws_profile", awsProfile},
			{"docker", "docker.enabled", docker},
			{"backend", "terraform.backend", backend},
			{"vm-driver", "vm.driver", vmDriver},
			{"vm-cpu", "vm.cpu", cpu},
			{"vm-disk", "vm.disk", disk},
			{"vm-memory", "vm.memory", memory},
			{"vm-arch", "vm.arch", arch},
			{"tools-manager", "toolsManager", toolsManager},
			{"git-livereload", "git.livereload.enabled", gitLivereload},
		}

		// Set the configurations
		for _, config := range configurations {
			if cmd.Flags().Changed(config.flagName) {
				err := configHandler.SetContextValue(config.configPath, config.value)
				if err != nil {
					return fmt.Errorf("Error setting %s configuration: %w", config.flagName, err)
				}
			}
		}

		// Set appropriate default windsor.yaml configuration
		vmDriverConfig := configHandler.GetString("vm.driver")
		if vmDriverConfig == "docker-desktop" {
			err := configHandler.SetDefault(config.DefaultConfig_Containerized)
			if err != nil {
				return fmt.Errorf("Error setting default containerized config: %w", err)
			}
		} else if vmDriverConfig == "colima" {
			err := configHandler.SetDefault(config.DefaultConfig_FullVM)
			if err != nil {
				return fmt.Errorf("Error setting default full VM config: %w", err)
			}
		}

		// Get the cli configuration path using shell to get the project root
		projectRoot, err := shell.GetProjectRoot()
		if err != nil {
			return fmt.Errorf("Error retrieving project root: %w", err)
		}
		yamlPath := filepath.Join(projectRoot, "windsor.yaml")
		ymlPath := filepath.Join(projectRoot, "windsor.yml")

		// Declare cliConfigPath variable
		var cliConfigPath string

		// Check if windsor.yaml exists
		if _, err := osStat(yamlPath); err == nil {
			cliConfigPath = yamlPath
		} else if _, err := osStat(ymlPath); err == nil {
			cliConfigPath = ymlPath
		} else {
			// Default to windsor.yaml if neither file exists
			cliConfigPath = yamlPath
		}

		// Save the cli configuration
		if err := configHandler.SaveConfig(cliConfigPath); err != nil {
			return fmt.Errorf("Error saving config file: %w", err)
		}

		// Create project components
		if err := controller.CreateProjectComponents(); err != nil {
			return fmt.Errorf("Error creating project components: %w", err)
		}

		// Create service components
		if err := controller.CreateServiceComponents(); err != nil {
			return fmt.Errorf("Error creating service components: %w", err)
		}

		// Create virtualization components
		if err := controller.CreateVirtualizationComponents(); err != nil {
			return fmt.Errorf("Error creating virtualization components: %w", err)
		}

		// Create stack components
		if err := controller.CreateStackComponents(); err != nil {
			return fmt.Errorf("Error creating stack components: %w", err)
		}

		// Initialize components
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
