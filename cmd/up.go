package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/network"
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

		// Handle when there is no contextConfig configured
		if contextConfig == nil {
			return nil
		}

		// Check if the VM driver is "colima"
		var colimaInfo *helpers.ColimaInfo
		if contextConfig.VM != nil && contextConfig.VM.Driver != nil && *contextConfig.VM.Driver == "colima" {
			// Run the "Up" command of the ColimaHelper
			if err := colimaHelper.Up(verbose); err != nil {
				return fmt.Errorf("Error running ColimaHelper Up command: %w", err)
			}
			// Get and hold on to colima's info
			info, err := colimaHelper.Info()
			if err != nil {
				return fmt.Errorf("Error retrieving Colima info: %w", err)
			}
			colimaInfo = info.(*helpers.ColimaInfo)
		}

		// Check if Docker is enabled
		var dockerInfo *helpers.DockerInfo
		if contextConfig.Docker != nil && *contextConfig.Docker.Enabled {
			// Run the "Up" command of the DockerHelper
			if err := dockerHelper.Up(verbose); err != nil {
				return fmt.Errorf("Error running DockerHelper Up command: %w", err)
			}
			// Get and hold on to Docker's info
			info, err := dockerHelper.Info()
			if err != nil {
				return fmt.Errorf("Error retrieving Docker info: %w", err)
			}
			// Type assertion to *helpers.DockerInfo
			dockerInfo = info.(*helpers.DockerInfo)
		}

		// Configure the network
		if _, err := networkManager.Configure(&network.NetworkConfig{}); err != nil {
			return fmt.Errorf("Error configuring network: %w", err)
		}

		// Print welcome status page
		fmt.Println(color.CyanString("Welcome to the Windsor Environment 📐"))
		fmt.Println(color.CyanString("-------------------------------------"))

		// Print Colima info if available
		if colimaInfo != nil {
			fmt.Println(color.GreenString("Colima VM Info:"))
			fmt.Printf("  Address: %s\n", colimaInfo.Address)
			fmt.Printf("  Arch: %s\n", colimaInfo.Arch)
			fmt.Printf("  CPUs: %d\n", colimaInfo.CPUs)
			fmt.Printf("  Disk: %.2f GB\n", colimaInfo.Disk)
			fmt.Printf("  Memory: %.2f GB\n", colimaInfo.Memory)
			fmt.Printf("  Name: %s\n", colimaInfo.Name)
			fmt.Printf("  Runtime: %s\n", colimaInfo.Runtime)
			fmt.Printf("  Status: %s\n", colimaInfo.Status)
			fmt.Println(color.CyanString("-------------------------------------"))
		}

		// Print Docker info if available
		if dockerInfo != nil {
			fmt.Println(color.GreenString("Docker Info:"))
			for role, services := range dockerInfo.Services {
				fmt.Println(color.YellowString("  %s:", role))
				for _, service := range services {
					fmt.Printf("    %s\n", service)
				}
			}
			fmt.Println(color.CyanString("-------------------------------------"))
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
}
