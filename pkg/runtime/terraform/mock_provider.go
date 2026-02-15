package terraform

import (
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// MockTerraformProvider is a mock implementation of TerraformProvider for testing.
type MockTerraformProvider struct {
	FindRelativeProjectPathFunc func(directory ...string) (string, error)
	IsInTerraformProjectFunc    func() bool
	GenerateBackendOverrideFunc func(directory string) error
	GenerateTerraformArgsFunc   func(componentID, modulePath string, interactive bool) (*TerraformArgs, error)
	GetTerraformComponentFunc   func(componentID string) *blueprintv1alpha1.TerraformComponent
	GetTerraformComponentsFunc  func() []blueprintv1alpha1.TerraformComponent
	SetTerraformComponentsFunc  func(components []blueprintv1alpha1.TerraformComponent)
	SetConfigScopeFunc          func(scope map[string]any)
	GetTerraformOutputsFunc     func(componentID string) (map[string]any, error)
	CacheOutputsFunc            func(componentID string) error
	GetTFDataDirFunc            func(componentID string) (string, error)
	GetEnvVarsFunc              func(componentID string, interactive bool) (map[string]string, *TerraformArgs, error)
	FormatArgsForEnvFunc        func(args []string) string
	ClearCacheFunc              func()
}

// FindRelativeProjectPath implements TerraformProvider.
func (m *MockTerraformProvider) FindRelativeProjectPath(directory ...string) (string, error) {
	if m.FindRelativeProjectPathFunc != nil {
		return m.FindRelativeProjectPathFunc(directory...)
	}
	return "", nil
}

// IsInTerraformProject implements TerraformProvider.
func (m *MockTerraformProvider) IsInTerraformProject() bool {
	if m.IsInTerraformProjectFunc != nil {
		return m.IsInTerraformProjectFunc()
	}
	return false
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

// SetTerraformComponents implements TerraformProvider.
func (m *MockTerraformProvider) SetTerraformComponents(components []blueprintv1alpha1.TerraformComponent) {
	if m.SetTerraformComponentsFunc != nil {
		m.SetTerraformComponentsFunc(components)
	}
}

// SetConfigScope implements TerraformProvider.
func (m *MockTerraformProvider) SetConfigScope(scope map[string]any) {
	if m.SetConfigScopeFunc != nil {
		m.SetConfigScopeFunc(scope)
	}
}

// GetTerraformOutputs implements TerraformProvider.
func (m *MockTerraformProvider) GetTerraformOutputs(componentID string) (map[string]any, error) {
	if m.GetTerraformOutputsFunc != nil {
		return m.GetTerraformOutputsFunc(componentID)
	}
	return make(map[string]any), nil
}

// CacheOutputs implements TerraformProvider.
func (m *MockTerraformProvider) CacheOutputs(componentID string) error {
	if m.CacheOutputsFunc != nil {
		return m.CacheOutputsFunc(componentID)
	}
	return nil
}

// GetTFDataDir implements TerraformProvider.
func (m *MockTerraformProvider) GetTFDataDir(componentID string) (string, error) {
	if m.GetTFDataDirFunc != nil {
		return m.GetTFDataDirFunc(componentID)
	}
	return "", nil
}

// GetEnvVars implements TerraformProvider.
func (m *MockTerraformProvider) GetEnvVars(componentID string, interactive bool) (map[string]string, *TerraformArgs, error) {
	if m.GetEnvVarsFunc != nil {
		return m.GetEnvVarsFunc(componentID, interactive)
	}
	return make(map[string]string), &TerraformArgs{}, nil
}

// FormatArgsForEnv implements TerraformProvider.
func (m *MockTerraformProvider) FormatArgsForEnv(args []string) string {
	if m.FormatArgsForEnvFunc != nil {
		return m.FormatArgsForEnvFunc(args)
	}
	return ""
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
