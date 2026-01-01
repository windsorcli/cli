package config

import (
	"fmt"
	"strings"

	"github.com/windsorcli/cli/api/v1alpha1"
)

// MockConfigHandler is a mock implementation of the ConfigHandler interface
type MockConfigHandler struct {
	LoadConfigFunc            func() error
	LoadConfigStringFunc      func(content string) error
	IsLoadedFunc              func() bool
	GetStringFunc             func(key string, defaultValue ...string) string
	GetIntFunc                func(key string, defaultValue ...int) int
	GetBoolFunc               func(key string, defaultValue ...bool) bool
	GetStringSliceFunc        func(key string, defaultValue ...[]string) []string
	GetStringMapFunc          func(key string, defaultValue ...map[string]string) map[string]string
	SetFunc                   func(key string, value any) error
	SaveConfigFunc            func(overwrite ...bool) error
	GetFunc                   func(key string) any
	SetDefaultFunc            func(context v1alpha1.Context) error
	GetConfigFunc             func() *v1alpha1.Context
	GetContextFunc            func() string
	IsDevModeFunc             func(contextName string) bool
	SetContextFunc            func(context string) error
	GetConfigRootFunc         func() (string, error)
	GetWindsorScratchPathFunc func() (string, error)
	CleanFunc                 func() error
	GenerateContextIDFunc     func() error
	LoadSchemaFunc            func(schemaPath string) error
	LoadSchemaFromBytesFunc   func(schemaContent []byte) error
	GetContextValuesFunc      func() (map[string]any, error)
	RegisterProviderFunc      func(prefix string, provider ValueProvider)
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockConfigHandler is a constructor for MockConfigHandler
func NewMockConfigHandler() *MockConfigHandler {
	return &MockConfigHandler{}
}

// =============================================================================
// Public Methods
// =============================================================================

// LoadConfig calls the mock LoadConfigFunc if set, otherwise returns nil
func (m *MockConfigHandler) LoadConfig() error {
	if m.LoadConfigFunc != nil {
		return m.LoadConfigFunc()
	}
	return nil
}

// LoadConfigString calls the mock LoadConfigStringFunc if set, otherwise returns nil
func (m *MockConfigHandler) LoadConfigString(content string) error {
	if m.LoadConfigStringFunc != nil {
		return m.LoadConfigStringFunc(content)
	}
	return nil
}

// IsLoaded calls the mock IsLoadedFunc if set, otherwise returns false
func (m *MockConfigHandler) IsLoaded() bool {
	if m.IsLoadedFunc != nil {
		return m.IsLoadedFunc()
	}
	return false
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

// GetStringSlice calls the mock GetStringSliceFunc if set, otherwise returns a reasonable default slice of strings
func (m *MockConfigHandler) GetStringSlice(key string, defaultValue ...[]string) []string {
	if m.GetStringSliceFunc != nil {
		return m.GetStringSliceFunc(key, defaultValue...)
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return []string{}
}

// GetStringMap calls the mock GetStringMapFunc if set, otherwise returns a reasonable default map of strings
func (m *MockConfigHandler) GetStringMap(key string, defaultValue ...map[string]string) map[string]string {
	if m.GetStringMapFunc != nil {
		return m.GetStringMapFunc(key, defaultValue...)
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return map[string]string{}
}

// Set calls the mock SetFunc if set, otherwise returns nil
func (m *MockConfigHandler) Set(key string, value any) error {
	if m.SetFunc != nil {
		return m.SetFunc(key, value)
	}
	return nil
}

// Get calls the mock GetFunc if set, otherwise returns a reasonable default value
func (m *MockConfigHandler) Get(key string) any {
	if m.GetFunc != nil {
		return m.GetFunc(key)
	}
	return "mock-value"
}

// SaveConfig calls the SaveConfigFunc if set, otherwise returns nil
func (m *MockConfigHandler) SaveConfig(overwrite ...bool) error {
	if m.SaveConfigFunc != nil {
		return m.SaveConfigFunc(overwrite...)
	}
	return nil
}

// SetDefault calls the mock SetDefaultFunc if set, otherwise does nothing
func (m *MockConfigHandler) SetDefault(context v1alpha1.Context) error {
	if m.SetDefaultFunc != nil {
		return m.SetDefaultFunc(context)
	}
	return nil
}

// GetConfig calls the mock GetConfigFunc if set, otherwise returns a reasonable default Context
func (m *MockConfigHandler) GetConfig() *v1alpha1.Context {
	if m.GetConfigFunc != nil {
		return m.GetConfigFunc()
	}
	return &v1alpha1.Context{}
}

// GetContext calls the mock GetContextFunc if set, otherwise returns a reasonable default string
func (m *MockConfigHandler) GetContext() string {
	if m.GetContextFunc != nil {
		return m.GetContextFunc()
	}
	return "mock-context"
}

// IsDevMode calls the mock IsDevModeFunc if set, otherwise returns default dev mode logic
func (m *MockConfigHandler) IsDevMode(contextName string) bool {
	if m.IsDevModeFunc != nil {
		return m.IsDevModeFunc(contextName)
	}
	return contextName == "local" || strings.HasPrefix(contextName, "local-")
}

// SetContext calls the mock SetContextFunc if set, otherwise returns nil
func (m *MockConfigHandler) SetContext(context string) error {
	if m.SetContextFunc != nil {
		return m.SetContextFunc(context)
	}
	return nil
}

// GetConfigRoot calls the mock GetConfigRootFunc if set, otherwise returns a reasonable default string
func (m *MockConfigHandler) GetConfigRoot() (string, error) {
	if m.GetConfigRootFunc != nil {
		return m.GetConfigRootFunc()
	}
	return "mock-config-root", nil
}

// GetWindsorScratchPath calls the mock GetWindsorScratchPathFunc if set, otherwise returns a reasonable default string
func (m *MockConfigHandler) GetWindsorScratchPath() (string, error) {
	if m.GetWindsorScratchPathFunc != nil {
		return m.GetWindsorScratchPathFunc()
	}
	return "mock-windsor-scratch-path", nil
}

// Clean calls the mock CleanFunc if set, otherwise returns nil
func (m *MockConfigHandler) Clean() error {
	if m.CleanFunc != nil {
		return m.CleanFunc()
	}
	return nil
}

// GenerateContextID calls the mock GenerateContextIDFunc if set, otherwise returns nil
func (m *MockConfigHandler) GenerateContextID() error {
	if m.GenerateContextIDFunc != nil {
		return m.GenerateContextIDFunc()
	}
	return nil
}

// LoadSchema calls the mock LoadSchemaFunc if set, otherwise returns an error
func (m *MockConfigHandler) LoadSchema(schemaPath string) error {
	if m.LoadSchemaFunc != nil {
		return m.LoadSchemaFunc(schemaPath)
	}
	return fmt.Errorf("LoadSchemaFunc not set")
}

// LoadSchemaFromBytes calls the mock LoadSchemaFromBytesFunc if set, otherwise returns an error
func (m *MockConfigHandler) LoadSchemaFromBytes(schemaContent []byte) error {
	if m.LoadSchemaFromBytesFunc != nil {
		return m.LoadSchemaFromBytesFunc(schemaContent)
	}
	return fmt.Errorf("LoadSchemaFromBytesFunc not set")
}

// GetContextValues calls the mock GetContextValuesFunc if set, otherwise returns an error
func (m *MockConfigHandler) GetContextValues() (map[string]any, error) {
	if m.GetContextValuesFunc != nil {
		return m.GetContextValuesFunc()
	}
	return nil, fmt.Errorf("GetContextValuesFunc not set")
}

// RegisterProvider calls the mock RegisterProviderFunc if set, otherwise does nothing
func (m *MockConfigHandler) RegisterProvider(prefix string, provider ValueProvider) {
	if m.RegisterProviderFunc != nil {
		m.RegisterProviderFunc(prefix, provider)
	}
}

// Ensure MockConfigHandler implements ConfigHandler
var _ ConfigHandler = (*MockConfigHandler)(nil)
