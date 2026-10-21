//go:build linux
// +build linux

// The LinuxNetworkManager is the network manager for Linux.
// It adds the host route to the guest VM network and makes the route persistent with systemd-networkd or NetworkManager.
// It configures DNS for the context domain with a systemd-resolved drop-in file.

package network

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Constants
// =============================================================================

// systemdResolvedNotRunningHint is the remediation for a host where systemd-resolved is not running.
const systemdResolvedNotRunningHint = "Run 'sudo systemctl enable --now systemd-resolved', then run 'windsor configure network' again. If your distribution does not include systemd-resolved, configure your resolver to send *.<domain> queries to the DNS address."

// systemdResolvedSymlinkHint is the remediation for a host where /etc/resolv.conf does not point at the systemd-resolved stub.
const systemdResolvedSymlinkHint = "Run 'sudo ln -sf /run/systemd/resolve/stub-resolv.conf /etc/resolv.conf', then run 'windsor configure network' again. If NetworkManager writes /etc/resolv.conf again, set 'dns=systemd-resolved' in /etc/NetworkManager/conf.d/dns.conf and reload NetworkManager."

// =============================================================================
// Public Methods
// =============================================================================

// ConfigureHostRoute sets up the local development network for Linux. Guest address is read from
// config (workstation.address). Installs the route in the live routing table via `ip route add`
// when missing, then writes a systemd-networkd drop-in at /etc/systemd/network/windsor-<context>.network
// (or registers a persistent route via `nmcli` when NetworkManager owns the connection) so the
// route survives reboot. When neither persistence mechanism is available the live route is left in
// place and an actionable warning is printed.
func (n *BaseNetworkManager) ConfigureHostRoute(ctx context.Context) error {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" {
		networkCIDR = constants.DefaultNetworkCIDR
	}
	guestIP := n.configHandler.GetString("workstation.address")
	if guestIP == "" {
		return fmt.Errorf("guest address is required")
	}

	routeInKernel, err := n.linuxRouteInKernel(networkCIDR, guestIP)
	if err != nil {
		return err
	}

	if !routeInKernel {
		if err := presenter.Track(ctx, "network:route", "Configuring host route", func(ctx context.Context) error {
			n.warnIfPrivilegeNeeded(ctx)
			if out, err := n.shell.ExecSudo("Adding host route", "ip", "route", "add", networkCIDR, "via", guestIP); err != nil {
				return fmt.Errorf("failed to add route: %w, output: %s", err, out)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	if err := n.persistLinuxHostRoute(networkCIDR, guestIP); err != nil {
		presenter.Warn(ctx, fmt.Sprintf("host route to %s installed but will not survive reboot on this system (%v). After every reboot, run 'windsor configure network' again.", guestIP, err))
	}

	return nil
}

// ConfigureDNS writes a systemd-resolved drop-in file that sends queries for dns.domain to the resolver IP.
// It does not change the global DNS. It returns an error with a remediation hint when the domain or the
// resolver IP is not set, when systemd-resolved is not running, or when /etc/resolv.conf does not point at
// the systemd-resolved stub. It shows a warning when the NetworkManager dns=default mode can replace the drop-in.
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

	if status, err := n.shell.ExecSilent("systemctl", "is-active", "systemd-resolved"); err != nil || strings.TrimSpace(status) != "active" {
		return werror.New("systemd-resolved is not running", systemdResolvedNotRunningHint, nil)
	}

	resolvConf, err := n.shims.ReadLink("/etc/resolv.conf")
	if err != nil || !isSystemdResolvedStubLink(resolvConf) {
		return werror.New("systemd-resolved is active, but /etc/resolv.conf does not point at its stub resolver", systemdResolvedSymlinkHint, nil)
	}

	dropInDir := "/etc/systemd/resolved.conf.d"
	dropInFile := fmt.Sprintf("%s/dns-override-%s.conf", dropInDir, domain)
	expectedContent := fmt.Sprintf("[Resolve]\nDomains=~%s\nDNS=%s\n", domain, dnsIP)

	existingContent, err := n.shims.ReadFile(dropInFile)
	if err == nil && string(existingContent) == expectedContent {
		return nil
	}

	n.dnsChanged = true
	return presenter.Track(ctx, "network:dns", "Configuring DNS", func(ctx context.Context) error {
		n.warnIfPrivilegeNeeded(ctx)
		if _, err := n.shell.ExecSudo("", "mkdir", "-p", dropInDir); err != nil {
			return werror.Wrap(err, "failed to create drop-in directory")
		}
		if err := n.writeFileWithSudo(dropInFile, []byte(expectedContent)); err != nil {
			return werror.Wrap(err, "failed to write DNS configuration")
		}
		if _, err := n.shell.ExecSudo("", "systemctl", "restart", "systemd-resolved"); err != nil {
			return werror.Wrap(err, "failed to restart systemd-resolved")
		}
		if msg := n.networkManagerShadowsResolverWarning(domain); msg != "" {
			presenter.Warn(ctx, msg)
		}
		return nil
	})
}

// =============================================================================
// Private Methods
// =============================================================================

// warnIfPrivilegeNeeded warns that sudo may ask for a password, unless the process runs as root.
func (n *BaseNetworkManager) warnIfPrivilegeNeeded(ctx context.Context) {
	if os.Geteuid() != 0 {
		presenter.Warn(ctx, "This step needs elevated privileges; sudo may ask for your password.")
	}
}

// RevertHostRoute removes the host-to-guest route previously added by ConfigureHostRoute on Linux.
// Idempotent: no-op when the network CIDR is unset, tolerates "No such process" for the live
// delete, removes any matching systemd-networkd drop-in, and drops the NetworkManager persistent
// route when one was installed via nmcli.
func (n *BaseNetworkManager) RevertHostRoute() error {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" {
		return nil
	}
	output, err := n.shell.ExecSudo("", "ip", "route", "del", networkCIDR)
	if err != nil && !strings.Contains(output, "No such process") {
		return fmt.Errorf("failed to delete host route: %w, output: %s", err, output)
	}
	if err := n.unpersistLinuxHostRoute(networkCIDR); err != nil {
		return werror.Wrap(err, "failed to remove persistent host route")
	}
	return nil
}

// RevertDNS removes the systemd-resolved drop-in installed by ConfigureDNS on Linux and restarts
// systemd-resolved so it picks up the change. Idempotent: no-op when dns.domain is unset; rm -f
// tolerates a missing file silently. The restart is best-effort — a failure surfaces an error
// because operators expect DNS state to be coherent after revert.
func (n *BaseNetworkManager) RevertDNS() error {
	domain := n.configHandler.GetString("dns.domain")
	if domain == "" {
		return nil
	}
	if err := validateDomain(domain); err != nil {
		return err
	}
	dropInFile := fmt.Sprintf("/etc/systemd/resolved.conf.d/dns-override-%s.conf", domain)
	if _, err := n.shell.ExecSudo("", "rm", "-f", dropInFile); err != nil {
		return werror.Wrap(err, "failed to remove DNS drop-in")
	}
	if _, err := n.shell.ExecSudo("", "systemctl", "restart", "systemd-resolved"); err != nil {
		return werror.Wrap(err, "failed to restart systemd-resolved")
	}
	return nil
}

// FlushDNS is a no-op on Linux; DNS cache is cleared by restarting systemd-resolved during ConfigureDNS.
func (n *BaseNetworkManager) FlushDNS(ctx context.Context) error {
	return nil
}

// isSystemdResolvedStubLink reports whether the /etc/resolv.conf symlink target points at
// systemd-resolved's stub resolver. Accepts both the relative form (most distros) and the
// absolute form (Fedora, some Ubuntu cloud images).
func isSystemdResolvedStubLink(target string) bool {
	return target == "../run/systemd/resolve/stub-resolv.conf" ||
		target == "/run/systemd/resolve/stub-resolv.conf"
}

// needsPrivilegeForResolver reports whether sudo is required to apply the desired DNS resolver IP
// for the configured domain. It returns true when systemd-resolved is in use and the current
// drop-in for dns.domain is missing, unreadable, or has different Domains= or DNS= values.
func (n *BaseNetworkManager) needsPrivilegeForResolver(desiredIP string) bool {
	domain := n.configHandler.GetString("dns.domain")
	if domain == "" {
		return false
	}
	if err := validateDomain(domain); err != nil {
		return false
	}
	resolvConf, err := n.shims.ReadLink("/etc/resolv.conf")
	if err != nil || !isSystemdResolvedStubLink(resolvConf) {
		return false
	}
	dropInFile := fmt.Sprintf("/etc/systemd/resolved.conf.d/dns-override-%s.conf", domain)
	existingContent, err := n.shims.ReadFile(dropInFile)
	if err != nil {
		return true
	}
	expectedContent := fmt.Sprintf("[Resolve]\nDomains=~%s\nDNS=%s\n", domain, desiredIP)
	return string(existingContent) != expectedContent
}

// networkManagerShadowsResolverWarning returns a non-fatal hint when NM is active and configured to
// rewrite /etc/resolv.conf directly (dns=default, or dns= unset). Walks NetworkManager.conf then
// conf.d/*.conf in lexical order — last [main] dns= wins. Returns "" otherwise.
func (n *BaseNetworkManager) networkManagerShadowsResolverWarning(domain string) string {
	status, err := n.shell.ExecSilent("systemctl", "is-active", "NetworkManager")
	if err != nil || strings.TrimSpace(status) != "active" {
		return ""
	}

	paths := []string{"/etc/NetworkManager/NetworkManager.conf"}
	if matches, _ := n.shims.Glob("/etc/NetworkManager/conf.d/*.conf"); len(matches) > 0 {
		paths = append(paths, matches...)
	}
	dnsSetting := ""
	for _, p := range paths {
		if v, ok := readNetworkManagerMainDNS(n.shims.ReadFile, p); ok {
			dnsSetting = v
		}
	}

	if dnsSetting != "" && dnsSetting != "default" {
		return ""
	}
	return fmt.Sprintf("\n⚠ NetworkManager is configured with 'dns=default' (or unset). NM will rewrite /etc/resolv.conf directly, which can shadow the systemd-resolved drop-in this command just wrote. If '*.%s' fails to resolve after this, add 'dns=systemd-resolved' to /etc/NetworkManager/conf.d/dns.conf and run 'sudo systemctl reload NetworkManager'.", domain)
}

// readNetworkManagerMainDNS parses an NM-style INI file and returns the dns=
// value from the [main] section if present. Honors comments (# and ;) and is
// section-aware so a stray `dns=` in `[connection]` or similar doesn't leak.
func readNetworkManagerMainDNS(readFile func(string) ([]byte, error), path string) (value string, found bool) {
	data, err := readFile(path)
	if err != nil {
		return "", false
	}
	inMain := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inMain = line == "[main]"
			continue
		}
		if !inMain {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) == "dns" {
			return strings.TrimSpace(parts[1]), true
		}
	}
	return "", false
}

// linuxRouteInKernel reports whether the live routing table already has the configured CIDR
// routed via guestIP. Backed by `ip route show <cidr>` parsed for the gateway.
func (n *BaseNetworkManager) linuxRouteInKernel(networkCIDR, guestIP string) (bool, error) {
	output, err := n.shell.ExecSilent("ip", "route", "show", networkCIDR)
	if err != nil {
		return false, werror.Wrap(err, "failed to check if route exists")
	}
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, guestIP) {
			return true, nil
		}
	}
	return false, nil
}

// persistLinuxHostRoute installs a reboot-surviving host route via the host's network manager.
// Tries systemd-networkd first (writes /etc/systemd/network/windsor-<context>.network and asks
// networkctl to reload). Falls back to NetworkManager when nmcli is active (appends an ipv4.routes
// entry on the first active connection). Returns an error when neither backend is available — the
// caller surfaces that as a non-fatal "won't survive reboot" warning to the operator.
func (n *BaseNetworkManager) persistLinuxHostRoute(networkCIDR, guestIP string) error {
	if status, err := n.shell.ExecSilent("systemctl", "is-active", "systemd-networkd"); err == nil && strings.TrimSpace(status) == "active" {
		return n.writeNetworkdHostRouteDropIn(networkCIDR, guestIP)
	}
	if status, err := n.shell.ExecSilent("systemctl", "is-active", "NetworkManager"); err == nil && strings.TrimSpace(status) == "active" {
		return n.addNetworkManagerPersistentRoute(networkCIDR, guestIP)
	}
	return fmt.Errorf("no systemd-networkd or NetworkManager detected")
}

// writeNetworkdHostRouteDropIn writes a per-context drop-in to /etc/systemd/network/ that
// declares the route, then asks `networkctl reload` to pick it up. The Match=* clause keeps the
// drop-in interface-agnostic since the route is "via <guestIP>" — systemd-networkd resolves the
// outgoing interface from the gateway at apply time.
func (n *BaseNetworkManager) writeNetworkdHostRouteDropIn(networkCIDR, guestIP string) error {
	contextName := n.configHandler.GetContext()
	if contextName == "" {
		contextName = "default"
	}
	path := fmt.Sprintf("/etc/systemd/network/windsor-%s.network", contextName)
	content := fmt.Sprintf("[Match]\nName=*\n\n[Route]\nDestination=%s\nGateway=%s\n", networkCIDR, guestIP)
	existing, readErr := n.shims.ReadFile(path)
	if readErr == nil && string(existing) == content {
		return nil
	}
	if err := n.writeFileWithSudo(path, []byte(content)); err != nil {
		return werror.Wrap(err, "failed to write systemd-networkd drop-in")
	}
	if _, err := n.shell.ExecSudo("", "networkctl", "reload"); err != nil {
		return werror.Wrap(err, "failed to reload systemd-networkd")
	}
	return nil
}

// addNetworkManagerPersistentRoute appends a persistent ipv4.routes entry on the first active
// NetworkManager connection. The duplicate guard probes for both the CIDR and the gateway as
// independent substrings in `nmcli -t -f ipv4.routes` — format-agnostic across NM versions
// (modern NM emits `{ ip = …, nh = …, mt = N }`; older ones emit `<cidr> <gateway>`). False
// positives are limited to the unlikely case of a different route already containing both
// strings, where the worst outcome is skipping an already-correct add.
func (n *BaseNetworkManager) addNetworkManagerPersistentRoute(networkCIDR, guestIP string) error {
	conn, err := n.firstActiveNetworkManagerConnection()
	if err != nil {
		return err
	}
	current, err := n.shell.ExecSilent("nmcli", "-t", "-f", "ipv4.routes", "connection", "show", conn)
	if err == nil && strings.Contains(current, networkCIDR) && strings.Contains(current, guestIP) {
		return nil
	}
	if _, err := n.shell.ExecSudo("", "nmcli", "connection", "modify", conn, "+ipv4.routes", networkCIDR+" "+guestIP); err != nil {
		return werror.Wrap(err, "nmcli connection modify failed")
	}
	return nil
}

// firstActiveNetworkManagerConnection returns the name of the first active NM connection
// (terse output: NAME on the first column). Used to pick the connection profile that owns the
// route we're persisting.
func (n *BaseNetworkManager) firstActiveNetworkManagerConnection() (string, error) {
	output, err := n.shell.ExecSilent("nmcli", "-t", "-f", "NAME", "connection", "show", "--active")
	if err != nil {
		return "", werror.Wrap(err, "nmcli connection show failed")
	}
	for _, line := range strings.Split(output, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			return name, nil
		}
	}
	return "", fmt.Errorf("no active NetworkManager connection")
}

// unpersistLinuxHostRoute removes whichever persistent route configuration ConfigureHostRoute
// installed: the systemd-networkd drop-in if it exists (followed by a best-effort
// `networkctl reload` whose failure does not break revert), and the matching nmcli ipv4.routes
// entry if NetworkManager is active. The NM removal uses the CIDR alone — NM 1.36+ (Ubuntu 22.04+,
// Debian 12+, Fedora 40+) matches routes by destination prefix on `-ipv4.routes`, so this catches
// stale entries whose gateway no longer matches the current workstation.address. Both branches are
// no-ops when their backend isn't in use; nmcli non-zero exits on no-match are intentionally
// swallowed since revert is idempotent.
func (n *BaseNetworkManager) unpersistLinuxHostRoute(networkCIDR string) error {
	contextName := n.configHandler.GetContext()
	if contextName == "" {
		contextName = "default"
	}
	path := fmt.Sprintf("/etc/systemd/network/windsor-%s.network", contextName)
	if _, err := n.shims.Stat(path); err == nil {
		if _, err := n.shell.ExecSudo("", "rm", "-f", path); err != nil {
			return werror.Wrap(err, "failed to remove systemd-networkd drop-in")
		}
		_, _ = n.shell.ExecSudo("", "networkctl", "reload")
	}
	if status, err := n.shell.ExecSilent("systemctl", "is-active", "NetworkManager"); err == nil && strings.TrimSpace(status) == "active" {
		if conn, err := n.firstActiveNetworkManagerConnection(); err == nil {
			_, _ = n.shell.ExecSudo("", "nmcli", "connection", "modify", conn, "-ipv4.routes", networkCIDR)
		}
	}
	return nil
}

// needsPrivilegeForHostRoute reports whether sudo is required to add the host route for the guest.
// It returns true when the route for network.cidr_block via guestAddress does not yet exist;
// it returns false when the route exists, when CIDR or guest is unset, or when the route check fails.
func (n *BaseNetworkManager) needsPrivilegeForHostRoute(guestAddress string) bool {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" || guestAddress == "" {
		return false
	}
	inKernel, err := n.linuxRouteInKernel(networkCIDR, guestAddress)
	if err != nil {
		return false
	}
	return !inKernel
}
