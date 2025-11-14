package network

import (
	"net"

	"github.com/windsorcli/cli/pkg/workstation/services"
)

// The MockNetworkManager is a test implementation of the NetworkManager interface.
// It provides mock implementations of network management functions for testing,
// The MockNetworkManager enables controlled testing of network-dependent code,
// allowing verification of network operations without actual system modifications.

// =============================================================================
// Types
// =============================================================================

// MockNetworkManager is a struct that simulates a network manager for testing purposes.
type MockNetworkManager struct {
	NetworkManager
	InitializeFunc         func([]services.Service) error
	ConfigureHostRouteFunc func() error
	ConfigureGuestFunc     func() error
	ConfigureDNSFunc       func() error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockNetworkManager creates a new instance of MockNetworkManager.
func NewMockNetworkManager() *MockNetworkManager {
	return &MockNetworkManager{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize calls the custom InitializeFunc if provided.
func (m *MockNetworkManager) Initialize(services []services.Service) error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc(services)
	}
	return nil
}

// ConfigureHostRoute calls the custom ConfigureHostRouteFunc if provided.
func (m *MockNetworkManager) ConfigureHostRoute() error {
	if m.ConfigureHostRouteFunc != nil {
		return m.ConfigureHostRouteFunc()
	}
	return nil
}

// ConfigureGuest calls the custom ConfigureGuestFunc if provided.
func (m *MockNetworkManager) ConfigureGuest() error {
	if m.ConfigureGuestFunc != nil {
		return m.ConfigureGuestFunc()
	}
	return nil
}

// ConfigureDNS calls the custom ConfigureDNSFunc if provided.
func (m *MockNetworkManager) ConfigureDNS() error {
	if m.ConfigureDNSFunc != nil {
		return m.ConfigureDNSFunc()
	}
	return nil
}

// Ensure MockNetworkManager implements the NetworkManager interface
var _ NetworkManager = (*MockNetworkManager)(nil)

// The MockNetworkInterfaceProvider is a test implementation of the NetworkInterfaceProvider interface.
// It provides mock implementations of network interface operations for testing,
// The MockNetworkInterfaceProvider enables controlled testing of network interface-dependent code,
// allowing verification of interface operations without actual system access.

// =============================================================================
// Types
// =============================================================================

// MockNetworkInterfaceProvider is a struct that simulates a network interface provider for testing purposes.
type MockNetworkInterfaceProvider struct {
	InterfacesFunc     func() ([]net.Interface, error)
	InterfaceAddrsFunc func(iface net.Interface) ([]net.Addr, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockNetworkInterfaceProvider creates a new instance of MockNetworkInterfaceProvider with default implementations.
func NewMockNetworkInterfaceProvider() *MockNetworkInterfaceProvider {
	return &MockNetworkInterfaceProvider{
		InterfacesFunc: func() ([]net.Interface, error) {
			return []net.Interface{}, nil
		},
		InterfaceAddrsFunc: func(iface net.Interface) ([]net.Addr, error) {
			return []net.Addr{}, nil
		},
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Interfaces calls the custom InterfacesFunc if provided.
func (m *MockNetworkInterfaceProvider) Interfaces() ([]net.Interface, error) {
	return m.InterfacesFunc()
}

// InterfaceAddrs calls the custom InterfaceAddrsFunc if provided.
func (m *MockNetworkInterfaceProvider) InterfaceAddrs(iface net.Interface) ([]net.Addr, error) {
	return m.InterfaceAddrsFunc(iface)
}

// Ensure MockNetworkInterfaceProvider implements the NetworkInterfaceProvider interface
var _ NetworkInterfaceProvider = (*MockNetworkInterfaceProvider)(nil)
