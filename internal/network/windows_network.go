//go:build windows
// +build windows

package network

import (
	"fmt"
)

// ConfigureHost sets up the local development network
func (n *networkManager) ConfigureHost() error {
	// Access the Docker configuration using GetString
	networkCIDR := n.configHandler.GetString("docker.network_cidr")
	if networkCIDR == "" {
		return fmt.Errorf("network CIDR is not configured")
	}

	// Access the VM configuration using GetString
	guestIP := n.configHandler.GetString("vm.address")
	if guestIP == "" {
		return fmt.Errorf("guest IP is not configured")
	}

	// Add route on the host to VM guest
	output, err := n.shell.Exec(
		true,
		"Adding route on the host to VM guest",
		"powershell",
		"-Command",
		fmt.Sprintf("New-NetRoute -DestinationPrefix %s -NextHop %s -RouteMetric 1", networkCIDR, guestIP),
	)
	if err != nil {
		return fmt.Errorf("failed to add route: %w, output: %s", err, output)
	}

	return nil
}

// ConfigureDNS sets up the DNS configuration
func (n *networkManager) ConfigureDNS() error {
	// Access the DNS configuration using GetString
	dnsDomain := n.configHandler.GetString("dns.name")
	if dnsDomain == "" {
		return fmt.Errorf("DNS domain is not configured")
	}
	dnsIP := n.configHandler.GetString("dns.ip")
	if dnsIP == "" {
		return fmt.Errorf("DNS IP is not configured")
	}

	// Execute PowerShell command to set DNS server
	output, err := n.shell.Exec(
		true,
		"Setting DNS server",
		"powershell",
		"-Command",
		fmt.Sprintf("Set-DnsClientServerAddress -InterfaceAlias 'Ethernet' -ServerAddresses %s", dnsIP),
	)
	if err != nil {
		return fmt.Errorf("failed to set DNS server: %w, output: %s", err, output)
	}

	return nil
}
