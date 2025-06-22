package blueprint

import (
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
)

// MockBlueprintHandler is a mock implementation of BlueprintHandler interface for testing
type MockBlueprintHandler struct {
	InitializeFunc             func() error
	LoadConfigFunc             func(reset ...bool) error
	GetMetadataFunc            func() blueprintv1alpha1.Metadata
	GetSourcesFunc             func() []blueprintv1alpha1.Source
	GetTerraformComponentsFunc func() []blueprintv1alpha1.TerraformComponent
	GetKustomizationsFunc      func() []blueprintv1alpha1.Kustomization

	WaitForKustomizationsFunc   func(message string, names ...string) error
	ProcessContextTemplatesFunc func(contextName string, reset ...bool) error
	InstallFunc                 func() error
	GetRepositoryFunc           func() blueprintv1alpha1.Repository

	DownFunc func() error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockBlueprintHandler creates a new instance of MockBlueprintHandler
func NewMockBlueprintHandler(injector di.Injector) *MockBlueprintHandler {
	return &MockBlueprintHandler{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize initializes the blueprint handler
func (m *MockBlueprintHandler) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

// LoadConfig calls the mock LoadConfigFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) LoadConfig(reset ...bool) error {
	if m.LoadConfigFunc != nil {
		return m.LoadConfigFunc(reset...)
	}
	return nil
}

// GetMetadata calls the mock GetMetadataFunc if set, otherwise returns a reasonable default
// MetadataV1Alpha1
func (m *MockBlueprintHandler) GetMetadata() blueprintv1alpha1.Metadata {
	if m.GetMetadataFunc != nil {
		return m.GetMetadataFunc()
	}
	return blueprintv1alpha1.Metadata{}
}

// GetSources calls the mock GetSourcesFunc if set, otherwise returns a reasonable default
// slice of SourceV1Alpha1
func (m *MockBlueprintHandler) GetSources() []blueprintv1alpha1.Source {
	if m.GetSourcesFunc != nil {
		return m.GetSourcesFunc()
	}
	return []blueprintv1alpha1.Source{}
}

// GetTerraformComponents calls the mock GetTerraformComponentsFunc if set, otherwise returns a
// reasonable default slice of TerraformComponentV1Alpha1
func (m *MockBlueprintHandler) GetTerraformComponents() []blueprintv1alpha1.TerraformComponent {
	if m.GetTerraformComponentsFunc != nil {
		return m.GetTerraformComponentsFunc()
	}
	return []blueprintv1alpha1.TerraformComponent{}
}

// GetKustomizations calls the mock GetKustomizationsFunc if set, otherwise returns a reasonable
// default slice of kustomizev1.Kustomization
func (m *MockBlueprintHandler) GetKustomizations() []blueprintv1alpha1.Kustomization {
	if m.GetKustomizationsFunc != nil {
		return m.GetKustomizationsFunc()
	}
	return []blueprintv1alpha1.Kustomization{}
}

// Install calls the mock InstallFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) Install() error {
	if m.InstallFunc != nil {
		return m.InstallFunc()
	}
	return nil
}

// GetRepository calls the mock GetRepositoryFunc if set, otherwise returns empty Repository
func (m *MockBlueprintHandler) GetRepository() blueprintv1alpha1.Repository {
	if m.GetRepositoryFunc != nil {
		return m.GetRepositoryFunc()
	}
	return blueprintv1alpha1.Repository{}
}

// Down mocks the Down method.
func (m *MockBlueprintHandler) Down() error {
	if m.DownFunc != nil {
		return m.DownFunc()
	}
	return nil
}

// WaitForKustomizations calls the mock WaitForKustomizationsFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) WaitForKustomizations(message string, names ...string) error {
	if m.WaitForKustomizationsFunc != nil {
		return m.WaitForKustomizationsFunc(message, names...)
	}
	return nil
}

// ProcessContextTemplates calls the mock ProcessContextTemplatesFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) ProcessContextTemplates(contextName string, reset ...bool) error {
	if m.ProcessContextTemplatesFunc != nil {
		return m.ProcessContextTemplatesFunc(contextName, reset...)
	}
	return nil
}

// Ensure MockBlueprintHandler implements BlueprintHandler
var _ BlueprintHandler = (*MockBlueprintHandler)(nil)
