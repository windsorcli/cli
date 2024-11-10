package network

import (
	"fmt"
	"strings"

	"github.com/windsor-hotel/cli/internal/di"
)

// colimaNetworkManager is a concrete implementation of NetworkManager
type colimaNetworkManager struct {
	networkManager
}

// NewColimaNetworkManager creates a new ColimaNetworkManager
func NewColimaNetworkManager(injector di.Injector) NetworkManager {
	nm := &colimaNetworkManager{
		networkManager: networkManager{
			injector: injector,
		},
	}
	return nm
}

// ConfigureGuest forwards the incoming guest traffic to the container network
func (n *colimaNetworkManager) ConfigureGuest() error {
	// Access the Docker configuration
	networkCIDR := n.configHandler.GetString("docker.network_cidr")
	if networkCIDR == "" {
		return fmt.Errorf("network CIDR is not configured")
	}

	// Access the VM IP
	guestIP := n.configHandler.GetString("vm.address")
	if guestIP == "" {
		return fmt.Errorf("guest IP is not configured")
	}

	contextName := "default"
	sshConfigOutput, err := n.shell.Exec(
		false,
		"",
		"colima",
		"ssh-config",
		"--profile",
		fmt.Sprintf("windsor-%s", contextName),
	)
	if err != nil {
		return fmt.Errorf("error executing VM SSH config command: %w", err)
	}

	// Pass the contents to the sshClient
	if err := n.sshClient.SetClientConfigFile(sshConfigOutput, fmt.Sprintf("colima-windsor-%s", contextName)); err != nil {
		return fmt.Errorf("error setting SSH client config: %w", err)
	}

	// Execute a command to get a list of network interfaces
	output, err := n.secureShell.Exec(
		false,
		"",
		"ls",
		"/sys/class/net",
	)
	if err != nil {
		return fmt.Errorf("error executing command to list network interfaces: %w", err)
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
		return fmt.Errorf("error: no docker bridge interface found")
	}

	// Check if the iptables rule already exists
	_, err = n.secureShell.Exec(
		false,
		"Checking for existing iptables rule...",
		"sudo", "iptables", "-t", "filter", "-C", "FORWARD",
		"-i", "col0", "-o", dockerBridgeInterface,
		"-s", guestIP, "-d", networkCIDR, "-j", "ACCEPT",
	)
	if err != nil {
		// Check if the error is due to the rule not existing
		if strings.Contains(err.Error(), "Bad rule") {
			// Rule does not exist, proceed to add it
			if _, err := n.secureShell.Exec(
				false,
				"Setting IP tables on VM...",
				"sudo", "iptables", "-t", "filter", "-A", "FORWARD",
				"-i", "col0", "-o", dockerBridgeInterface,
				"-s", guestIP, "-d", networkCIDR, "-j", "ACCEPT",
			); err != nil {
				return fmt.Errorf("error setting iptables rule: %w", err)
			}
		} else {
			// An unexpected error occurred
			return fmt.Errorf("error checking iptables rule: %w", err)
		}
	}

	return nil
}

// Ensure colimaNetworkManager implements NetworkManager
var _ NetworkManager = (*colimaNetworkManager)(nil)
