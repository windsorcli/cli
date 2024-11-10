package cmd

import (
	"fmt"

	"github.com/fatih/color"
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

		// Determine if DNS is configured
		dnsName := configHandler.GetString("dns.name")

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

			// Start the Docker VM
			if err := dockerVirt.Up(verbose); err != nil {
				return fmt.Errorf("Error running DockerVirt Up command: %w", err)
			}
		}

		// Configure the network for Colima
		if driver == "colima" {
			if err := colimaNetworkManager.ConfigureGuest(); err != nil {
				return fmt.Errorf("Error configuring guest network: %w", err)
			}
			if err := colimaNetworkManager.ConfigureHost(); err != nil {
				return fmt.Errorf("Error configuring host network: %w", err)
			}
		}

		// Configure DNS if dns.name is set
		if dnsName != "" {
			if err := colimaNetworkManager.ConfigureDNS(); err != nil {
				return fmt.Errorf("Error configuring DNS: %w", err)
			}
		}

		// Print welcome status page
		fmt.Println(color.CyanString("Welcome to the Windsor Environment 📐"))
		fmt.Println(color.CyanString("-------------------------------------"))
		fmt.Println()

		// Print Colima info if available
		if driver == "colima" {
			if err := colimaVirt.PrintInfo(); err != nil {
				return fmt.Errorf("Error printing Colima info: %w", err)
			}
		}

		// Print Docker info if available
		if dockerEnabled {
			if err := dockerVirt.PrintInfo(); err != nil {
				return fmt.Errorf("Error printing Docker info: %w", err)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
}
