//go:build darwin
// +build darwin

package network

import (
	"fmt"
	"os"
	"strings"
)

// The DarwinNetworkManager is a platform-specific network manager for macOS.
// It provides network configuration capabilities specific to Darwin-based systems,
// The DarwinNetworkManager handles host route configuration and DNS setup for macOS,
// ensuring proper network connectivity between the host and guest VM environments.

// =============================================================================
// Public Methods
// =============================================================================

// NeedsPrivilege returns true when network config will need sudo/admin. Resolves guest address and other
// settings from config like other privilege checks. Errors are treated as false.
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

// ConfigureHostRoute ensures that a network route from the host to the VM guest is established.
// Guest address is read from config (workstation.address). It checks if a route for the network
// CIDR already exists with that gateway; if not, adds the route with elevated permissions.
func (n *BaseNetworkManager) ConfigureHostRoute() error {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" {
		return fmt.Errorf("network CIDR is not configured")
	}
	guestIP := n.configHandler.GetString("workstation.address")
	if guestIP == "" {
		return fmt.Errorf("guest address is required")
	}

	networkPrefix := strings.Split(networkCIDR, "/")[0]
	output, err := n.shell.ExecSilent("route", "-n", "get", networkPrefix)
	if err != nil {
		return fmt.Errorf("failed to check if route exists: %w", err)
	}

	routeExists := false

	found := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
			key := strings.ToLower(strings.TrimSpace(parts[0]))
			val := strings.TrimSpace(parts[1])
			found[key] = val
		}
	}

	if strings.TrimSpace(found["destination"]) == networkPrefix && strings.TrimSpace(found["gateway"]) == guestIP {
		routeExists = true
	}

	if routeExists {
		return nil
	}

	fmt.Fprintf(os.Stderr, "\033[33m⚠\033[0m 🔐 Network configuration may require sudo password\n")
	output, err = n.shell.ExecSudo(
		"🔐 Adding host route",
		"route",
		"-nv",
		"add",
		"-net",
		networkCIDR,
		guestIP,
	)
	if err != nil {
		return fmt.Errorf("failed to add route: %w, output: %s", err, output)
	}
	return nil
}

// ConfigureDNS sets up DNS by modifying system files to route DNS queries.
// Resolver IP is read from config via effectiveResolverIP (dns.address, or 127.0.0.1 in localhost mode when unset).
func (n *BaseNetworkManager) ConfigureDNS() error {
	tld := n.configHandler.GetString("dns.domain")
	if tld == "" {
		return fmt.Errorf("DNS domain is not configured")
	}

	dnsIP := n.effectiveResolverIP()
	if dnsIP == "" {
		return fmt.Errorf("DNS address is not configured")
	}

	resolverDir := "/etc/resolver"
	resolverFile := fmt.Sprintf("%s/%s", resolverDir, tld)
	content := fmt.Sprintf("nameserver %s\n", dnsIP)

	existingContent, err := n.shims.ReadFile(resolverFile)
	if err == nil && resolverAlreadyConfigured(string(existingContent), dnsIP) {
		return nil
	}

	fmt.Fprintf(os.Stderr, "\033[33m⚠\033[0m 🔐 DNS configuration may require sudo password\n")

	if _, err := n.shims.Stat(resolverDir); os.IsNotExist(err) {
		if _, err := n.shell.ExecSudo(
			"🔐 Creating resolver directory",
			"mkdir",
			"-p",
			resolverDir,
		); err != nil {
			return fmt.Errorf("Error creating resolver directory: %w", err)
		}
	}

	tempResolverFile := fmt.Sprintf("/tmp/%s", tld)
	if err := n.shims.WriteFile(tempResolverFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("Error writing to temporary resolver file: %w", err)
	}

	if _, err := n.shell.ExecSudo(
		fmt.Sprintf("🔐 Configuring DNS resolver at %s", resolverFile),
		"mv",
		tempResolverFile,
		resolverFile,
	); err != nil {
		return fmt.Errorf("Error moving resolver file: %w", err)
	}
	if _, err := n.shell.ExecSudo(
		"🔐 Setting resolver file readable for next-run check",
		"chmod",
		"0644",
		resolverFile,
	); err != nil {
		return fmt.Errorf("Error setting resolver file mode: %w", err)
	}

	if _, err := n.shell.ExecSudo(
		"🔐 Flushing DNS cache",
		"dscacheutil",
		"-flushcache",
	); err != nil {
		return fmt.Errorf("Error flushing DNS cache: %w", err)
	}

	if _, err := n.shell.ExecSudo(
		"🔐 Restarting mDNSResponder",
		"killall",
		"-HUP",
		"mDNSResponder",
	); err != nil {
		return fmt.Errorf("Error restarting mDNSResponder: %w", err)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// resolverAlreadyConfigured reports whether the resolver file content already has a nameserver line for desiredIP.
// Used to skip writing and sudo when the effective config matches. Parses lines and checks the first nameserver line.
func resolverAlreadyConfigured(content, desiredIP string) bool {
	for _, line := range strings.FieldsFunc(content, func(r rune) bool { return r == '\n' }) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver ") {
			ip := strings.TrimSpace(strings.TrimPrefix(line, "nameserver "))
			return ip == desiredIP
		}
	}
	return false
}

// needsPrivilegeForResolver reports whether sudo is required to apply the desired DNS resolver IP on macOS.
// Returns true when dns.domain is set and the resolver file at /etc/resolver/<domain> is missing, unreadable,
// or does not already specify desiredIP.
func (n *BaseNetworkManager) needsPrivilegeForResolver(desiredIP string) bool {
	tld := n.configHandler.GetString("dns.domain")
	if tld == "" {
		return false
	}
	resolverFile := fmt.Sprintf("/etc/resolver/%s", tld)
	existingContent, err := n.shims.ReadFile(resolverFile)
	if err != nil {
		return true
	}
	return !resolverAlreadyConfigured(string(existingContent), desiredIP)
}

// needsPrivilegeForHostRoute reports whether sudo is required to add the host route for the guest on macOS.
// Returns true when no route exists for network.cidr_block with gateway guestAddress; returns false when the
// route exists, when CIDR or guest is unset, or when the route check fails.
func (n *BaseNetworkManager) needsPrivilegeForHostRoute(guestAddress string) bool {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" || guestAddress == "" {
		return false
	}
	networkPrefix := strings.Split(networkCIDR, "/")[0]
	output, err := n.shell.ExecSilent("route", "-n", "get", networkPrefix)
	if err != nil {
		return false
	}
	found := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
			key := strings.ToLower(strings.TrimSpace(parts[0]))
			val := strings.TrimSpace(parts[1])
			found[key] = val
		}
	}
	return strings.TrimSpace(found["destination"]) != networkPrefix || strings.TrimSpace(found["gateway"]) != guestAddress
}
