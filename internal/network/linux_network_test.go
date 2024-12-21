//go:build linux
// +build linux

package network

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/shell"
	"github.com/windsorcli/cli/internal/ssh"
)

type LinuxNetworkManagerMocks struct {
	Injector          di.Injector
	MockConfigHandler *config.MockConfigHandler
	MockShell         *shell.MockShell
	MockSecureShell   *shell.MockShell
	MockSSHClient     *ssh.MockClient
}

func setupLinuxNetworkManagerMocks() *LinuxNetworkManagerMocks {
	// Create a mock injector
	injector := di.NewInjector()

	// Create a mock shell
	mockShell := shell.NewMockShell(injector)
	mockShell.ExecFunc = func(command string, args ...string) (string, error) {
		if command == "sudo" && args[0] == "ip" && args[1] == "route" && args[2] == "add" {
			return "", nil
		}
		if command == "sudo" && args[0] == "systemctl" && args[1] == "restart" && args[2] == "systemd-resolved" {
			return "", nil
		}
		if command == "sudo" && args[0] == "mkdir" && args[1] == "-p" {
			return "", nil
		}
		if command == "sudo" && args[0] == "bash" && args[1] == "-c" {
			return "", nil
		}
		return "", fmt.Errorf("mock error")
	}

	// Use the same mock shell for both shell and secure shell
	mockSecureShell := mockShell

	// Create a mock config handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "docker.network_cidr":
			return "192.168.5.0/24"
		case "vm.address":
			return "192.168.5.100"
		case "dns.name":
			return "example.com"
		case "dns.address":
			return "1.2.3.4"
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
	}

	// Create a mock context
	mockContext := context.NewMockContext()

	// Create a mock SSH client
	mockSSHClient := &ssh.MockClient{}

	// Introduce a simple mock network interface
	mockNetworkInterface := &MockNetworkInterfaceProvider{
		InterfacesFunc: func() ([]net.Interface, error) {
			return []net.Interface{}, nil
		},
		InterfaceAddrsFunc: func(iface net.Interface) ([]net.Addr, error) {
			return []net.Addr{}, nil
		},
	}

	// Register mocks in the injector
	injector.Register("shell", mockShell)
	injector.Register("secureShell", mockSecureShell)
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("contextHandler", mockContext)
	injector.Register("sshClient", mockSSHClient)
	injector.Register("networkInterfaceProvider", mockNetworkInterface)

	// Return a struct containing all mocks
	return &LinuxNetworkManagerMocks{
		Injector:          injector,
		MockConfigHandler: mockConfigHandler,
		MockShell:         mockShell,
		MockSecureShell:   mockSecureShell,
		MockSSHClient:     mockSSHClient,
	}
}

func TestLinuxNetworkManager_ConfigureHostRoute(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Initialize the network manager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Mock the shell.ExecSilent function to simulate a successful route check
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ip" && args[0] == "route" && args[1] == "show" {
				return "", fmt.Errorf("mock error")
			}
			return "", nil
		}

		// Call the ConfigureHostRoute method and expect an error
		err = nm.ConfigureHostRoute()
		if err == nil || !strings.Contains(err.Error(), "failed to check if route exists: mock error") {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoNetworkCIDRConfigured", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the GetString function to return an empty string for "docker.network_cidr"
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "docker.network_cidr" {
				return ""
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return "some_value"
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Initialize the network manager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureHostRoute method and expect an error
		err = nm.ConfigureHostRoute()
		if err == nil || !strings.Contains(err.Error(), "network CIDR is not configured") {
			t.Fatalf("expected error 'network CIDR is not configured', got %v", err)
		}
	})

	t.Run("NoGuestIPConfigured", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the GetString function to return an empty string for "vm.address"
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "docker.network_cidr" {
				return "192.168.5.0/24"
			}
			if key == "vm.address" {
				return ""
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return "some_value"
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Initialize the network manager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureHostRoute method and expect an error
		err = nm.ConfigureHostRoute()
		if err == nil || !strings.Contains(err.Error(), "guest IP is not configured") {
			t.Fatalf("expected error 'guest IP is not configured', got %v", err)
		}
	})

	t.Run("RouteExists", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the Exec function to simulate an existing route with the guest IP
		mocks.MockShell.ExecFunc = func(command string, args ...string) (string, error) {
			if command == "ip" && args[0] == "route" && args[1] == "show" {
				// Simulate output that includes the guest IP to trigger routeExists = true
				return "192.168.5.0/24 via 192.168.1.2 dev eth0", nil
			}
			return "", nil
		}

		// Mock the GetString function to return specific values for testing
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "docker.network_cidr" {
				return "192.168.5.0/24"
			}
			if key == "vm.address" {
				return "192.168.1.2"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Initialize the network manager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureHostRoute method and expect no error since the route exists
		err = nm.ConfigureHostRoute()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("AddRouteError", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock an error in the ExecSilent function to simulate a route addition failure
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "sudo" && args[0] == "ip" && args[1] == "route" && args[2] == "add" {
				return "mock output", fmt.Errorf("mock error")
			}
			return "", nil
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Initialize the network manager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureHostRoute method and expect an error due to failed route addition
		err = nm.ConfigureHostRoute()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to add route: mock error, output: mock output"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}

func TestLinuxNetworkManager_ConfigureDNS(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Initialize the network manager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureDNS method and expect no error
		err = nm.ConfigureDNS()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoDNSAddressConfigured", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the GetString function to return an empty string for "dns.address"
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.address":
				return ""
			case "docker.network_cidr":
				return "192.168.5.0/24"
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return "some_value"
			}
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Initialize the network manager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureDNS method and expect an error due to missing DNS address
		err = nm.ConfigureDNS()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "DNS address is not configured"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoDNSDomainConfigured", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the GetString function to return an empty string for "dns.name"
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.name":
				return ""
			case "docker.network_cidr":
				return "192.168.5.0/24"
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return "some_value"
			}
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Initialize the network manager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureDNS method and expect an error due to missing DNS domain
		err = nm.ConfigureDNS()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "DNS domain is not configured"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SystemdResolvedNotInUse", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the readLink function to simulate that systemd-resolved is not in use
		originalReadLink := readLink
		defer func() { readLink = originalReadLink }()
		readLink = func(_ string) (string, error) {
			return "/etc/resolv.conf", nil
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Initialize the network manager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureDNS method and expect an error due to systemd-resolved not being in use
		err = nm.ConfigureDNS()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "systemd-resolved is not in use. Please configure DNS manually or use a compatible system"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("DropInFileAlreadyExistsWithCorrectContent", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the readFile function to simulate that the drop-in file already exists with the correct content
		originalReadFile := readFile
		defer func() { readFile = originalReadFile }()
		readFile = func(_ string) ([]byte, error) {
			return []byte("[Resolve]\nDNS=1.2.3.4\n"), nil
		}

		// Mock the readLink function to simulate that /etc/resolv.conf is a symlink to systemd-resolved
		originalReadLink := readLink
		defer func() { readLink = originalReadLink }()
		readLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Initialize the network manager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureDNS method and expect no error since the drop-in file already exists with correct content
		err = nm.ConfigureDNS()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("FailedToCreateDropInDirectory", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the shell.ExecSilent function to simulate an error when creating the drop-in directory
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "sudo" && args[0] == "mkdir" && args[1] == "-p" {
				return "", fmt.Errorf("mock mkdir error")
			}
			return "", nil
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Initialize the network manager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureDNS method and expect an error due to failure in creating the drop-in directory
		err = nm.ConfigureDNS()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to create drop-in directory: mock mkdir error"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("FailedToWriteDNSConfiguration", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the shell.ExecSudo function to simulate an error when writing the DNS configuration
		mocks.MockShell.ExecSudoFunc = func(description, command string, args ...string) (string, error) {
			if command == "bash" && args[0] == "-c" {
				return "", fmt.Errorf("mock write DNS configuration error")
			}
			return "", nil
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Initialize the network manager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureDNS method and expect an error due to failure in writing the DNS configuration
		err = nm.ConfigureDNS()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to write DNS configuration: mock write DNS configuration error"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("FailedToRestartSystemdResolved", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the shell.ExecSudo function to simulate an error when restarting systemd-resolved
		mocks.MockShell.ExecSudoFunc = func(description, command string, args ...string) (string, error) {
			if command == "systemctl" && args[0] == "restart" && args[1] == "systemd-resolved" {
				return "", fmt.Errorf("mock restart systemd-resolved error")
			}
			return "", nil
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Initialize the network manager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureDNS method and expect an error due to failure in restarting systemd-resolved
		err = nm.ConfigureDNS()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to restart systemd-resolved: mock restart systemd-resolved error"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}
