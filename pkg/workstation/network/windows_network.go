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

// ConfigureHostRoute sets up the local development network.
// Guest address is read from config (workstation.address). It checks if the route exists via
// PowerShell; if not, adds a route to the VM guest.
func (n *BaseNetworkManager) ConfigureHostRoute() error {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" {
		return fmt.Errorf("network CIDR is not configured")
	}
	guestIP := n.configHandler.GetString("workstation.address")
	if guestIP == "" {
		return fmt.Errorf("guest address is required")
	}

	output, err := n.shell.ExecSilent(
		"powershell",
		"-Command",
		fmt.Sprintf("Get-NetRoute -DestinationPrefix %s -ErrorAction SilentlyContinue | Where-Object { $_.NextHop -eq '%s' }", networkCIDR, guestIP),
	)
	if err != nil {
		return fmt.Errorf("failed to check if route exists: %w", err)
	}

	if strings.TrimSpace(output) != "" {
		return nil
	}

	fmt.Fprintf(os.Stderr, "\033[33m⚠\033[0m 🔐 Network configuration requires administrator privileges\n")

	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " 🔐 Configuring host route"
	spin.Start()

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

	spin.Stop()
	fmt.Fprintf(os.Stderr, "\033[32m✔ 🔐 Configuring host route - Done\033[0m\n")
	return nil
}

// ConfigureDNS sets up a per-domain DNS rule using Windows NRPT. Resolver IP is read from config
// (dns.resolver_ip, or derived: 127.0.0.1 in localhost mode, else dns.address). The domain is normalized
// to a leading-dot namespace for NRPT. If no matching rule exists or the rule's name servers differ,
// an add or update is performed with administrator privileges.
func (n *BaseNetworkManager) ConfigureDNS() error {
	domain := n.configHandler.GetString("dns.domain")
	if domain == "" {
		return fmt.Errorf("DNS domain is not configured")
	}

	dnsIP := n.effectiveResolverIP()
	if dnsIP == "" {
		return fmt.Errorf("DNS address is not configured")
	}

	namespace := "." + domain

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
		fmt.Fprintf(os.Stderr, "\033[31m✗ 🔐 Configuring DNS for '*.%s' - Failed\033[0m\n", domain)
		return fmt.Errorf("failed to check existing DNS rules for %s: %w", domain, err)
	}

	if strings.TrimSpace(output) == "False" || output == "" {
		fmt.Fprintf(os.Stderr, "\033[33m⚠\033[0m 🔐 DNS configuration requires administrator privileges\n")

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
`, namespace, dnsIP, dnsIP, domain)

		_, err = n.shell.ExecProgress(
			fmt.Sprintf("🔐 Configuring DNS for '*.%s'", domain),
			"powershell",
			"-Command",
			addOrUpdateScript,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\033[31m✗ 🔐 Configuring DNS for '*.%s' - Failed\033[0m\n", domain)
			return fmt.Errorf("failed to add or update DNS rule for %s: %w", domain, err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// needsPrivilegeForResolver reports whether the NRPT rule for the configured DNS domain is missing
// or has a different name server than desiredIP. Returns false on any error or when domain is unset.
func (n *BaseNetworkManager) needsPrivilegeForResolver(desiredIP string) bool {
	domain := n.configHandler.GetString("dns.domain")
	if domain == "" {
		return false
	}
	namespace := "." + domain
	script := fmt.Sprintf(`
$r = Get-DnsClientNrptRule | Where-Object { $_.Namespace -eq '%s' } | Select-Object -First 1
if (-not $r) { '' } else { ($r.NameServers -join ',') }
`, namespace)
	output, err := n.shell.ExecSilent("powershell", "-Command", script)
	if err != nil {
		return false
	}
	currentIP := strings.TrimSpace(output)
	if currentIP == "" {
		return true
	}
	if idx := strings.Index(currentIP, ","); idx > 0 {
		currentIP = strings.TrimSpace(currentIP[:idx])
	}
	return currentIP != desiredIP
}

// needsPrivilegeForHostRoute reports whether a host route for the configured network CIDR and guest IP
// is missing. Returns false when CIDR or guestAddress is unset or when the route check fails.
func (n *BaseNetworkManager) needsPrivilegeForHostRoute(guestAddress string) bool {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" || guestAddress == "" {
		return false
	}
	output, err := n.shell.ExecSilent(
		"powershell",
		"-Command",
		fmt.Sprintf("Get-NetRoute -DestinationPrefix %s -ErrorAction SilentlyContinue | Where-Object { $_.NextHop -eq '%s' }", networkCIDR, guestAddress),
	)
	if err != nil {
		return false
	}
	return strings.TrimSpace(output) == ""
}
