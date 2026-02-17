//go:build windows
// +build windows

package network

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/workstation/services"
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
		manager.AssignIPs([]services.Service{})
		return manager, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		manager, _ := setup(t)

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
		// Given a network manager with no guest IP configured
		manager, mocks := setup(t)

		mocks.ConfigHandler.Set("vm.address", "")

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "guest IP is not configured"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCheckingRoute", func(t *testing.T) {
		// Given a network manager with route check error
		manager, mocks := setup(t)

		mocks.ConfigHandler.Set("network.cidr_block", "192.168.1.0/24")
		mocks.ConfigHandler.Set("vm.address", "192.168.1.2")

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
		mocks.ConfigHandler.Set("vm.address", "192.168.1.2")

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
		manager.AssignIPs([]services.Service{})
		return manager, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		manager, mocks := setup(t)

		// And mocking DNS configuration
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("dns.address", "8.8.8.8")

		// And configuring DNS
		err := manager.ConfigureDNS("")

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
		mocks.ConfigHandler.Set("dns.address", "")
		mocks.ConfigHandler.Set("vm.driver", "docker-desktop")
		mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")

		// And configuring DNS
		err := manager.ConfigureDNS("")

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
		err := manager.ConfigureDNS("")

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
		mocks.ConfigHandler.Set("dns.address", "")

		// And configuring DNS
		err := manager.ConfigureDNS("")

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
		mocks.ConfigHandler.Set("dns.address", "192.168.1.1")

		// And configuring DNS
		err := manager.ConfigureDNS("")

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
		mocks.ConfigHandler.Set("dns.address", "")
		mocks.ConfigHandler.Set("vm.driver", "docker-desktop")
		mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")

		// And configuring DNS
		err := manager.ConfigureDNS("")

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

					// Extract nameservers from the script
					nameserversMatch := strings.Split(script, "NameServers -ne \"")
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
		err = manager.ConfigureDNS("")

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
		err := manager.ConfigureDNS("")

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
		mocks.ConfigHandler.Set("dns.address", "")

		// And configuring DNS
		err := manager.ConfigureDNS("")

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
		err := manager.ConfigureDNS("")

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
		err := manager.ConfigureDNS("")

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
		mocks.ConfigHandler.Set("dns.address", "")
		mocks.ConfigHandler.Set("vm.driver", "hyperv")

		// And configuring DNS
		err := manager.ConfigureDNS("")

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "DNS address is not configured"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}
