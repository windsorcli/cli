package blueprint

import (
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// MockBlueprintHandler is a mock implementation of BlueprintHandler interface for testing.
type MockBlueprintHandler struct {
	LoadBlueprintFunc          func(...string) error
	WriteFunc                  func(overwrite ...bool) error
	GetTerraformComponentsFunc func() []blueprintv1alpha1.TerraformComponent
	GetLocalTemplateDataFunc   func() (map[string][]byte, error)
	GenerateFunc               func() *blueprintv1alpha1.Blueprint
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockBlueprintHandler creates a new instance of MockBlueprintHandler.
func NewMockBlueprintHandler() *MockBlueprintHandler {
	return &MockBlueprintHandler{}
}

// =============================================================================
// Public Methods
// =============================================================================

// LoadBlueprint calls the mock LoadBlueprintFunc if set, otherwise returns nil.
func (m *MockBlueprintHandler) LoadBlueprint(blueprintURL ...string) error {
	if m.LoadBlueprintFunc != nil {
		return m.LoadBlueprintFunc(blueprintURL...)
	}
	return nil
}

// Write calls the mock WriteFunc if set, otherwise returns nil.
func (m *MockBlueprintHandler) Write(overwrite ...bool) error {
	if m.WriteFunc != nil {
		return m.WriteFunc(overwrite...)
	}
	return nil
}

// GetTerraformComponents calls the mock GetTerraformComponentsFunc if set, otherwise returns empty slice.
func (m *MockBlueprintHandler) GetTerraformComponents() []blueprintv1alpha1.TerraformComponent {
	if m.GetTerraformComponentsFunc != nil {
		return m.GetTerraformComponentsFunc()
	}
	return []blueprintv1alpha1.TerraformComponent{}
}

// GetLocalTemplateData calls the mock GetLocalTemplateDataFunc if set, otherwise returns empty map.
func (m *MockBlueprintHandler) GetLocalTemplateData() (map[string][]byte, error) {
	if m.GetLocalTemplateDataFunc != nil {
		return m.GetLocalTemplateDataFunc()
	}
	return map[string][]byte{}, nil
}

// Generate calls the mock GenerateFunc if set, otherwise returns nil.
func (m *MockBlueprintHandler) Generate() *blueprintv1alpha1.Blueprint {
	if m.GenerateFunc != nil {
		return m.GenerateFunc()
	}
	return nil
}

var _ BlueprintHandler = (*MockBlueprintHandler)(nil)
