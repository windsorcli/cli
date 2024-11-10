//go:build darwin
// +build darwin

package network

import (
	"fmt"
	"os"
)

// ConfigureHost sets up the local development network
func (n *networkManager) ConfigureHost() error {
	// Retrieve the entire configuration object
	contextConfig := n.configHandler.GetConfig()

	// Access the Docker configuration
	if contextConfig.Docker == nil || contextConfig.Docker.NetworkCIDR == nil {
		return fmt.Errorf("network CIDR is not configured")
	}
	networkCIDR := *contextConfig.Docker.NetworkCIDR

	// Access the VM configuration
	if contextConfig.VM == nil || contextConfig.VM.Driver == nil {
		return fmt.Errorf("guest IP is not configured")
	}
	guestIP := *contextConfig.VM.Driver

	// Add route on the host to VM guest
	output, err := n.shell.Exec(
		false,
		"",
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

	// Configure the DNS
	return n.configureDNS()
}

// ConfigureDNS sets up the DNS configuration
func (n *networkManager) configureDNS() error {
	// Retrieve the entire configuration object
	contextConfig := n.configHandler.GetConfig()

	// Access the DNS configuration
	if contextConfig.DNS == nil || contextConfig.DNS.Name == nil || contextConfig.DNS.IP == nil {
		return fmt.Errorf("DNS configuration is not properly set")
	}
	dnsDomain := *contextConfig.DNS.Name
	dnsIP := *contextConfig.DNS.IP

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
	if err := os.WriteFile(tempResolverFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("Error writing to temporary resolver file: %w", err)
	}

	// Move the temporary file to the /etc/resolver/<tld> file
	resolverFile := fmt.Sprintf("%s/%s", resolverDir, dnsDomain)
	if _, err := n.shell.Exec(
		false,
		"",
		"sudo",
		"mv",
		tempResolverFile,
		resolverFile,
	); err != nil {
		return fmt.Errorf("Error moving resolver file: %w", err)
	}

	// Flush the DNS cache
	if _, err := n.shell.Exec(false, "", "sudo", "dscacheutil", "-flushcache"); err != nil {
		return fmt.Errorf("Error flushing DNS cache: %w", err)
	}
	if _, err := n.shell.Exec(false, "", "sudo", "killall", "-HUP", "mDNSResponder"); err != nil {
		return fmt.Errorf("Error restarting mDNSResponder: %w", err)
	}

	return nil
}
