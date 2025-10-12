//go:build windows
// +build windows

package network

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
)

// The WindowsNetworkManager is a platform-specific network manager for Windows systems.
// It provides network configuration capabilities specific to Windows-based systems,
// The WindowsNetworkManager handles host route configuration and DNS setup for Windows,
// ensuring proper network connectivity between the host and guest VM environments.

// =============================================================================
// Public Methods
// =============================================================================

// ConfigureHostRoute sets up the local development network. It checks if the route
// already exists using a PowerShell command. If not, it adds a new route on the host
// to the VM guest using another PowerShell command.
func (n *BaseNetworkManager) ConfigureHostRoute() error {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" {
		return fmt.Errorf("network CIDR is not configured")
	}

	guestIP := n.configHandler.GetString("vm.address")
	if guestIP == "" {
		return fmt.Errorf("guest IP is not configured")
	}

	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " 🔐 Configuring host route"
	spin.Start()

	output, err := n.shell.ExecSilent(
		"powershell",
		"-Command",
		fmt.Sprintf("Get-NetRoute -DestinationPrefix %s | Where-Object { $_.NextHop -eq '%s' }", networkCIDR, guestIP),
	)
	if err != nil {
		spin.Stop()
		fmt.Fprintf(os.Stderr, "\033[31m✗ 🔐 Configuring host route - Failed\033[0m\n")
		return fmt.Errorf("failed to check if route exists: %w", err)
	}

	if output == "" {
		output, err = n.shell.ExecSilent(
			"powershell",
			"-Command",
			fmt.Sprintf("New-NetRoute -DestinationPrefix %s -NextHop %s -RouteMetric 1", networkCIDR, guestIP),
		)
		if err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ 🔐 Configuring host route - Failed\033[0m\n")
			return fmt.Errorf("failed to add route: %w, output: %s", err, output)
		}
	}

	spin.Stop()
	fmt.Fprintf(os.Stderr, "\033[32m✔ 🔐 Configuring host route - Done\033[0m\n")
	return nil
}

// ConfigureDNS sets up a per-domain DNS rule for a specific host name using Windows
// Name Resolution Policy Table (NRPT). This ensures only the specified domain queries
// are sent to the local DNS server.
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

	// Prepend a "." to the domain for the namespace
	namespace := "." + tld

	// Check if the DNS rule for the host name is already set
	checkScript := fmt.Sprintf(`
$namespace = '%s'
$allRules = Get-DnsClientNrptRule
$existingRule = $allRules | Where-Object { $_.Namespace -eq $namespace }
if ($existingRule) {
  if ($existingRule.NameServers -ne "%s") {
    $false
  } else {
    $true
  }
} else {
  $false
}
`, namespace, dnsIP)

	output, err := n.shell.ExecSilent(
		"powershell",
		"-Command",
		checkScript,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31m✗ 🔐 Configuring DNS for '*.%s' - Failed\033[0m\n", tld)
		return fmt.Errorf("failed to check existing DNS rules for %s: %w", tld, err)
	}

	// Add or update the DNS rule for the host name if necessary
	if strings.TrimSpace(output) == "False" || output == "" {
		addOrUpdateScript := fmt.Sprintf(`
$namespace = '%s'
$existingRule = Get-DnsClientNrptRule | Where-Object { $_.Namespace -eq $namespace }
if ($existingRule) {
  Set-DnsClientNrptRule -Namespace $namespace -NameServers "%s"
} else {
  Add-DnsClientNrptRule -Namespace $namespace -NameServers "%s" -DisplayName "Local DNS for %s"
}
if ($?) {
  Clear-DnsClientCache
}
`, namespace, dnsIP, dnsIP, tld)

		_, err = n.shell.ExecProgress(
			fmt.Sprintf("🔐 Configuring DNS for '*.%s'", tld),
			"powershell",
			"-Command",
			addOrUpdateScript,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\033[31m✗ 🔐 Configuring DNS for '*.%s' - Failed\033[0m\n", tld)
			return fmt.Errorf("failed to add or update DNS rule for %s: %w", tld, err)
		}
	}

	return nil
}
