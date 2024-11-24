package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/network"
	"github.com/windsor-hotel/cli/internal/services"
	"github.com/windsor-hotel/cli/internal/shell"
	"github.com/windsor-hotel/cli/internal/virt"
)

var upCmd = &cobra.Command{
	Use:          "up",
	Short:        "Set up the Windsor environment",
	Long:         "Set up the Windsor environment by executing necessary shell commands.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Resolve configHandler
		configHandlerInstance, err := injector.Resolve("configHandler")
		if err != nil {
			return fmt.Errorf("Error resolving configHandler: %w", err)
		}
		configHandler, ok := configHandlerInstance.(config.ConfigHandler)
		if !ok {
			return fmt.Errorf("Resolved instance is not of type config.ConfigHandler")
		}

		// Resolve and initialize shell
		shellInstance, err := injector.Resolve("shell")
		if err != nil {
			return fmt.Errorf("Error resolving shell: %w", err)
		}
		shellInstanceResolved, ok := shellInstance.(shell.Shell)
		if !ok {
			return fmt.Errorf("Resolved instance is not of type shell.Shell")
		}
		if err := shellInstanceResolved.Initialize(); err != nil {
			return fmt.Errorf("Error initializing shell: %w", err)
		}

		// Resolve and initialize secureShell
		secureShellInstance, err := injector.Resolve("secureShell")
		if err != nil {
			return fmt.Errorf("Error resolving secureShell: %w", err)
		}
		secureShell, ok := secureShellInstance.(shell.Shell)
		if !ok {
			return fmt.Errorf("Resolved instance is not of type shell.Shell")
		}
		if err := secureShell.Initialize(); err != nil {
			return fmt.Errorf("Error initializing secureShell: %w", err)
		}

		// Call the init command
		if err := initCmd.RunE(cmd, args); err != nil {
			return fmt.Errorf("Error running init command: %w", err)
		}

		// Determine if Colima is being used
		driver := configHandler.GetString("vm.driver")

		// Determine if Docker is being used
		dockerEnabled := configHandler.GetBool("docker.enabled")

		// Get the DNS name
		dnsName := configHandler.GetString("dns.name")

		// Initialize the DNS address
		dnsAddress := ""

		// Get the DNS create flag
		createDns := configHandler.GetBool("dns.create")

		// Configure ColimaVirt if enabled in configuration
		if driver == "colima" {
			// Resolve colimaVirt
			colimaVirtInstance, err := injector.Resolve("colimaVirt")
			if err != nil {
				return fmt.Errorf("Error resolving colimaVirt: %w", err)
			}
			colimaVirt, ok := colimaVirtInstance.(virt.VirtualMachine)
			if !ok {
				return fmt.Errorf("Resolved instance is not of type virt.VirtualMachine")
			}
			if err := colimaVirt.Up(verbose); err != nil {
				return fmt.Errorf("Error running Colima VM Up command: %w", err)
			}
		}

		// Configure DockerVirt if enabled in configuration
		if dockerEnabled {
			// Resolve dockerVirt
			dockerVirtInstance, err := injector.Resolve("dockerVirt")
			if err != nil {
				return fmt.Errorf("Error resolving dockerVirt: %w", err)
			}
			dockerVirt, ok := dockerVirtInstance.(virt.ContainerRuntime)
			if !ok {
				return fmt.Errorf("Resolved instance is not of type virt.ContainerRuntime")
			}

			// Resolve dnsService
			dnsServiceInstance, err := injector.Resolve("dnsService")
			if err != nil {
				return fmt.Errorf("Error resolving dnsService: %w", err)
			}
			dnsService, ok := dnsServiceInstance.(services.Service)
			if !ok {
				return fmt.Errorf("Resolved instance is not of type services.Service")
			}

			// Write the DNS configuration
			if createDns {
				if err := dnsService.WriteConfig(); err != nil {
					return fmt.Errorf("Error writing DNS config: %w", err)
				}
			}

			// Get the DNS address
			if dnsName != "" {
				dnsService, err := dockerVirt.GetContainerInfo("dns.test")
				if err != nil {
					return fmt.Errorf("Error getting DNS service: %w", err)
				}
				if len(dnsService) == 0 {
					return fmt.Errorf("DNS service not found")
				}
				dnsAddress = dnsService[0].Address
			}

			// Run the DockerVirt Up command
			if err := dockerVirt.Up(verbose); err != nil {
				return fmt.Errorf("Error running DockerVirt Up command: %w", err)
			}
		}

		// Resolve colimaNetworkManager
		colimaNetworkManagerInstance, err := injector.Resolve("colimaNetworkManager")
		if err != nil {
			return fmt.Errorf("Error resolving colimaNetworkManager: %w", err)
		}
		colimaNetworkManager, ok := colimaNetworkManagerInstance.(network.NetworkManager)
		if !ok {
			return fmt.Errorf("Resolved instance is not of type network.NetworkManager")
		}

		// Initialize the network manager
		if err := colimaNetworkManager.Initialize(); err != nil {
			return fmt.Errorf("Error initializing network manager: %w", err)
		}

		// Configure the network for Colima
		if driver == "colima" {
			if err := colimaNetworkManager.ConfigureGuest(); err != nil {
				return fmt.Errorf("Error configuring guest network: %w", err)
			}
			if err := colimaNetworkManager.ConfigureHostRoute(); err != nil {
				return fmt.Errorf("Error configuring host network: %w", err)
			}
		}

		// Configure DNS if dns.name is set
		if dnsName != "" && dnsAddress != "" {

			// Begin hack to get the DNS address into the config
			configData := configHandler.GetConfig()
			configData.DNS.Address = &dnsAddress
			// End hack

			if err := colimaNetworkManager.ConfigureDNS(); err != nil {
				return fmt.Errorf("Error configuring DNS: %w", err)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
}
