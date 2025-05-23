package blueprint

import (
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
)

// MockBlueprintHandler is a mock implementation of BlueprintHandler interface for testing
type MockBlueprintHandler struct {
	InitializeFunc             func() error
	LoadConfigFunc             func(path ...string) error
	GetMetadataFunc            func() blueprintv1alpha1.Metadata
	GetSourcesFunc             func() []blueprintv1alpha1.Source
	GetTerraformComponentsFunc func() []blueprintv1alpha1.TerraformComponent
	GetKustomizationsFunc      func() []blueprintv1alpha1.Kustomization
	SetMetadataFunc            func(metadata blueprintv1alpha1.Metadata) error
	SetSourcesFunc             func(sources []blueprintv1alpha1.Source) error
	SetTerraformComponentsFunc func(terraformComponents []blueprintv1alpha1.TerraformComponent) error
	SetKustomizationsFunc      func(kustomizations []blueprintv1alpha1.Kustomization) error
	WriteConfigFunc            func(path ...string) error
	InstallFunc                func() error
	GetRepositoryFunc          func() blueprintv1alpha1.Repository
	SetRepositoryFunc          func(repository blueprintv1alpha1.Repository) error
	WaitForKustomizationsFunc  func(message string, names ...string) error
	DownFunc                   func() error
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
func (m *MockBlueprintHandler) LoadConfig(path ...string) error {
	if m.LoadConfigFunc != nil {
		return m.LoadConfigFunc(path...)
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

// SetMetadata calls the mock SetMetadataFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) SetMetadata(metadata blueprintv1alpha1.Metadata) error {
	if m.SetMetadataFunc != nil {
		return m.SetMetadataFunc(metadata)
	}
	return nil
}

// SetSources calls the mock SetSourcesFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) SetSources(sources []blueprintv1alpha1.Source) error {
	if m.SetSourcesFunc != nil {
		return m.SetSourcesFunc(sources)
	}
	return nil
}

// SetTerraformComponents calls the mock SetTerraformComponentsFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) SetTerraformComponents(terraformComponents []blueprintv1alpha1.TerraformComponent) error {
	if m.SetTerraformComponentsFunc != nil {
		return m.SetTerraformComponentsFunc(terraformComponents)
	}
	return nil
}

// SetKustomizations calls the mock SetKustomizationsFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) SetKustomizations(kustomizations []blueprintv1alpha1.Kustomization) error {
	if m.SetKustomizationsFunc != nil {
		return m.SetKustomizationsFunc(kustomizations)
	}
	return nil
}

// WriteConfig calls the mock WriteConfigFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) WriteConfig(path ...string) error {
	if m.WriteConfigFunc != nil {
		return m.WriteConfigFunc(path...)
	}
	return nil
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

// SetRepository calls the mock SetRepositoryFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) SetRepository(repository blueprintv1alpha1.Repository) error {
	if m.SetRepositoryFunc != nil {
		return m.SetRepositoryFunc(repository)
	}
	return nil
}

// WaitForKustomizations mocks the WaitForKustomizations method.
func (m *MockBlueprintHandler) WaitForKustomizations(message string, names ...string) error {
	if m.WaitForKustomizationsFunc != nil {
		return m.WaitForKustomizationsFunc(message, names...)
	}
	return nil
}

// Down mocks the Down method.
func (m *MockBlueprintHandler) Down() error {
	if m.DownFunc != nil {
		return m.DownFunc()
	}
	return nil
}

// Ensure MockBlueprintHandler implements BlueprintHandler
var _ BlueprintHandler = (*MockBlueprintHandler)(nil)
