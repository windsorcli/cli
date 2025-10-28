package blueprint

import (
	"fmt"
	"reflect"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/resources/artifact"
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

	t.Run("WithError", func(t *testing.T) {
		// Given a mock handler with load config function
		handler := setup(t)
		handler.LoadConfigFunc = func() error {
			return mockLoadErr
		}
		// When loading config
		err := handler.LoadConfig()
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

func TestMockBlueprintHandler_GetDefaultTemplateData(t *testing.T) {
	setup := func(t *testing.T) *MockBlueprintHandler {
		t.Helper()
		return &MockBlueprintHandler{}
	}

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock handler with GetDefaultTemplateData function
		handler := setup(t)
		expectedData := map[string][]byte{
			"template1": []byte("template content 1"),
			"template2": []byte("template content 2"),
		}
		handler.GetDefaultTemplateDataFunc = func(contextName string) (map[string][]byte, error) {
			return expectedData, nil
		}
		// When getting default template data
		data, err := handler.GetDefaultTemplateData("test-context")
		// Then expected data should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
		if len(data) != len(expectedData) {
			t.Errorf("Expected data length = %v, got = %v", len(expectedData), len(data))
		}
		for key, expectedValue := range expectedData {
			if value, exists := data[key]; !exists || string(value) != string(expectedValue) {
				t.Errorf("Expected data[%s] = %s, got = %s", key, string(expectedValue), string(value))
			}
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock handler with no GetDefaultTemplateData function
		handler := setup(t)
		// When getting default template data
		data, err := handler.GetDefaultTemplateData("test-context")
		// Then empty map and no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
		if data == nil || len(data) != 0 {
			t.Errorf("Expected empty map, got = %v", data)
		}
	})

	t.Run("WithError", func(t *testing.T) {
		// Given a mock handler with GetDefaultTemplateData function that returns error
		handler := setup(t)
		mockErr := fmt.Errorf("mock error")
		handler.GetDefaultTemplateDataFunc = func(contextName string) (map[string][]byte, error) {
			return nil, mockErr
		}
		// When getting default template data
		data, err := handler.GetDefaultTemplateData("test-context")
		// Then expected error should be returned
		if err != mockErr {
			t.Errorf("Expected error = %v, got = %v", mockErr, err)
		}
		if data != nil {
			t.Errorf("Expected data = %v, got = %v", nil, data)
		}
	})
}

func TestMockBlueprintHandler_GetLocalTemplateData(t *testing.T) {
	setup := func(t *testing.T) *MockBlueprintHandler {
		t.Helper()
		return &MockBlueprintHandler{}
	}

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock handler with GetLocalTemplateData function
		handler := setup(t)
		expectedData := map[string][]byte{
			"local1": []byte("local content 1"),
			"local2": []byte("local content 2"),
		}
		handler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return expectedData, nil
		}
		// When getting local template data
		data, err := handler.GetLocalTemplateData()
		// Then expected data should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
		if len(data) != len(expectedData) {
			t.Errorf("Expected data length = %v, got = %v", len(expectedData), len(data))
		}
		for key, expectedValue := range expectedData {
			if value, exists := data[key]; !exists || string(value) != string(expectedValue) {
				t.Errorf("Expected data[%s] = %s, got = %s", key, string(expectedValue), string(value))
			}
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock handler with no GetLocalTemplateData function
		handler := setup(t)
		// When getting local template data
		data, err := handler.GetLocalTemplateData()
		// Then empty map and no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
		if data == nil || len(data) != 0 {
			t.Errorf("Expected empty map, got = %v", data)
		}
	})

	t.Run("WithError", func(t *testing.T) {
		// Given a mock handler with GetLocalTemplateData function that returns error
		handler := setup(t)
		mockErr := fmt.Errorf("mock error")
		handler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return nil, mockErr
		}
		// When getting local template data
		data, err := handler.GetLocalTemplateData()
		// Then expected error should be returned
		if err != mockErr {
			t.Errorf("Expected error = %v, got = %v", mockErr, err)
		}
		if data != nil {
			t.Errorf("Expected data = %v, got = %v", nil, data)
		}
	})
}

func TestMockBlueprintHandler_Down(t *testing.T) {
	setup := func(t *testing.T) *MockBlueprintHandler {
		t.Helper()
		return &MockBlueprintHandler{}
	}

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock handler with Down function
		handler := setup(t)
		handler.DownFunc = func() error {
			return nil
		}
		// When calling down
		err := handler.Down()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock handler with no Down function
		handler := setup(t)
		// When calling down
		err := handler.Down()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("WithError", func(t *testing.T) {
		// Given a mock handler with Down function that returns error
		handler := setup(t)
		mockErr := fmt.Errorf("mock error")
		handler.DownFunc = func() error {
			return mockErr
		}
		// When calling down
		err := handler.Down()
		// Then expected error should be returned
		if err != mockErr {
			t.Errorf("Expected error = %v, got = %v", mockErr, err)
		}
	})
}

func TestMockBlueprintHandler_Write(t *testing.T) {
	setup := func(t *testing.T) *MockBlueprintHandler {
		t.Helper()
		return &MockBlueprintHandler{}
	}

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock handler with Write function
		handler := setup(t)
		handler.WriteFunc = func(overwrite ...bool) error {
			return nil
		}
		// When calling write
		err := handler.Write()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock handler with no Write function
		handler := setup(t)
		// When calling write
		err := handler.Write()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("WithError", func(t *testing.T) {
		// Given a mock handler with Write function that returns error
		handler := setup(t)
		mockErr := fmt.Errorf("mock error")
		handler.WriteFunc = func(overwrite ...bool) error {
			return mockErr
		}
		// When calling write
		err := handler.Write()
		// Then expected error should be returned
		if err != mockErr {
			t.Errorf("Expected error = %v, got = %v", mockErr, err)
		}
	})

	t.Run("WithOverwriteParameter", func(t *testing.T) {
		// Given a mock handler with Write function that checks parameters
		handler := setup(t)
		var receivedOverwrite []bool
		handler.WriteFunc = func(overwrite ...bool) error {
			receivedOverwrite = overwrite
			return nil
		}
		// When calling write with overwrite parameter
		err := handler.Write(true)
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
		// And the overwrite parameter should be passed through
		if len(receivedOverwrite) != 1 || receivedOverwrite[0] != true {
			t.Errorf("Expected overwrite parameter [true], got %v", receivedOverwrite)
		}
	})
}

func TestMockBlueprintHandler_SetRenderedKustomizeData(t *testing.T) {
	setup := func(t *testing.T) *MockBlueprintHandler {
		t.Helper()
		return &MockBlueprintHandler{}
	}

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock handler with SetRenderedKustomizeData function
		handler := setup(t)
		expectedData := map[string]any{
			"key1": "value1",
			"key2": 42,
			"key3": []string{"item1", "item2"},
		}
		var receivedData map[string]any
		handler.SetRenderedKustomizeDataFunc = func(data map[string]any) {
			receivedData = data
		}
		// When setting rendered kustomize data
		handler.SetRenderedKustomizeData(expectedData)
		// Then the function should be called with correct data
		if !reflect.DeepEqual(receivedData, expectedData) {
			t.Errorf("Expected data = %v, got = %v", expectedData, receivedData)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock handler without SetRenderedKustomizeData function
		handler := setup(t)
		testData := map[string]any{
			"key1": "value1",
			"key2": 42,
		}
		// When setting rendered kustomize data
		// Then no panic should occur
		handler.SetRenderedKustomizeData(testData)
		// Test passes if no panic occurs
	})

	t.Run("WithEmptyData", func(t *testing.T) {
		// Given a mock handler with SetRenderedKustomizeData function
		handler := setup(t)
		expectedData := map[string]any{}
		var receivedData map[string]any
		handler.SetRenderedKustomizeDataFunc = func(data map[string]any) {
			receivedData = data
		}
		// When setting empty rendered kustomize data
		handler.SetRenderedKustomizeData(expectedData)
		// Then the function should be called with empty data
		if !reflect.DeepEqual(receivedData, expectedData) {
			t.Errorf("Expected data = %v, got = %v", expectedData, receivedData)
		}
	})

	t.Run("WithComplexData", func(t *testing.T) {
		// Given a mock handler with SetRenderedKustomizeData function
		handler := setup(t)
		expectedData := map[string]any{
			"nested": map[string]any{
				"level1": map[string]any{
					"level2": []any{
						"string1",
						123,
						map[string]any{"key": "value"},
					},
				},
			},
			"array": []any{
				"item1",
				456,
				map[string]any{"nested": "data"},
			},
			"mixed": []map[string]any{
				{"key1": "value1"},
				{"key2": 789},
			},
		}
		var receivedData map[string]any
		handler.SetRenderedKustomizeDataFunc = func(data map[string]any) {
			receivedData = data
		}
		// When setting complex rendered kustomize data
		handler.SetRenderedKustomizeData(expectedData)
		// Then the function should be called with correct complex data
		if !reflect.DeepEqual(receivedData, expectedData) {
			t.Errorf("Expected data = %v, got = %v", expectedData, receivedData)
		}
	})
}

func TestMockBlueprintHandler_LoadData(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock blueprint handler with LoadDataFunc set
		handler := NewMockBlueprintHandler(di.NewInjector())
		expectedError := fmt.Errorf("mock load data error")
		handler.LoadDataFunc = func(data map[string]any, ociInfo ...*artifact.OCIArtifactInfo) error {
			return expectedError
		}

		// When LoadData is called
		err := handler.LoadData(map[string]any{})

		// Then it should return the expected error
		if err != expectedError {
			t.Errorf("expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock blueprint handler without LoadDataFunc set
		handler := NewMockBlueprintHandler(di.NewInjector())

		// When LoadData is called
		err := handler.LoadData(map[string]any{})

		// Then it should return nil
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})
}
