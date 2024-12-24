package blueprint

import "github.com/windsorcli/cli/pkg/di"

// MockBlueprintHandler is a mock implementation of the BlueprintHandler interface for testing purposes
type MockBlueprintHandler struct {
	InitializeFunc             func() error
	LoadConfigFunc             func(path ...string) error
	GetMetadataFunc            func() MetadataV1Alpha1
	GetSourcesFunc             func() []SourceV1Alpha1
	GetTerraformComponentsFunc func() []TerraformComponentV1Alpha1
	SetMetadataFunc            func(metadata MetadataV1Alpha1) error
	SetSourcesFunc             func(sources []SourceV1Alpha1) error
	SetTerraformComponentsFunc func(terraformComponents []TerraformComponentV1Alpha1) error
	WriteConfigFunc            func(path ...string) error
}

// NewMockBlueprintHandler creates a new instance of MockBlueprintHandler
func NewMockBlueprintHandler(injector di.Injector) *MockBlueprintHandler {
	return &MockBlueprintHandler{}
}

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

// GetMetadata calls the mock GetMetadataFunc if set, otherwise returns a reasonable default MetadataV1Alpha1
func (m *MockBlueprintHandler) GetMetadata() MetadataV1Alpha1 {
	if m.GetMetadataFunc != nil {
		return m.GetMetadataFunc()
	}
	return MetadataV1Alpha1{}
}

// GetSources calls the mock GetSourcesFunc if set, otherwise returns a reasonable default slice of SourceV1Alpha1
func (m *MockBlueprintHandler) GetSources() []SourceV1Alpha1 {
	if m.GetSourcesFunc != nil {
		return m.GetSourcesFunc()
	}
	return []SourceV1Alpha1{}
}

// GetTerraformComponents calls the mock GetTerraformComponentsFunc if set, otherwise returns a reasonable default slice of TerraformComponentV1Alpha1
func (m *MockBlueprintHandler) GetTerraformComponents() []TerraformComponentV1Alpha1 {
	if m.GetTerraformComponentsFunc != nil {
		return m.GetTerraformComponentsFunc()
	}
	return []TerraformComponentV1Alpha1{}
}

// SetMetadata calls the mock SetMetadataFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) SetMetadata(metadata MetadataV1Alpha1) error {
	if m.SetMetadataFunc != nil {
		return m.SetMetadataFunc(metadata)
	}
	return nil
}

// SetSources calls the mock SetSourcesFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) SetSources(sources []SourceV1Alpha1) error {
	if m.SetSourcesFunc != nil {
		return m.SetSourcesFunc(sources)
	}
	return nil
}

// SetTerraformComponents calls the mock SetTerraformComponentsFunc if set, otherwise returns nil
func (m *MockBlueprintHandler) SetTerraformComponents(terraformComponents []TerraformComponentV1Alpha1) error {
	if m.SetTerraformComponentsFunc != nil {
		return m.SetTerraformComponentsFunc(terraformComponents)
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

// Ensure MockBlueprintHandler implements BlueprintHandler
var _ BlueprintHandler = (*MockBlueprintHandler)(nil)
