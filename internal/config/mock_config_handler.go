package config

// MockConfigHandler is a mock implementation of the ConfigHandler interface
type MockConfigHandler struct {
	LoadConfigFunc func(path string) error
	GetStringFunc  func(key string, defaultValue ...string) (string, error)
	GetIntFunc     func(key string, defaultValue ...int) (int, error)
	GetBoolFunc    func(key string, defaultValue ...bool) (bool, error)
	SetFunc        func(key string, value interface{}) error
	SaveConfigFunc func(path string) error
	GetFunc        func(key string) (interface{}, error)
	SetDefaultFunc func(context Context) error
	GetConfigFunc  func() (*Context, error)
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
		return m.GetStringFunc(key, defaultValue...)
	}
	if len(defaultValue) > 0 {
		return defaultValue[0], nil
	}
	return "", nil
}

// GetInt calls the mock GetIntFunc if set, otherwise returns 0 and nil error
func (m *MockConfigHandler) GetInt(key string, defaultValue ...int) (int, error) {
	if m.GetIntFunc != nil {
		return m.GetIntFunc(key, defaultValue...)
	}
	if len(defaultValue) > 0 {
		return defaultValue[0], nil
	}
	return 0, nil
}

// GetBool calls the mock GetBoolFunc if set, otherwise returns false and nil error
func (m *MockConfigHandler) GetBool(key string, defaultValue ...bool) (bool, error) {
	if m.GetBoolFunc != nil {
		return m.GetBoolFunc(key, defaultValue...)
	}
	if len(defaultValue) > 0 {
		return defaultValue[0], nil
	}
	return false, nil
}

// Set calls the mock SetFunc if set, otherwise returns nil
func (m *MockConfigHandler) Set(key string, value interface{}) error {
	if m.SetFunc != nil {
		return m.SetFunc(key, value)
	}
	return nil
}

// Get calls the mock GetFunc if set, otherwise returns nil and nil error
func (m *MockConfigHandler) Get(key string) (interface{}, error) {
	if m.GetFunc != nil {
		return m.GetFunc(key)
	}
	return nil, nil
}

// SaveConfig calls the mock SaveConfigFunc if set, otherwise returns nil
func (m *MockConfigHandler) SaveConfig(path string) error {
	if m.SaveConfigFunc != nil {
		return m.SaveConfigFunc(path)
	}
	return nil
}

// SetDefault calls the mock SetDefaultFunc if set, otherwise does nothing
func (m *MockConfigHandler) SetDefault(context Context) error {
	if m.SetDefaultFunc != nil {
		return m.SetDefaultFunc(context)
	}
	return nil
}

// GetConfig calls the mock GetConfigFunc if set, otherwise returns nil and nil error
func (m *MockConfigHandler) GetConfig() (*Context, error) {
	if m.GetConfigFunc != nil {
		return m.GetConfigFunc()
	}
	return nil, nil
}

// Ensure MockConfigHandler implements ConfigHandler
var _ ConfigHandler = (*MockConfigHandler)(nil)
