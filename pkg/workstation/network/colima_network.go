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

// ConfigureGuest sets up forwarding of guest traffic to the container network.
// It retrieves network CIDR and guest IP from the config.
// It identifies the Docker bridge interface and ensures iptables rules are set.
// If the rule doesn't exist, it adds a new one to allow traffic forwarding.
// Uses colima ssh to execute commands directly, avoiding SSH session creation issues.
func (n *ColimaNetworkManager) ConfigureGuest() error {
	networkCIDR := n.configHandler.GetString("network.cidr_block", constants.DefaultNetworkCIDR)

	guestIP := n.configHandler.GetString("vm.address")
	if guestIP == "" {
		return fmt.Errorf("guest IP is not configured")
	}

	output, err := n.execInVMWithTimeout("ls /sys/class/net", 5*time.Second)
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

	hostIP, err := n.getHostIP()
	if err != nil {
		return fmt.Errorf("error getting host IP: %w", err)
	}

	if _, err := n.execInVMWithTimeout("sudo sysctl -w net.ipv4.ip_forward=1", 10*time.Second); err != nil {
		return fmt.Errorf("error enabling IP forwarding in VM: %w", err)
	}

	checkCommand := fmt.Sprintf("sudo iptables -t filter -C FORWARD -i col0 -o %s -s %s -d %s -j ACCEPT", dockerBridgeInterface, hostIP, networkCIDR)
	_, err = n.execInVMWithTimeout(checkCommand, 10*time.Second)
	if err != nil {
		if isExitCode(err, 1) {
			addCommand := fmt.Sprintf("sudo iptables -t filter -A FORWARD -i col0 -o %s -s %s -d %s -j ACCEPT", dockerBridgeInterface, hostIP, networkCIDR)
			if _, err := n.execInVMWithTimeout(addCommand, 10*time.Second); err != nil {
				return fmt.Errorf("error setting iptables rule: %w", err)
			}
		} else {
			return fmt.Errorf("error checking iptables rule: %w", err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// getProfileName returns the Colima profile name for the current context.
func (n *ColimaNetworkManager) getProfileName() string {
	contextName := n.configHandler.GetContext()
	return fmt.Sprintf("windsor-%s", contextName)
}

// execInVMWithTimeout executes a command in the VM via colima ssh with a timeout and returns the output.
// Respects the shell's verbosity setting - if verbose mode is enabled, output will be displayed.
func (n *ColimaNetworkManager) execInVMWithTimeout(command string, timeout time.Duration) (string, error) {
	profileName := n.getProfileName()
	sshArgs := []string{"ssh", "--profile", profileName, "--", "sh", "-c", command}
	output, err := n.shell.ExecSilentWithTimeout("colima", sshArgs, timeout)
	return output, err
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
