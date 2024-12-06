package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:          "up",
	Short:        "Set up the Windsor environment",
	Long:         "Set up the Windsor environment by executing necessary shell commands.",
	SilenceUsage: true,
	PreRunE:      preRunEInitializeCommonComponents,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Create service components
		if err := controller.CreateServiceComponents(); err != nil {
			return fmt.Errorf("Error creating services components: %w", err)
		}

		// Create virtualization components
		if err := controller.CreateVirtualizationComponents(); err != nil {
			return fmt.Errorf("Error creating virtualization components: %w", err)
		}

		// Initialize all components
		if err := controller.InitializeComponents(); err != nil {
			return fmt.Errorf("Error initializing components: %w", err)
		}

		// Write configuration files
		if err := controller.WriteConfigurationFiles(); err != nil {
			return fmt.Errorf("Error writing configuration files: %w", err)
		}

		// Resolve the config handler
		configHandler := controller.ResolveConfigHandler()
		if configHandler == nil {
			return fmt.Errorf("No config handler found")
		}

		// Determine if a virtualization driver is being used
		vmDriver := configHandler.GetString("vm.driver")

		// Start the virtual machine if enabled in configuration
		if vmDriver != "" {
			virtualMachine := controller.ResolveVirtualMachine()
			if virtualMachine == nil {
				return fmt.Errorf("No virtual machine found")
			}
			if err := virtualMachine.Up(verbose); err != nil {
				return fmt.Errorf("Error running virtual machine Up command: %w", err)
			}
		}

		// Start the container runtime if enabled in configuration
		containerRuntimeEnabled := configHandler.GetBool("docker.enabled")

		// Configure container runtime if enabled in configuration
		if containerRuntimeEnabled {
			// Resolve container runtime
			containerRuntime := controller.ResolveContainerRuntime()
			if containerRuntime == nil {
				return fmt.Errorf("No container runtime found")
			}

			// Run the container runtime Up command
			if err := containerRuntime.Up(verbose); err != nil {
				return fmt.Errorf("Error running container runtime Up command: %w", err)
			}
		}

		// Configure networking only if a VM driver is defined
		if vmDriver != "" {
			// Get the DNS name and address
			dnsName := configHandler.GetString("dns.name")
			dnsAddress := configHandler.GetString("dns.address")

			// Resolve networkManager
			networkManager := controller.ResolveNetworkManager()
			if networkManager == nil {
				return fmt.Errorf("No network manager found")
			}

			// Configure the guest network
			if err := networkManager.ConfigureGuest(); err != nil {
				return fmt.Errorf("Error configuring guest network: %w", err)
			}

			// Configure the host route for the network
			if err := networkManager.ConfigureHostRoute(); err != nil {
				return fmt.Errorf("Error configuring host network: %w", err)
			}

			// Configure DNS if dns.name is set
			if dnsName != "" && dnsAddress != "" {
				if err := networkManager.ConfigureDNS(); err != nil {
					return fmt.Errorf("Error configuring DNS: %w", err)
				}
			}
		}

		// Print success message
		fmt.Println("Windsor environment set up successfully.")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
}
