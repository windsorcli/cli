package terraform

import (
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// MockTerraformProvider is a mock implementation of TerraformProvider for testing.
type MockTerraformProvider struct {
	FindRelativeProjectPathFunc func(directory ...string) (string, error)
	GenerateBackendOverrideFunc func(directory string) error
	GetTerraformComponentFunc   func(componentID string) *blueprintv1alpha1.TerraformComponent
	GetTerraformComponentsFunc  func() []blueprintv1alpha1.TerraformComponent
	GetOutputsFunc              func(componentID string) (map[string]any, error)
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

// GetOutputs implements TerraformProvider.
func (m *MockTerraformProvider) GetOutputs(componentID string) (map[string]any, error) {
	if m.GetOutputsFunc != nil {
		return m.GetOutputsFunc(componentID)
	}
	return map[string]any{}, nil
}

// ClearCache implements TerraformProvider.
func (m *MockTerraformProvider) ClearCache() {
	if m.ClearCacheFunc != nil {
		m.ClearCacheFunc()
	}
}
