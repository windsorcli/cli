package network

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/services"
	"github.com/windsorcli/cli/internal/shell"
	"github.com/windsorcli/cli/internal/ssh"
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
	mockShell.ExecFunc = func(command string, args ...string) (string, error) {
		return "", nil
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

	// Create mock services
	mockService1 := services.NewMockService()
	mockService1.SetName("Service1")
	injector.Register("service1", mockService1)

	// Create another mock service
	mockService2 := services.NewMockService()
	mockService2.SetName("Service2")
	injector.Register("service2", mockService2)

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

	t.Run("ErrorResolvingSecureShell", func(t *testing.T) {
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
		} else if err.Error() != "error resolving configHandler" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
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
		} else if err.Error() != "failed to resolve context handler" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingNetworkInterfaceProvider", func(t *testing.T) {
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
		} else if err.Error() != "failed to resolve network interface provider" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingServices", func(t *testing.T) {
		// Setup mock components
		injector := di.NewMockInjector()
		mocks := setupNetworkManagerMocks(injector)
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Mock ResolveAll to return an error
		injector.SetResolveAllError(new(services.Service), fmt.Errorf("mock error resolving services"))

		// Call the Initialize method
		err = nm.Initialize()

		// Assert that an error occurred
		if err == nil {
			t.Errorf("expected error, got none")
		}

		// Verify the error message contains the expected substring
		expectedErrorSubstring := "error resolving services"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}
	})

	t.Run("ErrorAssigningIPAddresses", func(t *testing.T) {
		// Setup mock components
		injector := di.NewMockInjector()
		mocks := setupNetworkManagerMocks(injector)
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Simulate an error during IP address assignment
		originalAssignIPAddresses := assignIPAddresses
		defer func() { assignIPAddresses = originalAssignIPAddresses }()
		assignIPAddresses = func(services []services.Service, networkCIDR *string) error {
			return fmt.Errorf("mock assign IP addresses error")
		}

		// Call the Initialize method
		err = nm.Initialize()

		// Assert that an error occurred
		if err == nil {
			t.Errorf("expected error, got none")
		}

		// Verify the error message contains the expected substring
		expectedErrorSubstring := "error assigning IP addresses"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
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

func TestNetworkManager_assignIPAddresses(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		var setAddressCalls []string
		services := []services.Service{
			&services.MockService{
				SetAddressFunc: func(address string) error {
					setAddressCalls = append(setAddressCalls, address)
					return nil
				},
			},
			&services.MockService{
				SetAddressFunc: func(address string) error {
					setAddressCalls = append(setAddressCalls, address)
					return nil
				},
			},
		}
		networkCIDR := "10.5.0.0/16"

		err := assignIPAddresses(services, &networkCIDR)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		expectedIPs := []string{"10.5.0.2", "10.5.0.3"}
		for i, expectedIP := range expectedIPs {
			if setAddressCalls[i] != expectedIP {
				t.Errorf("expected SetAddress to be called with IP %s, got %s", expectedIP, setAddressCalls[i])
			}
		}
	})

	t.Run("NilNetworkCIDR", func(t *testing.T) {
		services := []services.Service{
			&services.MockService{},
			&services.MockService{},
		}

		err := assignIPAddresses(services, nil)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		for _, service := range services {
			if service.GetAddress() != "" {
				t.Errorf("expected empty address, got %s", service.GetAddress())
			}
		}
	})

	t.Run("InvalidNetworkCIDR", func(t *testing.T) {
		services := []services.Service{
			&services.MockService{},
			&services.MockService{},
		}
		networkCIDR := "invalid-cidr"

		err := assignIPAddresses(services, &networkCIDR)
		if err == nil {
			t.Fatal("expected an error, got none")
		}
		if !strings.Contains(err.Error(), "error parsing network CIDR") {
			t.Fatalf("expected error message to contain 'error parsing network CIDR', got %v", err)
		}
	})

	t.Run("ErrorSettingAddress", func(t *testing.T) {
		services := []services.Service{
			&services.MockService{
				SetAddressFunc: func(address string) error {
					return fmt.Errorf("error setting address")
				},
			},
		}
		networkCIDR := "10.5.0.0/16"

		err := assignIPAddresses(services, &networkCIDR)
		if err == nil {
			t.Fatal("expected an error, got none")
		}
		if !strings.Contains(err.Error(), "error setting address") {
			t.Fatalf("expected error message to contain 'error setting address', got %v", err)
		}
	})

	t.Run("NotEnoughIPAddresses", func(t *testing.T) {
		services := []services.Service{
			&services.MockService{},
			&services.MockService{},
			&services.MockService{},
		}
		networkCIDR := "10.5.0.0/30"

		err := assignIPAddresses(services, &networkCIDR)
		if err == nil {
			t.Fatal("expected an error, got none")
		}
		if !strings.Contains(err.Error(), "not enough IP addresses in the CIDR range") {
			t.Fatalf("expected error message to contain 'not enough IP addresses in the CIDR range', got %v", err)
		}
	})
}
