//go:build windows
// +build windows

package network

import (
	"fmt"
	"strings"
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestWindowsNetworkManager_ConfigureHostRoute(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("workstation.address", "192.168.1.10")

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoNetworkCIDR", func(t *testing.T) {
		// Given a network manager with no CIDR configured
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "")
		mocks.ConfigHandler.Set("workstation.address", "192.168.1.10")

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "network CIDR is not configured"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoGuestIP", func(t *testing.T) {
		// Given a network manager with no guest address in config
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("workstation.address", "")

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "guest address is required"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCheckingRoute", func(t *testing.T) {
		// Given a network manager with route check error
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.1.0/24")
		mocks.ConfigHandler.Set("workstation.address", "192.168.1.2")

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "powershell" && args[0] == "-Command" && strings.Contains(args[1], "Get-NetRoute") {
				return "", fmt.Errorf("mocked shell execution error")
			}
			return "", nil
		}

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to check if route exists: mocked shell execution error"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("AddRouteError", func(t *testing.T) {
		// Given a network manager with route addition error
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.1.0/24")
		mocks.ConfigHandler.Set("workstation.address", "192.168.1.2")

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "powershell" && args[0] == "-Command" {
				if strings.Contains(args[1], "Get-NetRoute") {
					return "", nil
				}
				if strings.Contains(args[1], "New-NetRoute") {
					return "", fmt.Errorf("mocked shell execution error")
				}
			}
			return "", nil
		}

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to add route: mocked shell execution error, output: "
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}

func TestWindowsNetworkManager_ConfigureDNS(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		manager, mocks := setup(t)

		// And mocking DNS configuration
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "8.8.8.8")

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SuccessLocalhostMode", func(t *testing.T) {
		// Given a network manager in localhost mode
		manager, mocks := setup(t)

		// And mocking localhost mode configuration
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "")
		mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoDNSName", func(t *testing.T) {
		// Given a network manager with no DNS domain
		manager, mocks := setup(t)

		// And mocking missing DNS domain
		mocks.ConfigHandler.Set("dns.domain", "")

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "DNS domain is not configured"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoDNSIP", func(t *testing.T) {
		// Given a network manager with no DNS address
		manager, mocks := setup(t)

		// And mocking missing DNS address
		mocks.ConfigHandler.Set("workstation.dns.address", "")

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "DNS address is not configured"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("CheckDNSError", func(t *testing.T) {
		// Given a network manager with DNS check error
		manager, mocks := setup(t)

		// And mocking DNS check error
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "192.168.1.1")

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SuccessLocalhostMode", func(t *testing.T) {
		// Given a network manager in localhost mode
		manager, mocks := setup(t)

		// And mocking localhost mode configuration
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "")
		mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")
		mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And capturing namespace and nameservers from the env map (R24: scripts are now
		// fixed constants that read $env:WINDSOR_NRPT_*; the values flow via the env arg)
		var capturedNamespace string
		var capturedNameServers string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "powershell" && env != nil {
				if ns, ok := env["WINDSOR_NRPT_NAMESPACE"]; ok {
					capturedNamespace = ns
				}
			}
			return "", nil
		}
		mocks.Shell.ExecProgressWithEnvFunc = func(message, command string, env map[string]string, args ...string) (string, error) {
			if command == "powershell" && env != nil {
				if dns, ok := env["WINDSOR_NRPT_DNS"]; ok {
					capturedNameServers = dns
				}
			}
			return "", nil
		}

		// And configuring DNS
		err = manager.ConfigureDNS()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the DNS rule should be configured with localhost
		expectedNamespace := ".example.com"
		if capturedNamespace != expectedNamespace {
			t.Errorf("expected namespace to be %q, got %q", expectedNamespace, capturedNamespace)
		}

		expectedNameServers := "127.0.0.1"
		if capturedNameServers != expectedNameServers {
			t.Errorf("expected nameservers to be %q, got %q", expectedNameServers, capturedNameServers)
		}
	})

	t.Run("ExistingRuleWithOurIPFirstIsLeftAlone", func(t *testing.T) {
		// Given an NRPT rule whose first NameServer is our desired IP, followed by
		// operator-added entries (NameServers comma-joins as "1.2.3.4,5.6.7.8")
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")

		var addOrUpdateCalled bool
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "powershell" && len(args) > 1 && strings.Contains(args[1], "Get-DnsClientNrptRule") {
				return "1.2.3.4,5.6.7.8", nil
			}
			return "", nil
		}
		mocks.Shell.ExecProgressFunc = func(message, command string, args ...string) (string, error) {
			if command == "powershell" && len(args) > 1 && (strings.Contains(args[1], "Set-DnsClientNrptRule") || strings.Contains(args[1], "Add-DnsClientNrptRule")) {
				addOrUpdateCalled = true
			}
			return "", nil
		}

		// When configuring DNS
		if err := manager.ConfigureDNS(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then no rewrite is issued — the first NameServer already matches our IP.
		// The pre-fix comparison ($_.NameServers -join ',' -ne "1.2.3.4") would have
		// returned $false here and triggered a redundant elevation.
		if addOrUpdateCalled {
			t.Errorf("expected no NRPT rewrite when first NameServer already matches; got privileged update")
		}
	})

	t.Run("NoDNSName", func(t *testing.T) {
		// Given a network manager with no DNS domain
		manager, mocks := setup(t)

		// And mocking missing DNS domain
		mocks.ConfigHandler.Set("dns.domain", "")

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "DNS domain is not configured"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoDNSIP", func(t *testing.T) {
		// Given a network manager with no DNS address
		manager, mocks := setup(t)

		// And mocking missing DNS address
		mocks.ConfigHandler.Set("workstation.dns.address", "")

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "DNS address is not configured"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("CheckDNSError", func(t *testing.T) {
		// Given a network manager with DNS check error
		manager, mocks := setup(t)

		var capturedCommand string
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			capturedCommand = command + " " + strings.Join(args, " ")
			if command == "powershell" && args[0] == "-Command" {
				if strings.Contains(args[1], "Get-DnsClientNrptRule") {
					return "", fmt.Errorf("failed to add DNS rule")
				}
			}
			return "", nil
		}

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to add DNS rule"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}

		// And the command should contain Get-DnsClientNrptRule
		if !strings.Contains(capturedCommand, "Get-DnsClientNrptRule") {
			t.Fatalf("expected command to contain 'Get-DnsClientNrptRule', got %v", capturedCommand)
		}
	})

	t.Run("ErrorAddingOrUpdatingDNSRule", func(t *testing.T) {
		// Given a network manager with DNS rule update error
		manager, mocks := setup(t)

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "powershell" && args[0] == "-Command" {
				if strings.Contains(args[1], "Get-DnsClientNrptRule") {
					return "False", nil // Simulate that DNS rule is not set
				}
			}
			return "", nil
		}
		mocks.Shell.ExecProgressFunc = func(description string, command string, args ...string) (string, error) {
			if command == "powershell" && args[0] == "-Command" {
				if strings.Contains(args[1], "Set-DnsClientNrptRule") || strings.Contains(args[1], "Add-DnsClientNrptRule") {
					return "", fmt.Errorf("failed to add or update DNS rule")
				}
			}
			return "", nil
		}

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to add or update DNS rule"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoDNSAddressConfigured", func(t *testing.T) {
		// Given a network manager with no DNS address
		manager, mocks := setup(t)

		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "")
		mocks.ConfigHandler.Set("workstation.runtime", "hyperv")

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "DNS address is not configured"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("DomainWithSingleQuoteRejectedBeforePowerShellRuns", func(t *testing.T) {
		// Given a DNS domain containing a single quote, which would close the PowerShell
		// single-quoted string literal `$namespace = '.<domain>'` and inject arbitrary commands.
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", `evil'; calc; '`)
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")

		// And tracking whether any PowerShell ever runs
		var psInvoked bool
		mocks.Shell.ExecSilentFunc = func(command string, _ ...string) (string, error) {
			if command == "powershell" {
				psInvoked = true
			}
			return "", nil
		}
		mocks.Shell.ExecProgressFunc = func(_, command string, _ ...string) (string, error) {
			if command == "powershell" {
				psInvoked = true
			}
			return "", nil
		}

		// When configuring DNS
		err := manager.ConfigureDNS()

		// Then validation rejects before any PowerShell process is spawned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "must contain only letters, digits, hyphen, and dot") {
			t.Fatalf("expected allowlist rejection error, got %q", err.Error())
		}
		if psInvoked {
			t.Fatal("PowerShell was invoked despite invalid domain — validation must run before any script execution")
		}
	})

	t.Run("CheckScriptHandlesNameServersAsArray", func(t *testing.T) {
		// Regression guard for the original array-vs-scalar NRPT bug: a plain
		// $r.NameServers -ne "<ip>" compares array-to-scalar and always reports mismatch.
		// The current check joins NameServers and returns the comma-string for Go-side
		// first-IP comparison (see needsPrivilegeForResolver), so the array case is handled
		// regardless of where the compare lives.
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")

		var checkScript string
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "powershell" && len(args) > 1 && args[0] == "-Command" && strings.Contains(args[1], "Get-DnsClientNrptRule") {
				checkScript = args[1]
			}
			return "", nil
		}

		// When configuring DNS
		if err := manager.ConfigureDNS(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the check script must join NameServers (handles the multi-element array)
		if !strings.Contains(checkScript, "NameServers -join ','") {
			t.Fatalf("expected check script to join NameServers (handles array), got: %q", checkScript)
		}
		// And must not regress to the broken array-vs-scalar PowerShell comparison
		for _, broken := range []string{"$existingRule.NameServers -ne", "$r.NameServers -ne"} {
			if strings.Contains(checkScript, broken) {
				t.Fatalf("check script still uses the broken array-vs-scalar comparison %q, got: %q", broken, checkScript)
			}
		}
	})

	t.Run("NrptValuesFlowThroughEnvVarsNotScriptInterpolation", func(t *testing.T) {
		// R24 regression guard: namespace, DNS, and domain values must reach PowerShell via
		// $env:WINDSOR_NRPT_* (defense-in-depth against script-interpolation injection if
		// validateDomain / validateIPAddress ever regress). The PS script itself must NOT
		// contain the literal namespace / IP strings.
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")

		var checkEnv map[string]string
		var checkScript string
		var addOrUpdateEnv map[string]string
		var addOrUpdateScript string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "powershell" && len(args) > 1 && strings.Contains(args[1], "Get-DnsClientNrptRule") {
				checkEnv = env
				checkScript = args[1]
			}
			return "", nil
		}
		mocks.Shell.ExecProgressWithEnvFunc = func(message, command string, env map[string]string, args ...string) (string, error) {
			if command == "powershell" && len(args) > 1 && strings.Contains(args[1], "Add-DnsClientNrptRule") {
				addOrUpdateEnv = env
				addOrUpdateScript = args[1]
			}
			return "", nil
		}

		if err := manager.ConfigureDNS(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Check script: env carries namespace, script must reference $env:WINDSOR_NRPT_NAMESPACE
		if got := checkEnv["WINDSOR_NRPT_NAMESPACE"]; got != ".example.com" {
			t.Errorf("expected check env WINDSOR_NRPT_NAMESPACE=.example.com, got %q", got)
		}
		if !strings.Contains(checkScript, "$env:WINDSOR_NRPT_NAMESPACE") {
			t.Errorf("expected check script to reference $env:WINDSOR_NRPT_NAMESPACE, got: %q", checkScript)
		}
		if strings.Contains(checkScript, ".example.com") {
			t.Errorf("check script must not interpolate the namespace literal, got: %q", checkScript)
		}

		// AddOrUpdate script: env carries dns + domain, script must reference both env vars
		if got := addOrUpdateEnv["WINDSOR_NRPT_DNS"]; got != "1.2.3.4" {
			t.Errorf("expected addOrUpdate env WINDSOR_NRPT_DNS=1.2.3.4, got %q", got)
		}
		if got := addOrUpdateEnv["WINDSOR_NRPT_DOMAIN"]; got != "example.com" {
			t.Errorf("expected addOrUpdate env WINDSOR_NRPT_DOMAIN=example.com, got %q", got)
		}
		for _, want := range []string{"$env:WINDSOR_NRPT_NAMESPACE", "$env:WINDSOR_NRPT_DNS", "$env:WINDSOR_NRPT_DOMAIN"} {
			if !strings.Contains(addOrUpdateScript, want) {
				t.Errorf("expected addOrUpdate script to reference %s, got: %q", want, addOrUpdateScript)
			}
		}
		for _, banned := range []string{"1.2.3.4", "'.example.com'", "\"example.com\""} {
			if strings.Contains(addOrUpdateScript, banned) {
				t.Errorf("addOrUpdate script must not interpolate %q, got: %q", banned, addOrUpdateScript)
			}
		}
	})

	t.Run("GpoOverrideWarningFiresWhenEffectiveNameServerDiffers", func(t *testing.T) {
		// R23: when a Group Policy NRPT rule resolves *.<domain> to a different name server
		// than the one we just installed, the helper returns a non-fatal warning naming the
		// GPO-served IP. Asserts on the helper's return value (no os.Stderr swap, parallel-safe).
		manager, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "powershell" && len(args) >= 2 && strings.Contains(args[1], "Get-DnsClientNrptPolicy -Effective") {
				return "10.0.0.1", nil
			}
			return "", nil
		}

		env := map[string]string{"WINDSOR_NRPT_NAMESPACE": ".corp.test", "WINDSOR_NRPT_DOMAIN": "corp.test"}
		msg := manager.gpoOverridesNrptRuleWarning(env, "1.2.3.4")
		if msg == "" {
			t.Fatal("expected GPO override warning, got empty string")
		}
		for _, want := range []string{
			"NRPT rule for *.corp.test",
			"10.0.0.1",
			"Group Policy",
		} {
			if !strings.Contains(msg, want) {
				t.Errorf("expected warning to include %q, got: %s", want, msg)
			}
		}
	})

	t.Run("NoGpoWarningWhenEffectiveMatchesOurIP", func(t *testing.T) {
		// When the effective NRPT name server matches what we set, there's no GPO override.
		manager, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "powershell" && len(args) >= 2 && strings.Contains(args[1], "Get-DnsClientNrptPolicy -Effective") {
				return "1.2.3.4", nil
			}
			return "", nil
		}

		env := map[string]string{"WINDSOR_NRPT_NAMESPACE": ".corp.test", "WINDSOR_NRPT_DOMAIN": "corp.test"}
		if msg := manager.gpoOverridesNrptRuleWarning(env, "1.2.3.4"); msg != "" {
			t.Errorf("expected no warning when effective NRPT matches; got: %s", msg)
		}
	})

	t.Run("NoGpoWarningWhenEffectiveQueryFails", func(t *testing.T) {
		// Some Windows editions / WSL2 hosts don't expose Get-DnsClientNrptPolicy; the probe
		// must fail open (return "") rather than scaring the operator with a noise message.
		manager, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "powershell" && len(args) >= 2 && strings.Contains(args[1], "Get-DnsClientNrptPolicy -Effective") {
				return "", fmt.Errorf("Get-DnsClientNrptPolicy not recognized")
			}
			return "", nil
		}

		env := map[string]string{"WINDSOR_NRPT_NAMESPACE": ".corp.test", "WINDSOR_NRPT_DOMAIN": "corp.test"}
		if msg := manager.gpoOverridesNrptRuleWarning(env, "1.2.3.4"); msg != "" {
			t.Errorf("expected GPO probe failure to be silent; got: %s", msg)
		}
	})
}

func TestWindowsNetworkManager_RevertHostRoute(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	t.Run("NoOpWhenCIDRUnset", func(t *testing.T) {
		// Given no network CIDR in config
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "")
		var called bool
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			called = true
			return "", nil
		}

		// When reverting the host route
		if err := manager.RevertHostRoute(); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// Then no PowerShell invocation occurs
		if called {
			t.Errorf("expected no PowerShell invocation when CIDR is unset")
		}
	})

	t.Run("RemoveScriptIsSilentlyIdempotentAndInterpolatesCIDR", func(t *testing.T) {
		// Given a configured CIDR
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.5.0/24")
		var capturedScript string
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "powershell" && len(args) > 1 && args[0] == "-Command" {
				capturedScript = args[1]
			}
			return "", nil
		}

		// When reverting the host route
		if err := manager.RevertHostRoute(); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// Then the PowerShell script invokes Remove-NetRoute with the CIDR and silent error
		// handling so a missing route is not surfaced as an error to the operator
		if !strings.Contains(capturedScript, "Remove-NetRoute") {
			t.Errorf("expected Remove-NetRoute in script, got: %q", capturedScript)
		}
		if !strings.Contains(capturedScript, "192.168.5.0/24") {
			t.Errorf("expected CIDR interpolated, got: %q", capturedScript)
		}
		if !strings.Contains(capturedScript, "ErrorAction SilentlyContinue") {
			t.Errorf("expected SilentlyContinue for idempotency, got: %q", capturedScript)
		}
	})
}

func TestWindowsNetworkManager_RevertDNS(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	t.Run("NoOpWhenDomainUnset", func(t *testing.T) {
		// Given no DNS domain in config
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "")
		var called bool
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			called = true
			return "", nil
		}

		// When reverting DNS
		if err := manager.RevertDNS(); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// Then no PowerShell invocation occurs
		if called {
			t.Errorf("expected no PowerShell invocation when domain is unset")
		}
	})

	t.Run("RemoveScriptUsesEnvVarNamespace", func(t *testing.T) {
		// Given a configured domain
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "local.test")
		var capturedScript string
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "powershell" && len(args) > 1 && args[0] == "-Command" {
				capturedScript = args[1]
				capturedEnv = env
			}
			return "", nil
		}

		// When reverting DNS
		if err := manager.RevertDNS(); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// Then the env carries the namespace with leading dot, the script references it via
		// $env:WINDSOR_NRPT_NAMESPACE (no interpolated literal), and idempotency is preserved
		if got := capturedEnv["WINDSOR_NRPT_NAMESPACE"]; got != ".local.test" {
			t.Errorf("expected WINDSOR_NRPT_NAMESPACE=.local.test, got %q", got)
		}
		if !strings.Contains(capturedScript, "Remove-DnsClientNrptRule") {
			t.Errorf("expected Remove-DnsClientNrptRule in script, got: %q", capturedScript)
		}
		if !strings.Contains(capturedScript, "$env:WINDSOR_NRPT_NAMESPACE") {
			t.Errorf("expected script to reference $env:WINDSOR_NRPT_NAMESPACE, got: %q", capturedScript)
		}
		if strings.Contains(capturedScript, ".local.test") {
			t.Errorf("script must not interpolate the namespace literal, got: %q", capturedScript)
		}
		if !strings.Contains(capturedScript, "ErrorAction SilentlyContinue") {
			t.Errorf("expected SilentlyContinue for idempotency, got: %q", capturedScript)
		}
	})

	t.Run("RejectsDomainWithSingleQuoteBeforePowerShellRuns", func(t *testing.T) {
		// Given a malformed domain that would inject into the PowerShell single-quoted namespace string
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", `evil'; calc; '`)
		var psInvoked bool
		mocks.Shell.ExecSilentFunc = func(command string, _ ...string) (string, error) {
			if command == "powershell" {
				psInvoked = true
			}
			return "", nil
		}

		// When reverting DNS
		err := manager.RevertDNS()

		// Then validation rejects before any PowerShell spawn
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if psInvoked {
			t.Fatal("PowerShell must not be invoked when domain validation fails")
		}
	})
}
