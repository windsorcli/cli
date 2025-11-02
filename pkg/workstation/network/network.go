package network

import (
	"fmt"
	"net"
	"sort"

	"github.com/windsorcli/cli/pkg/context/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/context/shell"
	"github.com/windsorcli/cli/pkg/context/shell/ssh"
	"github.com/windsorcli/cli/pkg/workstation/services"
)

// The NetworkManager is a core component that manages local development network configuration.
// It provides a unified interface for configuring host routes, guest VM networking, and DNS settings.
// The NetworkManager acts as the central network orchestrator for the application,
// coordinating IP address assignment, network interface configuration, and service networking.

// =============================================================================
// Types
// =============================================================================

// NetworkManager handles configuring the local development network
type NetworkManager interface {
	Initialize() error
	ConfigureHostRoute() error
	ConfigureGuest() error
	ConfigureDNS() error
}

// BaseNetworkManager is a concrete implementation of NetworkManager
type BaseNetworkManager struct {
	injector                 di.Injector
	sshClient                ssh.Client
	shell                    shell.Shell
	secureShell              shell.Shell
	configHandler            config.ConfigHandler
	services                 []services.Service
	shims                    *Shims
	networkInterfaceProvider NetworkInterfaceProvider
}

// =============================================================================
// Constructor
// =============================================================================

// NewNetworkManager creates a new NetworkManager
func NewBaseNetworkManager(injector di.Injector) *BaseNetworkManager {
	return &BaseNetworkManager{
		injector: injector,
		shims:    NewShims(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize resolves dependencies, sorts services, and assigns IPs based on network CIDR
func (n *BaseNetworkManager) Initialize() error {
	shellInterface, ok := n.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("resolved shell instance is not of type shell.Shell")
	}
	n.shell = shellInterface

	configHandler, ok := n.injector.Resolve("configHandler").(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("error resolving configHandler")
	}
	n.configHandler = configHandler

	resolvedServices, err := n.injector.ResolveAll(new(services.Service))
	if err != nil {
		return fmt.Errorf("error resolving services: %w", err)
	}

	var serviceList []services.Service
	for _, serviceInterface := range resolvedServices {
		service, _ := serviceInterface.(services.Service)
		serviceList = append(serviceList, service)
	}

	sort.Slice(serviceList, func(i, j int) bool {
		return serviceList[i].GetName() < serviceList[j].GetName()
	})

	n.services = serviceList

	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" {
		networkCIDR = constants.DefaultNetworkCIDR
		if err := n.configHandler.Set("network.cidr_block", networkCIDR); err != nil {
			return fmt.Errorf("error setting default network CIDR: %w", err)
		}
	}
	if err := assignIPAddresses(n.services, &networkCIDR); err != nil {
		return fmt.Errorf("error assigning IP addresses: %w", err)
	}

	return nil
}

// ConfigureGuest sets up the guest VM network
func (n *BaseNetworkManager) ConfigureGuest() error {
	// no-op
	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// isLocalhostMode checks if the system is in localhost mode
func (n *BaseNetworkManager) isLocalhostMode() bool {
	return n.configHandler.GetString("vm.driver") == "docker-desktop"
}

// =============================================================================
// Helpers
// =============================================================================

// assignIPAddresses assigns IP addresses to services based on the network CIDR.
var assignIPAddresses = func(services []services.Service, networkCIDR *string) error {
	if networkCIDR == nil || *networkCIDR == "" {
		return fmt.Errorf("network CIDR is not defined")
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

// Ensure BaseNetworkManager implements NetworkManager
var _ NetworkManager = (*BaseNetworkManager)(nil)
