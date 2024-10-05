package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
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
)

var initCmd = &cobra.Command{
	Use:   "init [context]",
	Short: "Initialize the application",
	Long:  "Initialize the application by setting up necessary configurations and environment",
	Args:  cobra.ExactArgs(1), // Ensure exactly one argument is provided
	RunE: func(cmd *cobra.Command, args []string) error {
		contextName := args[0]

		// Set the context value
		if err := configHandler.SetConfigValue("context", contextName); err != nil {
			return fmt.Errorf("Error setting config value: %w", err)
		}

		// Pass the backend flag to the terraformHelper.SetConfig function
		if err := terraformHelper.SetConfig("backend", backend); err != nil {
			return fmt.Errorf("Error setting backend value: %w", err)
		}

		// Set the AWS configuration values using the AwsHelper
		if err := awsHelper.SetConfig("aws_endpoint_url", awsEndpointURL); err != nil {
			return fmt.Errorf("error setting AWS configuration: %w", err)
		}
		if err := awsHelper.SetConfig("aws_profile", awsProfile); err != nil {
			return fmt.Errorf("error setting AWS configuration: %w", err)
		}

		// Set the Colima configuration values using the ColimaHelper
		if err := colimaHelper.SetConfig("driver", vmType); err != nil {
			return fmt.Errorf("error setting Colima configuration: %w", err)
		}
		if err := colimaHelper.SetConfig("cpu", cpu); err != nil {
			return fmt.Errorf("error setting Colima configuration: %w", err)
		}
		if err := colimaHelper.SetConfig("disk", disk); err != nil {
			return fmt.Errorf("error setting Colima configuration: %w", err)
		}
		if err := colimaHelper.SetConfig("memory", memory); err != nil {
			return fmt.Errorf("error setting Colima configuration: %w", err)
		}
		if err := colimaHelper.SetConfig("arch", arch); err != nil {
			return fmt.Errorf("error setting Colima configuration: %w", err)
		}

		// Save the cli configuration
		if err := configHandler.SaveConfig(""); err != nil {
			return fmt.Errorf("Error saving config file: %w", err)
		}

		// Save the project configuration
		if err := projectConfigHandler.SaveConfig(""); err != nil {
			return fmt.Errorf("Error saving project config file: %w", err)
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
	rootCmd.AddCommand(initCmd)
}
