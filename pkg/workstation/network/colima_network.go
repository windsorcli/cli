package network

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/workstation/virt"
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
func NewColimaNetworkManager(rt *runtime.Runtime, networkInterfaceProvider NetworkInterfaceProvider) *ColimaNetworkManager {
	if rt == nil {
		panic("runtime is required")
	}
	if networkInterfaceProvider == nil {
		panic("network interface provider is required")
	}

	manager := &ColimaNetworkManager{
		BaseNetworkManager:       *NewBaseNetworkManager(rt),
		networkInterfaceProvider: networkInterfaceProvider,
	}

	return manager
}

// ConfigureGuest sets up guest traffic forwarding in a Colima environment. Reads guest address from config (workstation.address).
// For Docker, configures iptables for the Docker bridge; for Incus, Incus network. No-op when guest address is empty.
func (n *ColimaNetworkManager) ConfigureGuest() error {
	if n.configHandler.GetString("workstation.address") == "" {
		return nil
	}

	networkCIDR := n.configHandler.GetString("network.cidr_block", constants.DefaultNetworkCIDR)
	if n.configHandler.GetString("provider") == "incus" {
		if err := n.configureIncusNetwork(networkCIDR); err != nil {
			return fmt.Errorf("error configuring incus network: %w", err)
		}
		return n.setupForwardingRule(networkCIDR, virt.IncusNetworkName)
	}
	return n.configureDockerForwarding(networkCIDR)
}

// =============================================================================
// Private Methods
// =============================================================================

// configureDockerForwarding sets up iptables forwarding from col0 to the Docker bridge interface.
// Host IP is resolved from config (workstation.address) via getHostIP. Uses colima ssh to run commands in the VM.
func (n *ColimaNetworkManager) configureDockerForwarding(networkCIDR string) error {
	contextName := n.configHandler.GetContext()
	profileName := fmt.Sprintf("windsor-%s", contextName)

	output, err := n.shell.ExecSilentWithTimeout(
		"colima",
		[]string{"ssh", "--profile", profileName, "--", "sh", "-c", "ls /sys/class/net"},
		5*time.Second,
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
// Uses colima ssh to execute the command directly, avoiding SSH session creation issues
func (n *ColimaNetworkManager) configureIncusNetwork(networkCIDR string) error {
	_, ipNet, err := net.ParseCIDR(networkCIDR)
	if err != nil {
		return fmt.Errorf("error parsing network CIDR: %w", err)
	}

	contextName := n.configHandler.GetContext()
	profileName := fmt.Sprintf("windsor-%s", contextName)
	ones, _ := ipNet.Mask.Size()
	gatewayIP := incrementIP(ipNet.IP).String()
	command := fmt.Sprintf("incus network set %s ipv4.address=%s/%d", virt.IncusNetworkName, gatewayIP, ones)
	_, err = n.shell.ExecSilentWithTimeout(
		"colima",
		[]string{"ssh", "--profile", profileName, "--", "sh", "-c", command},
		15*time.Second,
	)
	if err != nil {
		return fmt.Errorf("error setting incus network address: %w", err)
	}

	return nil
}

// setupForwardingRule sets up iptables forwarding from col0 to the specified output interface.
// Host IP is resolved from config (workstation.address) via getHostIP.
func (n *ColimaNetworkManager) setupForwardingRule(networkCIDR, outputInterface string) error {
	hostIP, err := n.getHostIP()
	if err != nil {
		return fmt.Errorf("error getting host IP: %w", err)
	}
	contextName := n.configHandler.GetContext()
	profileName := fmt.Sprintf("windsor-%s", contextName)
	_, err = n.shell.ExecSilentWithTimeout(
		"colima",
		[]string{"ssh", "--profile", profileName, "--", "sh", "-c", "sudo sysctl -w net.ipv4.ip_forward=1 2>/dev/null </dev/null"},
		10*time.Second,
	)
	if err != nil {
		return fmt.Errorf("error enabling IP forwarding in VM: %w", err)
	}

	checkCommand := fmt.Sprintf("sudo iptables -t filter -C FORWARD -i col0 -o %s -s %s -d %s -j ACCEPT 2>/dev/null </dev/null", outputInterface, hostIP, networkCIDR)
	_, err = n.shell.ExecSilentWithTimeout(
		"colima",
		[]string{"ssh", "--profile", profileName, "--", "sh", "-c", checkCommand},
		10*time.Second,
	)
	if err != nil {
		addCommand := fmt.Sprintf("sudo iptables -t filter -A FORWARD -i col0 -o %s -s %s -d %s -j ACCEPT 2>/dev/null </dev/null", outputInterface, hostIP, networkCIDR)
		_, addErr := n.shell.ExecSilentWithTimeout(
			"colima",
			[]string{"ssh", "--profile", profileName, "--", "sh", "-c", addCommand},
			10*time.Second,
		)
		if addErr != nil {
			return fmt.Errorf("error setting iptables rule: %w", addErr)
		}
	}

	return nil
}

// getHostIP returns the host IP that shares the same subnet as the guest. Guest address is read from config (workstation.address).
func (n *ColimaNetworkManager) getHostIP() (string, error) {
	guestAddress := n.configHandler.GetString("workstation.address")
	if guestAddress == "" {
		return "", fmt.Errorf("guest address is required")
	}

	guestIPAddr := net.ParseIP(guestAddress)
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

// =============================================================================
// Helpers
// =============================================================================

// isExitCode checks if an error is an ExitError with the specified exit code.
// It unwraps the error chain to find the underlying ExitError.
// For iptables -C, exit code 1 means the rule doesn't exist (expected case).
func isExitCode(err error, code int) bool {
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode() == code
	}
	return false
}
