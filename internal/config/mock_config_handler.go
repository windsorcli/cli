package config

// MockConfigHandler is a mock implementation of the ConfigHandler interface
type MockConfigHandler struct {
	LoadConfigFunc func(path string) error
	GetContextFunc func() *string
	SetContextFunc func(context string) error
	GetStringFunc  func(key string, defaultValue ...string) string
	GetIntFunc     func(key string, defaultValue ...int) int
	GetBoolFunc    func(key string, defaultValue ...bool) bool
	SetFunc        func(key string, value interface{}) error
	SaveConfigFunc func(path string) error
	GetFunc        func(key string) (interface{}, error)
	SetDefaultFunc func(context Context) error
	GetConfigFunc  func() *Context
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

// GetContext calls the mock GetContextFunc if set, otherwise returns nil
func (m *MockConfigHandler) GetContext() *string {
	if m.GetContextFunc != nil {
		return m.GetContextFunc()
	}
	return nil
}

// SetContext calls the mock SetContextFunc if set, otherwise returns nil
func (m *MockConfigHandler) SetContext(context string) error {
	if m.SetContextFunc != nil {
		return m.SetContextFunc(context)
	}
	return nil
}

// GetString calls the mock GetStringFunc if set, otherwise returns a reasonable default string
func (m *MockConfigHandler) GetString(key string, defaultValue ...string) string {
	if m.GetStringFunc != nil {
		return m.GetStringFunc(key, defaultValue...)
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return "mock-string"
}

// GetInt calls the mock GetIntFunc if set, otherwise returns a reasonable default int
func (m *MockConfigHandler) GetInt(key string, defaultValue ...int) int {
	if m.GetIntFunc != nil {
		return m.GetIntFunc(key, defaultValue...)
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return 42
}

// GetBool calls the mock GetBoolFunc if set, otherwise returns a reasonable default bool
func (m *MockConfigHandler) GetBool(key string, defaultValue ...bool) bool {
	if m.GetBoolFunc != nil {
		return m.GetBoolFunc(key, defaultValue...)
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return true
}

// Set calls the mock SetFunc if set, otherwise returns nil
func (m *MockConfigHandler) Set(key string, value interface{}) error {
	if m.SetFunc != nil {
		return m.SetFunc(key, value)
	}
	return nil
}

// Get calls the mock GetFunc if set, otherwise returns a reasonable default value and nil error
func (m *MockConfigHandler) Get(key string) (interface{}, error) {
	if m.GetFunc != nil {
		return m.GetFunc(key)
	}
	return "mock-value", nil
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

// GetConfig calls the mock GetConfigFunc if set, otherwise returns a reasonable default Context
func (m *MockConfigHandler) GetConfig() *Context {
	if m.GetConfigFunc != nil {
		return m.GetConfigFunc()
	}
	return &Context{}
}

// Ensure MockConfigHandler implements ConfigHandler
var _ ConfigHandler = (*MockConfigHandler)(nil)
