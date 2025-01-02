package blueprint

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
)

func TestMockBlueprintHandler_Initialize(t *testing.T) {
	t.Run("Initialize", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		handler.InitializeFunc = func() error {
			return nil
		}
		err := handler.Initialize()
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("NoInitializeFunc", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		err := handler.Initialize()
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockBlueprintHandler_LoadConfig(t *testing.T) {
	mockLoadErr := fmt.Errorf("mock load config error")

	t.Run("WithPath", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		handler.LoadConfigFunc = func(path ...string) error {
			return mockLoadErr
		}
		err := handler.LoadConfig("some/path")
		if err != mockLoadErr {
			t.Errorf("Expected error = %v, got = %v", mockLoadErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		err := handler.LoadConfig("some/path")
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockBlueprintHandler_GetMetadata(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		expectedMetadata := MetadataV1Alpha1{}
		handler.GetMetadataFunc = func() MetadataV1Alpha1 {
			return expectedMetadata
		}
		metadata := handler.GetMetadata()
		if !reflect.DeepEqual(metadata, expectedMetadata) {
			t.Errorf("Expected metadata = %v, got = %v", expectedMetadata, metadata)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		metadata := handler.GetMetadata()
		if !reflect.DeepEqual(metadata, MetadataV1Alpha1{}) {
			t.Errorf("Expected metadata = %v, got = %v", MetadataV1Alpha1{}, metadata)
		}
	})
}

func TestMockBlueprintHandler_GetSources(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		expectedSources := []SourceV1Alpha1{}
		handler.GetSourcesFunc = func() []SourceV1Alpha1 {
			return expectedSources
		}
		sources := handler.GetSources()
		if !reflect.DeepEqual(sources, expectedSources) {
			t.Errorf("Expected sources = %v, got = %v", expectedSources, sources)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		sources := handler.GetSources()
		if !reflect.DeepEqual(sources, []SourceV1Alpha1{}) {
			t.Errorf("Expected sources = %v, got = %v", []SourceV1Alpha1{}, sources)
		}
	})
}

func TestMockBlueprintHandler_GetTerraformComponents(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		expectedComponents := []TerraformComponentV1Alpha1{}
		handler.GetTerraformComponentsFunc = func() []TerraformComponentV1Alpha1 {
			return expectedComponents
		}
		components := handler.GetTerraformComponents()
		if !reflect.DeepEqual(components, expectedComponents) {
			t.Errorf("Expected components = %v, got = %v", expectedComponents, components)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		components := handler.GetTerraformComponents()
		if !reflect.DeepEqual(components, []TerraformComponentV1Alpha1{}) {
			t.Errorf("Expected components = %v, got = %v", []TerraformComponentV1Alpha1{}, components)
		}
	})
}

func TestMockBlueprintHandler_GetKustomizations(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		expectedKustomizations := []KustomizationV1Alpha1{}
		handler.GetKustomizationsFunc = func() []KustomizationV1Alpha1 {
			return expectedKustomizations
		}
		kustomizations := handler.GetKustomizations()
		if !reflect.DeepEqual(kustomizations, expectedKustomizations) {
			t.Errorf("Expected kustomizations = %v, got = %v", expectedKustomizations, kustomizations)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		kustomizations := handler.GetKustomizations()
		if !reflect.DeepEqual(kustomizations, []KustomizationV1Alpha1{}) {
			t.Errorf("Expected kustomizations = %v, got = %v", []KustomizationV1Alpha1{}, kustomizations)
		}
	})
}

func TestMockBlueprintHandler_SetMetadata(t *testing.T) {
	mockSetErr := fmt.Errorf("mock set metadata error")

	t.Run("WithFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		handler.SetMetadataFunc = func(metadata MetadataV1Alpha1) error {
			return mockSetErr
		}
		err := handler.SetMetadata(MetadataV1Alpha1{})
		if err != mockSetErr {
			t.Errorf("Expected error = %v, got = %v", mockSetErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		err := handler.SetMetadata(MetadataV1Alpha1{})
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockBlueprintHandler_SetSources(t *testing.T) {
	mockSetErr := fmt.Errorf("mock set sources error")

	t.Run("WithFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		handler.SetSourcesFunc = func(sources []SourceV1Alpha1) error {
			return mockSetErr
		}
		err := handler.SetSources([]SourceV1Alpha1{})
		if err != mockSetErr {
			t.Errorf("Expected error = %v, got = %v", mockSetErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		err := handler.SetSources([]SourceV1Alpha1{})
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockBlueprintHandler_SetTerraformComponents(t *testing.T) {
	mockSetErr := fmt.Errorf("mock set terraform components error")

	t.Run("WithFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		handler.SetTerraformComponentsFunc = func(components []TerraformComponentV1Alpha1) error {
			return mockSetErr
		}
		err := handler.SetTerraformComponents([]TerraformComponentV1Alpha1{})
		if err != mockSetErr {
			t.Errorf("Expected error = %v, got = %v", mockSetErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		err := handler.SetTerraformComponents([]TerraformComponentV1Alpha1{})
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockBlueprintHandler_SetKustomizations(t *testing.T) {
	mockSetErr := fmt.Errorf("mock set kustomizations error")

	t.Run("WithFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		handler.SetKustomizationsFunc = func(kustomizations []KustomizationV1Alpha1) error {
			return mockSetErr
		}
		err := handler.SetKustomizations([]KustomizationV1Alpha1{})
		if err != mockSetErr {
			t.Errorf("Expected error = %v, got = %v", mockSetErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		err := handler.SetKustomizations([]KustomizationV1Alpha1{})
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockBlueprintHandler_WriteConfig(t *testing.T) {
	mockWriteErr := fmt.Errorf("mock write config error")

	t.Run("WithFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		handler.WriteConfigFunc = func(path ...string) error {
			return mockWriteErr
		}
		err := handler.WriteConfig("some/path")
		if err != mockWriteErr {
			t.Errorf("Expected error = %v, got = %v", mockWriteErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		err := handler.WriteConfig("some/path")
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockBlueprintHandler_Install(t *testing.T) {
	mockInstallErr := fmt.Errorf("mock install error")

	t.Run("WithFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		handler.InstallFunc = func() error {
			return mockInstallErr
		}
		err := handler.Install()
		if err != mockInstallErr {
			t.Errorf("Expected error = %v, got = %v", mockInstallErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		err := handler.Install()
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}
