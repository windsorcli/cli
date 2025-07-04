package blueprint

import (
	"fmt"
	"reflect"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMockBlueprintHandler_Initialize(t *testing.T) {
	setup := func(t *testing.T) *MockBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		return handler
	}

	t.Run("Initialize", func(t *testing.T) {
		// Given a mock handler with initialize function
		handler := setup(t)
		handler.InitializeFunc = func() error {
			return nil
		}
		// When initializing
		err := handler.Initialize()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("NoInitializeFunc", func(t *testing.T) {
		// Given a mock handler without initialize function
		handler := setup(t)
		// When initializing
		err := handler.Initialize()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockBlueprintHandler_LoadConfig(t *testing.T) {
	setup := func(t *testing.T) *MockBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		return handler
	}

	mockLoadErr := fmt.Errorf("mock load config error")

	t.Run("WithReset", func(t *testing.T) {
		// Given a mock handler with load config function
		handler := setup(t)
		handler.LoadConfigFunc = func(reset ...bool) error {
			return mockLoadErr
		}
		// When loading config with reset
		err := handler.LoadConfig(true)
		// Then expected error should be returned
		if err != mockLoadErr {
			t.Errorf("Expected error = %v, got = %v", mockLoadErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock handler without load config function
		handler := setup(t)
		// When loading config
		err := handler.LoadConfig()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockBlueprintHandler_GetMetadata(t *testing.T) {
	setup := func(t *testing.T) *MockBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		return handler
	}

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock handler with get metadata function
		handler := setup(t)
		expectedMetadata := blueprintv1alpha1.Metadata{}
		handler.GetMetadataFunc = func() blueprintv1alpha1.Metadata {
			return expectedMetadata
		}
		// When getting metadata
		metadata := handler.GetMetadata()
		// Then expected metadata should be returned
		if !reflect.DeepEqual(metadata, expectedMetadata) {
			t.Errorf("Expected metadata = %v, got = %v", expectedMetadata, metadata)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock handler without get metadata function
		handler := setup(t)
		// When getting metadata
		metadata := handler.GetMetadata()
		// Then empty metadata should be returned
		if !reflect.DeepEqual(metadata, blueprintv1alpha1.Metadata{}) {
			t.Errorf("Expected metadata = %v, got = %v", blueprintv1alpha1.Metadata{}, metadata)
		}
	})
}

func TestMockBlueprintHandler_GetSources(t *testing.T) {
	setup := func(t *testing.T) *MockBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		return handler
	}

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock handler with get sources function
		handler := setup(t)
		expectedSources := []blueprintv1alpha1.Source{}
		handler.GetSourcesFunc = func() []blueprintv1alpha1.Source {
			return expectedSources
		}
		// When getting sources
		sources := handler.GetSources()
		// Then expected sources should be returned
		if !reflect.DeepEqual(sources, expectedSources) {
			t.Errorf("Expected sources = %v, got = %v", expectedSources, sources)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock handler without get sources function
		handler := setup(t)
		// When getting sources
		sources := handler.GetSources()
		// Then empty sources should be returned
		if !reflect.DeepEqual(sources, []blueprintv1alpha1.Source{}) {
			t.Errorf("Expected sources = %v, got = %v", []blueprintv1alpha1.Source{}, sources)
		}
	})
}

func TestMockBlueprintHandler_GetTerraformComponents(t *testing.T) {
	setup := func(t *testing.T) *MockBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		return handler
	}

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock handler with get terraform components function
		handler := setup(t)
		expectedComponents := []blueprintv1alpha1.TerraformComponent{}
		handler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return expectedComponents
		}
		// When getting terraform components
		components := handler.GetTerraformComponents()
		// Then expected components should be returned
		if !reflect.DeepEqual(components, expectedComponents) {
			t.Errorf("Expected components = %v, got = %v", expectedComponents, components)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock handler without get terraform components function
		handler := setup(t)
		// When getting terraform components
		components := handler.GetTerraformComponents()
		// Then empty components should be returned
		if !reflect.DeepEqual(components, []blueprintv1alpha1.TerraformComponent{}) {
			t.Errorf("Expected components = %v, got = %v", []blueprintv1alpha1.TerraformComponent{}, components)
		}
	})
}

func TestMockBlueprintHandler_GetKustomizations(t *testing.T) {
	setup := func(t *testing.T) *MockBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		return handler
	}

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock handler with get kustomizations function
		handler := setup(t)
		expectedKustomizations := []blueprintv1alpha1.Kustomization{}
		handler.GetKustomizationsFunc = func() []blueprintv1alpha1.Kustomization {
			return expectedKustomizations
		}
		// When getting kustomizations
		kustomizations := handler.GetKustomizations()
		// Then expected kustomizations should be returned
		if !reflect.DeepEqual(kustomizations, expectedKustomizations) {
			t.Errorf("Expected kustomizations = %v, got = %v", expectedKustomizations, kustomizations)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock handler without get kustomizations function
		handler := setup(t)
		// When getting kustomizations
		kustomizations := handler.GetKustomizations()
		// Then empty kustomizations should be returned
		if !reflect.DeepEqual(kustomizations, []blueprintv1alpha1.Kustomization{}) {
			t.Errorf("Expected kustomizations = %v, got = %v", []blueprintv1alpha1.Kustomization{}, kustomizations)
		}
	})
}

func TestMockBlueprintHandler_Install(t *testing.T) {
	setup := func(t *testing.T) *MockBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		return handler
	}

	mockInstallErr := fmt.Errorf("mock install error")

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock handler with install function
		handler := setup(t)
		handler.InstallFunc = func() error {
			return mockInstallErr
		}
		// When installing
		err := handler.Install()
		// Then expected error should be returned
		if err != mockInstallErr {
			t.Errorf("Expected error = %v, got = %v", mockInstallErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock handler without install function
		handler := setup(t)
		// When installing
		err := handler.Install()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockBlueprintHandler_GetRepository(t *testing.T) {
	setup := func(t *testing.T) *MockBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		return handler
	}

	t.Run("DefaultBehavior", func(t *testing.T) {
		// Given a mock handler without get repository function
		handler := setup(t)
		// When getting repository
		repo := handler.GetRepository()
		// Then empty repository should be returned
		if repo != (blueprintv1alpha1.Repository{}) {
			t.Errorf("Expected empty Repository, got %+v", repo)
		}
	})

	t.Run("WithMockFunction", func(t *testing.T) {
		// Given a mock handler with get repository function
		handler := setup(t)
		expected := blueprintv1alpha1.Repository{
			Url: "test-url",
			Ref: blueprintv1alpha1.Reference{Branch: "main"},
		}
		handler.GetRepositoryFunc = func() blueprintv1alpha1.Repository {
			return expected
		}
		// When getting repository
		repo := handler.GetRepository()
		// Then expected repository should be returned
		if repo != expected {
			t.Errorf("Expected %+v, got %+v", expected, repo)
		}
	})
}

func TestMockBlueprintHandler_WaitForKustomizations(t *testing.T) {
	setup := func(t *testing.T) *MockBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		return handler
	}

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock handler with wait for kustomizations function
		handler := setup(t)
		message := "test message"
		names := []string{"kustomization1", "kustomization2"}
		expectedErr := fmt.Errorf("mock error")
		handler.WaitForKustomizationsFunc = func(msg string, nms ...string) error {
			if msg != message || len(nms) != len(names) {
				t.Errorf("Expected message %s and names %v, got %s and %v", message, names, msg, nms)
			}
			return expectedErr
		}
		// When waiting for kustomizations
		err := handler.WaitForKustomizations(message, names...)
		// Then expected error should be returned
		if err != expectedErr {
			t.Errorf("Expected error = %v, got = %v", expectedErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock handler without wait for kustomizations function
		handler := setup(t)
		message := "test message"
		names := []string{"kustomization1", "kustomization2"}
		// When waiting for kustomizations
		err := handler.WaitForKustomizations(message, names...)
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}
