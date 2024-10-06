package config

// MockConfigHandler is a mock implementation of the ConfigHandler interface
type MockConfigHandler struct {
	LoadConfigFunc     func(path string) error
	GetConfigValueFunc func(key string) (string, error)
	SetConfigValueFunc func(key string, value interface{}) error
	SaveConfigFunc     func(path string) error
	GetNestedMapFunc   func(key string) (map[string]interface{}, error)
	ListKeysFunc       func(key string) ([]string, error)
}

// NewMockConfigHandler is a constructor for MockConfigHandler
func NewMockConfigHandler(
	loadConfigFunc func(path string) error,
	getConfigValueFunc func(key string) (string, error),
	setConfigValueFunc func(key string, value interface{}) error,
	saveConfigFunc func(path string) error,
	getNestedMapFunc func(key string) (map[string]interface{}, error),
	listKeysFunc func(key string) ([]string, error),
) *MockConfigHandler {
	return &MockConfigHandler{
		LoadConfigFunc:     loadConfigFunc,
		GetConfigValueFunc: getConfigValueFunc,
		SetConfigValueFunc: setConfigValueFunc,
		SaveConfigFunc:     saveConfigFunc,
		GetNestedMapFunc:   getNestedMapFunc,
		ListKeysFunc:       listKeysFunc,
	}
}

// LoadConfig calls the mock LoadConfigFunc if set, otherwise returns nil
func (m *MockConfigHandler) LoadConfig(path string) error {
	if m.LoadConfigFunc != nil {
		return m.LoadConfigFunc(path)
	}
	return nil
}

// GetConfigValue calls the mock GetConfigValueFunc if set, otherwise returns an empty string and nil error
func (m *MockConfigHandler) GetConfigValue(key string) (string, error) {
	if m.GetConfigValueFunc != nil {
		return m.GetConfigValueFunc(key)
	}
	return "", nil
}

// SetConfigValue calls the mock SetConfigValueFunc if set, otherwise returns nil
func (m *MockConfigHandler) SetConfigValue(key string, value interface{}) error {
	if m.SetConfigValueFunc != nil {
		return m.SetConfigValueFunc(key, value)
	}
	return nil
}

// SaveConfig calls the mock SaveConfigFunc if set, otherwise returns nil
func (m *MockConfigHandler) SaveConfig(path string) error {
	if m.SaveConfigFunc != nil {
		return m.SaveConfigFunc(path)
	}
	return nil
}

// GetNestedMap calls the mock GetNestedMapFunc if set, otherwise returns nil and nil error
func (m *MockConfigHandler) GetNestedMap(key string) (map[string]interface{}, error) {
	if m.GetNestedMapFunc != nil {
		return m.GetNestedMapFunc(key)
	}
	return nil, nil
}

// ListKeys calls the mock ListKeysFunc if set, otherwise returns nil and nil error
func (m *MockConfigHandler) ListKeys(key string) ([]string, error) {
	if m.ListKeysFunc != nil {
		return m.ListKeysFunc(key)
	}
	return nil, nil
}

// Ensure MockConfigHandler implements ConfigHandler
var _ ConfigHandler = (*MockConfigHandler)(nil)
