//go:build windows
// +build windows

package network

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/ssh"
)

// =============================================================================
// Test Setup
// =============================================================================

type WindowsNetworkManagerMocks struct {
	Injector                     di.Injector
	MockShell                    *shell.MockShell
	MockSecureShell              *shell.MockShell
	MockConfigHandler            *config.MockConfigHandler
	MockSSHClient                *ssh.MockClient
	MockNetworkInterfaceProvider *MockNetworkInterfaceProvider
}

func setupWindowsNetworkManagerMocks() *WindowsNetworkManagerMocks {
	// Create a mock injector
	injector := di.NewMockInjector()

	// Create a mock shell
	mockShell := shell.NewMockShell()
	mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		if command == "powershell" && args[0] == "-Command" {
			return "Route added successfully", nil
		}
		return "", fmt.Errorf("unexpected command")
	}

	// Use the same mock shell for both shell and secure shell
	mockSecureShell := mockShell

	// Create a mock config handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "network.cidr_block":
			return "192.168.1.0/24"
		case "vm.address":
			return "192.168.1.10"
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
	}
	mockConfigHandler.GetContextFunc = func() string {
		return "mocked context"
	}

	// Create a mock SSH client
	mockSSHClient := &ssh.MockClient{}

	// Create a mock network interface provider
	mockNetworkInterfaceProvider := &MockNetworkInterfaceProvider{}

	// Register mocks in the injector
	injector.Register("shell", mockShell)
	injector.Register("secureShell", mockSecureShell)
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("sshClient", mockSSHClient)
	injector.Register("networkInterfaceProvider", mockNetworkInterfaceProvider)

	// Return a struct containing all mocks
	return &WindowsNetworkManagerMocks{
		Injector:                     injector,
		MockShell:                    mockShell,
		MockSecureShell:              mockSecureShell,
		MockConfigHandler:            mockConfigHandler,
		MockSSHClient:                mockSSHClient,
		MockNetworkInterfaceProvider: mockNetworkInterfaceProvider,
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestWindowsNetworkManager_ConfigureHostRoute(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		mocks := setupWindowsNetworkManagerMocks()
		nm := NewBaseNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the host route
		err = nm.ConfigureHostRoute()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoNetworkCIDR", func(t *testing.T) {
		// Given a network manager with no CIDR configured
		mocks := setupWindowsNetworkManagerMocks()
		nm := NewBaseNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And mocking empty CIDR block
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "network.cidr_block" {
				return ""
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// And configuring the host route
		err = nm.ConfigureHostRoute()

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
		mocks := setupWindowsNetworkManagerMocks()
		nm := NewBaseNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And mocking empty guest IP
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "network.cidr_block" {
				return "192.168.1.0/24"
			}
			if key == "vm.address" {
				return ""
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// And configuring the host route
		err = nm.ConfigureHostRoute()

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
		mocks := setupWindowsNetworkManagerMocks()
		nm := NewBaseNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And mocking route check error
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "network.cidr_block" {
				return "192.168.1.0/24"
			}
			if key == "vm.address" {
				return "192.168.1.2"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "powershell" && args[0] == "-Command" {
				if args[1] == fmt.Sprintf("Get-NetRoute -DestinationPrefix %s | Where-Object { $_.NextHop -eq '%s' }", "192.168.1.0/24", "192.168.1.2") {
					return "", fmt.Errorf("mocked shell execution error")
				}
			}
			return "", nil
		}

		// And configuring the host route
		err = nm.ConfigureHostRoute()

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
		mocks := setupWindowsNetworkManagerMocks()
		nm := NewBaseNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And mocking route addition error
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "network.cidr_block" {
				return "192.168.1.0/24"
			}
			if key == "vm.address" {
				return "192.168.1.2"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "powershell" && args[0] == "-Command" {
				if args[1] == fmt.Sprintf("Get-NetRoute -DestinationPrefix %s | Where-Object { $_.NextHop -eq '%s' }", "192.168.1.0/24", "192.168.1.2") {
					return "", nil // Simulate that the route does not exist
				}
				if args[1] == fmt.Sprintf("New-NetRoute -DestinationPrefix %s -NextHop %s -RouteMetric 1", "192.168.1.0/24", "192.168.1.2") {
					return "", fmt.Errorf("mocked shell execution error")
				}
			}
			return "", nil
		}

		// And configuring the host route
		err = nm.ConfigureHostRoute()

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
	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		mocks := setupWindowsNetworkManagerMocks()

		// And mocking DNS configuration
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "example.com"
			}
			if key == "dns.address" {
				return "8.8.8.8"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "powershell" && args[0] == "-Command" {
				if strings.Contains(args[1], "Get-DnsClientNrptRule") {
					return "", nil // Simulate no existing rule
				}
				if strings.Contains(args[1], "Add-DnsClientNrptRule") {
					return "", nil // Simulate successful rule addition
				}
				if strings.Contains(args[1], "Clear-DnsClientCache") {
					return "", nil // Simulate successful DNS cache clear
				}
			}
			return "", fmt.Errorf("unexpected command")
		}

		// And creating the network manager
		nm := NewBaseNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring DNS
		err = nm.ConfigureDNS()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SuccessLocalhostMode", func(t *testing.T) {
		// Given a network manager in localhost mode
		mocks := setupWindowsNetworkManagerMocks()

		// And mocking localhost mode configuration
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.domain":
				return "example.com"
			case "dns.address":
				return "" // Empty DNS address is fine in localhost mode
			case "vm.driver":
				return "docker-desktop" // This enables localhost mode
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}

		// And capturing namespace and nameservers
		var capturedNamespace string
		var capturedNameServers string
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
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

		// And creating the network manager
		nm := NewBaseNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring DNS
		err = nm.ConfigureDNS()

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
		mocks := setupWindowsNetworkManagerMocks()

		// And mocking missing DNS domain
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.address" {
				return "8.8.8.8"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("unexpected command")
		}

		// And creating the network manager
		nm := NewBaseNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring DNS
		err = nm.ConfigureDNS()

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
		mocks := setupWindowsNetworkManagerMocks()

		// And mocking missing DNS address
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "example.com"
			}
			if key == "dns.address" {
				return ""
			}
			if key == "vm.driver" {
				return "hyperv" // Not localhost mode
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("unexpected command")
		}

		// And creating the network manager
		nm := NewBaseNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring DNS
		err = nm.ConfigureDNS()

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
		mocks := setupWindowsNetworkManagerMocks()

		// And mocking DNS check error
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.domain":
				return "example.com"
			case "dns.address":
				return "192.168.1.1"
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}

		var capturedCommand string
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			capturedCommand = command + " " + strings.Join(args, " ")
			if command == "powershell" && args[0] == "-Command" {
				if strings.Contains(args[1], "Get-DnsClientNrptRule") {
					return "", fmt.Errorf("failed to add DNS rule")
				}
			}
			return "", nil
		}

		// And creating the network manager
		nm := NewBaseNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring DNS
		err = nm.ConfigureDNS()

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
		mocks := setupWindowsNetworkManagerMocks()

		// And mocking DNS rule update error
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.domain":
				return "example.com"
			case "dns.address":
				return "8.8.8.8"
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "powershell" && args[0] == "-Command" {
				if strings.Contains(args[1], "Get-DnsClientNrptRule") {
					return "False", nil // Simulate that DNS rule is not set
				}
			}
			return "", nil
		}
		mocks.MockShell.ExecProgressFunc = func(description string, command string, args ...string) (string, error) {
			if command == "powershell" && args[0] == "-Command" {
				if strings.Contains(args[1], "Set-DnsClientNrptRule") || strings.Contains(args[1], "Add-DnsClientNrptRule") {
					return "", fmt.Errorf("failed to add or update DNS rule")
				}
			}
			return "", nil
		}

		// And creating the network manager
		nm := NewBaseNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring DNS
		err = nm.ConfigureDNS()

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
		mocks := setupWindowsNetworkManagerMocks()

		// And mocking missing DNS address
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.domain":
				return "example.com"
			case "dns.address":
				return ""
			case "vm.driver":
				return "hyperv" // Not localhost mode
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}

		// And creating the network manager
		nm := NewBaseNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring DNS
		err = nm.ConfigureDNS()

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
