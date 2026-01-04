package terraform

import (
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// MockTerraformProvider is a mock implementation of TerraformProvider for testing.
type MockTerraformProvider struct {
	FindRelativeProjectPathFunc func(directory ...string) (string, error)
	GenerateBackendOverrideFunc func(directory string) error
	GenerateTerraformArgsFunc   func(componentID, modulePath string, interactive bool) (*TerraformArgs, error)
	GetTerraformComponentFunc   func(componentID string) *blueprintv1alpha1.TerraformComponent
	GetTerraformComponentsFunc  func() []blueprintv1alpha1.TerraformComponent
	GetTFDataDirFunc            func(componentID string) (string, error)
	ClearCacheFunc              func()
}

// FindRelativeProjectPath implements TerraformProvider.
func (m *MockTerraformProvider) FindRelativeProjectPath(directory ...string) (string, error) {
	if m.FindRelativeProjectPathFunc != nil {
		return m.FindRelativeProjectPathFunc(directory...)
	}
	return "", nil
}

// GenerateBackendOverride implements TerraformProvider.
func (m *MockTerraformProvider) GenerateBackendOverride(directory string) error {
	if m.GenerateBackendOverrideFunc != nil {
		return m.GenerateBackendOverrideFunc(directory)
	}
	return nil
}

// GenerateTerraformArgs implements TerraformProvider.
func (m *MockTerraformProvider) GenerateTerraformArgs(componentID, modulePath string, interactive bool) (*TerraformArgs, error) {
	if m.GenerateTerraformArgsFunc != nil {
		return m.GenerateTerraformArgsFunc(componentID, modulePath, interactive)
	}
	return &TerraformArgs{}, nil
}

// GetTerraformComponent implements TerraformProvider.
func (m *MockTerraformProvider) GetTerraformComponent(componentID string) *blueprintv1alpha1.TerraformComponent {
	if m.GetTerraformComponentFunc != nil {
		return m.GetTerraformComponentFunc(componentID)
	}
	return nil
}

// GetTerraformComponents implements TerraformProvider.
func (m *MockTerraformProvider) GetTerraformComponents() []blueprintv1alpha1.TerraformComponent {
	if m.GetTerraformComponentsFunc != nil {
		return m.GetTerraformComponentsFunc()
	}
	return []blueprintv1alpha1.TerraformComponent{}
}

// GetTFDataDir implements TerraformProvider.
func (m *MockTerraformProvider) GetTFDataDir(componentID string) (string, error) {
	if m.GetTFDataDirFunc != nil {
		return m.GetTFDataDirFunc(componentID)
	}
	return "", nil
}

// ClearCache implements TerraformProvider.
func (m *MockTerraformProvider) ClearCache() {
	if m.ClearCacheFunc != nil {
		m.ClearCacheFunc()
	}
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockTerraformProvider implements the TerraformProvider interface
var _ TerraformProvider = (*MockTerraformProvider)(nil)
