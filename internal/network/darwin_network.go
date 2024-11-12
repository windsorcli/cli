//go:build darwin
// +build darwin

package network

import (
	"fmt"
	"os"
)

// ConfigureHost sets up the local development network
func (n *BaseNetworkManager) ConfigureHost() error {
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

	// Add route on the host to VM guest
	output, err := n.shell.Exec(
		false,
		"Configuring host route",
		"sudo",
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
		if _, err := n.shell.Exec(
			false,
			"",
			"sudo",
			"mkdir",
			"-p",
			resolverDir,
		); err != nil {
			return fmt.Errorf("Error creating resolver directory: %w", err)
		}
	}

	// Write the DNS server to a temporary file
	tempResolverFile := fmt.Sprintf("/tmp/%s", dnsDomain)
	content := fmt.Sprintf("nameserver %s\n", dnsIP)
	// #nosec G306 - /etc/resolver files require 0644 permissions
	if err := writeFile(tempResolverFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("Error writing to temporary resolver file: %w", err)
	}

	// Move the temporary file to the /etc/resolver/<tld> file
	resolverFile := fmt.Sprintf("%s/%s", resolverDir, dnsDomain)
	if _, err := n.shell.Exec(
		false,
		"Configuring DNS resolver at "+resolverFile,
		"sudo",
		"mv",
		tempResolverFile,
		resolverFile,
	); err != nil {
		return fmt.Errorf("Error moving resolver file: %w", err)
	}

	// Flush the DNS cache
	if _, err := n.shell.Exec(false, "Flushing DNS cache", "sudo", "dscacheutil", "-flushcache"); err != nil {
		return fmt.Errorf("Error flushing DNS cache: %w", err)
	}
	if _, err := n.shell.Exec(false, "Restarting DNS daemon", "sudo", "killall", "-HUP", "mDNSResponder"); err != nil {
		return fmt.Errorf("Error restarting mDNSResponder: %w", err)
	}

	return nil
}
