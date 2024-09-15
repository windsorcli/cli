package config

// MockConfigHandler is a mock implementation of the ConfigHandler interface
type MockConfigHandler struct {
	LoadConfigFunc     func(path string) error
	GetConfigValueFunc func(key string) (string, error)
	SetConfigValueFunc func(key, value string) error
	SaveConfigFunc     func(path string) error
}

func (m *MockConfigHandler) LoadConfig(path string) error {
	if m.LoadConfigFunc != nil {
		return m.LoadConfigFunc(path)
	}
	return nil
}

func (m *MockConfigHandler) GetConfigValue(key string) (string, error) {
	if m.GetConfigValueFunc != nil {
		return m.GetConfigValueFunc(key)
	}
	return "", nil
}

func (m *MockConfigHandler) SetConfigValue(key, value string) error {
	if m.SetConfigValueFunc != nil {
		return m.SetConfigValueFunc(key, value)
	}
	return nil
}

func (m *MockConfigHandler) SaveConfig(path string) error {
	if m.SaveConfigFunc != nil {
		return m.SaveConfigFunc(path)
	}
	return nil
}

// Ensure MockConfigHandler implements ConfigHandler
var _ ConfigHandler = (*MockConfigHandler)(nil)
