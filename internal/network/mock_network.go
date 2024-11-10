package network

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
