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

// =============================================================================
// Test Setup
// =============================================================================

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
	mockService2 := services.NewMockService()
	injector.Register("service1", mockService1)
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

// =============================================================================
// Test Public Methods
// =============================================================================

func TestNetworkManager_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		mocks := setupNetworkManagerMocks()

		// And tracking IP address assignments
		var setAddressCalls []string
		mockService1 := services.NewMockService()
		mockService1.SetAddressFunc = func(address string) error {
			setAddressCalls = append(setAddressCalls, address)
			return nil
		}
		mockService2 := services.NewMockService()
		mockService2.SetAddressFunc = func(address string) error {
			setAddressCalls = append(setAddressCalls, address)
			return nil
		}
		mocks.Injector.Register("service1", mockService1)
		mocks.Injector.Register("service2", mockService2)

		// When creating and initializing the network manager
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And services should be assigned IP addresses from the CIDR range
		expectedIPs := []string{"192.168.1.2", "192.168.1.3"}
		if len(setAddressCalls) != len(expectedIPs) {
			t.Errorf("expected %d IP assignments, got %d", len(expectedIPs), len(setAddressCalls))
		}
		for i, expectedIP := range expectedIPs {
			if i >= len(setAddressCalls) {
				break
			}
			if setAddressCalls[i] != expectedIP {
				t.Errorf("expected IP %s to be assigned, got %s", expectedIP, setAddressCalls[i])
			}
		}
	})

	t.Run("SetAddressFailure", func(t *testing.T) {
		// Given a network manager with service address failure
		mocks := setupNetworkManagerMocks()
		nm := NewBaseNetworkManager(mocks.Injector)

		// And a service that fails to set address
		mockService := services.NewMockService()
		mockService.SetAddressFunc = func(address string) error {
			return fmt.Errorf("mock error setting address for service")
		}
		mocks.Injector.Register("service", mockService)

		// When initializing the network manager
		err := nm.Initialize()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error during Initialize, got nil")
		}

		// And the error should contain the expected message
		expectedErrorSubstring := "error setting address for service"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given a network manager with invalid shell
		mocks := setupNetworkManagerMocks()
		mocks.Injector.Register("shell", "invalid")

		// When creating and initializing the network manager
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		}

		// And the error should be about shell type
		if err.Error() != "resolved shell instance is not of type shell.Shell" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Given a network manager with invalid config handler
		mocks := setupNetworkManagerMocks()
		mocks.Injector.Register("configHandler", "invalid")

		// When creating and initializing the network manager
		nm := NewBaseNetworkManager(mocks.Injector)
		err := nm.Initialize()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		}

		// And the error should be about config handler
		if err.Error() != "error resolving configHandler" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingServices", func(t *testing.T) {
		// Given a network manager with service resolution error
		injector := di.NewMockInjector()
		mocks := setupNetworkManagerMocks(injector)
		nm := NewBaseNetworkManager(mocks.Injector)

		// And mocking service resolution to fail
		injector.SetResolveAllError(new(services.Service), fmt.Errorf("mock error resolving services"))

		// When initializing the network manager
		err := nm.Initialize()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}

		// And the error should contain the expected message
		expectedErrorSubstring := "error resolving services"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}
	})

	t.Run("ErrorSettingNetworkCidr", func(t *testing.T) {
		// Given a network manager with CIDR setting error
		mocks := setupNetworkManagerMocks()
		nm := NewBaseNetworkManager(mocks.Injector)

		// And mocking empty CIDR block
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "network.cidr_block" {
				return ""
			}
			return ""
		}

		// And mocking CIDR setting failure
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value any) error {
			if key == "network.cidr_block" {
				return fmt.Errorf("mock error setting network CIDR")
			}
			return nil
		}

		// When initializing the network manager
		err := nm.Initialize()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}

		// And the error should contain the expected message
		expectedErrorSubstring := "error setting default network CIDR"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}
	})

	t.Run("ErrorAssigningIPAddresses", func(t *testing.T) {
		// Given a network manager with IP assignment error
		injector := di.NewMockInjector()
		mocks := setupNetworkManagerMocks(injector)
		nm := NewBaseNetworkManager(mocks.Injector)

		// And mocking IP assignment to fail
		originalAssignIPAddresses := assignIPAddresses
		defer func() { assignIPAddresses = originalAssignIPAddresses }()
		assignIPAddresses = func(services []services.Service, networkCIDR *string) error {
			return fmt.Errorf("mock assign IP addresses error")
		}

		// When initializing the network manager
		err := nm.Initialize()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}

		// And the error should contain the expected message
		expectedErrorSubstring := "error assigning IP addresses"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}
	})
}

func TestNetworkManager_NewNetworkManager(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a DI container
		injector := di.NewInjector()

		// When creating a new BaseNetworkManager
		nm := NewBaseNetworkManager(injector)

		// Then the NetworkManager should not be nil
		if nm == nil {
			t.Fatalf("expected NetworkManager to be created, got nil")
		}
	})
}

func TestNetworkManager_ConfigureGuest(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a DI container
		injector := di.NewInjector()

		// When creating a NetworkManager and configuring the guest
		nm := NewBaseNetworkManager(injector)
		err := nm.ConfigureGuest()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestNetworkManager_assignIPAddresses(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a list of services and a network CIDR
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

		// When assigning IP addresses
		err := assignIPAddresses(services, &networkCIDR)

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And services should be assigned the correct IPs
		expectedIPs := []string{"10.5.0.2", "10.5.0.3"}
		for i, expectedIP := range expectedIPs {
			if setAddressCalls[i] != expectedIP {
				t.Errorf("expected SetAddress to be called with IP %s, got %s", expectedIP, setAddressCalls[i])
			}
		}
	})

	t.Run("InvalidNetworkCIDR", func(t *testing.T) {
		// Given services and an invalid network CIDR
		services := []services.Service{
			&services.MockService{},
			&services.MockService{},
		}
		networkCIDR := "invalid-cidr"

		// When assigning IP addresses
		err := assignIPAddresses(services, &networkCIDR)

		// Then an error should occur
		if err == nil {
			t.Fatal("expected an error, got none")
		}

		// And the error should be about parsing the CIDR
		if !strings.Contains(err.Error(), "error parsing network CIDR") {
			t.Fatalf("expected error message to contain 'error parsing network CIDR', got %v", err)
		}
	})

	t.Run("ErrorSettingAddress", func(t *testing.T) {
		// Given a service that fails to set address
		services := []services.Service{
			&services.MockService{
				SetAddressFunc: func(address string) error {
					return fmt.Errorf("error setting address")
				},
			},
		}
		networkCIDR := "10.5.0.0/16"

		// When assigning IP addresses
		err := assignIPAddresses(services, &networkCIDR)

		// Then an error should occur
		if err == nil {
			t.Fatal("expected an error, got none")
		}

		// And the error should be about setting the address
		if !strings.Contains(err.Error(), "error setting address") {
			t.Fatalf("expected error message to contain 'error setting address', got %v", err)
		}
	})

	t.Run("NotEnoughIPAddresses", func(t *testing.T) {
		// Given more services than available IPs
		services := []services.Service{
			&services.MockService{},
			&services.MockService{},
			&services.MockService{},
		}
		networkCIDR := "10.5.0.0/30"

		// When assigning IP addresses
		err := assignIPAddresses(services, &networkCIDR)

		// Then an error should occur
		if err == nil {
			t.Fatal("expected an error, got none")
		}

		// And the error should be about insufficient IPs
		if !strings.Contains(err.Error(), "not enough IP addresses in the CIDR range") {
			t.Fatalf("expected error message to contain 'not enough IP addresses in the CIDR range', got %v", err)
		}
	})

	t.Run("NetworkCIDRNotDefined", func(t *testing.T) {
		// Given services but no network CIDR
		services := []services.Service{
			&services.MockService{},
		}
		var networkCIDR *string

		// When assigning IP addresses
		err := assignIPAddresses(services, networkCIDR)

		// Then an error should occur
		if err == nil {
			t.Fatal("expected an error, got none")
		}

		// And the error should be about undefined CIDR
		if !strings.Contains(err.Error(), "network CIDR is not defined") {
			t.Fatalf("expected error message to contain 'network CIDR is not defined', got %v", err)
		}
	})
}
