package blueprint

import (
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// MockBlueprintHandler is a mock implementation of BlueprintHandler interface for testing.
type MockBlueprintHandler struct {
	LoadBlueprintFunc          func(...string) error
	SetSkipValidationFunc      func(skip bool)
	WriteFunc                  func(overwrite ...bool) error
	GetTerraformComponentsFunc func() []blueprintv1alpha1.TerraformComponent
	GetLocalTemplateDataFunc   func() (map[string][]byte, error)
	GenerateFunc               func() *blueprintv1alpha1.Blueprint
	GenerateResolvedFunc       func() (*blueprintv1alpha1.Blueprint, error)
	ExplainFunc                func(string) (*ExplainTrace, error)
	GetDeferredPathsFunc       func() map[string]bool
	skipValidation             bool
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

// SetSkipValidation calls the mock SetSkipValidationFunc if set, otherwise records the
// flag on the mock so tests can assert prepareProject toggled it.
func (m *MockBlueprintHandler) SetSkipValidation(skip bool) {
	if m.SetSkipValidationFunc != nil {
		m.SetSkipValidationFunc(skip)
		return
	}
	m.skipValidation = skip
}

// SkipValidation reports the recorded skip flag (only meaningful when SetSkipValidationFunc
// is not set). Test-only accessor.
func (m *MockBlueprintHandler) SkipValidation() bool {
	return m.skipValidation
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

// GenerateResolved calls the mock GenerateResolvedFunc if set, otherwise falls back to Generate.
func (m *MockBlueprintHandler) GenerateResolved() (*blueprintv1alpha1.Blueprint, error) {
	if m.GenerateResolvedFunc != nil {
		return m.GenerateResolvedFunc()
	}
	return m.Generate(), nil
}

// Explain calls the mock ExplainFunc if set, otherwise returns nil.
func (m *MockBlueprintHandler) Explain(path string) (*ExplainTrace, error) {
	if m.ExplainFunc != nil {
		return m.ExplainFunc(path)
	}
	return nil, nil
}

// GetDeferredPaths calls the mock GetDeferredPathsFunc if set, otherwise returns nil.
func (m *MockBlueprintHandler) GetDeferredPaths() map[string]bool {
	if m.GetDeferredPathsFunc != nil {
		return m.GetDeferredPathsFunc()
	}
	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ BlueprintHandler = (*MockBlueprintHandler)(nil)
