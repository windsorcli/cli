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
	RunE: func(cmd *cobra.Command, args []string) error {
		// Resolve configHandler
		configHandler, err := controller.ResolveConfigHandler()
		if err != nil {
			return fmt.Errorf("Error resolving configHandler: %w", err)
		}

		// Resolve and initialize shell
		shellInstance, err := controller.ResolveShell()
		if err != nil {
			return fmt.Errorf("Error resolving shell: %w", err)
		}
		if err := shellInstance.Initialize(); err != nil {
			return fmt.Errorf("Error initializing shell: %w", err)
		}

		// Resolve and initialize secureShell
		secureShell, err := controller.ResolveSecureShell()
		if err != nil {
			return fmt.Errorf("Error resolving secureShell: %w", err)
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
			colimaVirt, err := controller.ResolveVirtualMachine()
			if err != nil {
				return fmt.Errorf("Error resolving colimaVirt: %w", err)
			}
			if err := colimaVirt.Up(verbose); err != nil {
				return fmt.Errorf("Error running Colima VM Up command: %w", err)
			}
		}

		// Configure DockerVirt if enabled in configuration
		if dockerEnabled {
			// Resolve dockerVirt
			dockerVirt, err := controller.ResolveContainerRuntime()
			if err != nil {
				return fmt.Errorf("Error resolving dockerVirt: %w", err)
			}

			// Resolve dnsService
			dnsService, err := controller.ResolveService("dnsService")
			if err != nil {
				return fmt.Errorf("Error resolving dnsService: %w", err)
			}

			// Write the docker-compose file
			if err := dockerVirt.WriteConfig(); err != nil {
				return fmt.Errorf("Error writing docker-compose file: %w", err)
			}

			// Write the DNS configuration
			if createDns {
				if err := dnsService.WriteConfig(); err != nil {
					return fmt.Errorf("Error writing DNS config: %w", err)
				}
			}

			// Run the DockerVirt Up command
			if err := dockerVirt.Up(verbose); err != nil {
				return fmt.Errorf("Error running DockerVirt Up command: %w", err)
			}

			// Get the DNS address
			if dnsName != "" {
				dnsServiceInfo, err := dockerVirt.GetContainerInfo("dns.test")
				if err != nil {
					return fmt.Errorf("Error getting DNS service: %w", err)
				}
				if len(dnsServiceInfo) == 0 {
					return fmt.Errorf("DNS service not found")
				}
				dnsAddress = dnsServiceInfo[0].Address
			}
		}

		// Resolve colimaNetworkManager
		colimaNetworkManager, err := controller.ResolveNetworkManager()
		if err != nil {
			return fmt.Errorf("Error resolving colimaNetworkManager: %w", err)
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
