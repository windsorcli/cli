package network

import (
	"fmt"
	"net"
	"sort"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/services"
	"github.com/windsor-hotel/cli/internal/shell"
	"github.com/windsor-hotel/cli/internal/ssh"
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
	networkInterfaceProvider NetworkInterfaceProvider
	services                 []services.Service
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
	sshClient, ok := n.injector.Resolve("sshClient").(ssh.Client)
	if !ok {
		return fmt.Errorf("resolved ssh client instance is not of type ssh.Client")
	}
	n.sshClient = sshClient

	// Get the shell from the injector
	shellInterface, ok := n.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("resolved shell instance is not of type shell.Shell")
	}
	n.shell = shellInterface

	// Get the secure shell from the injector
	secureShell, ok := n.injector.Resolve("secureShell").(shell.Shell)
	if !ok {
		return fmt.Errorf("resolved secure shell instance is not of type shell.Shell")
	}
	n.secureShell = secureShell

	// Get the CLI config handler from the injector
	configHandler, ok := n.injector.Resolve("configHandler").(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("error resolving configHandler")
	}
	n.configHandler = configHandler

	// Get the context handler from the injector
	contextHandler, ok := n.injector.Resolve("contextHandler").(context.ContextHandler)
	if !ok {
		return fmt.Errorf("failed to resolve context handler")
	}
	n.contextHandler = contextHandler

	// Get the network interface provider from the injector
	networkInterfaceProvider, ok := n.injector.Resolve("networkInterfaceProvider").(NetworkInterfaceProvider)
	if !ok {
		return fmt.Errorf("failed to resolve network interface provider")
	}
	n.networkInterfaceProvider = networkInterfaceProvider

	// Resolve all services from the injector
	resolvedServices, err := n.injector.ResolveAll(new(services.Service))
	if err != nil {
		return fmt.Errorf("error resolving services: %w", err)
	}

	// Cast all instances to Service type
	var serviceList []services.Service
	for _, serviceInterface := range resolvedServices {
		service, _ := serviceInterface.(services.Service)
		serviceList = append(serviceList, service)
	}

	// Sort the services alphabetically by their Name
	sort.Slice(serviceList, func(i, j int) bool {
		return serviceList[i].GetName() < serviceList[j].GetName()
	})

	// Assign IP addresses to services
	networkCIDR := n.configHandler.GetString("docker.network_cidr")
	if networkCIDR != "" {
		if err := assignIPAddresses(serviceList, &networkCIDR); err != nil {
			return fmt.Errorf("error assigning IP addresses: %w", err)
		}
	}

	n.services = serviceList

	return nil
}

// ConfigureGuest sets up the guest VM network
func (n *BaseNetworkManager) ConfigureGuest() error {
	// no-op
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

// assignIPAddresses assigns IP addresses to services based on the network CIDR.
var assignIPAddresses = func(services []services.Service, networkCIDR *string) error {
	if networkCIDR == nil {
		return nil
	}

	ip, ipNet, err := net.ParseCIDR(*networkCIDR)
	if err != nil {
		return fmt.Errorf("error parsing network CIDR: %w", err)
	}

	// Skip the network address
	ip = incrementIP(ip)

	// Skip the first IP address
	ip = incrementIP(ip)

	for i := range services {
		if err := services[i].SetAddress(ip.String()); err != nil {
			return fmt.Errorf("error setting address for service: %w", err)
		}
		ip = incrementIP(ip)
		if !ipNet.Contains(ip) {
			return fmt.Errorf("not enough IP addresses in the CIDR range")
		}
	}

	return nil
}

// incrementIP increments an IP address by one
func incrementIP(ip net.IP) net.IP {
	ip = ip.To4()
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
	return ip
}
