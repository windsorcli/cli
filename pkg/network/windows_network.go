//go:build windows
// +build windows

package network

import (
	"fmt"
	"strings"
)

// ConfigureHostRoute sets up the local development network. It checks if the route
// already exists using a PowerShell command. If not, it adds a new route on the host
// to the VM guest using another PowerShell command.
func (n *BaseNetworkManager) ConfigureHostRoute() error {
	networkCIDR := n.configHandler.GetString("docker.network_cidr")
	if networkCIDR == "" {
		return fmt.Errorf("network CIDR is not configured")
	}

	guestIP := n.configHandler.GetString("vm.address")
	if guestIP == "" {
		return fmt.Errorf("guest IP is not configured")
	}

	output, err := n.shell.ExecSilent(
		"powershell",
		"-Command",
		fmt.Sprintf("Get-NetRoute -DestinationPrefix %s | Where-Object { $_.NextHop -eq '%s' }", networkCIDR, guestIP),
	)
	if err != nil {
		return fmt.Errorf("failed to check if route exists: %w", err)
	}

	if output != "" {
		return nil
	}

	fmt.Println("🔐 Adding route on the host to VM guest")
	output, err = n.shell.ExecSilent(
		"powershell",
		"-Command",
		fmt.Sprintf("New-NetRoute -DestinationPrefix %s -NextHop %s -RouteMetric 1", networkCIDR, guestIP),
	)
	if err != nil {
		return fmt.Errorf("failed to add route: %w, output: %s", err, output)
	}

	return nil
}

// ConfigureDNS sets up the DNS configuration. If the DNS address is not configured, it routes
// the DNS name wildcard to localhost using the hosts file. If the DNS address is configured,
// it sets the DNS server using PowerShell and flushes the DNS cache to ensure changes take
// effect.
func (n *BaseNetworkManager) ConfigureDNS() error {
	tld := n.configHandler.GetString("dns.name")
	if tld == "" {
		return fmt.Errorf("DNS TLD is not configured")
	}
	dnsIP := n.configHandler.GetString("dns.address")

	// Always update the hosts file
	if err := n.updateHostsFile(tld); err != nil {
		return fmt.Errorf("failed to update hosts file: %w", err)
	}

	// Proceed with DNS server configuration if DNS IP is provided
	if dnsIP != "" {
		currentDNSOutput, err := n.shell.ExecSilent(
			"powershell",
			"-Command",
			"Get-DnsClientServerAddress -InterfaceAlias 'Ethernet' | Select-Object -ExpandProperty ServerAddresses",
		)
		if err != nil {
			return fmt.Errorf("failed to get current DNS server: %w", err)
		}

		if strings.Contains(currentDNSOutput, dnsIP) {
			return nil
		}

		fmt.Println("🔐 Setting DNS server")
		output, err := n.shell.ExecSilent(
			"powershell",
			"-Command",
			fmt.Sprintf("Set-DnsClientServerAddress -InterfaceAlias 'Ethernet' -ServerAddresses %s", dnsIP),
		)
		if err != nil {
			return fmt.Errorf("failed to set DNS server: %w, output: %s", err, output)
		}

		_, err = n.shell.ExecSilent(
			"powershell",
			"-Command",
			"Clear-DnsClientCache",
		)
		if err != nil {
			return fmt.Errorf("failed to flush DNS cache: %w", err)
		}
	}

	return nil
}
