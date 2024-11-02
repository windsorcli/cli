package cmd

import (
	"fmt"
	"net"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/helpers"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Set up the Windsor environment",
	Long:  "Set up the Windsor environment by executing necessary shell commands.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get the context name
		contextName, err := contextInstance.GetContext()
		if err != nil {
			return fmt.Errorf("Error getting context: %w", err)
		}

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

		// Configure route tables on the VM only if the VM driver is "colima" and Docker network CIDR is defined
		if contextConfig.VM != nil &&
			contextConfig.VM.Driver != nil &&
			*contextConfig.VM.Driver == "colima" &&
			contextConfig.Docker != nil &&
			contextConfig.Docker.Enabled != nil &&
			*contextConfig.Docker.Enabled &&
			contextConfig.Docker.NetworkCIDR != nil {
			// Execute "colima ssh-config --profile windsor-<context-name>"
			sshConfigOutput, err := shellInstance.Exec(
				verbose,
				"",
				"colima",
				"ssh-config",
				"--profile",
				fmt.Sprintf("windsor-%s", contextName),
			)
			if err != nil {
				return fmt.Errorf("Error executing Colima SSH config command: %w", err)
			}

			// Pass the contents to the sshClient
			if err := sshClient.SetClientConfigFile(sshConfigOutput, fmt.Sprintf("colima-windsor-%s", contextName)); err != nil {
				return fmt.Errorf("Error setting SSH client config: %w", err)
			}

			// Execute a command to get a list of network interfaces
			output, err := secureShellInstance.Exec(
				verbose,
				"",
				"ls",
				"/sys/class/net",
			)
			if err != nil {
				return fmt.Errorf("Error executing command to list network interfaces: %w", err)
			}

			// Find the name of the interface that starts with "br-"
			var dockerBridgeInterface string
			interfaces := strings.Split(output, "\n")
			for _, iface := range interfaces {
				if strings.HasPrefix(iface, "br-") {
					dockerBridgeInterface = iface
					break
				}
			}
			if dockerBridgeInterface == "" {
				return fmt.Errorf("Error: No interface starting with 'br-' found")
			}

			// Get Colima host IP from colimaInfo
			colimaHostIP := colimaInfo.Address

			// Determine the network interface associated with the Colima host IP
			var colimaInterfaceIP string
			colimaIP := net.ParseIP(colimaHostIP)
			if colimaIP == nil {
				return fmt.Errorf("Error parsing Colima host IP: %s", colimaHostIP)
			}
			netInterfaces, err := netInterfaces()
			if err != nil {
				return fmt.Errorf("Error getting network interfaces: %w", err)
			}
			for _, iface := range netInterfaces {
				addrs, err := iface.Addrs()
				if err != nil {
					return fmt.Errorf("Error getting addresses for interface %s: %w", iface.Name, err)
				}
				for _, addr := range addrs {
					ipNet, ok := addr.(*net.IPNet)
					if !ok {
						continue
					}
					if ipNet.Contains(colimaIP) {
						colimaInterfaceIP = ipNet.IP.String()
						break
					}
				}
				if colimaInterfaceIP != "" {
					break
				}
			}

			// Get cluster IPv4 CIDR from contextConfig
			clusterIPv4CIDR := *contextConfig.Docker.NetworkCIDR

			// Check if the iptables rule already exists
			_, err = secureShellInstance.Exec(
				verbose,
				"Checking for existing iptables rule...",
				"sudo", "iptables", "-t", "filter", "-C", "FORWARD",
				"-i", "col0", "-o", dockerBridgeInterface,
				"-s", colimaInterfaceIP, "-d", clusterIPv4CIDR, "-j", "ACCEPT",
			)
			if err != nil {
				// Check if the error is due to the rule not existing
				if strings.Contains(err.Error(), "Bad rule") {
					// Rule does not exist, proceed to add it
					if _, err := secureShellInstance.Exec(
						verbose,
						"Setting IP tables on Colima VM...",
						"sudo", "iptables", "-t", "filter", "-A", "FORWARD",
						"-i", "col0", "-o", dockerBridgeInterface,
						"-s", colimaInterfaceIP, "-d", clusterIPv4CIDR, "-j", "ACCEPT",
					); err != nil {
						return fmt.Errorf("Error setting iptables rule: %w", err)
					}
				} else {
					// An unexpected error occurred
					return fmt.Errorf("Error checking iptables rule: %w", err)
				}
			}

			// Add route on the host to VM guest
			output, err = shellInstance.Exec(
				false,
				"",
				"sudo",
				"route",
				"-nv",
				"add",
				"-net",
				clusterIPv4CIDR,
				colimaHostIP,
			)
			if err != nil {
				return fmt.Errorf("failed to add route: %w, output: %s", err, output)
			}
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
