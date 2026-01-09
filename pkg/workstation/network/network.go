package network

import (
	"fmt"
	"net"
	"sort"

	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/shell/ssh"
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
	AssignIPs(services []services.Service) error
	ConfigureHostRoute() error
	ConfigureGuest() error
	ConfigureDNS() error
}

// BaseNetworkManager is a concrete implementation of NetworkManager
type BaseNetworkManager struct {
	sshClient                ssh.Client
	shell                    shell.Shell
	secureShell              shell.Shell
	configHandler            config.ConfigHandler
	services                 []services.Service
	shims                    *Shims
	networkInterfaceProvider NetworkInterfaceProvider
	portAllocator            *services.PortAllocator
}

// =============================================================================
// Constructor
// =============================================================================

// NewNetworkManager creates a new NetworkManager
func NewBaseNetworkManager(rt *runtime.Runtime) *BaseNetworkManager {
	if rt == nil {
		panic("runtime is required")
	}
	if rt.Shell == nil {
		panic("shell is required on runtime")
	}
	if rt.ConfigHandler == nil {
		panic("config handler is required on runtime")
	}

	return &BaseNetworkManager{
		shell:         rt.Shell,
		configHandler: rt.ConfigHandler,
		shims:         NewShims(),
		portAllocator: services.NewPortAllocator(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// AssignIPs sorts services and assigns IPs based on network CIDR.
// Services are passed explicitly from Workstation to ensure we work with the same instances.
func (n *BaseNetworkManager) AssignIPs(serviceList []services.Service) error {
	// Sort services by name for consistent IP assignment
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
	if err := assignIPAddresses(n.services, &networkCIDR, n.portAllocator); err != nil {
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
var assignIPAddresses = func(serviceList []services.Service, networkCIDR *string, portAllocator *services.PortAllocator) error {
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

	for i := range serviceList {
		serviceAddress := ip.String()
		if err := serviceList[i].SetAddress(serviceAddress, portAllocator); err != nil {
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

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure BaseNetworkManager implements NetworkManager
var _ NetworkManager = (*BaseNetworkManager)(nil)
