//go:build darwin
// +build darwin

package network

import (
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/ssh"
)

type DarwinNetworkManagerMocks struct {
	Injector                     di.Injector
	MockConfigHandler            *config.MockConfigHandler
	MockShell                    *shell.MockShell
	MockSecureShell              *shell.MockShell
	MockSSHClient                *ssh.MockClient
	MockNetworkInterfaceProvider *MockNetworkInterfaceProvider
}

func setupDarwinNetworkManagerMocks() *DarwinNetworkManagerMocks {
	// Create a mock injector
	injector := di.NewInjector()

	// Create a mock shell
	mockShell := shell.NewMockShell(injector)
	mockShell.ExecSilentFunc = func(command string, args ...string) (string, int, error) {
		return "", 0, nil
	}
	mockShell.ExecSudoFunc = func(message string, command string, args ...string) (string, int, error) {
		if command == "route" && args[0] == "-nv" && args[1] == "add" {
			return "", 0, nil
		}
		if command == "route" && args[0] == "get" {
			return "", 0, nil
		}
		if command == "dscacheutil" && args[0] == "-flushcache" {
			return "", 0, nil
		}
		if command == "killall" && args[0] == "-HUP" {
			return "", 0, nil
		}
		if command == "mv" {
			return "", 0, nil
		}
		return "", 0, fmt.Errorf("mock error")
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

	// Create a mock network interface provider
	mockNetworkInterfaceProvider := &MockNetworkInterfaceProvider{
		InterfacesFunc: func() ([]net.Interface, error) {
			return []net.Interface{
				{Name: "eth0"},
				{Name: "lo"},
				{Name: "br-bridge0"},
			}, nil
		},
		InterfaceAddrsFunc: func(iface net.Interface) ([]net.Addr, error) {
			switch iface.Name {
			case "br-bridge0":
				return []net.Addr{
					&net.IPNet{
						IP:   net.ParseIP("192.168.5.1"),
						Mask: net.CIDRMask(24, 32),
					},
				}, nil
			case "eth0":
				return []net.Addr{
					&net.IPNet{
						IP:   net.ParseIP("10.0.0.2"),
						Mask: net.CIDRMask(24, 32),
					},
				}, nil
			case "lo":
				return []net.Addr{
					&net.IPNet{
						IP:   net.ParseIP("127.0.0.1"),
						Mask: net.CIDRMask(8, 32),
					},
				}, nil
			default:
				return nil, fmt.Errorf("unknown interface")
			}
		},
	}

	// Register mocks in the injector
	injector.Register("shell", mockShell)
	injector.Register("secureShell", mockSecureShell)
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("sshClient", mockSSHClient)
	injector.Register("networkInterfaceProvider", mockNetworkInterfaceProvider)

	// Return a struct containing all mocks
	return &DarwinNetworkManagerMocks{
		Injector:                     injector,
		MockConfigHandler:            mockConfigHandler,
		MockShell:                    mockShell,
		MockSecureShell:              mockSecureShell,
		MockSSHClient:                mockSSHClient,
		MockNetworkInterfaceProvider: mockNetworkInterfaceProvider,
	}
}

func TestDarwinNetworkManager_ConfigureHostRoute(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mocks using setupDarwinNetworkManagerMocks
		mocks := setupDarwinNetworkManagerMocks()

		// Create a networkManager using NewNetworkManager with the mock DI container
		nm := NewBaseNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureHostRoute method
		err = nm.ConfigureHostRoute()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoNetworkCIDRConfigured", func(t *testing.T) {
		mocks := setupDarwinNetworkManagerMocks()

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

		// Create a networkManager using NewNetworkManager with the mock DI container
		nm := NewBaseNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureHostRoute method and expect an error due to missing network CIDR
		err = nm.ConfigureHostRoute()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "network CIDR is not configured"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoGuestIPConfigured", func(t *testing.T) {
		mocks := setupDarwinNetworkManagerMocks()

		// Mock the GetString function to return valid CIDR but an empty string for "vm.address"
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
			return "some_value"
		}

		// Create a networkManager using NewNetworkManager with the mock DI container
		nm := NewBaseNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureHostRoute method and expect an error due to missing guest IP
		err = nm.ConfigureHostRoute()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "guest IP is not configured"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("RouteAlreadyExists", func(t *testing.T) {
		mocks := setupDarwinNetworkManagerMocks()

		// Mock the Exec function to simulate the route already existing
		originalExecSilentFunc := mocks.MockShell.ExecSilentFunc
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, int, error) {
			if command == "route" && args[0] == "get" {
				return "gateway: " + mocks.MockConfigHandler.GetStringFunc("vm.address"), 0, nil
			}
			if originalExecSilentFunc != nil {
				return originalExecSilentFunc(command, args...)
			}
			return "", 0, fmt.Errorf("mock error")
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm := NewBaseNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureHostRoute method and expect no error since the route already exists
		err = nm.ConfigureHostRoute()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("CheckRouteExistsError", func(t *testing.T) {
		mocks := setupDarwinNetworkManagerMocks()

		// Mock an error in the Exec function to simulate a route check failure
		originalExecSilentFunc := mocks.MockShell.ExecSilentFunc
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, int, error) {
			if command == "route" && args[0] == "get" {
				return "", 0, fmt.Errorf("mock error")
			}
			if originalExecSilentFunc != nil {
				return originalExecSilentFunc(command, args...)
			}
			return "", 0, fmt.Errorf("mock error")
		}

		// Create a networkManager using NewBaseNetworkManager with the mock DI container
		nm := NewBaseNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureHostRoute method and expect an error due to failed route check
		err = nm.ConfigureHostRoute()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to check if route exists: mock error"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("AddRouteError", func(t *testing.T) {
		mocks := setupDarwinNetworkManagerMocks()

		// Mock an error in the Exec function to simulate a route addition failure
		originalExecSudoFunc := mocks.MockShell.ExecSudoFunc
		mocks.MockShell.ExecSudoFunc = func(message string, command string, args ...string) (string, int, error) {
			if command == "route" && args[0] == "-nv" && args[1] == "add" {
				return "mock output", 0, fmt.Errorf("mock error")
			}
			if originalExecSudoFunc != nil {
				return originalExecSudoFunc(message, command, args...)
			}
			return "", 0, fmt.Errorf("mock error")
		}

		// Create a networkManager using NewNetworkManager with the mock DI container
		nm := NewBaseNetworkManager(mocks.Injector)

		// Initialize the network manager
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
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}

func TestDarwinNetworkManager_ConfigureDNS(t *testing.T) {
	originalStat := stat
	originalWriteFile := writeFile
	defer func() {
		stat = originalStat
		writeFile = originalWriteFile
	}()

	stat = func(_ string) (os.FileInfo, error) {
		return nil, nil
	}

	writeFile = func(_ string, _ []byte, _ os.FileMode) error {
		return nil
	}

	t.Run("Success", func(t *testing.T) {
		mocks := setupDarwinNetworkManagerMocks()

		nm := NewBaseNetworkManager(mocks.Injector)

		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		err = nm.ConfigureDNS()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SuccessLocalhost", func(t *testing.T) {
		mocks := setupDarwinNetworkManagerMocks()

		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			return "some_value"
		}

		nm := NewBaseNetworkManager(mocks.Injector)

		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		err = nm.ConfigureDNS()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoDNSDomainConfigured", func(t *testing.T) {
		mocks := setupDarwinNetworkManagerMocks()

		nm := NewBaseNetworkManager(mocks.Injector)

		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return ""
			}
			return "some_value"
		}

		err = nm.ConfigureDNS()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "DNS domain is not configured"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ResolverFileAlreadyExists", func(t *testing.T) {
		mocks := setupDarwinNetworkManagerMocks()

		nm := NewBaseNetworkManager(mocks.Injector)

		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		originalReadFile := readFile
		defer func() { readFile = originalReadFile }()
		readFile = func(filename string) ([]byte, error) {
			if filename == fmt.Sprintf("/etc/resolver/%s", mocks.MockConfigHandler.GetStringFunc("dns.domain")) {
				return []byte(fmt.Sprintf("nameserver %s\n", mocks.MockConfigHandler.GetStringFunc("dns.address"))), nil
			}
			return nil, nil // Return nil error to simulate file existing
		}

		err = nm.ConfigureDNS()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("CreateResolverDirectoryError", func(t *testing.T) {
		mocks := setupDarwinNetworkManagerMocks()

		nm := NewBaseNetworkManager(mocks.Injector)

		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if name == "/etc/resolver" {
				return nil, os.ErrNotExist
			}
			return nil, nil
		}

		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, int, error) {
			if command == "sudo" && args[0] == "mkdir" && args[1] == "-p" {
				return "", 0, fmt.Errorf("mock error creating resolver directory")
			}
			return "", 0, nil
		}

		err = nm.ConfigureDNS()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "Error creating resolver directory: mock error creating resolver directory"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("WriteFileError", func(t *testing.T) {
		mocks := setupDarwinNetworkManagerMocks()

		nm := NewBaseNetworkManager(mocks.Injector)

		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		writeFile = func(_ string, _ []byte, _ os.FileMode) error {
			return fmt.Errorf("mock error writing to temporary resolver file")
		}

		err = nm.ConfigureDNS()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "Error writing to temporary resolver file: mock error writing to temporary resolver file"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("MoveResolverFileError", func(t *testing.T) {
		mocks := setupDarwinNetworkManagerMocks()

		nm := NewBaseNetworkManager(mocks.Injector)

		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		writeFile = func(_ string, _ []byte, _ os.FileMode) error {
			return nil // Mock successful write to temporary resolver file
		}

		mocks.MockShell.ExecSudoFunc = func(message string, command string, args ...string) (string, int, error) {
			if command == "mv" {
				return "", 0, fmt.Errorf("mock error moving resolver file")
			}
			return "", 0, nil
		}

		err = nm.ConfigureDNS()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "Error moving resolver file: mock error moving resolver file"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("FlushDNSCacheError", func(t *testing.T) {
		mocks := setupDarwinNetworkManagerMocks()

		nm := NewBaseNetworkManager(mocks.Injector)

		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		writeFile = func(_ string, _ []byte, _ os.FileMode) error {
			return nil // Mock successful write to temporary resolver file
		}

		mocks.MockShell.ExecSudoFunc = func(message string, command string, args ...string) (string, int, error) {
			if command == "dscacheutil" && args[0] == "-flushcache" {
				return "", 0, fmt.Errorf("mock error flushing DNS cache")
			}
			return "", 0, nil
		}

		err = nm.ConfigureDNS()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "Error flushing DNS cache: mock error flushing DNS cache"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("RestartMDNSResponderError", func(t *testing.T) {
		mocks := setupDarwinNetworkManagerMocks()

		nm := NewBaseNetworkManager(mocks.Injector)

		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		writeFile = func(_ string, _ []byte, _ os.FileMode) error {
			return nil // Mock successful write to temporary resolver file
		}

		mocks.MockShell.ExecSudoFunc = func(message string, command string, args ...string) (string, int, error) {
			if command == "killall" && args[0] == "-HUP" {
				return "", 0, fmt.Errorf("mock error restarting mDNSResponder")
			}
			return "", 0, nil
		}

		err = nm.ConfigureDNS()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "Error restarting mDNSResponder: mock error restarting mDNSResponder"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("IsLocalhostScenario", func(t *testing.T) {
		mocks := setupDarwinNetworkManagerMocks()

		nm := NewBaseNetworkManager(mocks.Injector)

		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		writeFile = func(_ string, _ []byte, _ os.FileMode) error {
			return nil // Mock successful write to temporary resolver file
		}

		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.address" {
				return "127.0.0.1"
			}
			return "some_value"
		}

		err = nm.ConfigureDNS()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
