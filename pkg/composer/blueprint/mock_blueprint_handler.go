package blueprint

import (
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/composer/artifact"
)

// MockBlueprintHandler is a mock implementation of BlueprintHandler interface for testing
type MockBlueprintHandler struct {
	InitializeFunc             func() error
	LoadBlueprintFunc          func() error
	LoadConfigFunc             func() error
	LoadDataFunc               func(data map[string]any, ociInfo ...*artifact.OCIArtifactInfo) error
	WriteFunc                  func(overwrite ...bool) error
	GetMetadataFunc            func() blueprintv1alpha1.Metadata
	GetSourcesFunc             func() []blueprintv1alpha1.Source
	GetTerraformComponentsFunc func() []blueprintv1alpha1.TerraformComponent
	GetKustomizationsFunc      func() []blueprintv1alpha1.Kustomization

	WaitForKustomizationsFunc  func(message string, names ...string) error
	GetDefaultTemplateDataFunc func(contextName string) (map[string][]byte, error)
	GetLocalTemplateDataFunc   func() (map[string][]byte, error)
	InstallFunc                func() error
	GetRepositoryFunc          func() blueprintv1alpha1.Repository

	DownFunc                     func() error
	SetRenderedKustomizeDataFunc func(data map[string]any)
	GenerateFunc                 func() *blueprintv1alpha1.Blueprint
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

// LoadBlueprint calls the mock LoadBlueprintFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) LoadBlueprint() error {
	if m.LoadBlueprintFunc != nil {
		return m.LoadBlueprintFunc()
	}
	return nil
}

// LoadConfig calls the mock LoadConfigFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) LoadConfig() error {
	if m.LoadConfigFunc != nil {
		return m.LoadConfigFunc()
	}
	return nil
}

// LoadData calls the mock LoadDataFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) LoadData(data map[string]any, ociInfo ...*artifact.OCIArtifactInfo) error {
	if m.LoadDataFunc != nil {
		return m.LoadDataFunc(data, ociInfo...)
	}
	return nil
}

// Write calls the mock WriteFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) Write(overwrite ...bool) error {
	if m.WriteFunc != nil {
		return m.WriteFunc(overwrite...)
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

// SetRenderedKustomizeData implements BlueprintHandler interface
func (m *MockBlueprintHandler) SetRenderedKustomizeData(data map[string]any) {
	if m.SetRenderedKustomizeDataFunc != nil {
		m.SetRenderedKustomizeDataFunc(data)
	}
}

// WaitForKustomizations calls the mock WaitForKustomizationsFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) WaitForKustomizations(message string, names ...string) error {
	if m.WaitForKustomizationsFunc != nil {
		return m.WaitForKustomizationsFunc(message, names...)
	}
	return nil
}

// GetDefaultTemplateData calls the mock GetDefaultTemplateDataFunc if set, otherwise returns empty map
func (m *MockBlueprintHandler) GetDefaultTemplateData(contextName string) (map[string][]byte, error) {
	if m.GetDefaultTemplateDataFunc != nil {
		return m.GetDefaultTemplateDataFunc(contextName)
	}
	return map[string][]byte{}, nil
}

// GetLocalTemplateData calls the mock GetLocalTemplateDataFunc if set, otherwise returns empty map
func (m *MockBlueprintHandler) GetLocalTemplateData() (map[string][]byte, error) {
	if m.GetLocalTemplateDataFunc != nil {
		return m.GetLocalTemplateDataFunc()
	}
	return map[string][]byte{}, nil
}

// Generate calls the mock GenerateFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) Generate() *blueprintv1alpha1.Blueprint {
	if m.GenerateFunc != nil {
		return m.GenerateFunc()
	}
	return nil
}

// Ensure MockBlueprintHandler implements BlueprintHandler
var _ BlueprintHandler = (*MockBlueprintHandler)(nil)
