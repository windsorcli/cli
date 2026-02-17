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

// ConfigureHostRoute ensures that a network route from the host to the VM guest is established.
// It first checks if a route for the specified network CIDR already exists with the guest IP as the gateway.
// If the route does not exist, it adds a new route using elevated permissions to facilitate communication
// between the host and the guest VM.
func (n *BaseNetworkManager) ConfigureHostRoute() error {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" {
		return fmt.Errorf("network CIDR is not configured")
	}

	guestIP := n.configHandler.GetString("vm.address")
	if guestIP == "" {
		return fmt.Errorf("guest IP is not configured")
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
// It creates a resolver file for a specified DNS IP. When dnsAddressOverride is non-empty it is used;
// otherwise the address is read from config (dns.address or 127.0.0.1 in localhost mode).
func (n *BaseNetworkManager) ConfigureDNS(dnsAddressOverride string) error {
	tld := n.configHandler.GetString("dns.domain")
	if tld == "" {
		return fmt.Errorf("DNS domain is not configured")
	}

	var dnsIP string
	if dnsAddressOverride != "" {
		dnsIP = dnsAddressOverride
	} else if n.isLocalhostMode() {
		dnsIP = "127.0.0.1"
	} else {
		dnsIP = n.configHandler.GetString("dns.address")
		if dnsIP == "" {
			return fmt.Errorf("DNS address is not configured")
		}
	}

	resolverDir := "/etc/resolver"
	resolverFile := fmt.Sprintf("%s/%s", resolverDir, tld)
	content := fmt.Sprintf("nameserver %s\n", dnsIP)

	existingContent, err := n.shims.ReadFile(resolverFile)
	if err == nil && string(existingContent) == content {
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
