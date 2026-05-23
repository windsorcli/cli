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

		// And capturing namespace and nameservers
		var capturedNamespace string
		var capturedNameServers string
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "powershell" && len(args) > 1 && args[0] == "-Command" {
				script := args[1]
				if strings.Contains(script, "Get-DnsClientNrptRule") {
					// Extract namespace from the script
					namespaceMatch := strings.Split(script, "$namespace = '")
					if len(namespaceMatch) > 1 {
						namespaceParts := strings.Split(namespaceMatch[1], "'")
						if len(namespaceParts) > 0 {
							capturedNamespace = namespaceParts[0]
						}
					}

					// Extract nameservers from the script — the check joins NameServers before comparing
					// to handle multi-server NRPT rules, so the right-hand side of -ne is the only place
					// the literal lives.
					nameserversMatch := strings.Split(script, "-join ',') -ne \"")
					if len(nameserversMatch) > 1 {
						parts := strings.Split(nameserversMatch[1], "\"")
						if len(parts) > 1 {
							capturedNameServers = strings.Trim(parts[0], "\"")
						}
					}
					return "", nil
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

	t.Run("CheckScriptJoinsNameServersBeforeCompare", func(t *testing.T) {
		// Given the rule check must handle NRPT rules whose NameServers is a multi-element array
		// (a plain $existingRule.NameServers -ne "<ip>" compares array-to-scalar and always reports mismatch).
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

		// Then the check script joins NameServers into a comma-separated string before comparing
		if !strings.Contains(checkScript, "($existingRule.NameServers -join ',') -ne") {
			t.Fatalf("expected check script to join NameServers before comparing, got: %q", checkScript)
		}
		if strings.Contains(checkScript, "$existingRule.NameServers -ne") {
			t.Fatalf("check script still uses the broken array-vs-scalar comparison, got: %q", checkScript)
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

	t.Run("RemoveScriptInterpolatesNamespaceWithLeadingDot", func(t *testing.T) {
		// Given a configured domain
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "local.test")
		var capturedScript string
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "powershell" && len(args) > 1 && args[0] == "-Command" {
				capturedScript = args[1]
			}
			return "", nil
		}

		// When reverting DNS
		if err := manager.RevertDNS(); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// Then the PowerShell script invokes Remove-DnsClientNrptRule with .<domain> namespace
		// and silent error handling
		if !strings.Contains(capturedScript, "Remove-DnsClientNrptRule") {
			t.Errorf("expected Remove-DnsClientNrptRule in script, got: %q", capturedScript)
		}
		if !strings.Contains(capturedScript, "'.local.test'") {
			t.Errorf("expected '.local.test' namespace interpolated, got: %q", capturedScript)
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
