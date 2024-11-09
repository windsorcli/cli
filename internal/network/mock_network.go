package network

// MockNetworkManager is a struct that simulates a network manager for testing purposes.
type MockNetworkManager struct {
	NetworkManager
	ConfigureHostFunc  func() error
	ConfigureGuestFunc func() error
}

// NewMockNetworkManager creates a new instance of MockNetworkManager.
func NewMockNetworkManager() *MockNetworkManager {
	return &MockNetworkManager{}
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

// Ensure MockNetworkManager implements the NetworkManager interface
var _ NetworkManager = (*MockNetworkManager)(nil)
