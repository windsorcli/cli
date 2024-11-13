package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Set up the Windsor environment",
	Long:  "Set up the Windsor environment by executing necessary shell commands.",
	RunE: func(cmd *cobra.Command, args []string) error {
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

		// Start Colima if it is being used
		if driver == "colima" {
			// Write the Colima configuration
			if err := colimaVirt.WriteConfig(); err != nil {
				return fmt.Errorf("Error writing Colima config: %w", err)
			}

			// Start the Colima VM
			if err := colimaVirt.Up(verbose); err != nil {
				return fmt.Errorf("Error running Colima VM Up command: %w", err)
			}
		}

		// Start Docker if it is being used
		if dockerEnabled {
			// Write the Docker configuration
			if err := dockerVirt.WriteConfig(); err != nil {
				return fmt.Errorf("Error writing Docker config: %w", err)
			}

			// Write the DNS configuration
			if createDns {
				if err := dnsHelper.WriteConfig(); err != nil {
					return fmt.Errorf("Error writing DNS config: %w", err)
				}
			}

			// Start the Docker VM
			if err := dockerVirt.Up(verbose); err != nil {
				return fmt.Errorf("Error running DockerVirt Up command: %w", err)
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
