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

		// Get the context configuration
		contextConfig, err := cliConfigHandler.GetConfig()
		if err != nil {
			if verbose {
				return fmt.Errorf("Error getting context configuration: %w", err)
			}
			return nil
		}

		// Determine if Colima is being used
		useColima := contextConfig.VM != nil && contextConfig.VM.Driver != nil && *contextConfig.VM.Driver == "colima"

		// Determine if Docker is being used
		useDocker := contextConfig.Docker != nil && *contextConfig.Docker.Enabled

		// Start Colima if it is being used
		if useColima {
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
		if useDocker {
			// Write the Docker configuration
			if err := dockerVirt.WriteConfig(); err != nil {
				return fmt.Errorf("Error writing Docker config: %w", err)
			}

			// Start the Docker VM
			if err := dockerVirt.Up(verbose); err != nil {
				return fmt.Errorf("Error running DockerVirt Up command: %w", err)
			}
		}

		// Configure the guest network
		if useColima {
			if err := colimaNetworkManager.ConfigureGuest(); err != nil {
				return fmt.Errorf("Error configuring guest network: %w", err)
			}
		}

		// Configure the host network
		if useColima {
			if err := colimaNetworkManager.ConfigureHost(); err != nil {
				return fmt.Errorf("Error configuring host network: %w", err)
			}
		}

		// Print welcome status page
		fmt.Println(color.CyanString("Welcome to the Windsor Environment 📐"))
		fmt.Println(color.CyanString("-------------------------------------"))

		// Print Colima info if available
		if useColima {
			if err := colimaVirt.PrintInfo(); err != nil {
				return fmt.Errorf("Error printing Colima info: %w", err)
			}
		}

		// Print Docker info if available
		if useDocker {
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
