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
	dnsDomain := n.configHandler.GetString("dns.name")
	if dnsDomain == "" {
		return fmt.Errorf("DNS domain is not configured")
	}
	dnsIP := n.configHandler.GetString("dns.address")

	if dnsIP == "" {
		hostsFile := "C:\\Windows\\System32\\drivers\\etc\\hosts"
		existingContent, err := readFile(hostsFile)
		if err != nil {
			return fmt.Errorf("Error reading hosts file: %w", err)
		}

		hostsEntry := fmt.Sprintf("127.0.0.1 %s", dnsDomain)
		lines := strings.Split(string(existingContent), "\n")
		entryExists := false

		for i, line := range lines {
			if strings.Contains(line, dnsDomain) {
				lines[i] = hostsEntry
				entryExists = true
				break
			}
		}

		if !entryExists {
			lines = append(lines, hostsEntry)
		}

		if err := writeFile(hostsFile, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			return fmt.Errorf("Error writing to hosts file: %w", err)
		}

		return nil
	}

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

	return nil
}
