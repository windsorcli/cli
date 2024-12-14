//go:build linux
// +build linux

package network

import (
	"fmt"
)

// ConfigureHostRoute sets up the local development network for Linux
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

	// Check if the route already exists
	checkOutput, err := n.shell.Exec(
		"Checking existing host route",
		"ip",
		"route",
		"show",
		networkCIDR,
	)
	if err == nil && checkOutput != "" {
		// Route already exists, no need to add it again
		return nil
	}

	// Add route on the host to VM guest
	output, err := n.shell.Exec(
		"Configuring host route",
		"sudo",
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

// ConfigureDNS sets up the DNS configuration using systemd-resolved
func (n *BaseNetworkManager) ConfigureDNS() error {
	// Access the DNS configuration
	dnsIP := n.configHandler.GetString("dns.address")
	if dnsIP == "" {
		return fmt.Errorf("DNS address is not configured")
	}

	dnsDomain := n.configHandler.GetString("dns.name")
	if dnsDomain == "" {
		return fmt.Errorf("DNS domain is not configured")
	}

	// Check if /etc/resolv.conf is a symlink to systemd-resolved
	resolvConf, err := readLink("/etc/resolv.conf")
	if err != nil || resolvConf != "../run/systemd/resolve/stub-resolv.conf" {
		return fmt.Errorf("systemd-resolved is not in use. Please configure DNS manually or use a compatible system")
	}

	// Create a drop-in configuration file for DNS settings
	dropInDir := "/etc/systemd/resolved.conf.d"
	dropInFile := fmt.Sprintf("%s/dns-override-%s.conf", dropInDir, dnsDomain)

	// Check if the drop-in file already exists with the correct content
	existingContent, err := readFile(dropInFile)
	expectedContent := fmt.Sprintf("[Resolve]\nDNS=%s\n", dnsIP)
	if err == nil && string(existingContent) == expectedContent {
		// The drop-in file already exists with the correct content, no need to update
		return nil
	}

	// Ensure the drop-in directory exists
	_, err = n.shell.Exec(
		"Creating drop-in directory for resolved.conf",
		"sudo",
		"mkdir",
		"-p",
		dropInDir,
	)
	if err != nil {
		return fmt.Errorf("failed to create drop-in directory: %w", err)
	}

	// Write DNS configuration to the drop-in file
	_, err = n.shell.Exec(
		"Writing DNS configuration to drop-in file",
		"sudo",
		"bash",
		"-c",
		fmt.Sprintf("echo '%s' | sudo tee %s", expectedContent, dropInFile),
	)
	if err != nil {
		return fmt.Errorf("failed to write DNS configuration: %w", err)
	}

	// Restart systemd-resolved
	_, err = n.shell.Exec(
		"Restarting systemd-resolved",
		"sudo",
		"systemctl",
		"restart",
		"systemd-resolved",
	)
	if err != nil {
		return fmt.Errorf("failed to restart systemd-resolved: %w", err)
	}

	return nil
}
