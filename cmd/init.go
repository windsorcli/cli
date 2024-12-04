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
	cpu            int
	disk           int
	memory         int
	arch           string
	docker         bool
	gitLivereload  bool
)

var initCmd = &cobra.Command{
	Use:          "init [context]",
	Short:        "Initialize the application",
	Long:         "Initialize the application by setting up necessary configurations and environment",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	PreRunE:      preRunEInitializeCommonComponents,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Resolve the context handler
		contextHandler := controller.ResolveContextHandler()
		if contextHandler == nil {
			return fmt.Errorf("Error: no context handler found")
		}
		var contextName string
		if len(args) == 1 {
			contextName = args[0]
		} else {
			var err error
			contextName, err = contextHandler.GetContext()
			if err != nil {
				return fmt.Errorf("no context provided and no current context set: %w", err)
			}
		}

		// Set the context value
		if err := contextHandler.SetContext(contextName); err != nil {
			return fmt.Errorf("Error setting context value: %w", err)
		}

		// Resolve the config handler
		configHandler := controller.ResolveConfigHandler()

		// Conditionally set AWS configuration
		if cmd.Flags().Changed("aws-endpoint-url") {
			if err := configHandler.Set("aws.aws_endpoint_url", awsEndpointURL); err != nil {
				return fmt.Errorf("Error setting AWS endpoint URL: %w", err)
			}
		}
		if cmd.Flags().Changed("aws-profile") {
			if err := configHandler.Set("aws.aws_profile", awsProfile); err != nil {
				return fmt.Errorf("Error setting AWS profile: %w", err)
			}
		}

		// Conditionally set Docker configuration
		if cmd.Flags().Changed("docker") {
			if err := configHandler.Set("docker.enabled", docker); err != nil {
				return fmt.Errorf("Error setting Docker enabled: %w", err)
			}
		}

		// Conditionally set Terraform configuration
		if cmd.Flags().Changed("backend") {
			if err := configHandler.Set("terraform.backend", backend); err != nil {
				return fmt.Errorf("Error setting Terraform backend: %w", err)
			}
		}

		// Conditionally set VM configuration
		if cmd.Flags().Changed("vm-driver") {
			if err := configHandler.Set("vm.driver", vmType); err != nil {
				return fmt.Errorf("Error setting VM driver: %w", err)
			}
		}
		if cmd.Flags().Changed("vm-cpu") {
			if err := configHandler.Set("vm.cpu", cpu); err != nil {
				return fmt.Errorf("Error setting VM CPU: %w", err)
			}
		}
		if cmd.Flags().Changed("vm-disk") {
			if err := configHandler.Set("vm.disk", disk); err != nil {
				return fmt.Errorf("Error setting VM disk: %w", err)
			}
		}
		if cmd.Flags().Changed("vm-memory") {
			if err := configHandler.Set("vm.memory", memory); err != nil {
				return fmt.Errorf("Error setting VM memory: %w", err)
			}
		}
		if cmd.Flags().Changed("vm-arch") {
			if err := configHandler.Set("vm.arch", arch); err != nil {
				return fmt.Errorf("Error setting VM architecture: %w", err)
			}
		}

		// Conditionally set Git Livereload configuration
		if cmd.Flags().Changed("git-livereload") {
			if err := configHandler.Set("git.livereload.enabled", gitLivereload); err != nil {
				return fmt.Errorf("Error setting Git Livereload enabled: %w", err)
			}
		}

		// Get the cli configuration path
		cliConfigPath, err := getCliConfigPath()
		if err != nil {
			return fmt.Errorf("Error getting cli configuration path: %w", err)
		}

		// Save the cli configuration
		if err := configHandler.SaveConfig(cliConfigPath); err != nil {
			return fmt.Errorf("Error saving config file: %w", err)
		}

		// Create service components
		if err := controller.CreateServiceComponents(); err != nil {
			return fmt.Errorf("Error creating service components: %w", err)
		}

		// Create virtualization components
		if err := controller.CreateVirtualizationComponents(); err != nil {
			return fmt.Errorf("Error creating virtualization components: %w", err)
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
