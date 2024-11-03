package cmd

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/vm"
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

		// Check if the VM is configured and the driver is Colima
		var vmInfo *vm.VMInfo
		if contextConfig.VM != nil && contextConfig.VM.Driver != nil && *contextConfig.VM.Driver == "colima" {
			// Use the Colima VM instance
			if err := colimaVM.Up(verbose); err != nil {
				return fmt.Errorf("Error running Colima VM Up command: %w", err)
			}
			// Get and hold on to VM's info
			info, err := colimaVM.Info()
			if err != nil {
				return fmt.Errorf("Error retrieving Colima VM info: %w", err)
			}
			vmInfo = info.(*vm.VMInfo)
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

		// Configure route tables on the VM only if Docker network CIDR is defined
		if contextConfig.VM != nil &&
			contextConfig.VM.Driver != nil &&
			*contextConfig.VM.Driver == "colima" &&
			contextConfig.Docker != nil &&
			contextConfig.Docker.Enabled != nil &&
			*contextConfig.Docker.Enabled &&
			contextConfig.Docker.NetworkCIDR != nil {
			// Execute VM-specific SSH config command
			sshConfigOutput, err := shellInstance.Exec(
				verbose,
				"",
				"colima",
				"ssh-config",
				"--profile",
				fmt.Sprintf("windsor-%s", contextName),
			)
			if err != nil {
				return fmt.Errorf("Error executing VM SSH config command: %w", err)
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

			// Get VM host IP from vmInfo
			vmHostIP := vmInfo.Address

			// Determine the network interface associated with the VM host IP
			var vmInterfaceIP string
			vmIP := net.ParseIP(vmHostIP)
			if vmIP == nil {
				return fmt.Errorf("Error parsing VM host IP: %s", vmHostIP)
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
					if ipNet.Contains(vmIP) {
						vmInterfaceIP = ipNet.IP.String()
						break
					}
				}
				if vmInterfaceIP != "" {
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
				"-s", vmInterfaceIP, "-d", clusterIPv4CIDR, "-j", "ACCEPT",
			)
			if err != nil {
				// Check if the error is due to the rule not existing
				if strings.Contains(err.Error(), "Bad rule") {
					// Rule does not exist, proceed to add it
					if _, err := secureShellInstance.Exec(
						verbose,
						"Setting IP tables on VM...",
						"sudo", "iptables", "-t", "filter", "-A", "FORWARD",
						"-i", "col0", "-o", dockerBridgeInterface,
						"-s", vmInterfaceIP, "-d", clusterIPv4CIDR, "-j", "ACCEPT",
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
				vmHostIP,
			)
			if err != nil {
				return fmt.Errorf("failed to add route: %w, output: %s", err, output)
			}

			// Update DNS settings for the private TLD (MacOS)
			if contextConfig.DNS != nil && contextConfig.DNS.Name != nil {
				tld := *contextConfig.DNS.Name

				// Get the IP address of the container called "dns.test"
				var dnsIP string
				if contextConfig.DNS.IP != nil {
					dnsIP = *contextConfig.DNS.IP
				} else if dockerInfo != nil {
					if serviceInfo, exists := dockerInfo.Services["dns.test"]; exists {
						dnsIP = serviceInfo.IP
					}
				}

				if dnsIP == "" {
					return fmt.Errorf("Error: No IP address found for dns.test")
				}

				// Ensure the /etc/resolver directory exists
				resolverDir := "/etc/resolver"
				if _, err := os.Stat(resolverDir); os.IsNotExist(err) {
					if _, err := shellInstance.Exec(
						false,
						"",
						"sudo",
						"mkdir",
						"-p",
						resolverDir,
					); err != nil {
						return fmt.Errorf("Error creating resolver directory: %w", err)
					}
				}

				// Write the DNS server to a temporary file
				tempResolverFile := fmt.Sprintf("/tmp/%s", tld)
				content := fmt.Sprintf("nameserver %s\n", dnsIP)
				// #nosec G306 - /etc/resolver files require 0644 permissions
				if err := os.WriteFile(tempResolverFile, []byte(content), 0644); err != nil {
					return fmt.Errorf("Error writing to temporary resolver file: %w", err)
				}

				// Move the temporary file to the /etc/resolver/<tld> file
				resolverFile := fmt.Sprintf("%s/%s", resolverDir, tld)
				if _, err := shellInstance.Exec(
					false,
					"",
					"sudo",
					"mv",
					tempResolverFile,
					resolverFile,
				); err != nil {
					return fmt.Errorf("Error moving resolver file: %w", err)
				}

				// Flush the DNS cache
				if _, err := shellInstance.Exec(false, "", "sudo", "dscacheutil", "-flushcache"); err != nil {
					return fmt.Errorf("Error flushing DNS cache: %w", err)
				}
				if _, err := shellInstance.Exec(false, "", "sudo", "killall", "-HUP", "mDNSResponder"); err != nil {
					return fmt.Errorf("Error restarting mDNSResponder: %w", err)
				}
			}
		}

		// Print welcome status page
		fmt.Println(color.CyanString("Welcome to the Windsor Environment 📐"))
		fmt.Println(color.CyanString("-------------------------------------"))

		// Print VM info if available
		if vmInfo != nil {
			fmt.Println(color.GreenString("VM Info:"))
			fmt.Printf("  Address: %s\n", vmInfo.Address)
			fmt.Printf("  Arch: %s\n", vmInfo.Arch)
			fmt.Printf("  CPUs: %d\n", vmInfo.CPUs)
			fmt.Printf("  Disk: %.2f GB\n", vmInfo.Disk)
			fmt.Printf("  Memory: %.2f GB\n", vmInfo.Memory)
			fmt.Printf("  Name: %s\n", vmInfo.Name)
			fmt.Printf("  Runtime: %s\n", vmInfo.Runtime)
			fmt.Printf("  Status: %s\n", vmInfo.Status)
			fmt.Println(color.CyanString("-------------------------------------"))
		}

		// Print Docker info if available
		if dockerInfo != nil {
			fmt.Println(color.GreenString("Docker Info:"))
			for serviceName, serviceInfo := range dockerInfo.Services {
				fmt.Println(color.YellowString("  %s:", serviceName))
				fmt.Printf("    Role: %s\n", serviceInfo.Role)
				fmt.Printf("    IP: %s\n", serviceInfo.IP)
			}
			fmt.Println(color.CyanString("-------------------------------------"))
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
}
