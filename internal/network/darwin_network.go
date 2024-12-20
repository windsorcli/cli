//go:build darwin
// +build darwin

package network

import (
	"fmt"
	"os"
	"strings"
)

// ConfigureHostRoute sets up the local development network
func (n *BaseNetworkManager) ConfigureHostRoute() error {
	// Access the Docker configuration
	networkCIDR := n.configHandler.GetString("docker.network_cidr")
	if networkCIDR == "" {
		return fmt.Errorf("network CIDR is not configured")
	}

	// Access the VM configuration
	guestIP := n.configHandler.GetString("vm.address")
	if guestIP == "" {
		return fmt.Errorf("guest IP is not configured")
	}

	// Use the shell to execute a command that checks the routing table for the specific route
	output, err := n.shell.ExecSilent(
		"route",
		"get",
		networkCIDR,
	)
	if err != nil {
		return fmt.Errorf("failed to check if route exists: %w", err)
	}

	// Check if the output contains the gateway IP, indicating the route exists
	lines := strings.Split(output, "\n")
	routeExists := false
	for _, line := range lines {
		if strings.Contains(line, "gateway:") {
			parts := strings.Fields(line)
			if len(parts) == 2 && parts[1] == guestIP {
				routeExists = true
				break
			}
		}
	}

	if routeExists {
		return nil
	}

	// Add route on the host to VM guest
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

// ConfigureDNS sets up the DNS configuration
func (n *BaseNetworkManager) ConfigureDNS() error {
	// Access the DNS configuration using GetString
	dnsDomain := n.configHandler.GetString("dns.name")
	if dnsDomain == "" {
		return fmt.Errorf("DNS domain is not configured")
	}
	dnsIP := n.configHandler.GetString("dns.address")
	if dnsIP == "" {
		return fmt.Errorf("DNS address is not configured")
	}

	// Ensure the /etc/resolver directory exists
	resolverDir := "/etc/resolver"
	if _, err := stat(resolverDir); os.IsNotExist(err) {
		if _, err := n.shell.ExecSilent(
			"sudo",
			"mkdir",
			"-p",
			resolverDir,
		); err != nil {
			return fmt.Errorf("Error creating resolver directory: %w", err)
		}
	}

	// Check if the resolver file already exists with the correct content
	resolverFile := fmt.Sprintf("%s/%s", resolverDir, dnsDomain)
	existingContent, err := readFile(resolverFile)
	if err == nil && string(existingContent) == fmt.Sprintf("nameserver %s\n", dnsIP) {
		// The resolver file already exists with the correct content, no need to update
		return nil
	}

	// Write the DNS server to a temporary file
	tempResolverFile := fmt.Sprintf("/tmp/%s", dnsDomain)
	content := fmt.Sprintf("nameserver %s\n", dnsIP)
	// #nosec G306 - /etc/resolver files require 0644 permissions
	if err := writeFile(tempResolverFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("Error writing to temporary resolver file: %w", err)
	}

	// Move the temporary file to the /etc/resolver/<tld> file
	if _, err := n.shell.ExecSudo(
		fmt.Sprintf("🔐 Configuring DNS resolver at %s\n", resolverFile),
		"mv",
		tempResolverFile,
		resolverFile,
	); err != nil {
		return fmt.Errorf("Error moving resolver file: %w", err)
	}

	// Flush the DNS cache
	if _, err := n.shell.ExecSudo(
		"🔐 Flushing DNS cache",
		"dscacheutil",
		"-flushcache",
	); err != nil {
		return fmt.Errorf("Error flushing DNS cache: %w", err)
	}

	// Restart mDNSResponder
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
