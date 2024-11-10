package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/config"
)

var (
	backend        string
	awsProfile     string
	awsEndpointURL string
	vmType         string
	cpu            int
	disk           int
	memory         int
	arch           string
	docker         bool
	gitLivereload  bool
)

var initCmd = &cobra.Command{
	Use:   "init [context]",
	Short: "Initialize the application",
	Long:  "Initialize the application by setting up necessary configurations and environment",
	Args:  cobra.ExactArgs(1), // Ensure exactly one argument is provided
	RunE: func(cmd *cobra.Command, args []string) error {
		contextName := args[0]

		// Determine the cliConfig path
		cliConfigPath := os.Getenv("WINDSOR_CONFIG")
		if cliConfigPath == "" {
			homeDir, err := osUserHomeDir()
			if err != nil {
				return fmt.Errorf("error retrieving home directory: %w", err)
			}
			cliConfigPath = filepath.Join(homeDir, ".config", "windsor", "config.yaml")
		}

		// Set the context value
		if err := cliConfigHandler.Set("context", contextName); err != nil {
			return fmt.Errorf("Error setting config value: %w", err)
		}

		// If the context is local or starts with "local-", set the defaults to the default local config
		if contextName == "local" || len(contextName) > 6 && contextName[:6] == "local-" {
			if err := cliConfigHandler.SetDefault(config.DefaultLocalConfig); err != nil {
				return fmt.Errorf("Error setting default local config: %w", err)
			}
		} else {
			if err := cliConfigHandler.SetDefault(config.DefaultConfig); err != nil {
				return fmt.Errorf("Error setting default config: %w", err)
			}
		}

		// Conditionally set AWS configuration
		if cmd.Flags().Changed("aws-endpoint-url") {
			if err := cliConfigHandler.Set(fmt.Sprintf("contexts.%s.aws.aws_endpoint_url", contextName), awsEndpointURL); err != nil {
				return fmt.Errorf("Error setting AWS endpoint URL: %w", err)
			}
		}
		if cmd.Flags().Changed("aws-profile") {
			if err := cliConfigHandler.Set(fmt.Sprintf("contexts.%s.aws.aws_profile", contextName), awsProfile); err != nil {
				return fmt.Errorf("Error setting AWS profile: %w", err)
			}
		}

		// Conditionally set Docker configuration
		if cmd.Flags().Changed("docker") {
			if err := cliConfigHandler.Set(fmt.Sprintf("contexts.%s.docker.enabled", contextName), docker); err != nil {
				return fmt.Errorf("Error setting Docker enabled: %w", err)
			}
		}

		// Conditionally set Terraform configuration
		if cmd.Flags().Changed("backend") {
			if err := cliConfigHandler.Set(fmt.Sprintf("contexts.%s.terraform.backend", contextName), backend); err != nil {
				return fmt.Errorf("Error setting Terraform backend: %w", err)
			}
		}

		// Conditionally set VM configuration
		if cmd.Flags().Changed("vm-driver") {
			if err := cliConfigHandler.Set(fmt.Sprintf("contexts.%s.vm.driver", contextName), vmType); err != nil {
				return fmt.Errorf("Error setting VM driver: %w", err)
			}
		}
		if cmd.Flags().Changed("vm-cpu") {
			if err := cliConfigHandler.Set(fmt.Sprintf("contexts.%s.vm.cpu", contextName), cpu); err != nil {
				return fmt.Errorf("Error setting VM CPU: %w", err)
			}
		}
		if cmd.Flags().Changed("vm-disk") {
			if err := cliConfigHandler.Set(fmt.Sprintf("contexts.%s.vm.disk", contextName), disk); err != nil {
				return fmt.Errorf("Error setting VM disk: %w", err)
			}
		}
		if cmd.Flags().Changed("vm-memory") {
			if err := cliConfigHandler.Set(fmt.Sprintf("contexts.%s.vm.memory", contextName), memory); err != nil {
				return fmt.Errorf("Error setting VM memory: %w", err)
			}
		}
		if cmd.Flags().Changed("vm-arch") {
			if err := cliConfigHandler.Set(fmt.Sprintf("contexts.%s.vm.arch", contextName), arch); err != nil {
				return fmt.Errorf("Error setting VM architecture: %w", err)
			}
		}

		// Conditionally set Git Livereload configuration
		if cmd.Flags().Changed("git-livereload") {
			if err := cliConfigHandler.Set(fmt.Sprintf("contexts.%s.git.livereload.enabled", contextName), gitLivereload); err != nil {
				return fmt.Errorf("Error setting Git Livereload enabled: %w", err)
			}
		}

		// Save the cli configuration
		if err := cliConfigHandler.SaveConfig(cliConfigPath); err != nil {
			return fmt.Errorf("Error saving config file: %w", err)
		}

		// Configure ColimaVirt if enabled in configuration
		driver := cliConfigHandler.GetString("vm.driver")
		if driver == "colima" {
			if err := colimaVirt.WriteConfig(); err != nil {
				return fmt.Errorf("error writing Colima config: %w", err)
			}
		}

		// Configure DockerVirt if enabled in configuration
		dockerEnabled := cliConfigHandler.GetBool("docker.enabled")
		if dockerEnabled {
			if err := dockerVirt.WriteConfig(); err != nil {
				return fmt.Errorf("error writing Docker config: %w", err)
			}
		}

		fmt.Println("Initialization successful")
		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&backend, "backend", "", "Specify the terraform backend to use")
	initCmd.Flags().StringVar(&awsProfile, "aws-profile", "", "Specify the AWS profile to use")
	initCmd.Flags().StringVar(&awsEndpointURL, "aws-endpoint-url", "", "Specify the AWS endpoint URL to use")
	initCmd.Flags().StringVar(&vmType, "vm-driver", "", "Specify the VM driver. Only Colima is supported for now.")
	initCmd.Flags().IntVar(&cpu, "vm-cpu", 0, "Specify the number of CPUs for Colima")
	initCmd.Flags().IntVar(&disk, "vm-disk", 0, "Specify the disk size for Colima")
	initCmd.Flags().IntVar(&memory, "vm-memory", 0, "Specify the memory size for Colima")
	initCmd.Flags().StringVar(&arch, "vm-arch", "", "Specify the architecture for Colima")
	initCmd.Flags().BoolVar(&docker, "docker", false, "Enable Docker")
	initCmd.Flags().BoolVar(&gitLivereload, "git-livereload", false, "Enable Git Livereload")
	rootCmd.AddCommand(initCmd)
}
