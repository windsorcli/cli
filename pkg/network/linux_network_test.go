//go:build linux
// +build linux

package network

import (
	"fmt"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/ssh"
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
		case "network.cidr_block":
			return "192.168.5.0/24"
		case "vm.address":
			return "192.168.5.100"
		case "dns.domain":
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
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()

		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Mock the shell.ExecSilent function to simulate a successful route check
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ip" && args[0] == "route" && args[1] == "show" {
				return "192.168.5.0/24 via 192.168.5.100 dev eth0", nil
			}
			return "", nil
		}

		// Call the ConfigureHostRoute method and expect no error since the route exists
		err = nm.ConfigureHostRoute()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoNetworkCIDRConfigured", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the GetString function to return an empty string for "network.cidr_block"
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "network.cidr_block" {
				return ""
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return "some_value"
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Call the ConfigureHostRoute method and expect an error
		err = nm.ConfigureHostRoute()
		if err == nil || !strings.Contains(err.Error(), "network CIDR is not configured") {
			t.Fatalf("expected error 'network CIDR is not configured', got %v", err)
		}
	})

	t.Run("ErrorCheckingRouteTable", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the ExecSilent function to simulate an error when checking the routing table
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ip" && args[0] == "route" && args[1] == "show" {
				return "", fmt.Errorf("mock error checking route table")
			}
			return "", nil
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()

		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureHostRoute method and expect an error due to route table check failure
		err = nm.ConfigureHostRoute()
		if err == nil || !strings.Contains(err.Error(), "failed to check if route exists: mock error checking route table") {
			t.Fatalf("expected error 'failed to check if route exists: mock error checking route table', got %v", err)
		}
	})

	t.Run("NoGuestIPConfigured", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the GetString function to return an empty string for "vm.address"
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "network.cidr_block" {
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
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Call the ConfigureHostRoute method and expect an error
		err = nm.ConfigureHostRoute()
		if err == nil || !strings.Contains(err.Error(), "guest IP is not configured") {
			t.Fatalf("expected error 'guest IP is not configured', got %v", err)
		}
	})

	t.Run("RouteExists", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the ExecSilent function to simulate checking the routing table
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ip" && args[0] == "route" && args[1] == "show" && args[2] == "192.168.5.0/24" {
				// Simulate output that includes the guest IP to trigger routeExists = true
				return "192.168.5.0/24 via 192.168.1.2 dev eth0", nil
			}
			return "", nil
		}

		// Mock the GetString function to return specific values for testing
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "network.cidr_block" {
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
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureHostRoute method and expect no error since the route exists
		err = nm.ConfigureHostRoute()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("RouteExistsWithGuestIP", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the ExecSilent function to simulate checking the routing table
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ip" && args[0] == "route" && args[1] == "show" && args[2] == "192.168.5.0/24" {
				// Simulate output that includes the guest IP to trigger routeExists = true
				return "192.168.5.0/24 via 192.168.5.100 dev eth0", nil
			}
			return "", nil
		}

		// Mock the GetString function to return specific values for testing
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "network.cidr_block" {
				return "192.168.5.0/24"
			}
			if key == "vm.address" {
				return "192.168.5.100"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()
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
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()
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
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureDNS method and expect no error
		err = nm.ConfigureDNS()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SuccessLocalhostMode", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the config handler to return docker-desktop for vm.driver
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "vm.driver":
				return "docker-desktop"
			case "dns.domain":
				return "example.com"
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}

		// Mock the readLink function to simulate systemd-resolved being in use
		originalReadLink := readLink
		defer func() { readLink = originalReadLink }()
		readLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}

		// Mock the readFile function to capture the content
		var capturedContent []byte
		originalReadFile := readFile
		defer func() { readFile = originalReadFile }()
		readFile = func(_ string) ([]byte, error) {
			if capturedContent != nil {
				return capturedContent, nil
			}
			return nil, os.ErrNotExist
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Mock the shell.ExecSudo function to capture the content
		mocks.MockShell.ExecSudoFunc = func(description, command string, args ...string) (string, error) {
			if command == "bash" && args[0] == "-c" {
				// Extract the content from the echo command
				cmdStr := args[1]

				// The command is in the format: echo '[Resolve]\nDNS=127.0.0.1\n' | sudo tee /etc/systemd/resolved.conf.d/dns-override-example.com.con
				// We need to extract the content between the first and last single quote before the pipe
				if strings.Contains(cmdStr, "echo '") && strings.Contains(cmdStr, "' | sudo tee") {
					start := strings.Index(cmdStr, "echo '") + 6
					end := strings.Index(cmdStr, "' | sudo tee")
					if start < end {
						content := cmdStr[start:end]
						capturedContent = []byte(content)
					}
				}
				return "", nil
			}
			return "", nil
		}

		// Call the ConfigureDNS method
		err = nm.ConfigureDNS()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify that the drop-in file contains 127.0.0.1
		expectedContent := "[Resolve]\nDNS=127.0.0.1\n"
		if string(capturedContent) != expectedContent {
			t.Errorf("expected drop-in file content to be %q, got %q", expectedContent, string(capturedContent))
		}
	})

	t.Run("domainNotConfigured", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the GetString function to simulate missing domain configuration
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return ""
			}
			return ""
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureDNS method and expect an error due to missing domain
		err = nm.ConfigureDNS()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "DNS domain is not configured"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoDNSAddressConfigured", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the config handler to return empty DNS address but valid domain
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "dns.domain":
				return "example.com"
			case "dns.address":
				return ""
			case "vm.driver":
				return "lima" // Not localhost mode
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}

		// Mock the readLink function to simulate systemd-resolved being in use
		originalReadLink := readLink
		defer func() { readLink = originalReadLink }()
		readLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()
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

	t.Run("SystemdResolvedNotInUse", func(t *testing.T) {
		mocks := setupLinuxNetworkManagerMocks()

		// Mock the readLink function to simulate that systemd-resolved is not in use
		originalReadLink := readLink
		defer func() { readLink = originalReadLink }()
		readLink = func(_ string) (string, error) {
			return "/etc/resolv.conf", nil
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()
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
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()
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
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()
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
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()
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
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()
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
