package config

// MockConfigHandler is a mock implementation of the ConfigHandler interface
type MockConfigHandler struct {
	LoadConfigErr     error
	GetConfigValueErr error
	SetConfigValueErr error
	SaveConfigErr     error
}

func (m *MockConfigHandler) LoadConfig(path string) error {
	return m.LoadConfigErr
}

func (m *MockConfigHandler) GetConfigValue(key string) (string, error) {
	return "", m.GetConfigValueErr
}

func (m *MockConfigHandler) SetConfigValue(key, value string) error {
	return m.SetConfigValueErr
}

func (m *MockConfigHandler) SaveConfig(path string) error {
	return m.SaveConfigErr
}

// Ensure MockConfigHandler implements ConfigHandler
var _ ConfigHandler = (*MockConfigHandler)(nil)
