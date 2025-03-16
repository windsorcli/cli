//go:build darwin
// +build darwin

package network

import (
	"fmt"
	"os"
	"strings"
)

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

	output, _, err := n.shell.ExecSilent("route", "get", networkCIDR)
	if err != nil {
		return fmt.Errorf("failed to check if route exists: %w", err)
	}

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

	output, _, err = n.shell.ExecSudo(
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
// It creates a resolver file for a specified DNS IP. It ensures directories exist,
// updates files, and flushes the DNS cache to apply changes.
func (n *BaseNetworkManager) ConfigureDNS() error {
	tld := n.configHandler.GetString("dns.domain")
	if tld == "" {
		return fmt.Errorf("DNS domain is not configured")
	}

	dnsIP := "127.0.0.1"
	if !n.UseHostNetwork() {
		dnsIP = n.configHandler.GetString("dns.address")
	}

	resolverDir := "/etc/resolver"
	if _, err := stat(resolverDir); os.IsNotExist(err) {
		_, _, err := n.shell.ExecSilent(
			"sudo",
			"mkdir",
			"-p",
			resolverDir,
		)
		if err != nil {
			return fmt.Errorf("Error creating resolver directory: %w", err)
		}
	}

	resolverFile := fmt.Sprintf("%s/%s", resolverDir, tld)
	content := fmt.Sprintf("nameserver %s\n", dnsIP)

	existingContent, err := readFile(resolverFile)
	if err == nil && string(existingContent) == content {
		return nil
	}

	tempResolverFile := fmt.Sprintf("/tmp/%s", tld)
	if err := writeFile(tempResolverFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("Error writing to temporary resolver file: %w", err)
	}

	_, _, err = n.shell.ExecSudo(
		fmt.Sprintf("🔐 Configuring DNS resolver at %s", resolverFile),
		"mv",
		tempResolverFile,
		resolverFile,
	)
	if err != nil {
		return fmt.Errorf("Error moving resolver file: %w", err)
	}

	_, _, err = n.shell.ExecSudo(
		"🔐 Flushing DNS cache",
		"dscacheutil",
		"-flushcache",
	)
	if err != nil {
		return fmt.Errorf("Error flushing DNS cache: %w", err)
	}

	_, _, err = n.shell.ExecSudo(
		"🔐 Restarting mDNSResponder",
		"killall",
		"-HUP",
		"mDNSResponder",
	)
	if err != nil {
		return fmt.Errorf("Error restarting mDNSResponder: %w", err)
	}

	return nil
}
