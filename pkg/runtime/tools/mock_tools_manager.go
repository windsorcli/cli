package tools

// MockToolsManager is a mock implementation of the ToolsManager interface for testing purposes.
type MockToolsManager struct {
	WriteManifestFunc       func() error
	InstallFunc             func() error
	CheckFunc               func() error
	GetTerraformCommandFunc func() string
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

// GetTerraformCommand calls the mock GetTerraformCommandFunc if set, otherwise returns "terraform"
func (m *MockToolsManager) GetTerraformCommand() string {
	if m.GetTerraformCommandFunc != nil {
		return m.GetTerraformCommandFunc()
	}
	return "terraform"
}

// Ensure MockToolsManager implements ToolsManager.
var _ ToolsManager = (*MockToolsManager)(nil)
