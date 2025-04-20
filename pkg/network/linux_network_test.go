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

// =============================================================================
// Test Setup
// =============================================================================

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

// =============================================================================
// Test Public Methods
// =============================================================================

func TestLinuxNetworkManager_ConfigureHostRoute(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		mocks := setupLinuxNetworkManagerMocks()
		nm := NewBaseNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And mocking a successful route check
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ip" && args[0] == "route" && args[1] == "show" {
				return "192.168.5.0/24 via 192.168.5.100 dev eth0", nil
			}
			return "", nil
		}

		// And configuring the host route
		err = nm.ConfigureHostRoute()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoNetworkCIDRConfigured", func(t *testing.T) {
		// Given a network manager with no CIDR configured
		mocks := setupLinuxNetworkManagerMocks()
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "network.cidr_block" {
				return ""
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return "some_value"
		}
		nm := NewBaseNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
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

	t.Run("ErrorCheckingRouteTable", func(t *testing.T) {
		// Given a network manager with route check error
		mocks := setupLinuxNetworkManagerMocks()
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ip" && args[0] == "route" && args[1] == "show" {
				return "", fmt.Errorf("mock error checking route table")
			}
			return "", nil
		}
		nm := NewBaseNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the host route
		err = nm.ConfigureHostRoute()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to check if route exists: mock error checking route table"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoGuestIPConfigured", func(t *testing.T) {
		// Given a network manager with no guest IP configured
		mocks := setupLinuxNetworkManagerMocks()
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
		nm := NewBaseNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
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

	t.Run("RouteExists", func(t *testing.T) {
		// Given a network manager with existing route
		mocks := setupLinuxNetworkManagerMocks()
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ip" && args[0] == "route" && args[1] == "show" && args[2] == "192.168.5.0/24" {
				return "192.168.5.0/24 via 192.168.1.2 dev eth0", nil
			}
			return "", nil
		}
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

	t.Run("RouteExistsWithGuestIP", func(t *testing.T) {
		// Given a network manager with existing route matching guest IP
		mocks := setupLinuxNetworkManagerMocks()
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ip" && args[0] == "route" && args[1] == "show" && args[2] == "192.168.5.0/24" {
				return "192.168.5.0/24 via 192.168.5.100 dev eth0", nil
			}
			return "", nil
		}
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

	t.Run("AddRouteError", func(t *testing.T) {
		// Given a network manager with route addition error
		mocks := setupLinuxNetworkManagerMocks()
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "sudo" && args[0] == "ip" && args[1] == "route" && args[2] == "add" {
				return "mock output", fmt.Errorf("mock error")
			}
			return "", nil
		}
		nm := NewBaseNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the host route
		err = nm.ConfigureHostRoute()

		// Then an error should occur
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
		// Given a properly configured network manager
		mocks := setupLinuxNetworkManagerMocks()
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
		mocks := setupLinuxNetworkManagerMocks()
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

		// And mocking systemd-resolved being in use
		originalReadLink := readLink
		defer func() { readLink = originalReadLink }()
		readLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}

		// And capturing the content
		var capturedContent []byte
		originalReadFile := readFile
		defer func() { readFile = originalReadFile }()
		readFile = func(_ string) ([]byte, error) {
			if capturedContent != nil {
				return capturedContent, nil
			}
			return nil, os.ErrNotExist
		}

		// And capturing the drop-in file content
		mocks.MockShell.ExecSudoFunc = func(description, command string, args ...string) (string, error) {
			if command == "bash" && args[0] == "-c" {
				cmdStr := args[1]
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

		// When creating and initializing the network manager
		nm := NewBaseNetworkManager(mocks.Injector)
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

		// And the drop-in file should contain localhost
		expectedContent := "[Resolve]\nDNS=127.0.0.1\n"
		if string(capturedContent) != expectedContent {
			t.Errorf("expected drop-in file content to be %q, got %q", expectedContent, string(capturedContent))
		}
	})

	t.Run("domainNotConfigured", func(t *testing.T) {
		// Given a network manager with no DNS domain
		mocks := setupLinuxNetworkManagerMocks()
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return ""
			}
			return ""
		}
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

	t.Run("NoDNSAddressConfigured", func(t *testing.T) {
		// Given a network manager with no DNS address
		mocks := setupLinuxNetworkManagerMocks()
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

		// And mocking systemd-resolved being in use
		originalReadLink := readLink
		defer func() { readLink = originalReadLink }()
		readLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}
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

	t.Run("SystemdResolvedNotInUse", func(t *testing.T) {
		// Given a network manager with systemd-resolved not in use
		mocks := setupLinuxNetworkManagerMocks()
		originalReadLink := readLink
		defer func() { readLink = originalReadLink }()
		readLink = func(_ string) (string, error) {
			return "/etc/resolv.conf", nil
		}
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
		expectedError := "systemd-resolved is not in use. Please configure DNS manually or use a compatible system"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("DropInFileAlreadyExistsWithCorrectContent", func(t *testing.T) {
		// Given a network manager with existing drop-in file
		mocks := setupLinuxNetworkManagerMocks()
		originalReadFile := readFile
		defer func() { readFile = originalReadFile }()
		readFile = func(_ string) ([]byte, error) {
			return []byte("[Resolve]\nDNS=1.2.3.4\n"), nil
		}

		// And mocking systemd-resolved being in use
		originalReadLink := readLink
		defer func() { readLink = originalReadLink }()
		readLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}
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

	t.Run("FailedToCreateDropInDirectory", func(t *testing.T) {
		// Given a network manager with directory creation error
		mocks := setupLinuxNetworkManagerMocks()
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "sudo" && args[0] == "mkdir" && args[1] == "-p" {
				return "", fmt.Errorf("mock mkdir error")
			}
			return "", nil
		}
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
		expectedError := "failed to create drop-in directory: mock mkdir error"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("FailedToWriteDNSConfiguration", func(t *testing.T) {
		// Given a network manager with DNS configuration error
		mocks := setupLinuxNetworkManagerMocks()
		mocks.MockShell.ExecSudoFunc = func(description, command string, args ...string) (string, error) {
			if command == "bash" && args[0] == "-c" {
				return "", fmt.Errorf("mock write DNS configuration error")
			}
			return "", nil
		}
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
		expectedError := "failed to write DNS configuration: mock write DNS configuration error"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("FailedToRestartSystemdResolved", func(t *testing.T) {
		// Given a network manager with systemd-resolved restart error
		mocks := setupLinuxNetworkManagerMocks()
		mocks.MockShell.ExecSudoFunc = func(description, command string, args ...string) (string, error) {
			if command == "systemctl" && args[0] == "restart" && args[1] == "systemd-resolved" {
				return "", fmt.Errorf("mock restart systemd-resolved error")
			}
			return "", nil
		}
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
		expectedError := "failed to restart systemd-resolved: mock restart systemd-resolved error"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}
