package config

// MockConfigHandler is a mock implementation of the ConfigHandler interface
type MockConfigHandler struct {
	LoadConfigFunc   func(path string) error
	GetStringFunc    func(key string) (string, error)
	GetIntFunc       func(key string) (int, error)
	GetBoolFunc      func(key string) (bool, error)
	SetValueFunc     func(key string, value interface{}) error
	SaveConfigFunc   func(path string) error
	GetNestedMapFunc func(key string) (map[string]interface{}, error)
	ListKeysFunc     func(key string) ([]string, error)
}

// NewMockConfigHandler is a constructor for MockConfigHandler
func NewMockConfigHandler() *MockConfigHandler {
	return &MockConfigHandler{}
}

// LoadConfig calls the mock LoadConfigFunc if set, otherwise returns nil
func (m *MockConfigHandler) LoadConfig(path string) error {
	if m.LoadConfigFunc != nil {
		return m.LoadConfigFunc(path)
	}
	return nil
}

// GetString calls the mock GetStringFunc if set, otherwise returns an empty string and nil error
func (m *MockConfigHandler) GetString(key string, defaultValue ...string) (string, error) {
	if m.GetStringFunc != nil {
		return m.GetStringFunc(key)
	}
	if len(defaultValue) > 0 {
		return defaultValue[0], nil
	}
	return "", nil
}

// GetInt calls the mock GetIntFunc if set, otherwise returns 0 and nil error
func (m *MockConfigHandler) GetInt(key string, defaultValue ...int) (int, error) {
	if m.GetIntFunc != nil {
		return m.GetIntFunc(key)
	}
	if len(defaultValue) > 0 {
		return defaultValue[0], nil
	}
	return 0, nil
}

// GetBool calls the mock GetBoolFunc if set, otherwise returns false and nil error
func (m *MockConfigHandler) GetBool(key string, defaultValue ...bool) (bool, error) {
	if m.GetBoolFunc != nil {
		return m.GetBoolFunc(key)
	}
	if len(defaultValue) > 0 {
		return defaultValue[0], nil
	}
	return false, nil
}

// SetValue calls the mock SetValueFunc if set, otherwise returns nil
func (m *MockConfigHandler) SetValue(key string, value interface{}) error {
	if m.SetValueFunc != nil {
		return m.SetValueFunc(key, value)
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

// Ensure MockConfigHandler implements ConfigHandler
var _ ConfigHandler = (*MockConfigHandler)(nil)
