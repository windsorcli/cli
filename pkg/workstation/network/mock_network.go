package network

import (
	"net"
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
	ConfigureHostRouteFunc       func() error
	ConfigureGuestFunc           func() error
	ConfigureDNSFunc             func() error
	RevertHostRouteFunc          func() error
	RevertGuestFunc              func() error
	RevertDNSFunc                func() error
	FlushDNSFunc                 func() error
	NeedsPrivilegeForClusterFunc func() bool
	NeedsPrivilegeForDNSFunc     func() bool
	IsHostRouteInstalledFunc     func() bool
	IsResolverInstalledFunc      func() bool
	DNSChangedFunc               func() bool
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

// RevertHostRoute calls the custom RevertHostRouteFunc if provided.
func (m *MockNetworkManager) RevertHostRoute() error {
	if m.RevertHostRouteFunc != nil {
		return m.RevertHostRouteFunc()
	}
	return nil
}

// RevertGuest calls the custom RevertGuestFunc if provided.
func (m *MockNetworkManager) RevertGuest() error {
	if m.RevertGuestFunc != nil {
		return m.RevertGuestFunc()
	}
	return nil
}

// RevertDNS calls the custom RevertDNSFunc if provided.
func (m *MockNetworkManager) RevertDNS() error {
	if m.RevertDNSFunc != nil {
		return m.RevertDNSFunc()
	}
	return nil
}

// FlushDNS calls the custom FlushDNSFunc if provided.
func (m *MockNetworkManager) FlushDNS() error {
	if m.FlushDNSFunc != nil {
		return m.FlushDNSFunc()
	}
	return nil
}

// NeedsPrivilegeForCluster calls the custom NeedsPrivilegeForClusterFunc if provided.
func (m *MockNetworkManager) NeedsPrivilegeForCluster() bool {
	if m.NeedsPrivilegeForClusterFunc != nil {
		return m.NeedsPrivilegeForClusterFunc()
	}
	return false
}

// NeedsPrivilegeForDNS calls the custom NeedsPrivilegeForDNSFunc if provided.
func (m *MockNetworkManager) NeedsPrivilegeForDNS() bool {
	if m.NeedsPrivilegeForDNSFunc != nil {
		return m.NeedsPrivilegeForDNSFunc()
	}
	return false
}

// IsHostRouteInstalled calls the custom IsHostRouteInstalledFunc if provided.
func (m *MockNetworkManager) IsHostRouteInstalled() bool {
	if m.IsHostRouteInstalledFunc != nil {
		return m.IsHostRouteInstalledFunc()
	}
	return false
}

// IsResolverInstalled calls the custom IsResolverInstalledFunc if provided.
func (m *MockNetworkManager) IsResolverInstalled() bool {
	if m.IsResolverInstalledFunc != nil {
		return m.IsResolverInstalledFunc()
	}
	return false
}

// DNSChanged calls the custom DNSChangedFunc if provided.
func (m *MockNetworkManager) DNSChanged() bool {
	if m.DNSChangedFunc != nil {
		return m.DNSChangedFunc()
	}
	return false
}

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

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockNetworkInterfaceProvider implements the NetworkInterfaceProvider interface
var _ NetworkInterfaceProvider = (*MockNetworkInterfaceProvider)(nil)

// Ensure MockNetworkManager implements the NetworkManager interface
var _ NetworkManager = (*MockNetworkManager)(nil)
