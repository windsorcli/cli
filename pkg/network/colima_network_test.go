package network

import (
	"fmt"
	"net"
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

type ColimaNetworkManagerMocks struct {
	Injector                     di.Injector
	MockShell                    *shell.MockShell
	MockSecureShell              *shell.MockShell
	MockConfigHandler            *config.MockConfigHandler
	MockSSHClient                *ssh.MockClient
	MockNetworkInterfaceProvider *MockNetworkInterfaceProvider
}

func setupColimaNetworkManagerMocks() *ColimaNetworkManagerMocks {
	// Create a mock injector
	injector := di.NewInjector()

	// Create a mock shell
	mockShell := shell.NewMockShell(injector)
	mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
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
		case "network.cidr_block":
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

	// Create a mock SSH client
	mockSSHClient := &ssh.MockClient{}

	// Register mocks in the injector
	injector.Register("shell", mockShell)
	injector.Register("secureShell", mockSecureShell)
	injector.Register("configHandler", mockConfigHandler)
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
		MockSSHClient:                mockSSHClient,
		MockNetworkInterfaceProvider: mockNetworkInterfaceProvider,
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestColimaNetworkManager_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		mocks := setupColimaNetworkManagerMocks()
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And all dependencies should be correctly initialized
		if nm.sshClient == nil {
			t.Fatalf("expected sshClient to be initialized, got nil")
		}
		if nm.secureShell == nil {
			t.Fatalf("expected secureShell to be initialized, got nil")
		}
		if nm.networkInterfaceProvider == nil {
			t.Fatalf("expected networkInterfaceProvider to be initialized, got nil")
		}

		// And network.cidr_block should be set correctly
		if actualCIDR := nm.configHandler.GetString("network.cidr_block"); actualCIDR != "192.168.5.0/24" {
			t.Fatalf("expected network.cidr_block to be 192.168.5.0/24, got %s", actualCIDR)
		}
	})

	t.Run("ErrorResolvingSSHClient", func(t *testing.T) {
		// Given a network manager with missing sshClient
		mocks := setupColimaNetworkManagerMocks()
		mocks.Injector.Register("sshClient", nil)
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error due to unresolved sshClient, got nil")
		}
	})

	t.Run("ErrorResolvingSecureShell", func(t *testing.T) {
		// Given a network manager with missing secureShell
		mocks := setupColimaNetworkManagerMocks()
		mocks.Injector.Register("secureShell", nil)
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error due to unresolved secureShell, got nil")
		}
	})

	t.Run("ErrorResolvingNetworkInterfaceProvider", func(t *testing.T) {
		// Given a network manager with missing networkInterfaceProvider
		mocks := setupColimaNetworkManagerMocks()
		mocks.Injector.Register("networkInterfaceProvider", nil)
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error due to unresolved networkInterfaceProvider, got nil")
		}
	})
}

func TestColimaNetworkManager_ConfigureGuest(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		mocks := setupColimaNetworkManagerMocks()
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = nm.ConfigureGuest()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoNetworkCIDRConfigured", func(t *testing.T) {
		// Given a network manager with no CIDR configured
		mocks := setupColimaNetworkManagerMocks()
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "network.cidr_block" {
				return ""
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return "some-value"
		}
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = nm.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "network CIDR is not configured"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoGuestIPConfigured", func(t *testing.T) {
		// Given a network manager with no guest IP configured
		mocks := setupColimaNetworkManagerMocks()
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
			return "some-value"
		}
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = nm.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "guest IP is not configured"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorGettingSSHConfig", func(t *testing.T) {
		// Given a network manager with SSH config error
		mocks := setupColimaNetworkManagerMocks()
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && args[0] == "ssh-config" {
				return "", fmt.Errorf("mock error getting SSH config")
			}
			return "", nil
		}
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = nm.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error executing VM SSH config command: mock error getting SSH config"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSettingSSHClient", func(t *testing.T) {
		// Given a network manager with SSH client error
		mocks := setupColimaNetworkManagerMocks()
		mocks.MockSSHClient.SetClientConfigFileFunc = func(config string, contextName string) error {
			return fmt.Errorf("mock error setting SSH client config")
		}
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = nm.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error setting SSH client config: mock error setting SSH client config"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorListingInterfaces", func(t *testing.T) {
		// Given a network manager with interface listing error
		mocks := setupColimaNetworkManagerMocks()
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "", fmt.Errorf("mock error listing interfaces")
			}
			return "", nil
		}
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = nm.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error executing command to list network interfaces: mock error listing interfaces"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoDockerBridgeInterfaceFound", func(t *testing.T) {
		// Given a network manager with no docker bridge interface
		mocks := setupColimaNetworkManagerMocks()
		mocks.MockSecureShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "eth0\nlo\nwlan0", nil // No "br-" interface
			}
			return "", nil
		}
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = nm.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error: no docker bridge interface found"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSettingIptablesRule", func(t *testing.T) {
		// Given a network manager with iptables rule error
		mocks := setupColimaNetworkManagerMocks()
		mocks.MockSecureShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
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
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = nm.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error setting iptables rule") {
			t.Fatalf("expected error to contain 'error setting iptables rule', got %q", err.Error())
		}
	})

	t.Run("ErrorFindingHostIP", func(t *testing.T) {
		// Given a network manager with host IP error
		mocks := setupColimaNetworkManagerMocks()
		mocks.MockNetworkInterfaceProvider.InterfaceAddrsFunc = func(iface net.Interface) ([]net.Addr, error) {
			// Return an empty list of addresses to simulate no matching subnet
			return []net.Addr{}, nil
		}
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = nm.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to find host IP in the same subnet as guest IP") {
			t.Fatalf("expected error to contain 'failed to find host IP in the same subnet as guest IP', got %q", err.Error())
		}
	})

	t.Run("ErrorCheckingIptablesRule", func(t *testing.T) {
		// Given a network manager with iptables rule check error
		mocks := setupColimaNetworkManagerMocks()
		originalExecSilentFunc := mocks.MockSecureShell.ExecSilentFunc
		mocks.MockSecureShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "sudo" && args[0] == "iptables" && args[1] == "-t" && args[2] == "filter" && args[3] == "-C" {
				return "", fmt.Errorf("unexpected error checking iptables rule")
			}
			return originalExecSilentFunc(command, args...)
		}
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = nm.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error checking iptables rule") {
			t.Fatalf("expected error to contain 'error checking iptables rule', got %q", err.Error())
		}
	})
}

func TestColimaNetworkManager_getHostIP(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		mocks := setupNetworkManagerMocks()
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// And getting the host IP
		hostIP, err := nm.getHostIP()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error during getHostIP, got %v", err)
		}

		// And the host IP should be correct
		expectedHostIP := "192.168.1.1"
		if hostIP != expectedHostIP {
			t.Fatalf("expected host IP %v, got %v", expectedHostIP, hostIP)
		}
	})

	t.Run("SuccessWithIpAddr", func(t *testing.T) {
		// Given a network manager with IPAddr type
		mocks := setupNetworkManagerMocks()
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// And mocking networkInterfaceProvider to return IPAddr
		mocks.MockNetworkInterfaceProvider.InterfaceAddrsFunc = func(iface net.Interface) ([]net.Addr, error) {
			return []net.Addr{
				&net.IPAddr{IP: net.ParseIP("192.168.1.1")},
			}, nil
		}

		// And getting the host IP
		hostIP, err := nm.getHostIP()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error during getHostIP, got %v", err)
		}

		// And the host IP should be correct
		expectedHostIP := "192.168.1.1"
		if hostIP != expectedHostIP {
			t.Fatalf("expected host IP %v, got %v", expectedHostIP, hostIP)
		}
	})

	t.Run("NoGuestAddressSet", func(t *testing.T) {
		// Given a network manager with no guest address
		mocks := setupNetworkManagerMocks()
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// And mocking configHandler to return empty guest address
		originalGetStringFunc := mocks.MockConfigHandler.GetStringFunc
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.address" {
				return ""
			}
			return originalGetStringFunc(key, defaultValue...)
		}

		// And getting the host IP
		hostIP, err := nm.getHostIP()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error during getHostIP, got none")
		}

		// And the error message should be correct
		expectedErrorMessage := "guest IP is not configured"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message %q, got %q", expectedErrorMessage, err.Error())
		}

		// And the host IP should be empty
		if hostIP != "" {
			t.Fatalf("expected empty host IP, got %v", hostIP)
		}
	})

	t.Run("ErrorParsingGuestIP", func(t *testing.T) {
		// Given a network manager with invalid guest IP
		mocks := setupNetworkManagerMocks()
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// And mocking configHandler to return invalid guest IP
		originalGetStringFunc := mocks.MockConfigHandler.GetStringFunc
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.address" {
				return "invalid_ip_address"
			}
			return originalGetStringFunc(key, defaultValue...)
		}

		// And getting the host IP
		hostIP, err := nm.getHostIP()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error during getHostIP, got none")
		}

		// And the error message should be correct
		expectedErrorMessage := "invalid guest IP address"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message %q, got %q", expectedErrorMessage, err.Error())
		}

		// And the host IP should be empty
		if hostIP != "" {
			t.Fatalf("expected empty host IP, got %v", hostIP)
		}
	})

	t.Run("ErrorGettingNetworkInterfaces", func(t *testing.T) {
		// Given a network manager with network interfaces error
		mocks := setupNetworkManagerMocks()
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// And mocking network interface provider to return error
		mocks.MockNetworkInterfaceProvider.InterfacesFunc = func() ([]net.Interface, error) {
			return nil, fmt.Errorf("mock error getting network interfaces")
		}

		// And getting the host IP
		hostIP, err := nm.getHostIP()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error during getHostIP, got none")
		}

		// And the error message should be correct
		expectedErrorMessage := "failed to get network interfaces: mock error getting network interfaces"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message %q, got %q", expectedErrorMessage, err.Error())
		}

		// And the host IP should be empty
		if hostIP != "" {
			t.Fatalf("expected empty host IP, got %v", hostIP)
		}
	})

	t.Run("ErrorGettingNetworkInterfaceAddresses", func(t *testing.T) {
		// Given a network manager with interface addresses error
		mocks := setupNetworkManagerMocks()
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// And mocking network interface provider to return error
		mocks.MockNetworkInterfaceProvider.InterfaceAddrsFunc = func(iface net.Interface) ([]net.Addr, error) {
			return nil, fmt.Errorf("mock error getting network interface addresses")
		}

		// And getting the host IP
		hostIP, err := nm.getHostIP()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error during getHostIP, got none")
		}

		// And the error message should contain the expected text
		if !strings.Contains(err.Error(), "mock error getting network interface addresses") {
			t.Fatalf("expected error message to contain %q, got %q", "mock error getting network interface addresses", err.Error())
		}

		// And the host IP should be empty
		if hostIP != "" {
			t.Fatalf("expected empty host IP, got %v", hostIP)
		}
	})

	t.Run("ErrorFindingHostIPInSameSubnet", func(t *testing.T) {
		// Given a network manager with no matching subnet
		mocks := setupNetworkManagerMocks()
		nm := NewColimaNetworkManager(mocks.Injector)

		// When initializing the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// And mocking network interface provider to return no matching subnet
		mocks.MockNetworkInterfaceProvider.InterfacesFunc = func() ([]net.Interface, error) {
			return []net.Interface{
				{Name: "eth0"},
			}, nil
		}
		mocks.MockNetworkInterfaceProvider.InterfaceAddrsFunc = func(iface net.Interface) ([]net.Addr, error) {
			if iface.Name == "eth0" {
				return []net.Addr{
					&net.IPNet{IP: net.ParseIP("10.0.0.1"), Mask: net.CIDRMask(24, 32)},
				}, nil
			}
			return nil, fmt.Errorf("no addresses found for interface %s", iface.Name)
		}

		// And getting the host IP
		hostIP, err := nm.getHostIP()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error during getHostIP, got none")
		}

		// And the error message should be correct
		expectedErrorMessage := "failed to find host IP in the same subnet as guest IP"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message %q, got %q", expectedErrorMessage, err.Error())
		}

		// And the host IP should be empty
		if hostIP != "" {
			t.Fatalf("expected empty host IP, got %v", hostIP)
		}
	})
}
