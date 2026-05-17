package network

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"

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
	FlushDNS() error
	NeedsPrivilege() bool
	DNSChanged() bool
}

// BaseNetworkManager is a concrete implementation of NetworkManager.
type BaseNetworkManager struct {
	shell                    shell.Shell
	configHandler            config.ConfigHandler
	shims                    *Shims
	networkInterfaceProvider NetworkInterfaceProvider
	dnsChanged               bool
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

// DNSChanged reports whether ConfigureDNS wrote a new configuration during this run.
func (n *BaseNetworkManager) DNSChanged() bool {
	return n.dnsChanged
}

// NeedsPrivilege returns true when network configuration will require elevated privileges (sudo or administrator).
// Uses effectiveResolverIP and platform-specific needsPrivilegeForResolver/needsPrivilegeForHostRoute; errors are treated as false.
func (n *BaseNetworkManager) NeedsPrivilege() bool {
	desiredIP := n.effectiveResolverIP()
	dnsEnabled := n.configHandler.Get("dns.enabled")
	willConfigureDNS := (dnsEnabled == nil || dnsEnabled == true) &&
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

// validateDomain rejects DNS domains containing path separators that would let configuration values
// escape the intended filesystem layout when interpolated into temp or drop-in file paths.
// Caller is responsible for the "empty domain" check.
func validateDomain(domain string) error {
	if strings.ContainsAny(domain, `/\`) {
		return fmt.Errorf("invalid DNS domain %q: contains path separator", domain)
	}
	return nil
}

// writeFileWithSudo stages content in a freshly-created private temp directory (mode 0700) and
// then sudo-moves it to destPath and sudo-chmods it to 0644. Using MkdirTemp ensures the source
// path is unpredictable and that an unprivileged local user cannot pre-create a symlink at the
// source before the sudo mv runs. The temp directory is removed on every exit path.
func (n *BaseNetworkManager) writeFileWithSudo(destPath string, content []byte) error {
	tempDir, err := n.shims.MkdirTemp("", "windsor-net-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer n.shims.RemoveAll(tempDir)

	tempPath := filepath.Join(tempDir, "drop-in")
	if err := n.shims.WriteFile(tempPath, content, 0644); err != nil {
		return fmt.Errorf("failed to stage file: %w", err)
	}

	if _, err := n.shell.ExecSudo("", "mv", tempPath, destPath); err != nil {
		return fmt.Errorf("failed to install file: %w", err)
	}

	if _, err := n.shell.ExecSudo("", "chmod", "0644", destPath); err != nil {
		return fmt.Errorf("failed to set file mode: %w", err)
	}

	return nil
}

// effectiveResolverIP returns the resolver IP for DNS config: dns.address when set (by config, migration for
// localhost, or ConfigureNetwork(override)); when unset and in localhost mode returns 127.0.0.1.
func (n *BaseNetworkManager) effectiveResolverIP() string {
	if v := n.configHandler.GetString("workstation.dns.address"); v != "" {
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
