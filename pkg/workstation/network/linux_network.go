//go:build linux
// +build linux

package network

import (
	"fmt"
	"os"
	"strings"
)

// The LinuxNetworkManager is a platform-specific network manager for Linux systems.
// It provides network configuration capabilities specific to Linux-based systems,
// The LinuxNetworkManager handles host route configuration and DNS setup for Linux,
// ensuring proper network connectivity between the host and guest VM environments.

// =============================================================================
// Public Methods
// =============================================================================

// ConfigureHostRoute sets up the local development network for Linux.
// Guest address is read from config (workstation.address).
func (n *BaseNetworkManager) ConfigureHostRoute() error {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" {
		return fmt.Errorf("network CIDR is not configured")
	}
	guestIP := n.configHandler.GetString("workstation.address")
	if guestIP == "" {
		return fmt.Errorf("guest address is required")
	}

	output, err := n.shell.ExecSilent("ip", "route", "show", networkCIDR)
	if err != nil {
		return fmt.Errorf("failed to check if route exists: %w", err)
	}

	routeExists := false
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, guestIP) {
			routeExists = true
			break
		}
	}

	if routeExists {
		return nil
	}

	fmt.Fprintf(os.Stderr, "\033[33m⚠\033[0m 🔐 Network configuration may require sudo password\n")
	output, err = n.shell.ExecSudo(
		"🔐 Adding host route",
		"ip",
		"route",
		"add",
		networkCIDR,
		"via",
		guestIP,
	)
	if err != nil {
		return fmt.Errorf("failed to add route: %w, output: %s", err, output)
	}

	return nil
}

// ConfigureDNS configures systemd-resolved so only the configured dns.domain uses our resolver;
// global DNS is unchanged. Resolver IP is read from config (dns.resolver_ip, or derived: 127.0.0.1 in
// localhost mode, else dns.address).
func (n *BaseNetworkManager) ConfigureDNS() error {
	domain := n.configHandler.GetString("dns.domain")
	if domain == "" {
		return fmt.Errorf("DNS domain is not configured")
	}

	dnsIP := n.effectiveResolverIP()
	if dnsIP == "" {
		return fmt.Errorf("DNS address is not configured")
	}

	resolvConf, err := n.shims.ReadLink("/etc/resolv.conf")
	if err != nil || resolvConf != "../run/systemd/resolve/stub-resolv.conf" {
		return fmt.Errorf("systemd-resolved is not in use. Please configure DNS manually or use a compatible system")
	}

	dropInDir := "/etc/systemd/resolved.conf.d"
	dropInFile := fmt.Sprintf("%s/dns-override-%s.conf", dropInDir, domain)
	expectedContent := fmt.Sprintf("[Resolve]\nDomains=~%s\nDNS=%s\n", domain, dnsIP)

	existingContent, err := n.shims.ReadFile(dropInFile)
	if err == nil && string(existingContent) == expectedContent {
		return nil
	}

	fmt.Fprintf(os.Stderr, "\033[33m⚠\033[0m 🔐 DNS configuration may require sudo password\n")

	_, err = n.shell.ExecSudo(
		"🔐 Creating DNS configuration directory",
		"mkdir",
		"-p",
		dropInDir,
	)
	if err != nil {
		return fmt.Errorf("failed to create drop-in directory: %w", err)
	}

	_, err = n.shell.ExecSudo(
		"🔐 Writing DNS configuration to "+dropInFile,
		"bash",
		"-c",
		fmt.Sprintf("echo '%s' | sudo tee %s", expectedContent, dropInFile),
	)
	if err != nil {
		return fmt.Errorf("failed to write DNS configuration: %w", err)
	}

	_, err = n.shell.ExecSudo(
		"🔐 Restarting systemd-resolved",
		"systemctl",
		"restart",
		"systemd-resolved",
	)
	if err != nil {
		return fmt.Errorf("failed to restart systemd-resolved: %w", err)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// needsPrivilegeForResolver reports whether sudo is required to apply the desired DNS resolver IP
// for the configured domain. It returns true when systemd-resolved is in use and the current
// drop-in for dns.domain is missing, unreadable, or has different Domains= or DNS= values.
func (n *BaseNetworkManager) needsPrivilegeForResolver(desiredIP string) bool {
	domain := n.configHandler.GetString("dns.domain")
	if domain == "" {
		return false
	}
	resolvConf, err := n.shims.ReadLink("/etc/resolv.conf")
	if err != nil || resolvConf != "../run/systemd/resolve/stub-resolv.conf" {
		return false
	}
	dropInFile := fmt.Sprintf("/etc/systemd/resolved.conf.d/dns-override-%s.conf", domain)
	existingContent, err := n.shims.ReadFile(dropInFile)
	if err != nil {
		return true
	}
	expectedContent := fmt.Sprintf("[Resolve]\nDomains=~%s\nDNS=%s\n", domain, desiredIP)
	return string(existingContent) != expectedContent
}

// needsPrivilegeForHostRoute reports whether sudo is required to add the host route for the guest.
// It returns true when the route for network.cidr_block via guestAddress does not yet exist;
// it returns false when the route exists, when CIDR or guest is unset, or when the route check fails.
func (n *BaseNetworkManager) needsPrivilegeForHostRoute(guestAddress string) bool {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" || guestAddress == "" {
		return false
	}
	output, err := n.shell.ExecSilent("ip", "route", "show", networkCIDR)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, guestAddress) {
			return false
		}
	}
	return true
}
