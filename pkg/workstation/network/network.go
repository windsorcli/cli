package network

import (
	"net"

	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// The NetworkManager manages local development network configuration: host routes,
// guest VM networking, and DNS. IP assignment for services is handled by Terraform in the stack.

// =============================================================================
// Types
// =============================================================================

// NetworkManager handles configuring the local development network.
// Guest address is read from config (workstation.address) where needed.
type NetworkManager interface {
	ConfigureHostRoute() error
	ConfigureGuest() error
	ConfigureDNS() error
	NeedsPrivilege() bool
}

// BaseNetworkManager is a concrete implementation of NetworkManager.
type BaseNetworkManager struct {
	shell                    shell.Shell
	configHandler            config.ConfigHandler
	shims                    *Shims
	networkInterfaceProvider NetworkInterfaceProvider
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
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// ConfigureGuest sets up the guest VM network. Base implementation is a no-op; Colima overrides and reads guest address from config.
func (n *BaseNetworkManager) ConfigureGuest() error {
	return nil
}

// NeedsPrivilege returns true when network configuration will require elevated privileges (sudo or administrator).
// Uses effectiveResolverIP and platform-specific needsPrivilegeForResolver/needsPrivilegeForHostRoute; errors are treated as false.
func (n *BaseNetworkManager) NeedsPrivilege() bool {
	desiredIP := n.effectiveResolverIP()
	willConfigureDNS := n.configHandler.GetBool("dns.enabled") &&
		n.configHandler.GetString("dns.domain") != "" && desiredIP != ""
	needForDNS := willConfigureDNS && n.needsPrivilegeForResolver(desiredIP)
	workstationRuntime := n.configHandler.GetString("workstation.runtime")
	guestAddress := n.configHandler.GetString("workstation.address")
	needForHostRoute := workstationRuntime == "colima" && guestAddress != "" && n.needsPrivilegeForHostRoute(guestAddress)
	return needForDNS || needForHostRoute
}

// =============================================================================
// Private Methods
// =============================================================================

// isLocalhostMode checks if the system is in localhost mode.
func (n *BaseNetworkManager) isLocalhostMode() bool {
	return n.configHandler.GetString("workstation.runtime") == "docker-desktop"
}

// effectiveResolverIP returns the resolver IP for DNS config: dns.address when set (by config, migration for
// localhost, or ConfigureNetwork(override)); when unset and in localhost mode returns 127.0.0.1.
func (n *BaseNetworkManager) effectiveResolverIP() string {
	if v := n.configHandler.GetString("dns.address"); v != "" {
		return v
	}
	if n.isLocalhostMode() {
		return "127.0.0.1"
	}
	return ""
}

// incrementIP increments an IP address by one.
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
