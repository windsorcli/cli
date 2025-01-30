package network

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/ssh"
)

// NetworkManagerMocks holds all the mock dependencies for NetworkManager
type NetworkManagerMocks struct {
	Injector                     di.Injector
	MockShell                    *shell.MockShell
	MockSecureShell              *shell.MockShell
	MockConfigHandler            *config.MockConfigHandler
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
		case "network.cidr_block":
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
		MockSSHClient:                mockSSHClient,
		MockNetworkInterfaceProvider: mockNetworkInterfaceProvider,
	}
}

func TestNetworkManager_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()
		nm := NewBaseNetworkManager(mocks.Injector)

		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if nm == nil {
			t.Fatalf("expected a valid NetworkManager instance, got nil")
		}
	})

	t.Run("SuccessLocalhost", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()
		nm := NewBaseNetworkManager(mocks.Injector)

		// Set the configuration to simulate docker-desktop
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			return ""
		}

		// Capture the SetAddress calls
		mockService := services.NewMockService()
		mockService.SetAddressFunc = func(address string) error {
			if address != "127.0.0.1" {
				return fmt.Errorf("expected address to be 127.0.0.1, got %v", address)
			}
			return nil
		}
		mocks.Injector.Register("service", mockService)

		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if !nm.isLocalhost {
			t.Fatalf("expected isLocalhost to be true, got false")
		}
	})

	t.Run("SetAddressFailure", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()
		nm := NewBaseNetworkManager(mocks.Injector)

		// Mock a failure in SetAddress using SetAddressFunc
		mockService := services.NewMockService()
		mockService.SetAddressFunc = func(address string) error {
			return fmt.Errorf("mock error setting address for service")
		}
		mocks.Injector.Register("service", mockService)

		err := nm.Initialize()
		if err == nil {
			t.Fatalf("expected error during Initialize, got nil")
		}

		expectedErrorSubstring := "error setting address for service"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()

		// Register the shell as "invalid"
		mocks.Injector.Register("shell", "invalid")

		// Create a new NetworkManager
		nm := NewBaseNetworkManager(mocks.Injector)

		// Run Initialize on the NetworkManager
		err := nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "resolved shell instance is not of type shell.Shell" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()

		// Register the configHandler as "invalid"
		mocks.Injector.Register("configHandler", "invalid")

		// Create a new NetworkManager
		nm := NewBaseNetworkManager(mocks.Injector)

		// Run Initialize on the NetworkManager
		err := nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "error resolving configHandler" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingServices", func(t *testing.T) {
		// Setup mock components
		injector := di.NewMockInjector()
		mocks := setupNetworkManagerMocks(injector)
		nm := NewBaseNetworkManager(mocks.Injector)

		// Mock ResolveAll to return an error
		injector.SetResolveAllError(new(services.Service), fmt.Errorf("mock error resolving services"))

		// Call the Initialize method
		err := nm.Initialize()

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

	t.Run("ErrorSettingLocalhostAddresses", func(t *testing.T) {
		// Setup mock components
		mocks := setupNetworkManagerMocks()
		nm := NewBaseNetworkManager(mocks.Injector)

		// Set the configuration to simulate docker-desktop
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			return ""
		}

		// Mock SetAddress to return an error
		mockService := services.NewMockService()
		mockService.SetAddressFunc = func(address string) error {
			if address == "127.0.0.1" {
				return fmt.Errorf("mock error setting address")
			}
			return nil
		}
		mocks.Injector.Register("service", mockService)

		// Call the Initialize method
		err := nm.Initialize()

		// Assert that an error occurred
		if err == nil {
			t.Errorf("expected error, got none")
		}

		// Verify the error message contains the expected substring
		expectedErrorSubstring := "error setting address for service"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}

		// Verify that isLocalhost is true
		if !nm.isLocalhost {
			t.Errorf("expected isLocalhost to be true, got false")
		}
	})

	t.Run("ErrorSettingNetworkCidr", func(t *testing.T) {
		// Setup mock components
		mocks := setupNetworkManagerMocks()
		nm := NewBaseNetworkManager(mocks.Injector)

		// Mock GetString to return an empty string for network.cidr_block
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "network.cidr_block" {
				return ""
			}
			return ""
		}

		// Mock SetContextValue to return an error when setting network.cidr_block
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			if key == "network.cidr_block" {
				return fmt.Errorf("mock error setting network CIDR")
			}
			return nil
		}

		// Call the Initialize method
		err := nm.Initialize()

		// Assert that an error occurred
		if err == nil {
			t.Errorf("expected error, got none")
		}

		// Verify the error message contains the expected substring
		expectedErrorSubstring := "error setting default network CIDR"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}
	})

	t.Run("ErrorAssigningIPAddresses", func(t *testing.T) {
		// Setup mock components
		injector := di.NewMockInjector()
		mocks := setupNetworkManagerMocks(injector)
		nm := NewBaseNetworkManager(mocks.Injector)

		// Simulate an error during IP address assignment
		originalAssignIPAddresses := assignIPAddresses
		defer func() { assignIPAddresses = originalAssignIPAddresses }()
		assignIPAddresses = func(services []services.Service, networkCIDR *string) error {
			return fmt.Errorf("mock assign IP addresses error")
		}

		// Call the Initialize method
		err := nm.Initialize()

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
	t.Run("Success", func(t *testing.T) {
		// Given: a DI container
		injector := di.NewInjector()

		// When: creating a new BaseNetworkManager
		nm := NewBaseNetworkManager(injector)

		// Then: the NetworkManager should not be nil
		if nm == nil {
			t.Fatalf("expected NetworkManager to be created, got nil")
		}
	})
}

func TestNetworkManager_ConfigureGuest(t *testing.T) {
	// Given: a DI container
	injector := di.NewInjector()

	// When: creating a NetworkManager and configuring the guest
	nm := NewBaseNetworkManager(injector)

	err := nm.ConfigureGuest()

	// Then: no error should be returned
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
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

	t.Run("NetworkCIDRNotDefined", func(t *testing.T) {
		services := []services.Service{
			&services.MockService{},
		}
		var networkCIDR *string

		err := assignIPAddresses(services, networkCIDR)
		if err == nil {
			t.Fatal("expected an error, got none")
		}
		if !strings.Contains(err.Error(), "network CIDR is not defined") {
			t.Fatalf("expected error message to contain 'network CIDR is not defined', got %v", err)
		}
	})
}
