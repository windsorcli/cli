package network

import (
	"fmt"
	"net"
	"strings"

	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/ssh"
)

// colimaNetworkManager is a concrete implementation of NetworkManager
type ColimaNetworkManager struct {
	BaseNetworkManager
}

// NewColimaNetworkManager creates a new ColimaNetworkManager
func NewColimaNetworkManager(injector di.Injector) *ColimaNetworkManager {
	nm := &ColimaNetworkManager{
		BaseNetworkManager: BaseNetworkManager{
			injector: injector,
		},
	}
	return nm
}

// Initialize sets up the ColimaNetworkManager by resolving dependencies for
// sshClient, shell, and secureShell from the injector.
func (n *ColimaNetworkManager) Initialize() error {
	if err := n.BaseNetworkManager.Initialize(); err != nil {
		return err
	}

	if err := n.resolveDependencies(); err != nil {
		return err
	}

	// Set docker.NetworkCIDR to the default value if it's not set
	if n.configHandler.GetString("network.cidr_block") == "" {
		return n.configHandler.SetContextValue("network.cidr_block", constants.DEFAULT_NETWORK_CIDR)
	}

	return nil
}

func (n *ColimaNetworkManager) resolveDependencies() error {
	sshClient, ok := n.injector.Resolve("sshClient").(ssh.Client)
	if !ok {
		return fmt.Errorf("resolved ssh client instance is not of type ssh.Client")
	}
	n.sshClient = sshClient

	secureShell, ok := n.injector.Resolve("secureShell").(shell.Shell)
	if !ok {
		return fmt.Errorf("resolved secure shell instance is not of type shell.Shell")
	}
	n.secureShell = secureShell

	networkInterfaceProvider, ok := n.injector.Resolve("networkInterfaceProvider").(NetworkInterfaceProvider)
	if !ok {
		return fmt.Errorf("failed to resolve network interface provider")
	}
	n.networkInterfaceProvider = networkInterfaceProvider

	return nil
}

// ConfigureGuest sets up forwarding of guest traffic to the container network.
// It retrieves network CIDR and guest IP from the config, and configures SSH.
// It identifies the Docker bridge interface and ensures iptables rules are set.
// If the rule doesn't exist, it adds a new one to allow traffic forwarding.
func (n *ColimaNetworkManager) ConfigureGuest() error {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" {
		return fmt.Errorf("network CIDR is not configured")
	}

	guestIP := n.configHandler.GetString("vm.address")
	if guestIP == "" {
		return fmt.Errorf("guest IP is not configured")
	}

	contextName := n.configHandler.GetContext()

	sshConfigOutput, _, err := n.shell.ExecSilent(
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

	output, _, err := n.secureShell.ExecSilent(
		"ls",
		"/sys/class/net",
	)
	if err != nil {
		return fmt.Errorf("error executing command to list network interfaces: %w", err)
	}

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

	hostIP, err := n.getHostIP()
	if err != nil {
		return fmt.Errorf("error getting host IP: %w", err)
	}

	_, _, err = n.secureShell.ExecSilent(
		"sudo", "iptables", "-t", "filter", "-C", "FORWARD",
		"-i", "col0", "-o", dockerBridgeInterface,
		"-s", hostIP, "-d", networkCIDR, "-j", "ACCEPT",
	)
	if err != nil {
		if strings.Contains(err.Error(), "Bad rule") {
			_, _, err = n.secureShell.ExecSilent(
				"sudo", "iptables", "-t", "filter", "-A", "FORWARD",
				"-i", "col0", "-o", dockerBridgeInterface,
				"-s", hostIP, "-d", networkCIDR, "-j", "ACCEPT",
			)
			if err != nil {
				return fmt.Errorf("error setting iptables rule: %w", err)
			}
		} else {
			return fmt.Errorf("error checking iptables rule: %w", err)
		}
	}

	return nil
}

// Ensure ColimaNetworkManager implements NetworkManager
var _ NetworkManager = (*ColimaNetworkManager)(nil)

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
