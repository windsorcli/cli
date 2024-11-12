//go:build windows
// +build windows

package network

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
	"github.com/windsor-hotel/cli/internal/ssh"
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
	mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
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
	mockContext.GetContextFunc = func() (string, error) {
		return "mocked context", nil
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

func TestWindowsNetworkManager_ConfigureHost(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mocks using setupWindowsNetworkManagerMocks
		mocks := setupWindowsNetworkManagerMocks()

		// Create a network manager using NewBaseNetworkManager with the mock injector
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the method under test
		err = nm.ConfigureHost()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoNetworkCIDR", func(t *testing.T) {
		// Setup mocks using setupWindowsNetworkManagerMocks
		mocks := setupWindowsNetworkManagerMocks()

		// Create a network manager using NewBaseNetworkManager with the mock injector
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

		err = nm.ConfigureHost()
		if err == nil || err.Error() != "network CIDR is not configured" {
			t.Errorf("expected error 'network CIDR is not configured', got %v", err)
		}
	})

	t.Run("NoGuestIP", func(t *testing.T) {
		// Setup mocks using setupWindowsNetworkManagerMocks
		mocks := setupWindowsNetworkManagerMocks()

		// Create a network manager using NewBaseNetworkManager with the mock injector
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

		err = nm.ConfigureHost()
		if err == nil || err.Error() != "guest IP is not configured" {
			t.Errorf("expected error 'guest IP is not configured', got %v", err)
		}
	})

	t.Run("AddRouteError", func(t *testing.T) {
		// Setup mocks using setupWindowsNetworkManagerMocks
		mocks := setupWindowsNetworkManagerMocks()

		// Create a network manager using NewBaseNetworkManager with the mock injector
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
		mocks.MockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mocked shell execution error")
		}

		err = nm.ConfigureHost()

		if err == nil || err.Error() != "failed to add route: mocked shell execution error, output: " {
			t.Errorf("expected error 'failed to add route: mocked shell execution error, output: ', got %v", err)
		}
	})
}

func TestWindowsNetworkManager_ConfigureDNS(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mocks using setupWindowsNetworkManagerMocks
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
		mocks.MockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "powershell" && args[0] == "-Command" && args[1] == "Set-DnsClientServerAddress -InterfaceAlias 'Ethernet' -ServerAddresses 8.8.8.8" {
				return "DNS server set successfully", nil
			}
			return "", fmt.Errorf("unexpected command")
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

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("NoDNSName", func(t *testing.T) {
		// Setup mocks using setupWindowsNetworkManagerMocks
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
		mocks.MockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("unexpected command")
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

		if err == nil || err.Error() != "DNS domain is not configured" {
			t.Errorf("expected error 'DNS domain is not configured', got %v", err)
		}
	})

	t.Run("NoDNSIP", func(t *testing.T) {
		// Setup mocks using setupWindowsNetworkManagerMocks
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
		mocks.MockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("unexpected command")
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

		if err == nil || err.Error() != "DNS address is not configured" {
			t.Errorf("expected error 'DNS address is not configured', got %v", err)
		}
	})

	t.Run("SetDNSError", func(t *testing.T) {
		// Setup mocks using setupWindowsNetworkManagerMocks
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
		mocks.MockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("failed to set DNS server")
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
