//go:build windows
// +build windows

// The WindowsNetworkManager is the network manager for Windows.
// It adds the host route to the guest VM network.
// It configures DNS for the context domain with a Name Resolution Policy Table rule.

package network

import (
	"context"
	"fmt"
	"strings"

	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Constants
// =============================================================================

// NRPT PowerShell scripts read their parameters from $env:WINDSOR_NRPT_* set by the Go side via
// ExecSilentWithEnv / ExecProgressWithEnv, eliminating fmt.Sprintf interpolation into PowerShell
// source strings. TF-output-derived values (domain, dns IP) are validated upstream, so this is
// defense-in-depth, but it also makes the scripts easier to read in isolation.

// nrptCheckScript returns the comma-joined NameServers for the first NRPT rule matching the
// configured namespace, or empty when no rule exists. Go parses the first entry for compare.
const nrptCheckScript = `
$r = Get-DnsClientNrptRule | Where-Object { $_.Namespace -eq $env:WINDSOR_NRPT_NAMESPACE } | Select-Object -First 1
if (-not $r) { '' } else { ($r.NameServers -join ',') }
`

// nrptAddOrUpdateScript installs or updates the per-domain NRPT rule with administrator privileges.
// Display name reads $env:WINDSOR_NRPT_DOMAIN; NameServer reads $env:WINDSOR_NRPT_DNS.
const nrptAddOrUpdateScript = `
$namespace = $env:WINDSOR_NRPT_NAMESPACE
$dns = $env:WINDSOR_NRPT_DNS
$existingRule = Get-DnsClientNrptRule | Where-Object { $_.Namespace -eq $namespace }
if ($existingRule) {
  Set-DnsClientNrptRule -Namespace $namespace -NameServers $dns
} else {
  Add-DnsClientNrptRule -Namespace $namespace -NameServers $dns -DisplayName ("Local DNS for " + $env:WINDSOR_NRPT_DOMAIN)
}
if ($?) {
  Clear-DnsClientCache
}
`

// nrptRevertScript removes the per-domain NRPT rule. -ErrorAction SilentlyContinue keeps revert
// idempotent: a missing rule is not an error.
const nrptRevertScript = `
Remove-DnsClientNrptRule -Namespace $env:WINDSOR_NRPT_NAMESPACE -Force -ErrorAction SilentlyContinue
`

// nrptEffectiveFirstNameServerScript returns the first effective NameServer for the configured
// namespace (post-GPO merge), or empty when no effective rule exists. Used by R23 to detect when a
// Group Policy is shadowing our local NRPT rule.
const nrptEffectiveFirstNameServerScript = `
$rule = Get-DnsClientNrptPolicy -Effective -ErrorAction SilentlyContinue | Where-Object { $_.Namespace -eq $env:WINDSOR_NRPT_NAMESPACE } | Select-Object -First 1
if ($rule) {
  $servers = $rule.NameServers -join ','
  ($servers.Split(',') | Select-Object -First 1).Trim()
}
`

// =============================================================================
// Public Methods
// =============================================================================

// ConfigureHostRoute sets up the local development network.
// Guest address is read from config (workstation.address). It checks if the route exists via
// PowerShell; if not, adds a route to the VM guest.
func (n *BaseNetworkManager) ConfigureHostRoute(ctx context.Context) error {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" {
		networkCIDR = constants.DefaultNetworkCIDR
	}
	cidr, err := validateCIDR(networkCIDR)
	if err != nil {
		return err
	}
	guestIP := n.configHandler.GetString("workstation.address")
	if guestIP == "" {
		return fmt.Errorf("guest address is required")
	}
	guest, err := validateIPAddress(guestIP)
	if err != nil {
		return werror.Wrap(err, "invalid workstation.address")
	}

	output, err := n.shell.ExecSilent(
		"powershell",
		"-Command",
		fmt.Sprintf("Get-NetRoute -DestinationPrefix %s -ErrorAction SilentlyContinue | Where-Object { $_.NextHop -eq '%s' }", cidr, guest),
	)
	if err != nil {
		return werror.Wrap(err, "failed to check if route exists")
	}

	if strings.TrimSpace(output) != "" {
		return nil
	}

	return presenter.Track(ctx, "network:route", "Configuring host route", func(ctx context.Context) error {
		presenter.Warn(ctx, "This step requires elevated privileges.")
		output, err := n.shell.ExecSilent(
			"powershell",
			"-Command",
			fmt.Sprintf("New-NetRoute -DestinationPrefix %s -NextHop %s -RouteMetric 1", cidr, guest),
		)
		if err != nil {
			return fmt.Errorf("failed to add route: %w, output: %s", err, output)
		}
		return nil
	})
}

// ConfigureDNS installs an NRPT rule that sends queries for dns.domain to the resolver IP.
// When the domain or the resolver IP is not set, it returns an error with a remediation hint.
// It adds or updates the rule only when the first name server of the rule is not the resolver IP.
// It then reads the effective NRPT policy and shows a warning when a GPO overrides the rule.
// The arguments go to PowerShell in $env:WINDSOR_NRPT_* variables, not in the script text.
func (n *BaseNetworkManager) ConfigureDNS(ctx context.Context) error {
	domain := n.configHandler.GetString("dns.domain")
	if domain == "" {
		return werror.New("DNS domain is not configured", dnsDomainHint, nil)
	}
	if err := validateDomain(domain); err != nil {
		return err
	}

	dnsIP := n.effectiveResolverIP()
	if dnsIP == "" {
		return werror.New("DNS address is not configured", dnsAddressHint, nil)
	}
	dns, err := validateIPAddress(dnsIP)
	if err != nil {
		return werror.Wrap(err, "invalid DNS resolver address")
	}

	namespace := "." + domain
	nrptEnv := map[string]string{
		"WINDSOR_NRPT_NAMESPACE": namespace,
		"WINDSOR_NRPT_DNS":       dns,
		"WINDSOR_NRPT_DOMAIN":    domain,
	}

	output, err := n.shell.ExecSilentWithEnv(
		"powershell",
		nrptEnv,
		"-Command",
		nrptCheckScript,
	)
	if err != nil {
		return werror.Wrap(err, "failed to check existing DNS rules for %s", domain)
	}

	currentIP := strings.TrimSpace(output)
	if idx := strings.Index(currentIP, ","); idx > 0 {
		currentIP = strings.TrimSpace(currentIP[:idx])
	}

	if currentIP != dns {
		n.dnsChanged = true
		if err := presenter.Track(ctx, "network:dns", fmt.Sprintf("Configuring DNS for '*.%s'", domain), func(ctx context.Context) error {
			presenter.Warn(ctx, "This step requires elevated privileges.")
			if _, err := n.shell.ExecProgressWithEnv(fmt.Sprintf("Configuring DNS for '*.%s'", domain), "powershell", nrptEnv, "-Command", nrptAddOrUpdateScript); err != nil {
				return werror.Wrap(err, "failed to add or update DNS rule for %s", domain)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	if msg := n.gpoOverridesNrptRuleWarning(nrptEnv, dns); msg != "" {
		presenter.Warn(ctx, msg)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// RevertHostRoute removes the host-to-guest route previously added by ConfigureHostRoute on
// Windows. Idempotent via -ErrorAction SilentlyContinue: a missing route does not surface an
// error. Like the other Windows network operations, requires the cobra command to already be
// running in an elevated PowerShell — there is no equivalent of 'sudo' to call from here.
func (n *BaseNetworkManager) RevertHostRoute() error {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" {
		return nil
	}
	cidr, err := validateCIDR(networkCIDR)
	if err != nil {
		return err
	}
	script := fmt.Sprintf("Remove-NetRoute -DestinationPrefix %s -Confirm:$false -ErrorAction SilentlyContinue", cidr)
	if _, err := n.shell.ExecSilent("powershell", "-Command", script); err != nil {
		return werror.Wrap(err, "failed to remove host route")
	}
	return nil
}

// RevertDNS removes the NRPT rule installed by ConfigureDNS on Windows. Idempotent via
// -ErrorAction SilentlyContinue: a missing rule does not surface an error.
func (n *BaseNetworkManager) RevertDNS() error {
	domain := n.configHandler.GetString("dns.domain")
	if domain == "" {
		return nil
	}
	if err := validateDomain(domain); err != nil {
		return err
	}
	env := map[string]string{"WINDSOR_NRPT_NAMESPACE": "." + domain}
	if _, err := n.shell.ExecSilentWithEnv("powershell", env, "-Command", nrptRevertScript); err != nil {
		return werror.Wrap(err, "failed to remove DNS rule")
	}
	return nil
}

// FlushDNS clears the Windows DNS client cache via PowerShell.
func (n *BaseNetworkManager) FlushDNS(ctx context.Context) error {
	if _, err := n.shell.ExecSilent("powershell", "-Command", "Clear-DnsClientCache"); err != nil {
		return werror.Wrap(err, "Error flushing DNS cache")
	}
	return nil
}

// needsPrivilegeForResolver reports whether the NRPT rule for the configured DNS domain is missing
// or has a different name server than desiredIP. Returns false on any error or when domain is unset.
func (n *BaseNetworkManager) needsPrivilegeForResolver(desiredIP string) bool {
	domain := n.configHandler.GetString("dns.domain")
	if domain == "" {
		return false
	}
	if err := validateDomain(domain); err != nil {
		return false
	}
	env := map[string]string{"WINDSOR_NRPT_NAMESPACE": "." + domain}
	output, err := n.shell.ExecSilentWithEnv("powershell", env, "-Command", nrptCheckScript)
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

// gpoOverridesNrptRuleWarning queries the effective NRPT policy (post-GPO merge) and returns a
// non-fatal hint when the first effective name server for our namespace differs from desiredIP.
// Returns "" when there is no effective rule, when the call fails, or when the effective server
// matches — all benign on stand-alone machines and on GPO-friendly enterprise images. The "%[1]s"
// in the warning is the GPO-served IP so operators can spot which policy is winning.
func (n *BaseNetworkManager) gpoOverridesNrptRuleWarning(env map[string]string, desiredIP string) string {
	output, err := n.shell.ExecSilentWithEnv("powershell", env, "-Command", nrptEffectiveFirstNameServerScript)
	if err != nil {
		return ""
	}
	effectiveIP := strings.TrimSpace(output)
	if effectiveIP == "" || effectiveIP == desiredIP {
		return ""
	}
	return fmt.Sprintf("\n⚠ NRPT rule for *.%s was added locally, but the effective rule resolves to a different name server (%s). This usually means a Group Policy is overriding the local rule. Contact your IT administrator to permit per-machine NRPT for *.%s.", env["WINDSOR_NRPT_DOMAIN"], effectiveIP, env["WINDSOR_NRPT_DOMAIN"])
}

// needsPrivilegeForHostRoute reports whether a host route for the configured network CIDR and guest IP
// is missing. Returns false when CIDR or guestAddress is unset or when the route check fails.
func (n *BaseNetworkManager) needsPrivilegeForHostRoute(guestAddress string) bool {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" || guestAddress == "" {
		return false
	}
	cidr, err := validateCIDR(networkCIDR)
	if err != nil {
		return false
	}
	guest, err := validateIPAddress(guestAddress)
	if err != nil {
		return false
	}
	output, err := n.shell.ExecSilent(
		"powershell",
		"-Command",
		fmt.Sprintf("Get-NetRoute -DestinationPrefix %s -ErrorAction SilentlyContinue | Where-Object { $_.NextHop -eq '%s' }", cidr, guest),
	)
	if err != nil {
		return false
	}
	return strings.TrimSpace(output) == ""
}
