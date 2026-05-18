package network

import (
	"fmt"
	"net"
	"path/filepath"
	"slices"
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
// Revert* counterparts undo Configure*, are idempotent (no-op when the
// resource is absent), and are invoked by 'windsor configure network --revert'
// and 'windsor down'.
// IsHostRouteInstalled / IsResolverInstalled answer the question "is the host
// state this manager would install currently present?" — used by 'windsor down'
// to detect orphaned host configuration when the operator can't elevate
// non-interactively, so a hint can point them at 'configure network --revert'.
type NetworkManager interface {
	ConfigureHostRoute() error
	ConfigureGuest() error
	ConfigureDNS() error
	RevertHostRoute() error
	RevertGuest() error
	RevertDNS() error
	FlushDNS() error
	NeedsPrivilegeForCluster() bool
	NeedsPrivilegeForDNS() bool
	IsHostRouteInstalled() bool
	IsResolverInstalled() bool
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

// RevertGuest removes guest-VM forwarding installed by ConfigureGuest. Base implementation is a
// no-op (nothing to revert when ConfigureGuest was a no-op); Colima overrides to remove the
// iptables FORWARD rule it installed in the VM.
func (n *BaseNetworkManager) RevertGuest() error {
	return nil
}

// DNSChanged reports whether ConfigureDNS wrote a new configuration during this run.
func (n *BaseNetworkManager) DNSChanged() bool {
	return n.dnsChanged
}

// NeedsPrivilegeForCluster reports whether the host needs elevated configuration before the rest
// of the blueprint can reach the cluster — host route + in-VM forwarding on VM-backed runtimes
// whose cluster IP is not on loopback. Returns true only on colima today; docker-desktop has the
// cluster on 127.0.0.1 and never needs a host route. Failures of underlying probes are treated as
// false (the privileged work is also gated by the actual ConfigureHostRoute call, which will
// surface its own error if invoked when it shouldn't have been).
func (n *BaseNetworkManager) NeedsPrivilegeForCluster() bool {
	if n.configHandler.GetString("workstation.runtime") != "colima" {
		return false
	}
	guestAddress := n.configHandler.GetString("workstation.address")
	if guestAddress == "" {
		return false
	}
	return n.needsPrivilegeForHostRoute(guestAddress)
}

// NeedsPrivilegeForDNS reports whether the host's DNS resolver needs elevated configuration to
// point *.dns.domain at the cluster resolver. Returns true when a configurable DNS service is
// available (dns.domain set; resolver IP derivable via effectiveResolverIP) and the host's
// current resolver entry does not already match. Failures of underlying probes are treated as
// false.
func (n *BaseNetworkManager) NeedsPrivilegeForDNS() bool {
	if n.configHandler.GetString("dns.domain") == "" {
		return false
	}
	desiredIP := n.effectiveResolverIP()
	if desiredIP == "" {
		return false
	}
	return n.needsPrivilegeForResolver(desiredIP)
}

// IsHostRouteInstalled reports whether the host route this NetworkManager would install (for
// the configured CIDR via the configured guest) is currently present and matches. Always false
// on runtimes that don't use host routes (anything other than colima). Used by 'windsor down'
// to detect orphaned cluster-reachability state when the operator can't elevate non-interactively.
func (n *BaseNetworkManager) IsHostRouteInstalled() bool {
	if n.configHandler.GetString("workstation.runtime") != "colima" {
		return false
	}
	guestAddress := n.configHandler.GetString("workstation.address")
	if guestAddress == "" {
		return false
	}
	return !n.needsPrivilegeForHostRoute(guestAddress)
}

// IsResolverInstalled reports whether the per-domain DNS resolver entry this NetworkManager
// would install is currently present and matches the configured resolver IP. False when no
// dns.domain is configured or when no resolver address is derivable. Used by 'windsor down' to
// detect orphaned DNS-resolver state. The "matches" semantic means a stale entry with a
// different IP will not be detected as installed — operators with that edge case can run
// 'configure network --revert' explicitly.
func (n *BaseNetworkManager) IsResolverInstalled() bool {
	if n.configHandler.GetString("dns.domain") == "" {
		return false
	}
	desiredIP := n.effectiveResolverIP()
	if desiredIP == "" {
		return false
	}
	return !n.needsPrivilegeForResolver(desiredIP)
}

// =============================================================================
// Private Methods
// =============================================================================

// isLocalhostMode checks if the system is in localhost mode.
func (n *BaseNetworkManager) isLocalhostMode() bool {
	return n.configHandler.GetString("workstation.runtime") == "docker-desktop"
}

// validateDomain restricts DNS domains to the RFC 1123 label character set (letters, digits,
// hyphen, dot) and rejects empty labels. The character allowlist excludes shell metacharacters,
// quotes, and path separators; the empty-label check rejects dot-only ("..", ".") and dot-edge
// ("foo.", ".foo", "foo..bar") values that would escape into parent directories when interpolated
// into a filesystem path under sudo (e.g. /etc/resolver/.. resolves to /etc/ on darwin).
// Caller is responsible for the "empty domain" check.
func validateDomain(domain string) error {
	for _, r := range domain {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '.' {
			continue
		}
		return fmt.Errorf("invalid DNS domain %q: must contain only letters, digits, hyphen, and dot", domain)
	}
	if slices.Contains(strings.Split(domain, "."), "") {
		return fmt.Errorf("invalid DNS domain %q: contains empty label", domain)
	}
	return nil
}

// validateCIDR parses cidr as a CIDR-notation network and returns the canonical form for
// interpolation into shell or PowerShell command strings. Callers that build commands by
// string concatenation (Windows PowerShell -Command, colima ssh sh -c) must use the returned
// value rather than the raw config string to ensure no shell or PowerShell metacharacters can
// be smuggled in via a tampered workstation.yaml.
func validateCIDR(cidr string) (string, error) {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	return network.String(), nil
}

// validateIPAddress parses ip as an IPv4 or IPv6 literal and returns the canonical form. Same
// rationale as validateCIDR: callers that interpolate IP-shaped config values into shell or
// PowerShell command strings must use the canonical return value rather than the raw input.
func validateIPAddress(ip string) (string, error) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "", fmt.Errorf("invalid IP address %q", ip)
	}
	return parsed.String(), nil
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
