//go:build linux
// +build linux

package network

import (
	"fmt"
	"strings"
)

// The LinuxNetworkManager is a platform-specific network manager for Linux systems.
// It provides network configuration capabilities specific to Linux-based systems,
// The LinuxNetworkManager handles host route configuration and DNS setup for Linux,
// ensuring proper network connectivity between the host and guest VM environments.

// =============================================================================
// Public Methods
// =============================================================================

// ConfigureHostRoute sets up the local development network for Linux
func (n *BaseNetworkManager) ConfigureHostRoute() error {
	// Access the Docker configuration
	networkCIDR := n.configHandler.GetString("network.cidr_block")
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
		"ip",
		"route",
		"show",
		networkCIDR,
	)
	if err != nil {
		return fmt.Errorf("failed to check if route exists: %w", err)
	}

	// Check if the output contains the guest IP, indicating the route exists
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

	// Add route on the host to VM guest
	fmt.Println("🔐 Configuring host route")
	output, err = n.shell.ExecSilent(
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

// ConfigureDNS sets up DNS using systemd-resolved. If the DNS IP is configured, it ensures systemd-resolved is used
// by creating a drop-in configuration file. The function checks if /etc/resolv.conf is a symlink to
// systemd-resolved and restarts the service if necessary. It handles errors at each step to ensure
// proper DNS configuration.
func (n *BaseNetworkManager) ConfigureDNS() error {
	tld := n.configHandler.GetString("dns.domain")
	if tld == "" {
		return fmt.Errorf("DNS domain is not configured")
	}

	var dnsIP string
	if n.isLocalhostMode() {
		dnsIP = "127.0.0.1"
	} else {
		dnsIP = n.configHandler.GetString("dns.address")
		if dnsIP == "" {
			return fmt.Errorf("DNS address is not configured")
		}
	}

	// If DNS address is configured, use systemd-resolved
	resolvConf, err := n.shims.ReadLink("/etc/resolv.conf")
	if err != nil || resolvConf != "../run/systemd/resolve/stub-resolv.conf" {
		return fmt.Errorf("systemd-resolved is not in use. Please configure DNS manually or use a compatible system")
	}

	dropInDir := "/etc/systemd/resolved.conf.d"
	dropInFile := fmt.Sprintf("%s/dns-override-%s.conf", dropInDir, tld)

	existingContent, err := n.shims.ReadFile(dropInFile)
	expectedContent := fmt.Sprintf("[Resolve]\nDNS=%s\n", dnsIP)
	if err == nil && string(existingContent) == expectedContent {
		return nil
	}

	_, err = n.shell.ExecSilent(
		"sudo",
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

	fmt.Println("🔐 Restarting systemd-resolved")
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
