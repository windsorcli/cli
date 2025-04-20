package network

import (
	"fmt"
	"net"
	"os"
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

type Mocks struct {
	Injector                 di.Injector
	ConfigHandler            config.ConfigHandler
	Shell                    *shell.MockShell
	SecureShell              *shell.MockShell
	SSHClient                *ssh.MockClient
	NetworkInterfaceProvider *MockNetworkInterfaceProvider
	Services                 []*services.MockService
	Shims                    *Shims
}

type SetupOptions struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	ConfigStr     string
}

func setupShims(t *testing.T) *Shims {
	t.Helper()

	return &Shims{
		Stat:      func(path string) (os.FileInfo, error) { return nil, nil },
		WriteFile: func(path string, data []byte, perm os.FileMode) error { return nil },
		ReadFile:  func(path string) ([]byte, error) { return nil, nil },
		ReadLink:  func(path string) (string, error) { return "", nil },
		MkdirAll:  func(path string, perm os.FileMode) error { return nil },
	}
}

func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	// Store original directory and create temp dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Set project root environment variable
	t.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	// Register cleanup to restore original state
	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	// Create injector if not provided
	var injector di.Injector
	if len(opts) > 0 && opts[0].Injector != nil {
		injector = opts[0].Injector
	} else {
		injector = di.NewInjector()
	}

	// Create config handler if not provided
	var configHandler config.ConfigHandler
	if len(opts) > 0 && opts[0].ConfigHandler != nil {
		configHandler = opts[0].ConfigHandler
	} else {
		configHandler = config.NewYamlConfigHandler(injector)
	}
	injector.Register("configHandler", configHandler)

	configYAML := `
version: v1alpha1
contexts:
  mock-context:
    network:
      cidr_block: "192.168.1.0/24"
    vm:
      address: "192.168.1.10"
    dns:
      domain: "example.com"
      address: "1.2.3.4"
`
	if err := configHandler.LoadConfigString(configYAML); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Load optional config if provided
	if len(opts) > 0 && opts[0].ConfigStr != "" {
		if err := configHandler.LoadConfigString(opts[0].ConfigStr); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}
	}

	// Create a mock shell
	mockShell := shell.NewMockShell(injector)
	mockShell.ExecFunc = func(command string, args ...string) (string, error) {
		return "", nil
	}
	mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		if command == "ls" && args[0] == "/sys/class/net" {
			return "br-bridge0\neth0\nlo", nil
		}
		if command == "sudo" && args[0] == "iptables" && args[1] == "-t" && args[2] == "filter" && args[3] == "-C" {
			return "", fmt.Errorf("Bad rule")
		}
		return "", nil
	}
	injector.Register("shell", mockShell)

	// Create a mock secure shell
	mockSecureShell := shell.NewMockShell(injector)
	mockSecureShell.ExecFunc = func(command string, args ...string) (string, error) {
		return "", nil
	}
	mockSecureShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		if command == "ls" && args[0] == "/sys/class/net" {
			return "br-bridge0\neth0\nlo", nil
		}
		if command == "sudo" && args[0] == "iptables" && args[1] == "-t" && args[2] == "filter" && args[3] == "-C" {
			return "", fmt.Errorf("Bad rule")
		}
		return "", nil
	}
	injector.Register("secureShell", mockSecureShell)

	// Create a mock SSH client
	mockSSHClient := ssh.NewMockSSHClient()
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

	// Create mocks struct with references to the same instances
	mocks := &Mocks{
		Injector:                 injector,
		ConfigHandler:            configHandler,
		Shell:                    mockShell,
		SecureShell:              mockSecureShell,
		SSHClient:                mockSSHClient,
		NetworkInterfaceProvider: mockNetworkInterfaceProvider,
		Services:                 []*services.MockService{mockService1, mockService2},
		Shims:                    setupShims(t),
	}

	configHandler.Initialize()
	configHandler.SetContext("mock-context")

	return mocks
}

// NetworkManagerMocks holds all the mock dependencies for NetworkManager
type NetworkManagerMocks struct {
	Injector                     di.Injector
	MockShell                    *shell.MockShell
	MockSecureShell              *shell.MockShell
	MockConfigHandler            *config.MockConfigHandler
	MockSSHClient                *ssh.MockClient
	MockNetworkInterfaceProvider *MockNetworkInterfaceProvider
}

// =============================================================================
// Test Constructor
// =============================================================================

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

// =============================================================================
// Test Public Methods
// =============================================================================

func TestNetworkManager_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		manager := NewBaseNetworkManager(mocks.Injector)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		manager, mocks := setup(t)

		// And tracking IP address assignments
		var setAddressCalls []string
		mockService1 := mocks.Services[0]
		mockService2 := mocks.Services[1]
		mockService1.SetAddressFunc = func(address string) error {
			setAddressCalls = append(setAddressCalls, address)
			return nil
		}
		mockService2.SetAddressFunc = func(address string) error {
			setAddressCalls = append(setAddressCalls, address)
			return nil
		}

		// When creating and initializing the network manager
		err := manager.Initialize()

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

	t.Run("ErrorResolvingSecureShell", func(t *testing.T) {
		// Given a network manager with invalid secure shell
		manager, mocks := setup(t)
		mocks.Injector.Register("secureShell", "invalid")

		// When initializing the network manager
		err := manager.Initialize()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		}

		// And the error should be about secure shell type
		if err.Error() != "resolved secure shell instance is not of type shell.Shell" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingSSHClient", func(t *testing.T) {
		// Given a network manager with invalid SSH client
		manager, mocks := setup(t)
		mocks.Injector.Register("sshClient", "invalid")

		// When initializing the network manager
		err := manager.Initialize()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		}

		// And the error should be about SSH client type
		if err.Error() != "resolved ssh client instance is not of type ssh.Client" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingNetworkInterfaceProvider", func(t *testing.T) {
		// Given a network manager with invalid network interface provider
		manager, mocks := setup(t)
		mocks.Injector.Register("networkInterfaceProvider", "invalid")

		// When initializing the network manager
		err := manager.Initialize()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		}

		// And the error should be about network interface provider type
		if err.Error() != "failed to resolve network interface provider" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("SetAddressFailure", func(t *testing.T) {
		// Given a network manager with service address failure
		manager, mocks := setup(t)
		mocks.Services[0].SetAddressFunc = func(address string) error {
			return fmt.Errorf("mock error setting address for service")
		}

		// When initializing the network manager
		err := manager.Initialize()

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
		manager, mocks := setup(t)
		mocks.Injector.Register("shell", "invalid")

		// When initializing the network manager
		err := manager.Initialize()

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
		manager, mocks := setup(t)
		mocks.Injector.Register("configHandler", "invalid")

		// When initializing the network manager
		err := manager.Initialize()

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
		mockInjector := di.NewMockInjector()
		setupMocks(t, &SetupOptions{
			Injector: mockInjector,
		})
		manager := NewBaseNetworkManager(mockInjector)
		mockInjector.SetResolveAllError(new(services.Service), fmt.Errorf("mock error resolving services"))

		// When initializing the network manager
		err := manager.Initialize()

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
		manager, mocks := setup(t)
		mocks.Services[0].SetAddressFunc = func(address string) error {
			return fmt.Errorf("error setting default network CIDR")
		}

		// When initializing the network manager
		err := manager.Initialize()

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
		manager, mocks := setup(t)
		mocks.Services[0].SetAddressFunc = func(address string) error {
			return fmt.Errorf("mock assign IP addresses error")
		}

		// When initializing the network manager
		err := manager.Initialize()

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

	t.Run("ResolveShellFailure", func(t *testing.T) {
		// Given a network manager with shell resolution failure
		manager, mocks := setup(t)
		mocks.Injector.Register("shell", "invalid")

		// When initializing the network manager
		err := manager.Initialize()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error during Initialize, got nil")
		}

		// And the error should contain the expected message
		expectedErrorSubstring := "resolved shell instance is not of type shell.Shell"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}
	})
}

func TestNetworkManager_ConfigureGuest(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		manager := NewBaseNetworkManager(mocks.Injector)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		manager, _ := setup(t)

		// When configuring the guest
		err := manager.ConfigureGuest()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestNetworkManager_assignIPAddresses(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		manager := NewBaseNetworkManager(mocks.Injector)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	// Helper to convert mock services to service interface slice
	toServices := func(mockServices []*services.MockService) []services.Service {
		services := make([]services.Service, len(mockServices))
		for i, s := range mockServices {
			services[i] = s
		}
		return services
	}

	t.Run("Success", func(t *testing.T) {
		// Given a list of services and a network CIDR
		_, mocks := setup(t)
		var setAddressCalls []string
		mocks.Services[0].SetAddressFunc = func(address string) error {
			setAddressCalls = append(setAddressCalls, address)
			return nil
		}
		mocks.Services[1].SetAddressFunc = func(address string) error {
			setAddressCalls = append(setAddressCalls, address)
			return nil
		}
		networkCIDR := "10.5.0.0/16"

		// When assigning IP addresses
		err := assignIPAddresses(toServices(mocks.Services), &networkCIDR)

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
		_, mocks := setup(t)
		networkCIDR := "invalid-cidr"

		// When assigning IP addresses
		err := assignIPAddresses(toServices(mocks.Services), &networkCIDR)

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
		_, mocks := setup(t)
		mocks.Services[0].SetAddressFunc = func(address string) error {
			return fmt.Errorf("error setting address")
		}
		networkCIDR := "10.5.0.0/16"

		// When assigning IP addresses
		err := assignIPAddresses(toServices(mocks.Services[:1]), &networkCIDR)

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
		_, mocks := setup(t)
		networkCIDR := "10.5.0.0/30"

		// When assigning IP addresses
		err := assignIPAddresses(toServices(mocks.Services), &networkCIDR)

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
		_, mocks := setup(t)
		var networkCIDR *string

		// When assigning IP addresses
		err := assignIPAddresses(toServices(mocks.Services[:1]), networkCIDR)

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
