package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/helpers"
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
)

var initCmd = &cobra.Command{
	Use:   "init [context]",
	Short: "Initialize the application",
	Long:  "Initialize the application by setting up necessary configurations and environment",
	Args:  cobra.ExactArgs(1), // Ensure exactly one argument is provided
	RunE: func(cmd *cobra.Command, args []string) error {
		contextName := args[0]

		// Start with an empty Context
		contextConfig := config.Context{}

		// Set the context value in the cliConfigHandler
		if err := cliConfigHandler.Set("context", contextName); err != nil {
			return fmt.Errorf("Error setting context value: %w", err)
		}

		// Set the specific context configuration in the cliConfigHandler
		if err := cliConfigHandler.Set(fmt.Sprintf("contexts.%s", contextName), contextConfig); err != nil {
			return fmt.Errorf("Error setting contexts value: %w", err)
		}

		// If the context is local or starts with "local-", set the defaults to the default local config
		if contextName == "local" || len(contextName) > 6 && contextName[:6] == "local-" {
			cliConfigHandler.SetDefault(fmt.Sprintf("contexts.%s", contextName), config.DefaultLocalConfig)
		}

		// Conditionally set AWS configuration
		if cmd.Flags().Changed("aws-endpoint-url") {
			contextConfig.AWS.AWSEndpointURL = awsEndpointURL
		}
		if cmd.Flags().Changed("aws-profile") {
			contextConfig.AWS.AWSProfile = awsProfile
		}

		// Conditionally set Docker configuration
		if cmd.Flags().Changed("docker") {
			contextConfig.Docker.Enabled = docker
		}

		// Conditionally set Terraform configuration
		if cmd.Flags().Changed("backend") {
			contextConfig.Terraform.Backend = backend
		}

		// Conditionally set VM configuration
		if cmd.Flags().Changed("vm-driver") {
			contextConfig.VM.Driver = vmType
		}
		if cmd.Flags().Changed("vm-cpu") {
			contextConfig.VM.CPU = cpu
		}
		if cmd.Flags().Changed("vm-disk") {
			contextConfig.VM.Disk = disk
		}
		if cmd.Flags().Changed("vm-memory") {
			contextConfig.VM.Memory = memory
		}
		if cmd.Flags().Changed("vm-arch") {
			contextConfig.VM.Arch = arch
		}

		// Save the cli configuration
		if err := cliConfigHandler.SaveConfig(cliConfigPath); err != nil {
			return fmt.Errorf("Error saving config file: %w", err)
		}

		// Save the project configuration only if projectConfigPath is present
		if projectConfigPath != "" {
			if err := projectConfigHandler.SaveConfig(projectConfigPath); err != nil {
				return fmt.Errorf("Error saving project config file: %w", err)
			}
		}

		// Write the vendor config files using the DI container
		helperInstances, err := container.ResolveAll((*helpers.Helper)(nil))
		if err != nil {
			return fmt.Errorf("error resolving helpers: %w", err)
		}

		for _, instance := range helperInstances {
			helper := instance.(helpers.Helper)
			if err := helper.WriteConfig(); err != nil {
				return fmt.Errorf("error writing config for helper: %w", err)
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
	rootCmd.AddCommand(initCmd)
}
