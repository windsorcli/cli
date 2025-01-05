package network

import (
	"fmt"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/context"
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

	t.Run("SuccessLocalhost", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

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

		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if !nm.isLocalhost {
			t.Fatalf("expected isLocalhost to be true, got false")
		}
	})

	t.Run("SetAddressFailure", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Mock a failure in SetAddress using SetAddressFunc
		mockService := services.NewMockService()
		mockService.SetAddressFunc = func(address string) error {
			return fmt.Errorf("mock error setting address for service")
		}
		mocks.Injector.Register("service", mockService)

		err = nm.Initialize()
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

	t.Run("ErrorSettingLocalhostAddresses", func(t *testing.T) {
		// Setup mock components
		mocks := setupNetworkManagerMocks()
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

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
		err = nm.Initialize()

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
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Mock GetString to return an empty string for docker.network_cidr
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "docker.network_cidr" {
				return ""
			}
			return ""
		}

		// Mock SetContextValue to return an error when setting docker.network_cidr
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			if key == "docker.network_cidr" {
				return fmt.Errorf("mock error setting network CIDR")
			}
			return nil
		}

		// Call the Initialize method
		err = nm.Initialize()

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

func TestNetworkManager_updateHostsFile(t *testing.T) {
	t.Run("AddEntrySuccess", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// Mock the shell execution
		mocks.MockShell.ExecSudoFunc = func(description string, command string, args ...string) (string, error) {
			return "", nil
		}

		// Mock the readFile and writeFile functions
		readFile = func(filename string) ([]byte, error) {
			return []byte(""), nil
		}
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return nil
		}

		nm.isLocalhost = true
		err = nm.updateHostsFile("example.com")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("AddEntryAlreadyExists", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// Mock the shell execution
		mocks.MockShell.ExecSudoFunc = func(description string, command string, args ...string) (string, error) {
			return "", nil
		}

		// Mock the readFile and writeFile functions
		readFile = func(filename string) ([]byte, error) {
			return []byte("127.0.0.1 example.com\n"), nil
		}
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("writeFile should not be called when entry already exists")
		}

		nm.isLocalhost = true
		err = nm.updateHostsFile("example.com")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("RemoveEntrySuccess", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// Mock the shell execution
		mocks.MockShell.ExecSudoFunc = func(description string, command string, args ...string) (string, error) {
			return "", nil
		}

		// Mock the readFile and writeFile functions
		readFile = func(filename string) ([]byte, error) {
			return []byte("127.0.0.1 example.com\n"), nil
		}
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return nil
		}

		nm.isLocalhost = false
		err = nm.updateHostsFile("example.com")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorReadingHostsFile", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// Mock the readFile function to return an error
		readFile = func(filename string) ([]byte, error) {
			return nil, fmt.Errorf("mock error reading file")
		}

		err = nm.updateHostsFile("example.com")
		if err == nil || !strings.Contains(err.Error(), "Error reading hosts file") {
			t.Fatalf("expected error reading hosts file, got %v", err)
		}
	})

	t.Run("ErrorWritingHostsFile", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// Mock the readFile function
		readFile = func(filename string) ([]byte, error) {
			return []byte(""), nil
		}

		// Mock the writeFile function to return an error
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock error writing file")
		}

		nm.isLocalhost = true
		err = nm.updateHostsFile("example.com")
		if err == nil || !strings.Contains(err.Error(), "Error writing to temporary hosts file") {
			t.Fatalf("expected error writing to temporary hosts file, got %v", err)
		}
	})

	t.Run("ErrorUpdatingHostsFile", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// Mock the shell execution to return an error
		mocks.MockShell.ExecSudoFunc = func(description string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock error updating hosts file")
		}

		// Mock the readFile and writeFile functions
		readFile = func(_ string) ([]byte, error) {
			return []byte(""), nil
		}
		writeFile = func(_ string, _ []byte, _ os.FileMode) error {
			return nil
		}

		nm.isLocalhost = true
		err = nm.updateHostsFile("example.com")
		if err == nil || !strings.Contains(err.Error(), "Error updating hosts file") {
			t.Fatalf("expected error updating hosts file, got %v", err)
		}
	})

	t.Run("WindowsHostsFilePath", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// Mock the goos function to return "windows"
		goos = func() string { return "windows" }

		// Mock the shell execution
		mocks.MockShell.ExecSudoFunc = func(description string, command string, args ...string) (string, error) {
			return "", nil
		}

		// Mock the readFile and writeFile functions
		readFile = func(filename string) ([]byte, error) {
			if filename != "C:\\Windows\\System32\\drivers\\etc\\hosts" {
				return nil, fmt.Errorf("unexpected file path: %s", filename)
			}
			return []byte(""), nil
		}
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			if filename != "C:\\Windows\\Temp\\hosts" {
				return fmt.Errorf("unexpected file path: %s", filename)
			}
			return nil
		}

		nm.isLocalhost = true
		err = nm.updateHostsFile("example.com")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorUpdatingWindowsHostsFile", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// Mock the goos function to return "windows"
		originalGoos := goos
		defer func() { goos = originalGoos }()
		goos = func() string { return "windows" }

		// Mock the shell execution to simulate an error
		mocks.MockShell.ExecSudoFunc = func(description string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock error updating hosts file")
		}

		// Mock the readFile and writeFile functions
		readFile = func(filename string) ([]byte, error) {
			return []byte(""), nil
		}
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return nil
		}

		nm.isLocalhost = true
		err = nm.updateHostsFile("example.com")
		if err == nil || !strings.Contains(err.Error(), "mock error updating hosts file") {
			t.Fatalf("expected mock error updating hosts file, got %v", err)
		}
	})

	t.Run("UnixHostsFilePath", func(t *testing.T) {
		mocks := setupNetworkManagerMocks()
		nm, err := NewBaseNetworkManager(mocks.Injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		err = nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during Initialize, got %v", err)
		}

		// Mock the goos function to return a Unix-like OS
		originalGoos := goos
		defer func() { goos = originalGoos }()
		goos = func() string { return "linux" }

		// Mock the shell execution
		mocks.MockShell.ExecSudoFunc = func(description string, command string, args ...string) (string, error) {
			return "", nil
		}

		// Mock the readFile and writeFile functions
		readFile = func(filename string) ([]byte, error) {
			if filename != "/etc/hosts" {
				return nil, fmt.Errorf("unexpected file path: %s", filename)
			}
			return []byte(""), nil
		}
		writeFile = func(filename string, _ []byte, _ os.FileMode) error {
			if filename != "/tmp/hosts" {
				return fmt.Errorf("unexpected file path: %s", filename)
			}
			return nil
		}

		nm.isLocalhost = true
		err = nm.updateHostsFile("example.com")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
