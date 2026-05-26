//go:build darwin
// +build darwin

package network

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/windsorcli/cli/pkg/tui"
)

// The DarwinNetworkManager is a platform-specific network manager for macOS.
// It provides network configuration capabilities specific to Darwin-based systems,
// The DarwinNetworkManager handles host route configuration and DNS setup for macOS,
// ensuring proper network connectivity between the host and guest VM environments.

// =============================================================================
// Public Methods
// =============================================================================

// ConfigureHostRoute ensures that a network route from the host to the VM guest is established.
// Guest address is read from config (workstation.address). Installs the route in the live routing
// table via `route add` when missing, then registers it as an additional route via
// `networksetup -setadditionalroutes` so it survives reboots. The persistence step is best-effort:
// if no primary network service can be detected, the live route is left in place and a warning is
// printed.
func (n *BaseNetworkManager) ConfigureHostRoute() error {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" {
		return fmt.Errorf("network CIDR is not configured")
	}
	guestIP := n.configHandler.GetString("workstation.address")
	if guestIP == "" {
		return fmt.Errorf("guest address is required")
	}

	routeInKernel, err := n.macOSRouteInKernel(networkCIDR, guestIP)
	if err != nil {
		return err
	}

	service, getRoutesOutput, err := n.macOSPrimaryServiceAndRoutes()
	if err != nil {
		// Persistence unavailable — fall back to ephemeral-only with warning.
		if !routeInKernel {
			tui.Pause()
			if os.Geteuid() != 0 && !n.sudoCached() {
				fmt.Fprintf(os.Stderr, "\033[33m⚠\033[0m Network configuration may require elevated privileges\n")
			}
			if out, addErr := n.shell.ExecSudo("Adding host route", "route", "-nv", "add", "-net", networkCIDR, guestIP); addErr != nil {
				return fmt.Errorf("failed to add route: %w, output: %s", addErr, out)
			}
		}
		fmt.Fprintf(os.Stderr, "⚠ host route to %s installed but will not survive reboot on this system: %v\n", guestIP, err)
		return nil
	}

	routePersistent := macOSRouteAlreadyPersistent(getRoutesOutput, networkCIDR, guestIP)

	if routeInKernel && routePersistent {
		return nil
	}

	tui.Pause()
	if os.Geteuid() != 0 && !n.sudoCached() {
		fmt.Fprintf(os.Stderr, "\033[33m⚠\033[0m Network configuration may require elevated privileges\n")
	}

	if !routeInKernel {
		if out, err := n.shell.ExecSudo("Adding host route", "route", "-nv", "add", "-net", networkCIDR, guestIP); err != nil {
			return fmt.Errorf("failed to add route: %w, output: %s", err, out)
		}
	}

	if !routePersistent {
		args, err := macOSSetAdditionalRoutesArgs(service, getRoutesOutput, networkCIDR, guestIP)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠ host route to %s installed but persistence step skipped: %v\n", guestIP, err)
			return nil
		}
		if _, err := n.shell.ExecSudo("Persisting host route", "networksetup", args...); err != nil {
			fmt.Fprintf(os.Stderr, "⚠ host route to %s installed but persistence step failed: %v\n", guestIP, err)
		}
	}
	return nil
}

// ConfigureDNS sets up DNS by modifying system files to route DNS queries.
// Resolver IP is read from config via effectiveResolverIP (dns.address, or 127.0.0.1 in localhost mode when unset).
func (n *BaseNetworkManager) ConfigureDNS() error {
	tld := n.configHandler.GetString("dns.domain")
	if tld == "" {
		return fmt.Errorf("DNS domain is not configured")
	}
	if err := validateDomain(tld); err != nil {
		return err
	}

	dnsIP := n.effectiveResolverIP()
	if dnsIP == "" {
		return fmt.Errorf("DNS address is not configured")
	}

	resolverDir := "/etc/resolver"
	resolverFile := fmt.Sprintf("%s/%s", resolverDir, tld)
	content := fmt.Sprintf("nameserver %s\n", dnsIP)

	existingContent, err := n.shims.ReadFile(resolverFile)
	if err == nil && resolverAlreadyConfigured(string(existingContent), dnsIP) {
		return nil
	}

	n.dnsChanged = true
	tui.Pause()
	if os.Geteuid() != 0 && !n.sudoCached() {
		fmt.Fprintf(os.Stderr, "\033[33m⚠\033[0m DNS configuration may require elevated privileges\n")
	}

	if _, err := n.shims.Stat(resolverDir); os.IsNotExist(err) {
		if _, err := n.shell.ExecSudo("", "mkdir", "-p", resolverDir); err != nil {
			return fmt.Errorf("Error creating resolver directory: %w", err)
		}
	}

	if err := n.writeFileWithSudo(resolverFile, []byte(content)); err != nil {
		return fmt.Errorf("Error installing resolver file: %w", err)
	}

	return nil
}

// RevertHostRoute removes the host-to-guest route previously added by ConfigureHostRoute on macOS.
// Idempotent: no-op when the network CIDR is unset, tolerates "not in table" for the live route
// delete, and removes the matching networksetup additional-route entry when present.
func (n *BaseNetworkManager) RevertHostRoute() error {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" {
		return nil
	}
	output, err := n.shell.ExecSudo("", "route", "-nv", "delete", "-net", networkCIDR)
	if err != nil {
		if !strings.Contains(output, "not in table") {
			return fmt.Errorf("Error deleting host route: %w, output: %s", err, output)
		}
	}
	if service, getRoutesOutput, err := n.macOSPrimaryServiceAndRoutes(); err == nil {
		guestIP := n.configHandler.GetString("workstation.address")
		args, removed := macOSRemoveAdditionalRouteArgs(service, getRoutesOutput, networkCIDR, guestIP)
		if removed {
			if _, err := n.shell.ExecSudo("", "networksetup", args...); err != nil {
				return fmt.Errorf("Error removing persistent host route: %w", err)
			}
		}
	}
	return nil
}

// RevertDNS removes the per-domain resolver file installed by ConfigureDNS on macOS.
// Idempotent: no-op when dns.domain is unset; rm -f tolerates a missing file silently.
func (n *BaseNetworkManager) RevertDNS() error {
	tld := n.configHandler.GetString("dns.domain")
	if tld == "" {
		return nil
	}
	if err := validateDomain(tld); err != nil {
		return err
	}
	resolverFile := fmt.Sprintf("/etc/resolver/%s", tld)
	if _, err := n.shell.ExecSudo("", "rm", "-f", resolverFile); err != nil {
		return fmt.Errorf("Error removing resolver file: %w", err)
	}
	return nil
}

// FlushDNS flushes the macOS DNS cache by running dscacheutil and restarting mDNSResponder.
func (n *BaseNetworkManager) FlushDNS() error {
	tui.Pause()
	if os.Geteuid() != 0 && !n.sudoCached() {
		fmt.Fprintf(os.Stderr, "\033[33m⚠\033[0m DNS cache flush may require elevated privileges\n")
	}
	if _, err := n.shell.ExecSudo(
		"",
		"dscacheutil",
		"-flushcache",
	); err != nil {
		return fmt.Errorf("Error flushing DNS cache: %w", err)
	}

	if _, err := n.shell.ExecSudo(
		"",
		"killall",
		"-HUP",
		"mDNSResponder",
	); err != nil {
		return fmt.Errorf("Error restarting mDNSResponder: %w", err)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// sudoCached reports whether sudo credentials are currently cached, i.e., sudo -n true succeeds without a password prompt.
func (n *BaseNetworkManager) sudoCached() bool {
	_, err := n.shell.ExecSilent("sudo", "-n", "true")
	return err == nil
}

// macOSRouteInKernel reports whether the live routing table already has the configured CIDR
// routed via guestIP. Backed by `route -n get <networkPrefix>` parsed for destination + gateway.
func (n *BaseNetworkManager) macOSRouteInKernel(networkCIDR, guestIP string) (bool, error) {
	networkPrefix := strings.Split(networkCIDR, "/")[0]
	output, err := n.shell.ExecSilent("route", "-n", "get", networkPrefix)
	if err != nil {
		return false, fmt.Errorf("failed to check if route exists: %w", err)
	}
	found := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
			key := strings.ToLower(strings.TrimSpace(parts[0]))
			val := strings.TrimSpace(parts[1])
			found[key] = val
		}
	}
	return strings.TrimSpace(found["destination"]) == networkPrefix && strings.TrimSpace(found["gateway"]) == guestIP, nil
}

// macOSPrimaryServiceAndRoutes returns the primary network service name (the first non-disabled
// service from `networksetup -listallnetworkservices`) and the current additional-routes block
// for that service (`networksetup -getadditionalroutes <service>`). The output blob is passed to
// downstream helpers so they can either parse existing triples or emit the right replacement set.
func (n *BaseNetworkManager) macOSPrimaryServiceAndRoutes() (service string, routesOutput string, err error) {
	listOutput, err := n.shell.ExecSilent("networksetup", "-listallnetworkservices")
	if err != nil {
		return "", "", fmt.Errorf("failed to list network services: %w", err)
	}
	service = pickPrimaryNetworkService(listOutput)
	if service == "" {
		return "", "", fmt.Errorf("no enabled network service found")
	}
	routesOutput, err = n.shell.ExecSilent("networksetup", "-getadditionalroutes", service)
	if err != nil {
		return "", "", fmt.Errorf("failed to read additional routes for %q: %w", service, err)
	}
	return service, routesOutput, nil
}

// pickPrimaryNetworkService returns the first enabled service name from the output of
// `networksetup -listallnetworkservices`. Disabled services are prefixed with `*` in the listing
// and skipped. The preamble line ("An asterisk (*) denotes ...") is recognised and skipped too.
func pickPrimaryNetworkService(listOutput string) string {
	for _, line := range strings.Split(listOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "An asterisk") || strings.HasPrefix(line, "*") {
			continue
		}
		return line
	}
	return ""
}

// parseAdditionalRoutes parses the body of `networksetup -getadditionalroutes <service>` into a
// slice of {dest, mask, router} triples. "There aren't any" / blank output yields an empty slice.
func parseAdditionalRoutes(getRoutesOutput string) [][3]string {
	var triples [][3]string
	for _, line := range strings.Split(getRoutesOutput, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && net.ParseIP(fields[0]) != nil && net.ParseIP(fields[2]) != nil {
			triples = append(triples, [3]string{fields[0], fields[1], fields[2]})
		}
	}
	return triples
}

// macOSRouteAlreadyPersistent reports whether the desired (cidr, gateway) is already among the
// service's persistent additional routes.
func macOSRouteAlreadyPersistent(getRoutesOutput, networkCIDR, guestIP string) bool {
	dest, mask, err := cidrToDestMask(networkCIDR)
	if err != nil {
		return false
	}
	for _, t := range parseAdditionalRoutes(getRoutesOutput) {
		if t[0] == dest && t[1] == mask && t[2] == guestIP {
			return true
		}
	}
	return false
}

// macOSSetAdditionalRoutesArgs builds the argv for `networksetup -setadditionalroutes` that
// appends the new (cidr, gateway) entry to the service's existing triples. Caller invokes
// `n.shell.ExecSudo("", "networksetup", args...)`.
func macOSSetAdditionalRoutesArgs(service, getRoutesOutput, networkCIDR, guestIP string) ([]string, error) {
	dest, mask, err := cidrToDestMask(networkCIDR)
	if err != nil {
		return nil, err
	}
	triples := parseAdditionalRoutes(getRoutesOutput)
	triples = append(triples, [3]string{dest, mask, guestIP})
	args := []string{"-setadditionalroutes", service}
	for _, t := range triples {
		args = append(args, t[0], t[1], t[2])
	}
	return args, nil
}

// macOSRemoveAdditionalRouteArgs builds the argv for `networksetup -setadditionalroutes` that
// drops any matching entry for the supplied (cidr, gateway). When guestIP is unset (revert called
// with no workstation.address), drops any entry whose dest+mask matches regardless of gateway —
// catches stale rows where the gateway changed between configure and revert. Returns removed=false
// when there is nothing to remove, so the caller can skip the sudo call entirely.
func macOSRemoveAdditionalRouteArgs(service, getRoutesOutput, networkCIDR, guestIP string) (args []string, removed bool) {
	dest, mask, err := cidrToDestMask(networkCIDR)
	if err != nil {
		return nil, false
	}
	original := parseAdditionalRoutes(getRoutesOutput)
	kept := original[:0:0]
	for _, t := range original {
		if t[0] == dest && t[1] == mask && (guestIP == "" || t[2] == guestIP) {
			removed = true
			continue
		}
		kept = append(kept, t)
	}
	if !removed {
		return nil, false
	}
	args = []string{"-setadditionalroutes", service}
	for _, t := range kept {
		args = append(args, t[0], t[1], t[2])
	}
	return args, true
}

// cidrToDestMask converts a CIDR string ("192.168.5.0/24") to the dest + dotted-decimal mask
// pair networksetup expects ("192.168.5.0", "255.255.255.0"). IPv4 only.
func cidrToDestMask(networkCIDR string) (dest, mask string, err error) {
	_, ipnet, err := net.ParseCIDR(networkCIDR)
	if err != nil {
		return "", "", fmt.Errorf("invalid CIDR %q: %w", networkCIDR, err)
	}
	if len(ipnet.Mask) != 4 {
		return "", "", fmt.Errorf("only IPv4 CIDRs are supported, got %q", networkCIDR)
	}
	mask = fmt.Sprintf("%d.%d.%d.%d", ipnet.Mask[0], ipnet.Mask[1], ipnet.Mask[2], ipnet.Mask[3])
	return ipnet.IP.String(), mask, nil
}

// resolverAlreadyConfigured reports whether the resolver file content already has a nameserver line for desiredIP.
// Used to skip writing and sudo when the effective config matches. Parses lines and checks the first nameserver line.
func resolverAlreadyConfigured(content, desiredIP string) bool {
	for _, line := range strings.FieldsFunc(content, func(r rune) bool { return r == '\n' }) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver ") {
			ip := strings.TrimSpace(strings.TrimPrefix(line, "nameserver "))
			return ip == desiredIP
		}
	}
	return false
}

// needsPrivilegeForResolver reports whether sudo is required to apply the desired DNS resolver IP on macOS.
// Returns true when dns.domain is set and the resolver file at /etc/resolver/<domain> is missing, unreadable,
// or does not already specify desiredIP.
func (n *BaseNetworkManager) needsPrivilegeForResolver(desiredIP string) bool {
	tld := n.configHandler.GetString("dns.domain")
	if tld == "" {
		return false
	}
	if err := validateDomain(tld); err != nil {
		return false
	}
	resolverFile := fmt.Sprintf("/etc/resolver/%s", tld)
	existingContent, err := n.shims.ReadFile(resolverFile)
	if err != nil {
		return true
	}
	return !resolverAlreadyConfigured(string(existingContent), desiredIP)
}

// needsPrivilegeForHostRoute reports whether sudo is required to add the host route for the guest on macOS.
// Returns true when no route exists for network.cidr_block with gateway guestAddress; returns false when the
// route exists, when CIDR or guest is unset, or when the route check fails.
func (n *BaseNetworkManager) needsPrivilegeForHostRoute(guestAddress string) bool {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" || guestAddress == "" {
		return false
	}
	inKernel, err := n.macOSRouteInKernel(networkCIDR, guestAddress)
	if err != nil {
		return false
	}
	return !inKernel
}
