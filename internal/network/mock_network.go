package network

import "net"

// MockNetworkManager is a struct that simulates a network manager for testing purposes.
type MockNetworkManager struct {
	NetworkManager
	InitializeFunc     func() error
	ConfigureHostFunc  func() error
	ConfigureGuestFunc func() error
	ConfigureDNSFunc   func() error
}

// NewMockNetworkManager creates a new instance of MockNetworkManager.
func NewMockNetworkManager() *MockNetworkManager {
	return &MockNetworkManager{}
}

// Initialize calls the custom InitializeFunc if provided.
func (m *MockNetworkManager) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

// ConfigureHost calls the custom ConfigureHostFunc if provided.
func (m *MockNetworkManager) ConfigureHost() error {
	if m.ConfigureHostFunc != nil {
		return m.ConfigureHostFunc()
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

// MockNetworkInterfaceProvider is a struct that simulates a network interface provider for testing purposes.
type MockNetworkInterfaceProvider struct {
	InterfacesFunc     func() ([]net.Interface, error)
	InterfaceAddrsFunc func(iface net.Interface) ([]net.Addr, error)
}

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
