package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/helpers"
)

var (
	backend        string
	awsProfile     string
	awsEndpointURL string
	vmType         string
	cpu            string
	disk           string
	memory         string
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

		// Set the context value
		if err := cliConfigHandler.SetConfigValue("context", contextName); err != nil {
			return fmt.Errorf("Error setting config value: %w", err)
		}

		// Pass the backend flag to the terraformHelper.SetConfig function
		if err := terraformHelper.SetConfig("backend", backend); err != nil {
			return fmt.Errorf("Error setting backend value: %w", err)
		}

		// Set the Docker configuration values using the DockerHelper
		dockerValue := ""
		if cmd.Flags().Changed("docker") || docker {
			dockerValue = strconv.FormatBool(docker)
		}
		if err := dockerHelper.SetConfig("enabled", dockerValue); err != nil {
			return fmt.Errorf("error setting Docker configuration: %w", err)
		}

		// Set the AWS configuration values using the AwsHelper
		if err := cliConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.aws.aws_endpoint_url", contextName), awsEndpointURL); err != nil {
			return fmt.Errorf("error setting aws_endpoint_url: %w", err)
		}
		if err := cliConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.aws.aws_profile", contextName), awsProfile); err != nil {
			return fmt.Errorf("error setting aws_profile: %w", err)
		}

		// Set the Colima configuration values using the cliConfigHandler
		if err := cliConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.vm.driver", contextName), vmType); err != nil {
			return fmt.Errorf("error setting vm driver: %w", err)
		}
		if err := cliConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.vm.cpu", contextName), cpu); err != nil {
			return fmt.Errorf("error setting vm cpu: %w", err)
		}
		if err := cliConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.vm.disk", contextName), disk); err != nil {
			return fmt.Errorf("error setting vm disk: %w", err)
		}
		if err := cliConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.vm.memory", contextName), memory); err != nil {
			return fmt.Errorf("error setting vm memory: %w", err)
		}
		if err := cliConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.vm.arch", contextName), arch); err != nil {
			return fmt.Errorf("error setting vm arch: %w", err)
		}

		// Save the cli configuration
		if err := cliConfigHandler.SaveConfig(""); err != nil {
			return fmt.Errorf("Error saving config file: %w", err)
		}

		// Save the project configuration
		if err := projectConfigHandler.SaveConfig(""); err != nil {
			return fmt.Errorf("Error saving project config file: %w", err)
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
	initCmd.Flags().StringVar(&cpu, "vm-cpu", "", "Specify the number of CPUs for Colima")
	initCmd.Flags().StringVar(&disk, "vm-disk", "", "Specify the disk size for Colima")
	initCmd.Flags().StringVar(&memory, "vm-memory", "", "Specify the memory size for Colima")
	initCmd.Flags().StringVar(&arch, "vm-arch", "", "Specify the architecture for Colima")
	initCmd.Flags().BoolVar(&docker, "docker", false, "Enable Docker")
	rootCmd.AddCommand(initCmd)
}
