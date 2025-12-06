package tools

// MockToolsManager is a mock implementation of the ToolsManager interface for testing purposes.
type MockToolsManager struct {
	WriteManifestFunc func() error
	InstallFunc       func() error
	CheckFunc         func() error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockToolsManager creates a new instance of MockToolsManager.
func NewMockToolsManager() *MockToolsManager {
	return &MockToolsManager{}
}

// =============================================================================
// Public Methods
// =============================================================================

// WriteManifest calls the mock WriteManifestFunc if set, otherwise returns nil.
func (m *MockToolsManager) WriteManifest() error {
	if m.WriteManifestFunc != nil {
		return m.WriteManifestFunc()
	}
	return nil
}

// InstallTools calls the mock InstallToolsFunc if set, otherwise returns nil.
func (m *MockToolsManager) Install() error {
	if m.InstallFunc != nil {
		return m.InstallFunc()
	}
	return nil
}

// Check calls the mock CheckFunc if set, otherwise returns nil.
func (m *MockToolsManager) Check() error {
	if m.CheckFunc != nil {
		return m.CheckFunc()
	}
	return nil
}

// Ensure MockToolsManager implements ToolsManager.
var _ ToolsManager = (*MockToolsManager)(nil)
