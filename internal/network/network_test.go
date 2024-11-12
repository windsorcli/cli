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

// NetworkManagerMocks holds all the mock dependencies for NetworkManager
type NetworkManagerMocks struct {
	Injector                     di.Injector
	MockShell                    *shell.MockShell
	MockSecureShell              *shell.MockShell
	MockConfigHandler            *config.MockConfigHandler
	MockContextHandler           *context.MockContext
	MockSSHClient                *ssh.MockClient
	MockNetworkInterfaceProvider *MockNetworkInterfaceProvider
}

func setupNetworkManagerMocks(optionalInjector ...di.Injector) *NetworkManagerMocks {
	// Use the provided injector or create a new one if not provided
	var injector di.Injector
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewInjector()
	}

	// Create a mock shell
	mockShell := shell.NewMockShell(injector)
	mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
		return "", nil
	}

	// Use the same mock shell for both shell and secure shell
	mockSecureShell := mockShell

	// Create a mock config handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "network.cidr":
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
				{Name: "br-bridge0"}, // Include a "br-" interface to simulate a docker bridge
			}, nil
		},
		InterfaceAddrsFunc: func(iface net.Interface) ([]net.Addr, error) {
			switch iface.Name {
			case "br-bridge0":
				return []net.Addr{
					&net.IPNet{
						IP:   net.ParseIP("192.168.1.1"),
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
	return &NetworkManagerMocks{
		Injector:                     injector,
		MockShell:                    mockShell,
		MockSecureShell:              mockSecureShell,
		MockConfigHandler:            mockConfigHandler,
		MockContextHandler:           mockContextHandler,
		MockSSHClient:                mockSSHClient,
		MockNetworkInterfaceProvider: mockNetworkInterfaceProvider,
	}
}

func TestNetworkManager_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if nm == nil {
			t.Fatalf("expected a valid NetworkManager instance, got nil")
		}
	})

	t.Run("ErrorResolvingSSHClient", func(t *testing.T) {
		// Setup mocks
		injector := di.NewMockInjector()
		setupNetworkManagerMocks(injector)

		// Mock the injector to return an error when resolving sshClient
		injector.SetResolveError("sshClient", fmt.Errorf("mock error resolving ssh client"))

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "failed to resolve ssh client instance: mock error resolving ssh client" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorCastingSSHClient", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()

		// Register the sshClient as "invalid"
		mocks.Injector.Register("sshClient", "invalid")

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "resolved ssh client instance is not of type ssh.Client" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		injector := di.NewMockInjector()
		setupNetworkManagerMocks(injector)

		// Mock the injector to return an error when resolving shell
		injector.SetResolveError("shell", fmt.Errorf("mock error resolving shell"))

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "failed to resolve shell instance: mock error resolving shell" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorCastingShell", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()

		// Register the shell as "invalid"
		mocks.Injector.Register("shell", "invalid")

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "resolved shell instance is not of type shell.Shell" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		injector := di.NewMockInjector()
		setupNetworkManagerMocks(injector)

		// Mock the injector to return an error when resolving configHandler
		injector.SetResolveError("configHandler", fmt.Errorf("mock error resolving configHandler"))

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "failed to resolve CLI config handler: mock error resolving configHandler" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingSecureShell", func(t *testing.T) {
		injector := di.NewMockInjector()
		setupNetworkManagerMocks(injector)

		// Mock the injector to return an error when resolving secureShell
		injector.SetResolveError("secureShell", fmt.Errorf("mock error resolving secureShell"))

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "failed to resolve secure shell instance: mock error resolving secureShell" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorCastingSecureShell", func(t *testing.T) {
		injector := di.NewMockInjector()
		setupNetworkManagerMocks(injector)

		// Register the secureShell as "invalid"
		injector.Register("secureShell", "invalid")

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "resolved secure shell instance is not of type shell.Shell" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		injector := di.NewMockInjector()
		setupNetworkManagerMocks(injector)

		// Mock the injector to return an error when resolving configHandler
		injector.SetResolveError("configHandler", fmt.Errorf("mock error resolving configHandler"))

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "failed to resolve CLI config handler: mock error resolving configHandler" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorCastingConfigHandler", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()

		// Register the configHandler as "invalid"
		mocks.Injector.Register("configHandler", "invalid")

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "resolved CLI config handler instance is not of type config.ConfigHandler" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		injector := di.NewMockInjector()

		// Mock the injector to return an error when resolving contextHandler
		injector.SetResolveError("contextHandler", fmt.Errorf("mock error resolving contextHandler"))

		// Setup mocks with the modified injector
		mocks := setupNetworkManagerMocks(injector)

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "failed to resolve context handler: mock error resolving contextHandler" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorCastingContextHandler", func(t *testing.T) {
		// Setup mocks
		mocks := setupNetworkManagerMocks()

		// Register a mock contextHandler with incorrect type
		mocks.Injector.Register("contextHandler", "incorrectType")

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "resolved context handler instance is not of type context.ContextInterface" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingNetworkInterfaceProvider", func(t *testing.T) {
		// Setup mocks with a new injector
		injector := di.NewMockInjector()
		injector.SetResolveError("networkInterfaceProvider", fmt.Errorf("mock error resolving network interface provider"))
		mocks := setupNetworkManagerMocks(injector)

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "failed to resolve network interface provider: mock error resolving network interface provider" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorCastingNetworkInterfaceProvider", func(t *testing.T) {
		// Setup mocks
		mocks := setupNetworkManagerMocks()

		// Register the networkInterfaceProvider as "invalid"
		mocks.Injector.Register("networkInterfaceProvider", "invalid")

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "resolved network interface provider instance is not of type NetworkInterfaceProvider" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})
}

func TestNetworkManager_NewNetworkManager(t *testing.T) {
	// Given: a DI container
	injector := di.NewInjector()

	// When: attempting to create NetworkManager
	_, err := NewBaseNetworkManager(injector)

	// Then: no error should be returned
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestNetworkManager_ConfigureGuest(t *testing.T) {
	// Given: a DI container
	injector := di.NewInjector()

	// When: creating a NetworkManager and configuring the guest
	nm, err := NewBaseNetworkManager(injector)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	err = nm.ConfigureGuest()

	// Then: no error should be returned
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestNetworkManager_getHostIP(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mocks using setupNetworkManagerMocks
		mocks := setupNetworkManagerMocks()

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Initialize the NetworkManager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// Run getHostIP on the NetworkManager
		hostIP, err := nm.getHostIP()
		if err != nil {
			t.Fatalf("expected no error during getHostIP, got %v", err)
		}

		// Verify the host IP
		expectedHostIP := "192.168.1.1"
		if hostIP != expectedHostIP {
			t.Fatalf("expected host IP %v, got %v", expectedHostIP, hostIP)
		}
	})

	t.Run("SuccessWithIpAddr", func(t *testing.T) {
		// Setup mocks using setupNetworkManagerMocks
		mocks := setupNetworkManagerMocks()

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Initialize the NetworkManager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// Mock networkInterfaceProvider.InterfaceAddrs to return a net.IPAddr
		mocks.MockNetworkInterfaceProvider.InterfaceAddrsFunc = func(iface net.Interface) ([]net.Addr, error) {
			return []net.Addr{
				&net.IPAddr{IP: net.ParseIP("192.168.1.1")},
			}, nil
		}

		// Run getHostIP on the NetworkManager
		hostIP, err := nm.getHostIP()
		if err != nil {
			t.Fatalf("expected no error during getHostIP, got %v", err)
		}

		// Verify the host IP
		expectedHostIP := "192.168.1.1"
		if hostIP != expectedHostIP {
			t.Fatalf("expected host IP %v, got %v", expectedHostIP, hostIP)
		}
	})

	t.Run("NoGuestAddressSet", func(t *testing.T) {
		// Setup mocks using setupNetworkManagerMocks
		mocks := setupNetworkManagerMocks()

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Initialize the NetworkManager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// Mock configHandler.GetString for vm.address to return an invalid IP
		originalGetStringFunc := mocks.MockConfigHandler.GetStringFunc
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.address" {
				return ""
			}
			return originalGetStringFunc(key, defaultValue...)
		}

		// Run getHostIP on the NetworkManager
		hostIP, err := nm.getHostIP()
		if err == nil {
			t.Fatalf("expected error during getHostIP, got none")
		}

		// Check the error message
		expectedErrorMessage := "guest IP is not configured"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message %q, got %q", expectedErrorMessage, err.Error())
		}

		// Verify the host IP is empty
		if hostIP != "" {
			t.Fatalf("expected empty host IP, got %v", hostIP)
		}
	})

	t.Run("ErrorParsingGuestIP", func(t *testing.T) {
		// Setup mocks using setupNetworkManagerMocks
		mocks := setupNetworkManagerMocks()

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Initialize the NetworkManager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// Mock configHandler.GetString for vm.address to return an invalid IP
		originalGetStringFunc := mocks.MockConfigHandler.GetStringFunc
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.address" {
				return "invalid_ip_address"
			}
			return originalGetStringFunc(key, defaultValue...)
		}

		// Run getHostIP on the NetworkManager
		hostIP, err := nm.getHostIP()
		if err == nil {
			t.Fatalf("expected error during getHostIP, got none")
		}

		// Check the error message
		expectedErrorMessage := "invalid guest IP address"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message %q, got %q", expectedErrorMessage, err.Error())
		}

		// Verify the host IP is empty
		if hostIP != "" {
			t.Fatalf("expected empty host IP, got %v", hostIP)
		}
	})

	t.Run("ErrorGettingNetworkInterfaces", func(t *testing.T) {
		// Setup mocks using setupNetworkManagerMocks
		mocks := setupNetworkManagerMocks()

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Initialize the NetworkManager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// Mock the network interface provider
		mocks.MockNetworkInterfaceProvider.InterfacesFunc = func() ([]net.Interface, error) {
			return nil, fmt.Errorf("mock error getting network interfaces")
		}

		// Run getHostIP on the NetworkManager
		hostIP, err := nm.getHostIP()
		if err == nil {
			t.Fatalf("expected error during getHostIP, got none")
		}

		// Check the error message
		expectedErrorMessage := "failed to get network interfaces: mock error getting network interfaces"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message %q, got %q", expectedErrorMessage, err.Error())
		}

		// Verify the host IP is empty
		if hostIP != "" {
			t.Fatalf("expected empty host IP, got %v", hostIP)
		}
	})

	t.Run("ErrorGettingNetworkInterfaceAddresses", func(t *testing.T) {
		// Setup mocks using setupNetworkManagerMocks
		mocks := setupNetworkManagerMocks()

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Initialize the NetworkManager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// Mock the network interface provider
		mocks.MockNetworkInterfaceProvider.InterfaceAddrsFunc = func(iface net.Interface) ([]net.Addr, error) {
			return nil, fmt.Errorf("mock error getting network interface addresses")
		}

		// Run getHostIP on the NetworkManager
		hostIP, err := nm.getHostIP()
		if err == nil {
			t.Fatalf("expected error during getHostIP, got none")
		}

		// Check the error message
		if !strings.Contains(err.Error(), "mock error getting network interface addresses") {
			t.Fatalf("expected error message to contain %q, got %q", "mock error getting network interface addresses", err.Error())
		}

		// Verify the host IP is empty
		if hostIP != "" {
			t.Fatalf("expected empty host IP, got %v", hostIP)
		}
	})

	t.Run("ErrorFindingHostIPInSameSubnet", func(t *testing.T) {
		// Setup mocks using setupNetworkManagerMocks
		mocks := setupNetworkManagerMocks()

		// Create a new NetworkManager
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Initialize the NetworkManager
		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// Mock the network interface provider to return interfaces with no matching subnet
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

		// Run getHostIP on the NetworkManager
		hostIP, err := nm.getHostIP()
		if err == nil {
			t.Fatalf("expected error during getHostIP, got none")
		}

		// Check the error message
		expectedErrorMessage := "failed to find host IP in the same subnet as guest IP"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message %q, got %q", expectedErrorMessage, err.Error())
		}

		// Verify the host IP is empty
		if hostIP != "" {
			t.Fatalf("expected empty host IP, got %v", hostIP)
		}
	})
}
