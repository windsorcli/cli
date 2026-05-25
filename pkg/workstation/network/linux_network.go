//go:build linux
// +build linux

package network

import (
	"fmt"
	"os"
	"strings"

	"github.com/windsorcli/cli/pkg/tui"
)

// The LinuxNetworkManager is a platform-specific network manager for Linux systems.
// It provides network configuration capabilities specific to Linux-based systems,
// The LinuxNetworkManager handles host route configuration and DNS setup for Linux,
// ensuring proper network connectivity between the host and guest VM environments.

// =============================================================================
// Constants
// =============================================================================

// systemdResolvedNotRunningHint surfaces when `systemctl is-active systemd-resolved` reports
// inactive. Names the supported distros + the manual-config escape hatch for distros that
// don't ship systemd-resolved at all (Alpine, Void, NixOS, etc.).
const systemdResolvedNotRunningHint = "DNS configuration on this distro requires systemd-resolved, which is not running. Windsor's supported Linux DNS setup uses a systemd-resolved drop-in at /etc/systemd/resolved.conf.d/. To enable: 'sudo systemctl enable --now systemd-resolved' (Ubuntu/Debian/Fedora/openSUSE), then re-run 'windsor configure network'. If your distro doesn't ship systemd-resolved (Alpine, Void, Devuan, NixOS, Slackware), you'll need to wire DNS manually — add 'nameserver <address>' for *.<domain> via your distro's resolver (resolvconf, dnsmasq, unbound). See docs/guides/troubleshooting.md#dns-on-non-systemd-linux."

// systemdResolvedSymlinkHint surfaces when the resolved service is active but /etc/resolv.conf
// does not symlink to its stub. The drop-in we'd write would be ignored in that state because
// resolv.conf bypasses resolved entirely. Common causes: NetworkManager with `dns=default`,
// manual edits, or distro defaults that haven't switched over.
const systemdResolvedSymlinkHint = "systemd-resolved is active but /etc/resolv.conf is not pointing at its stub resolver, so any drop-in we write would be ignored. Restore the symlink with 'sudo ln -sf /run/systemd/resolve/stub-resolv.conf /etc/resolv.conf', then re-run 'windsor configure network'. If NetworkManager keeps rewriting /etc/resolv.conf, set 'dns=systemd-resolved' in /etc/NetworkManager/conf.d/dns.conf and reload NetworkManager. See docs/guides/troubleshooting.md#dns-on-non-systemd-linux."

// =============================================================================
// Public Methods
// =============================================================================

// ConfigureHostRoute sets up the local development network for Linux.
// Guest address is read from config (workstation.address).
func (n *BaseNetworkManager) ConfigureHostRoute() error {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" {
		return fmt.Errorf("network CIDR is not configured")
	}
	guestIP := n.configHandler.GetString("workstation.address")
	if guestIP == "" {
		return fmt.Errorf("guest address is required")
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

	if os.Geteuid() != 0 {
		tui.Pause()
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠\033[0m Network configuration may require elevated privileges\n")
	}
	output, err = n.shell.ExecSudo(
		"Adding host route",
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

// ConfigureDNS installs a systemd-resolved drop-in scoping dns.domain to our resolver; global DNS is
// unchanged. Probes `systemctl is-active systemd-resolved` first so distros that don't ship resolved
// fail with an actionable hint, and warns non-fatally when NetworkManager's `dns=default` mode would
// rewrite /etc/resolv.conf and shadow the drop-in.
func (n *BaseNetworkManager) ConfigureDNS() error {
	domain := n.configHandler.GetString("dns.domain")
	if domain == "" {
		return fmt.Errorf("DNS domain is not configured")
	}
	if err := validateDomain(domain); err != nil {
		return err
	}

	dnsIP := n.effectiveResolverIP()
	if dnsIP == "" {
		return fmt.Errorf("DNS address is not configured")
	}

	if status, err := n.shell.ExecSilent("systemctl", "is-active", "systemd-resolved"); err != nil || strings.TrimSpace(status) != "active" {
		return fmt.Errorf("%s", systemdResolvedNotRunningHint)
	}

	resolvConf, err := n.shims.ReadLink("/etc/resolv.conf")
	if err != nil || !isSystemdResolvedStubLink(resolvConf) {
		return fmt.Errorf("%s", systemdResolvedSymlinkHint)
	}

	dropInDir := "/etc/systemd/resolved.conf.d"
	dropInFile := fmt.Sprintf("%s/dns-override-%s.conf", dropInDir, domain)
	expectedContent := fmt.Sprintf("[Resolve]\nDomains=~%s\nDNS=%s\n", domain, dnsIP)

	existingContent, err := n.shims.ReadFile(dropInFile)
	if err == nil && string(existingContent) == expectedContent {
		return nil
	}

	n.dnsChanged = true
	if os.Geteuid() != 0 {
		tui.Pause()
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠\033[0m DNS configuration may require elevated privileges\n")
	}

	if _, err := n.shell.ExecSudo("", "mkdir", "-p", dropInDir); err != nil {
		return fmt.Errorf("failed to create drop-in directory: %w", err)
	}

	if err := n.writeFileWithSudo(dropInFile, []byte(expectedContent)); err != nil {
		return fmt.Errorf("failed to write DNS configuration: %w", err)
	}

	if _, err := n.shell.ExecSudo("", "systemctl", "restart", "systemd-resolved"); err != nil {
		return fmt.Errorf("failed to restart systemd-resolved: %w", err)
	}

	if msg := n.networkManagerShadowsResolverWarning(domain); msg != "" {
		fmt.Fprintln(os.Stderr, msg)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// RevertHostRoute removes the host-to-guest route previously added by ConfigureHostRoute on Linux.
// Idempotent: no-op when the network CIDR is unset, and tolerates "No such process" (the exit
// signal from ip route del when the destination isn't in the table).
func (n *BaseNetworkManager) RevertHostRoute() error {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" {
		return nil
	}
	output, err := n.shell.ExecSudo("", "ip", "route", "del", networkCIDR)
	if err != nil {
		if strings.Contains(output, "No such process") {
			return nil
		}
		return fmt.Errorf("failed to delete host route: %w, output: %s", err, output)
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
		return fmt.Errorf("failed to remove DNS drop-in: %w", err)
	}
	if _, err := n.shell.ExecSudo("", "systemctl", "restart", "systemd-resolved"); err != nil {
		return fmt.Errorf("failed to restart systemd-resolved: %w", err)
	}
	return nil
}

// FlushDNS is a no-op on Linux; DNS cache is cleared by restarting systemd-resolved during ConfigureDNS.
func (n *BaseNetworkManager) FlushDNS() error {
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

// needsPrivilegeForHostRoute reports whether sudo is required to add the host route for the guest.
// It returns true when the route for network.cidr_block via guestAddress does not yet exist;
// it returns false when the route exists, when CIDR or guest is unset, or when the route check fails.
func (n *BaseNetworkManager) needsPrivilegeForHostRoute(guestAddress string) bool {
	networkCIDR := n.configHandler.GetString("network.cidr_block")
	if networkCIDR == "" || guestAddress == "" {
		return false
	}
	output, err := n.shell.ExecSilent("ip", "route", "show", networkCIDR)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, guestAddress) {
			return false
		}
	}
	return true
}
