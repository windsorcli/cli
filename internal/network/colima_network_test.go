package network

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
	"github.com/windsor-hotel/cli/internal/ssh"
)

type ColimaNetworkManagerMocks struct {
	Injector                     di.Injector
	MockShell                    *shell.MockShell
	MockSecureShell              *shell.MockShell
	MockConfigHandler            *config.MockConfigHandler
	MockContextHandler           *context.MockContext
	MockSSHClient                *ssh.MockClient
	MockNetworkInterfaceProvider *MockNetworkInterfaceProvider
}

func setupColimaNetworkManagerMocks() *ColimaNetworkManagerMocks {
	// Create a mock injector
	injector := di.NewInjector()

	// Create a mock shell
	mockShell := shell.NewMockShell(injector)
	mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
		if command == "ls" && args[0] == "/sys/class/net" {
			return "br-bridge0\neth0\nlo", nil
		}
		if command == "sudo" && args[0] == "iptables" && args[1] == "-t" && args[2] == "filter" && args[3] == "-C" {
			return "", fmt.Errorf("Bad rule")
		}
		return "", nil
	}

	// Use the same mock shell for both shell and secure shell
	mockSecureShell := mockShell

	// Create a mock config handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "docker.network_cidr":
			return "192.168.5.0/24"
		case "vm.driver":
			return "colima"
		case "vm.address":
			return "192.168.5.100"
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
	}

	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		switch key {
		case "docker.enabled":
			return true
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
	}

	// Create a mock context handler
	mockContextHandler := context.NewMockContext()

	// Create a mock SSH client
	mockSSHClient := &ssh.MockClient{}

	// Register mocks in the injector
	injector.Register("shell", mockShell)
	injector.Register("secureShell", mockSecureShell)
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("contextHandler", mockContextHandler)
	injector.Register("sshClient", mockSSHClient)

	// Create a mock network interface provider with mock functions
	mockNetworkInterfaceProvider := &MockNetworkInterfaceProvider{
		InterfacesFunc: func() ([]net.Interface, error) {
			return []net.Interface{
				{Name: "eth0"},
				{Name: "lo"},
				{Name: "br-1234"}, // Include a "br-" interface to simulate a docker bridge
			}, nil
		},
		InterfaceAddrsFunc: func(iface net.Interface) ([]net.Addr, error) {
			switch iface.Name {
			case "br-1234":
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
				return nil, fmt.Errorf("no addresses found for interface %s", iface.Name)
			}
		},
	}
	injector.Register("networkInterfaceProvider", mockNetworkInterfaceProvider)

	// Return a struct containing all mocks
	return &ColimaNetworkManagerMocks{
		Injector:                     injector,
		MockShell:                    mockShell,
		MockSecureShell:              mockSecureShell,
		MockConfigHandler:            mockConfigHandler,
		MockContextHandler:           mockContextHandler,
		MockSSHClient:                mockSSHClient,
		MockNetworkInterfaceProvider: mockNetworkInterfaceProvider,
	}
}

func TestColimaNetworkManager_ConfigureGuest(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mocks using setupSafeAwsEnvMocks
		mocks := setupColimaNetworkManagerMocks()

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method
		err = nm.ConfigureGuest()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoNetworkCIDRConfigured", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the GetString method to return an empty string for "docker.network_cidr"
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "docker.network_cidr" {
				return ""
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return "some-value"
		}

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to missing network CIDR
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "network CIDR is not configured"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoGuestIPConfigured", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the GetString method to return an empty string for "vm.address"
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.address" {
				return ""
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return "some-value"
		}

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to missing guest IP
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "guest IP is not configured"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorGettingContext", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the GetContext method to return an error
		mocks.MockContextHandler.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting context")
		}

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to failed context retrieval
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error retrieving context: mock error getting context"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorGettingSSHConfig", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the ExecFunc to return an error when getting SSH config
		mocks.MockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "colima" && args[0] == "ssh-config" {
				return "", fmt.Errorf("mock error getting SSH config")
			}
			return "", nil
		}

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to failed SSH config retrieval
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error executing VM SSH config command: mock error getting SSH config"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSettingSSHClient", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the SetClientConfigFileFunc to return an error
		mocks.MockSSHClient.SetClientConfigFileFunc = func(config string, contextName string) error {
			return fmt.Errorf("mock error setting SSH client config")
		}

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to failed SSH client config
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error setting SSH client config: mock error setting SSH client config"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorListingInterfaces", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the ExecFunc to return an error when listing interfaces
		mocks.MockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "", fmt.Errorf("mock error listing interfaces")
			}
			return "", nil
		}

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to failed interface listing
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error executing command to list network interfaces: mock error listing interfaces"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoDockerBridgeInterfaceFound", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the ExecFunc to return no interfaces starting with "br-"
		mocks.MockSecureShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "eth0\nlo\nwlan0", nil // No "br-" interface
			}
			return "", nil
		}

		// Use the mock injector from setupColimaNetworkManagerMocks
		injector := mocks.Injector

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to no docker bridge interface found
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error: no docker bridge interface found"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSettingIptablesRule", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the ExecFunc to simulate finding a docker bridge interface and an error when setting iptables rule
		mocks.MockSecureShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "br-1234\neth0\nlo\nwlan0", nil // Include a "br-" interface
			}
			if command == "sudo" && args[0] == "iptables" && args[1] == "-t" && args[2] == "filter" && args[3] == "-C" {
				return "", fmt.Errorf("Bad rule") // Simulate that the rule doesn't exist
			}
			if command == "sudo" && args[0] == "iptables" && args[1] == "-t" && args[2] == "filter" && args[3] == "-A" {
				return "", fmt.Errorf("mock error setting iptables rule")
			}
			return "", nil
		}

		// Use the mock injector from setupColimaNetworkManagerMocks
		injector := mocks.Injector

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to iptables rule setting failure
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error setting iptables rule") {
			t.Fatalf("expected error to contain 'error setting iptables rule', got %q", err.Error())
		}
	})

	t.Run("ErrorFindingHostIP", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the InterfaceAddrsFunc to simulate failure in finding host IP
		mocks.MockNetworkInterfaceProvider.InterfaceAddrsFunc = func(iface net.Interface) ([]net.Addr, error) {
			// Return an empty list of addresses to simulate no matching subnet
			return []net.Addr{}, nil
		}

		// Use the mock injector from setupColimaNetworkManagerMocks
		injector := mocks.Injector

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to failure in finding host IP
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to find host IP in the same subnet as guest IP") {
			t.Fatalf("expected error to contain 'failed to find host IP in the same subnet as guest IP', got %q", err.Error())
		}
	})
	t.Run("ErrorCheckingIptablesRule", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the ExecFunc to simulate an unexpected error when checking iptables rule
		originalExecFunc := mocks.MockSecureShell.ExecFunc
		mocks.MockSecureShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "sudo" && args[0] == "iptables" && args[1] == "-t" && args[2] == "filter" && args[3] == "-C" {
				return "", fmt.Errorf("unexpected error checking iptables rule")
			}
			return originalExecFunc(verbose, message, command, args...)
		}

		// Use the mock injector from setupColimaNetworkManagerMocks
		injector := mocks.Injector

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to unexpected iptables rule check failure
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error checking iptables rule") {
			t.Fatalf("expected error to contain 'error checking iptables rule', got %q", err.Error())
		}
	})
}
