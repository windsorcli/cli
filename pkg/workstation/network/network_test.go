package network

import (
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/workstation/services"
)

// =============================================================================
// Test Setup
// =============================================================================

type NetworkTestMocks struct {
	Runtime                  *runtime.Runtime
	ConfigHandler            config.ConfigHandler
	Shell                    *shell.MockShell
	NetworkInterfaceProvider *MockNetworkInterfaceProvider
	Services                 []*services.MockService
	Shims                    *Shims
}

func setupDefaultShims() *Shims {
	return &Shims{
		Stat:      func(path string) (os.FileInfo, error) { return nil, nil },
		WriteFile: func(path string, data []byte, perm os.FileMode) error { return nil },
		ReadFile:  func(path string) ([]byte, error) { return nil, nil },
		ReadLink:  func(path string) (string, error) { return "", nil },
		MkdirAll:  func(path string, perm os.FileMode) error { return nil },
	}
}

func setupNetworkMocks(t *testing.T, opts ...func(*NetworkTestMocks)) *NetworkTestMocks {
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

	// Create a mock shell first (needed for config handler)
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	// Create config handler
	configHandler := config.NewConfigHandler(mockShell)

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

	// Configure mock shell functions
	mockShell.ExecFunc = func(command string, args ...string) (string, error) {
		return "", nil
	}
	mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		return "", nil
	}
	mockShell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
		if command == "colima" && len(args) > 0 && args[0] == "ssh" {
			if len(args) > 4 {
				cmd := args[4]
				if strings.Contains(cmd, "ls /sys/class/net") {
					return "br-bridge0\neth0\nlo", nil
				}
				if strings.Contains(cmd, "iptables") && strings.Contains(cmd, "-C") {
					return "", fmt.Errorf("Bad rule")
				}
			}
		}
		return "", nil
	}


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

	// Create mock services
	mockService1 := services.NewMockService()
	mockService2 := services.NewMockService()

	rt := &runtime.Runtime{
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir,
		TemplateRoot:  tmpDir,
		ContextName:   "mock-context",
		ConfigHandler: configHandler,
		Shell:         mockShell,
	}

	// Create mocks struct with references to the same instances
	mocks := &NetworkTestMocks{
		Runtime:                  rt,
		ConfigHandler:            configHandler,
		Shell:                    mockShell,
		NetworkInterfaceProvider: mockNetworkInterfaceProvider,
		Services:                 []*services.MockService{mockService1, mockService2},
		Shims:                    setupDefaultShims(),
	}

	configHandler.SetContext("mock-context")

	// Apply any overrides
	for _, opt := range opts {
		opt(mocks)
	}

	return mocks
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNetworkManager_NewNetworkManager(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a runtime
		rt := &runtime.Runtime{
			ConfigHandler: config.NewMockConfigHandler(),
			Shell:         shell.NewMockShell(),
		}

		// When creating a new BaseNetworkManager
		nm := NewBaseNetworkManager(rt)

		// Then the NetworkManager should not be nil
		if nm == nil {
			t.Fatalf("expected NetworkManager to be created, got nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestNetworkManager_AssignIPs(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
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
		mockService1.SetAddressFunc = func(address string, portAllocator *services.PortAllocator) error {
			setAddressCalls = append(setAddressCalls, address)
			return nil
		}
		mockService2.SetAddressFunc = func(address string, portAllocator *services.PortAllocator) error {
			setAddressCalls = append(setAddressCalls, address)
			return nil
		}

		// Convert mock services to service interface slice
		serviceList := make([]services.Service, len(mocks.Services))
		for i, s := range mocks.Services {
			serviceList[i] = s
		}

		// When assigning IPs to services
		err := manager.AssignIPs(serviceList)

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
		manager, mocks := setup(t)
		mocks.Services[0].SetAddressFunc = func(address string, portAllocator *services.PortAllocator) error {
			return fmt.Errorf("mock error setting address for service")
		}

		// Convert mock services to service interface slice
		serviceList := make([]services.Service, len(mocks.Services))
		for i, s := range mocks.Services {
			serviceList[i] = s
		}

		// When assigning IPs
		err := manager.AssignIPs(serviceList)

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error during Initialize, got nil")
		}

		// And the error should contain the expected message
		expectedErrorSubstring := "error assigning IP addresses"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}
	})

	t.Run("ErrorResolvingServices", func(t *testing.T) {
		// Given a network manager
		manager, mocks := setup(t)

		// When assigning IPs with services
		// (services are now passed explicitly, so no resolution error can occur)
		err := manager.AssignIPs([]services.Service{})

		// Then no error should occur (services are passed directly, not resolved)
		if err != nil {
			t.Errorf("expected no error when services are passed explicitly, got %v", err)
		}

		// Verify manager was initialized
		if manager.configHandler == nil {
			t.Error("expected configHandler to be set")
		}
		if manager.shell == nil {
			t.Error("expected shell to be set")
		}
		_ = mocks // suppress unused variable warning
	})

	t.Run("ErrorSettingNetworkCidr", func(t *testing.T) {
		// Given a network manager with CIDR setting error
		// Create a mock config handler that returns error for Set
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "network.cidr_block" {
				return "" // Return empty so Set is called
			}
			return "10.5.0.0/16"
		}
		mockConfigHandler.SetFunc = func(key string, value any) error {
			if key == "network.cidr_block" {
				return fmt.Errorf("error setting default network CIDR")
			}
			return nil
		}

		// Setup with mock config handler
		mocks := setupNetworkMocks(t, func(m *NetworkTestMocks) {
			m.ConfigHandler = mockConfigHandler
			m.Runtime.ConfigHandler = mockConfigHandler
		})
		manager := NewBaseNetworkManager(mocks.Runtime)
		manager.shims = mocks.Shims

		// When assigning IPs
		err := manager.AssignIPs([]services.Service{})

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
		// Create a service with SetAddress that returns error
		mockService := services.NewMockService()
		mockService.SetAddressFunc = func(address string, portAllocator *services.PortAllocator) error {
			return fmt.Errorf("mock assign IP addresses error")
		}
		serviceList := []services.Service{mockService}

		// When assigning IPs
		err := manager.AssignIPs(serviceList)

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}

		// And the error should contain the expected message
		expectedErrorSubstring := "error assigning IP addresses"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}
		_ = mocks // suppress unused variable warning
	})

}

func TestNetworkManager_ConfigureGuest(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
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
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
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
		mocks.Services[0].SetAddressFunc = func(address string, portAllocator *services.PortAllocator) error {
			setAddressCalls = append(setAddressCalls, address)
			return nil
		}
		mocks.Services[1].SetAddressFunc = func(address string, portAllocator *services.PortAllocator) error {
			setAddressCalls = append(setAddressCalls, address)
			return nil
		}
		networkCIDR := "10.5.0.0/16"

		// When assigning IP addresses
		err := assignIPAddresses(toServices(mocks.Services), &networkCIDR, services.NewPortAllocator())

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
		err := assignIPAddresses(toServices(mocks.Services), &networkCIDR, services.NewPortAllocator())

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
		mocks.Services[0].SetAddressFunc = func(address string, portAllocator *services.PortAllocator) error {
			return fmt.Errorf("error setting address")
		}
		networkCIDR := "10.5.0.0/16"

		// When assigning IP addresses
		err := assignIPAddresses(toServices(mocks.Services[:1]), &networkCIDR, services.NewPortAllocator())

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
		err := assignIPAddresses(toServices(mocks.Services), &networkCIDR, services.NewPortAllocator())

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
		err := assignIPAddresses(toServices(mocks.Services[:1]), networkCIDR, services.NewPortAllocator())

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
