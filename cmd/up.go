package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

var (
	installFlag bool // Declare the install flag
)

var upCmd = &cobra.Command{
	Use:          "up",
	Short:        "Set up the Windsor environment",
	Long:         "Set up the Windsor environment by executing necessary shell commands.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		// Create and initialize all necessary components for the Windsor environment.
		// This includes project, environment, virtualization, service, and stack components.
		if err := controller.CreateProjectComponents(); err != nil {
			return fmt.Errorf("Error creating project components: %w", err)
		}
		if err := controller.CreateEnvComponents(); err != nil {
			return fmt.Errorf("Error creating environment components: %w", err)
		}
		if err := controller.CreateVirtualizationComponents(); err != nil {
			return fmt.Errorf("Error creating virtualization components: %w", err)
		}
		if err := controller.CreateServiceComponents(); err != nil {
			return fmt.Errorf("Error creating services components: %w", err)
		}
		if err := controller.CreateStackComponents(); err != nil {
			return fmt.Errorf("Error creating stack components: %w", err)
		}
		if err := controller.InitializeComponents(); err != nil {
			return fmt.Errorf("Error initializing components: %w", err)
		}
		if err := controller.WriteConfigurationFiles(); err != nil {
			return fmt.Errorf("Error writing configuration files: %w", err)
		}

		// Resolve configuration settings and determine if specific virtualization or container runtime
		// actions are required based on the configuration.
		configHandler := controller.ResolveConfigHandler()
		if configHandler == nil {
			return fmt.Errorf("No config handler found")
		}
		vmDriver := configHandler.GetString("vm.driver")

		// Resolve the tools manager, check the tools, and install them
		toolsManager := controller.ResolveToolsManager()
		if toolsManager != nil {
			if err := toolsManager.Check(); err != nil {
				return fmt.Errorf("Error checking tools: %w", err)
			}
			if err := toolsManager.Install(); err != nil {
				return fmt.Errorf("Error installing tools: %w", err)
			}
		}

		// If the virtualization driver is 'colima', start the virtual machine and configure networking.
		if vmDriver == "colima" {
			virtualMachine := controller.ResolveVirtualMachine()
			if virtualMachine == nil {
				return fmt.Errorf("No virtual machine found")
			}
			if err := virtualMachine.Up(); err != nil {
				return fmt.Errorf("Error running virtual machine Up command: %w", err)
			}
		}

		// If the container runtime is enabled in the configuration, start it.
		containerRuntimeEnabled := configHandler.GetBool("docker.enabled")
		if containerRuntimeEnabled {
			containerRuntime := controller.ResolveContainerRuntime()
			if containerRuntime == nil {
				return fmt.Errorf("No container runtime found")
			}
			if err := containerRuntime.Up(); err != nil {
				return fmt.Errorf("Error running container runtime Up command: %w", err)
			}
		}

		// Resolve the network manager
		networkManager := controller.ResolveNetworkManager()
		if networkManager == nil {
			return fmt.Errorf("No network manager found")
		}

		// Configure networking for the virtual machine
		if vmDriver == "colima" {
			if err := networkManager.ConfigureGuest(); err != nil {
				return fmt.Errorf("Error configuring guest network: %w", err)
			}
			if err := networkManager.ConfigureHostRoute(); err != nil {
				return fmt.Errorf("Error configuring host network: %w", err)
			}
		}

		// Configure DNS settings
		if err := networkManager.ConfigureDNS(); err != nil {
			return fmt.Errorf("Error configuring DNS: %w", err)
		}

		// Start the stack components
		stack := controller.ResolveStack()
		if stack == nil {
			return fmt.Errorf("No stack found")
		}
		if err := stack.Up(); err != nil {
			return fmt.Errorf("Error running stack Up command: %w", err)
		}

		// If the install flag is set, install the blueprint to finalize the setup process.
		if installFlag {
			blueprintHandler := controller.ResolveBlueprintHandler()
			if blueprintHandler == nil {
				return fmt.Errorf("No blueprint handler found")
			}
			if err := blueprintHandler.Install(); err != nil {
				return fmt.Errorf("Error installing blueprint: %w", err)
			}
		}

		// Indicate successful setup of the Windsor environment.
		fmt.Fprintln(os.Stderr, "Windsor environment set up successfully.")

		return nil
	},
}

func init() {
	upCmd.Flags().BoolVar(&installFlag, "install", false, "Install the blueprint after setting up the environment")
	rootCmd.AddCommand(upCmd)
}
