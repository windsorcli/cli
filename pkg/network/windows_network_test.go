//go:build windows
// +build windows

package network

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/ssh"
)

func stringPtr(s string) *string {
	return &s
}

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
		case "docker.network_cidr":
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

	// Create a mock context handler
	mockContext := context.NewMockContext()
	mockContext.GetContextFunc = func() string {
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
	injector.Register("contextHandler", mockContext)
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

func TestWindowsNetworkManager_ConfigureHostRoute(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given setup mocks using setupWindowsNetworkManagerMocks
		mocks := setupWindowsNetworkManagerMocks()

		// And create a network manager using NewBaseNetworkManager with the mock injector
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// When call the method under test
		err = nm.ConfigureHostRoute()

		// Then expect no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoNetworkCIDR", func(t *testing.T) {
		// Given setup mocks using setupWindowsNetworkManagerMocks
		mocks := setupWindowsNetworkManagerMocks()

		// And create a network manager using NewBaseNetworkManager with the mock injector
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "docker.network_cidr" {
				return ""
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// When call the method under test
		err = nm.ConfigureHostRoute()

		// Then expect error 'network CIDR is not configured'
		if err == nil || err.Error() != "network CIDR is not configured" {
			t.Errorf("expected error 'network CIDR is not configured', got %v", err)
		}
	})

	t.Run("NoGuestIP", func(t *testing.T) {
		// Given setup mocks using setupWindowsNetworkManagerMocks
		mocks := setupWindowsNetworkManagerMocks()

		// And create a network manager using NewBaseNetworkManager with the mock injector
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "docker.network_cidr" {
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

		// When call the method under test
		err = nm.ConfigureHostRoute()

		// Then expect error 'guest IP is not configured'
		if err == nil || err.Error() != "guest IP is not configured" {
			t.Errorf("expected error 'guest IP is not configured', got %v", err)
		}
	})

	t.Run("ErrorCheckingRoute", func(t *testing.T) {
		// Given setup mocks using setupWindowsNetworkManagerMocks
		mocks := setupWindowsNetworkManagerMocks()

		// And create a network manager using NewBaseNetworkManager with the mock injector
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "docker.network_cidr" {
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

		// When call the method under test
		err = nm.ConfigureHostRoute()

		// Then expect error 'failed to check if route exists: mocked shell execution error'
		if err == nil || err.Error() != "failed to check if route exists: mocked shell execution error" {
			t.Errorf("expected error 'failed to check if route exists: mocked shell execution error', got %v", err)
		}
	})

	t.Run("AddRouteError", func(t *testing.T) {
		// Given setup mocks using setupWindowsNetworkManagerMocks
		mocks := setupWindowsNetworkManagerMocks()

		// And create a network manager using NewBaseNetworkManager with the mock injector
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "docker.network_cidr" {
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

		// When call the method under test
		err = nm.ConfigureHostRoute()

		// Then expect error 'failed to add route: mocked shell execution error, output: '
		if err == nil || err.Error() != "failed to add route: mocked shell execution error, output: " {
			t.Errorf("expected error 'failed to add route: mocked shell execution error, output: ', got %v", err)
		}
	})
}

func TestWindowsNetworkManager_ConfigureDNS(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given setup mocks using setupWindowsNetworkManagerMocks
		mocks := setupWindowsNetworkManagerMocks()

		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.name" {
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
		mocks.MockShell.ExecFunc = func(command string, args ...string) (string, error) {
			if command == "powershell" && args[0] == "-Command" {
				if args[1] == "Get-DnsClientServerAddress -InterfaceAlias 'Ethernet' | Select-Object -ExpandProperty ServerAddresses" {
					return "8.8.8.8", nil
				}
				if args[1] == "Set-DnsClientServerAddress -InterfaceAlias 'Ethernet' -ServerAddresses 8.8.8.8" {
					return "DNS server set successfully", nil
				}
			}
			return "", fmt.Errorf("unexpected command")
		}

		// And create a network manager using NewBaseNetworkManager with the mock injector
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		err = nm.Initialize()
		if err != nil {
			t.Errorf("expected no error during initialization, got %v", err)
		}

		// When call the method under test
		err = nm.ConfigureDNS()

		// Then expect no error
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("NoDNSName", func(t *testing.T) {
		// Given setup mocks using setupWindowsNetworkManagerMocks
		mocks := setupWindowsNetworkManagerMocks()

		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.address" {
				return "8.8.8.8"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mocks.MockShell.ExecFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("unexpected command")
		}

		// And create a network manager using NewBaseNetworkManager with the mock injector
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		err = nm.Initialize()
		if err != nil {
			t.Errorf("expected no error during initialization, got %v", err)
		}

		// When call the method under test
		err = nm.ConfigureDNS()

		// Then expect error 'DNS domain is not configured'
		if err == nil || err.Error() != "DNS domain is not configured" {
			t.Errorf("expected error 'DNS domain is not configured', got %v", err)
		}
	})

	t.Run("NoDNSIP", func(t *testing.T) {
		// Given setup mocks using setupWindowsNetworkManagerMocks
		mocks := setupWindowsNetworkManagerMocks()

		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.name" {
				return "example.com"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mocks.MockShell.ExecFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("unexpected command")
		}

		// And create a network manager using NewBaseNetworkManager with the mock injector
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		err = nm.Initialize()
		if err != nil {
			t.Errorf("expected no error during initialization, got %v", err)
		}

		// When call the method under test
		err = nm.ConfigureDNS()

		// Then expect error 'DNS address is not configured'
		if err == nil || err.Error() != "DNS address is not configured" {
			t.Errorf("expected error 'DNS address is not configured', got %v", err)
		}
	})

	t.Run("SetDNSError", func(t *testing.T) {
		// Given setup mocks using setupWindowsNetworkManagerMocks
		mocks := setupWindowsNetworkManagerMocks()

		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.name" {
				return "example.com"
			}
			if key == "dns.address" {
				return "192.168.1.1"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("failed to set DNS server")
		}

		// And create a network manager using NewBaseNetworkManager with the mock injector
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		err = nm.Initialize()
		if err != nil {
			t.Errorf("expected no error during initialization, got %v", err)
		}

		// When call the method under test
		err = nm.ConfigureDNS()

		// Then expect error containing 'failed to set DNS server'
		if err == nil || !strings.Contains(err.Error(), "failed to set DNS server") {
			t.Errorf("expected error containing 'failed to set DNS server', got %v", err)
		}
	})

	t.Run("ResolverFileAlreadyExists", func(t *testing.T) {
		// Given setup mocks using setupWindowsNetworkManagerMocks
		mocks := setupWindowsNetworkManagerMocks()

		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.name" {
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
				if args[1] == "Get-DnsClientServerAddress -InterfaceAlias 'Ethernet' | Select-Object -ExpandProperty ServerAddresses" {
					return "8.8.8.8", nil
				}
				if args[1] == "Test-Path -Path 'C:\\Windows\\System32\\drivers\\etc\\resolv.conf'" {
					return "True", nil
				}
			}
			return "", fmt.Errorf("unexpected command")
		}

		// And create a network manager using NewBaseNetworkManager with the mock injector
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		err = nm.Initialize()
		if err != nil {
			t.Errorf("expected no error during initialization, got %v", err)
		}

		// When call the method under test
		err = nm.ConfigureDNS()

		// Then expect no error
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("SetDNSServerError", func(t *testing.T) {
		mocks := setupWindowsNetworkManagerMocks()

		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.name" {
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
				if args[1] == fmt.Sprintf("Set-DnsClientServerAddress -InterfaceAlias 'Ethernet' -ServerAddresses %s", "8.8.8.8") {
					return "", fmt.Errorf("mocked shell execution error")
				}
			}
			return "", nil
		}

		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		err = nm.Initialize()
		if err != nil {
			t.Errorf("expected no error during initialization, got %v", err)
		}

		err = nm.ConfigureDNS()

		if err == nil || !strings.Contains(err.Error(), "failed to set DNS server") {
			t.Errorf("expected error containing 'failed to set DNS server', got %v", err)
		}
	})
}
