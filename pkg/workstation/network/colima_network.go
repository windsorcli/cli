package network

import (
	"fmt"
	"net"
	"strings"

	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/shell/ssh"
)

// The ColimaNetworkManager is a specialized network manager for Colima-based environments.
// It provides Colima-specific network configuration including SSH setup, iptables rules,
// The ColimaNetworkManager extends the base network manager with Colima-specific functionality,
// handling guest VM networking, host-guest communication, and Docker bridge integration.

// =============================================================================
// Types
// =============================================================================

// colimaNetworkManager is a concrete implementation of NetworkManager
type ColimaNetworkManager struct {
	BaseNetworkManager
	networkInterfaceProvider NetworkInterfaceProvider
}

// =============================================================================
// Constructor
// =============================================================================

// NewColimaNetworkManager creates a new ColimaNetworkManager
func NewColimaNetworkManager(rt *runtime.Runtime, sshClient ssh.Client, secureShell shell.Shell, networkInterfaceProvider NetworkInterfaceProvider) *ColimaNetworkManager {
	if rt == nil {
		panic("runtime is required")
	}
	if sshClient == nil {
		panic("ssh client is required")
	}
	if secureShell == nil {
		panic("secure shell is required")
	}
	if networkInterfaceProvider == nil {
		panic("network interface provider is required")
	}

	manager := &ColimaNetworkManager{
		BaseNetworkManager:       *NewBaseNetworkManager(rt),
		networkInterfaceProvider: networkInterfaceProvider,
	}
	manager.sshClient = sshClient
	manager.secureShell = secureShell

	return manager
}

// ConfigureGuest sets up forwarding of guest traffic to the container network.
// It retrieves network CIDR and guest IP from the config, and configures SSH.
// For Docker runtime, it identifies the Docker bridge interface and ensures iptables rules are set.
// For incus runtime, Docker-specific configuration is skipped since incus networking is handled independently.
// If vm.address is not configured, only SSH configuration is performed (useful for Down operations).
func (n *ColimaNetworkManager) ConfigureGuest() error {
	contextName := n.configHandler.GetContext()

	sshConfigOutput, err := n.shell.ExecSilent(
		"colima",
		"ssh-config",
		"--profile",
		fmt.Sprintf("windsor-%s", contextName),
	)
	if err != nil {
		return fmt.Errorf("error executing VM SSH config command: %w", err)
	}

	if err := n.sshClient.SetClientConfigFile(sshConfigOutput, fmt.Sprintf("colima-windsor-%s", contextName)); err != nil {
		return fmt.Errorf("error setting SSH client config: %w", err)
	}

	guestIP := n.configHandler.GetString("vm.address")
	if guestIP == "" {
		return nil
	}

	networkCIDR := n.configHandler.GetString("network.cidr_block", constants.DefaultNetworkCIDR)

	vmRuntime := n.configHandler.GetString("vm.runtime", "docker")
	if vmRuntime == "incus" {
		if err := n.configureIncusNetwork(networkCIDR); err != nil {
			return fmt.Errorf("error configuring incus network: %w", err)
		}
		return n.setupForwardingRule(networkCIDR, "incusbr0")
	}
	return n.configureDockerForwarding(networkCIDR)
}

// =============================================================================
// Private Methods
// =============================================================================

// configureDockerForwarding sets up iptables forwarding from col0 to the Docker bridge interface
// It identifies the Docker bridge interface and configures forwarding rules
func (n *ColimaNetworkManager) configureDockerForwarding(networkCIDR string) error {
	output, err := n.secureShell.ExecSilent(
		"ls",
		"/sys/class/net",
	)
	if err != nil {
		return fmt.Errorf("error executing command to list network interfaces: %w", err)
	}

	var dockerBridgeInterface string
	for _, iface := range strings.FieldsFunc(output, func(r rune) bool { return r == '\n' }) {
		if strings.HasPrefix(iface, "br-") {
			dockerBridgeInterface = iface
			break
		}
	}
	if dockerBridgeInterface == "" {
		return fmt.Errorf("error: no docker bridge interface found")
	}

	return n.setupForwardingRule(networkCIDR, dockerBridgeInterface)
}

// configureIncusNetwork configures the Incus bridge network with the specified CIDR
// It calculates the gateway IP (first IP in the CIDR) and sets it on incusbr0 using the CIDR mask from the config
func (n *ColimaNetworkManager) configureIncusNetwork(networkCIDR string) error {
	_, ipNet, err := net.ParseCIDR(networkCIDR)
	if err != nil {
		return fmt.Errorf("error parsing network CIDR: %w", err)
	}

	ones, _ := ipNet.Mask.Size()
	gatewayIP := incrementIP(ipNet.IP).String()
	if _, err := n.secureShell.ExecSilent(
		"incus", "network", "set", "incusbr0", fmt.Sprintf("ipv4.address=%s/%d", gatewayIP, ones),
	); err != nil {
		return fmt.Errorf("error setting incus network address: %w", err)
	}

	return nil
}

// setupForwardingRule sets up iptables forwarding from col0 to the specified output interface
// It enables IP forwarding and adds the necessary iptables rule
func (n *ColimaNetworkManager) setupForwardingRule(networkCIDR, outputInterface string) error {
	hostIP, err := n.getHostIP()
	if err != nil {
		return fmt.Errorf("error getting host IP: %w", err)
	}

	if _, err := n.secureShell.ExecSilent(
		"sudo", "sysctl", "-w", "net.ipv4.ip_forward=1",
	); err != nil {
		return fmt.Errorf("error enabling IP forwarding in VM: %w", err)
	}

	_, err = n.secureShell.ExecSilent(
		"sudo", "iptables", "-t", "filter", "-C", "FORWARD",
		"-i", "col0", "-o", outputInterface,
		"-s", hostIP, "-d", networkCIDR, "-j", "ACCEPT",
	)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "No chain/target/match") || strings.Contains(errStr, "Bad rule") {
			if _, err := n.secureShell.ExecSilent(
				"sudo", "iptables", "-t", "filter", "-A", "FORWARD",
				"-i", "col0", "-o", outputInterface,
				"-s", hostIP, "-d", networkCIDR, "-j", "ACCEPT",
			); err != nil {
				return fmt.Errorf("error setting iptables rule: %w", err)
			}
		} else {
			return fmt.Errorf("error checking iptables rule: %w", err)
		}
	}

	return nil
}

// getHostIP retrieves the host IP address that shares the same subnet as the guest IP address.
// It first obtains and validates the guest IP from the configuration. Then, it iterates over the network interfaces
// to find an IP address that belongs to the same subnet as the guest IP. If found, it returns this host IP address.
func (n *ColimaNetworkManager) getHostIP() (string, error) {
	guestIP := n.configHandler.GetString("vm.address")
	if guestIP == "" {
		return "", fmt.Errorf("guest IP is not configured")
	}

	guestIPAddr := net.ParseIP(guestIP)
	if guestIPAddr == nil {
		return "", fmt.Errorf("invalid guest IP address")
	}

	interfaces, err := n.networkInterfaceProvider.Interfaces()
	if err != nil {
		return "", fmt.Errorf("failed to get network interfaces: %w", err)
	}

	for _, iface := range interfaces {
		addrs, err := n.networkInterfaceProvider.InterfaceAddrs(iface)
		if err != nil {
			return "", fmt.Errorf("failed to get addresses for interface %s: %w", iface.Name, err)
		}

		for _, addr := range addrs {
			var ipNet *net.IPNet
			switch v := addr.(type) {
			case *net.IPNet:
				ipNet = v
			case *net.IPAddr:
				ipNet = &net.IPNet{IP: v.IP, Mask: v.IP.DefaultMask()}
			}

			if ipNet != nil && ipNet.Contains(guestIPAddr) {
				return ipNet.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("failed to find host IP in the same subnet as guest IP")
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure ColimaNetworkManager implements NetworkManager
var _ NetworkManager = (*ColimaNetworkManager)(nil)
