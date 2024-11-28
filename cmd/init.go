package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/services"
	"github.com/windsor-hotel/cli/internal/virt"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		contextHandler, err := getContextHandler()
		if err != nil {
			return fmt.Errorf("Error getting context handler: %w", err)
		}
		var contextName string
		if len(args) == 1 {
			contextName = args[0]
		} else {
			contextName, err = contextHandler.GetContext()
			if err != nil {
				return fmt.Errorf("no context provided and no current context set: %w", err)
			}
		}

		// Determine the cliConfig path
		cliConfigPath := os.Getenv("WINDSORCONFIG")
		if cliConfigPath == "" {
			homeDir, err := osUserHomeDir()
			if err != nil {
				return fmt.Errorf("error retrieving home directory: %w", err)
			}
			cliConfigPath = filepath.Join(homeDir, ".config", "windsor", "config.yaml")
		}
		// Load the configuration
		if err := configHandler.LoadConfig(cliConfigPath); err != nil {
			return fmt.Errorf("Error loading config file: %w", err)
		}

		// Set the context value
		if err := contextHandler.SetContext(contextName); err != nil {
			return fmt.Errorf("Error setting context value: %w", err)
		}

		// If the context is local or starts with "local-", set the defaults to the default local config
		if contextName == "local" || len(contextName) > 6 && contextName[:6] == "local-" {
			if err := configHandler.SetDefault(config.DefaultLocalConfig); err != nil {
				return fmt.Errorf("Error setting default local config: %w", err)
			}
		} else {
			if err := configHandler.SetDefault(config.DefaultConfig); err != nil {
				return fmt.Errorf("Error setting default config: %w", err)
			}
		}

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

		// Save the cli configuration
		if err := configHandler.SaveConfig(cliConfigPath); err != nil {
			return fmt.Errorf("Error saving config file: %w", err)
		}

		// Resolve all services
		resolvedServices, err := injector.ResolveAll((*services.Service)(nil))
		if err != nil {
			return fmt.Errorf("Error resolving services: %w", err)
		}

		// Initialize all services
		for _, serviceInterface := range resolvedServices {
			service, ok := serviceInterface.(services.Service)
			if !ok {
				return fmt.Errorf("Resolved instance is not of expected type")
			}

			if err := service.Initialize(); err != nil {
				return fmt.Errorf("error initializing service: %w", err)
			}
		}

		// Write configuration for all services
		for _, serviceInterface := range resolvedServices {
			service, ok := serviceInterface.(services.Service)
			if !ok {
				return fmt.Errorf("Resolved instance is not of expected type")
			}

			if err := service.WriteConfig(); err != nil {
				return fmt.Errorf("error writing service config: %w", err)
			}
		}

		// Initialize DockerVirt if enabled
		dockerEnabled := configHandler.GetBool("docker.enabled")
		if dockerEnabled {
			dockerVirtInstance, err := injector.Resolve("dockerVirt")
			if err != nil {
				return fmt.Errorf("Error resolving dockerVirt: %w", err)
			}
			dockerVirt, ok := dockerVirtInstance.(virt.ContainerRuntime)
			if !ok {
				return fmt.Errorf("Resolved instance is not of type virt.ContainerRuntime")
			}
			if err := dockerVirt.Initialize(); err != nil {
				return fmt.Errorf("Error initializing dockerVirt: %w", err)
			}
		}

		// Initialize ColimaVirt if enabled
		vmType := configHandler.GetString("vm.driver")
		if vmType == "colima" {
			colimaVirtInstance, err := injector.Resolve("colimaVirt")
			if err != nil {
				return fmt.Errorf("Error resolving colimaVirt: %w", err)
			}
			colimaVirt, ok := colimaVirtInstance.(virt.VirtualMachine)
			if !ok {
				return fmt.Errorf("Resolved instance is not of type virt.VirtualMachine")
			}
			if err := colimaVirt.Initialize(); err != nil {
				return fmt.Errorf("Error initializing colimaVirt: %w", err)
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
