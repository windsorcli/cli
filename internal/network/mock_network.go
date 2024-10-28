package network

// MockNetworkManager is a mock implementation of the NetworkManager interface
type MockNetworkManager struct {
	// ConfigureFunc is a function that mocks the Configure method
	ConfigureFunc func(*NetworkConfig) (*NetworkConfig, error)
}

// NewMockNetworkManager is a constructor for MockNetworkManager
func NewMockNetworkManager() *MockNetworkManager {
	return &MockNetworkManager{}
}

// Configure calls the mock ConfigureFunc if it is set, otherwise returns nil
func (m *MockNetworkManager) Configure(networkConfig *NetworkConfig) (*NetworkConfig, error) {
	if m.ConfigureFunc != nil {
		return m.ConfigureFunc(networkConfig)
	}
	return networkConfig, nil
}

// SetConfigureFunc sets the ConfigureFunc for the mock network manager
func (m *MockNetworkManager) SetConfigureFunc(configureFunc func(*NetworkConfig) (*NetworkConfig, error)) {
	m.ConfigureFunc = configureFunc
}

// Ensure MockNetworkManager implements NetworkManager interface
var _ NetworkManager = (*MockNetworkManager)(nil)
