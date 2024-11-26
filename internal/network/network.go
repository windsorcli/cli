package network

import (
	"fmt"
	"net"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
	"github.com/windsor-hotel/cli/internal/ssh"
	"github.com/windsor-hotel/cli/internal/virt"
)

// NetworkManager handles configuring the local development network
type NetworkManager interface {
	// Initialize the network manager
	Initialize() error
	// ConfigureHostRoute sets up the local development network
	ConfigureHostRoute() error
	// ConfigureGuest sets up the guest VM network
	ConfigureGuest() error
	// ConfigureDNS sets up the DNS configuration
	ConfigureDNS() error
}

// BaseNetworkManager is a concrete implementation of NetworkManager
type BaseNetworkManager struct {
	injector                 di.Injector
	sshClient                ssh.Client
	shell                    shell.Shell
	secureShell              shell.Shell
	configHandler            config.ConfigHandler
	contextHandler           context.ContextHandler
	colimaVirt               virt.Virt
	dockerVirt               virt.Virt
	networkInterfaceProvider NetworkInterfaceProvider
}

// NewNetworkManager creates a new NetworkManager
func NewBaseNetworkManager(injector di.Injector) (*BaseNetworkManager, error) {
	nm := &BaseNetworkManager{
		injector: injector,
	}
	return nm, nil
}

// Initialize the network manager
func (n *BaseNetworkManager) Initialize() error {
	// Resolve the sshClient from the injector
	sshClientInstance, err := n.injector.Resolve("sshClient")
	if err != nil {
		return fmt.Errorf("failed to resolve ssh client instance: %w", err)
	}
	sshClient, ok := sshClientInstance.(ssh.Client)
	if !ok {
		return fmt.Errorf("resolved ssh client instance is not of type ssh.Client")
	}
	n.sshClient = sshClient

	// Get the shell from the injector
	shellInstance, err := n.injector.Resolve("shell")
	if err != nil {
		return fmt.Errorf("failed to resolve shell instance: %w", err)
	}
	shellInterface, ok := shellInstance.(shell.Shell)
	if !ok {
		return fmt.Errorf("resolved shell instance is not of type shell.Shell")
	}
	n.shell = shellInterface

	// Get the secure shell from the injector
	secureShellInstance, err := n.injector.Resolve("secureShell")
	if err != nil {
		return fmt.Errorf("failed to resolve secure shell instance: %w", err)
	}
	secureShell, ok := secureShellInstance.(shell.Shell)
	if !ok {
		return fmt.Errorf("resolved secure shell instance is not of type shell.Shell")
	}
	n.secureShell = secureShell

	// Get the CLI config handler from the injector
	configHandlerInstance, err := n.injector.Resolve("configHandler")
	if err != nil {
		return fmt.Errorf("failed to resolve CLI config handler: %w", err)
	}
	configHandler, ok := configHandlerInstance.(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("resolved CLI config handler instance is not of type config.ConfigHandler")
	}
	n.configHandler = configHandler

	// Get the context handler from the injector
	contextHandlerInstance, err := n.injector.Resolve("contextHandler")
	if err != nil {
		return fmt.Errorf("failed to resolve context handler: %w", err)
	}
	contextHandler, ok := contextHandlerInstance.(context.ContextHandler)
	if !ok {
		return fmt.Errorf("resolved context handler instance is not of type context.ContextHandler")
	}
	n.contextHandler = contextHandler

	// Get the network interface provider from the injector
	networkInterfaceProviderInstance, err := n.injector.Resolve("networkInterfaceProvider")
	if err != nil {
		return fmt.Errorf("failed to resolve network interface provider: %w", err)
	}
	networkInterfaceProvider, ok := networkInterfaceProviderInstance.(NetworkInterfaceProvider)
	if !ok {
		return fmt.Errorf("resolved network interface provider instance is not of type NetworkInterfaceProvider")
	}
	n.networkInterfaceProvider = networkInterfaceProvider

	return nil
}

// ConfigureGuest sets up the guest VM network
func (n *BaseNetworkManager) ConfigureGuest() error {
	return nil
}

// Ensure BaseNetworkManager implements NetworkManager
var _ NetworkManager = (*BaseNetworkManager)(nil)

// getHostIP gets the host IP address
func (n *BaseNetworkManager) getHostIP() (string, error) {
	// Get the guest IP address
	guestIP := n.configHandler.GetString("vm.address")
	if guestIP == "" {
		return "", fmt.Errorf("guest IP is not configured")
	}

	// Parse the guest IP
	guestIPAddr := net.ParseIP(guestIP)
	if guestIPAddr == nil {
		return "", fmt.Errorf("invalid guest IP address")
	}

	// Get a list of network interfaces
	interfaces, err := n.networkInterfaceProvider.Interfaces()
	if err != nil {
		return "", fmt.Errorf("failed to get network interfaces: %w", err)
	}

	// Iterate over each network interface
	for _, iface := range interfaces {
		addrs, err := n.networkInterfaceProvider.InterfaceAddrs(iface)
		if err != nil {
			return "", fmt.Errorf("failed to get addresses for interface %s: %w", iface.Name, err)
		}

		// Check each address associated with the interface
		for _, addr := range addrs {
			var ipNet *net.IPNet
			switch v := addr.(type) {
			case *net.IPNet:
				ipNet = v
			case *net.IPAddr:
				ipNet = &net.IPNet{IP: v.IP, Mask: v.IP.DefaultMask()}
			}

			// Check if the IP is in the same subnet as the guest IP
			if ipNet != nil && ipNet.Contains(guestIPAddr) {
				// Return the host IP in the same subnet as the guest IP
				return ipNet.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("failed to find host IP in the same subnet as guest IP")
}
