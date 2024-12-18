//go:build windows
// +build windows

package network

import (
	"fmt"
)

// ConfigureHostRoute sets up the local development network
func (n *BaseNetworkManager) ConfigureHostRoute() error {
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

	// Check if the route already exists using PowerShell command
	output, err := n.shell.Exec(
		"Checking if route exists",
		"powershell",
		"-Command",
		fmt.Sprintf("Get-NetRoute -DestinationPrefix %s | Where-Object { $_.NextHop -eq '%s' }", networkCIDR, guestIP),
	)
	if err != nil {
		return fmt.Errorf("failed to check if route exists: %w", err)
	}

	// If the output is not empty, the route exists
	if output != "" {
		return nil
	}

	// Add route on the host to VM guest using PowerShell command
	output, err = n.shell.Exec(
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

	// Check the current DNS server configuration
	currentDNSOutput, err := n.shell.Exec(
		"Checking current DNS server",
		"powershell",
		"-Command",
		"Get-DnsClientServerAddress -InterfaceAlias 'Ethernet' | Select-Object -ExpandProperty ServerAddresses",
	)
	if err != nil {
		return fmt.Errorf("failed to get current DNS server: %w", err)
	}

	// If the current DNS server is already set to the desired IP, do nothing
	if currentDNSOutput == dnsIP {
		return nil
	}

	// Execute PowerShell command to set DNS server
	output, err := n.shell.Exec(
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
