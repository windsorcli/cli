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

// ConfigureHostRoute sets up the local development network for Linux
func (n *BaseNetworkManager) ConfigureHostRoute() error {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" {
		return fmt.Errorf("network CIDR is not configured")
	}

	guestIP := n.configHandler.GetString("vm.address")
	if guestIP == "" {
		return fmt.Errorf("guest IP is not configured")
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

// ConfigureDNS sets up DNS using systemd-resolved. When dnsAddressOverride is non-empty it is used;
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
