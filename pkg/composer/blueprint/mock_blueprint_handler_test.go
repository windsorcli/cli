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

func TestMockBlueprintHandler_Generate(t *testing.T) {
	setup := func(t *testing.T) *MockBlueprintHandler {
		t.Helper()
		injector := di.NewInjector()
		handler := NewMockBlueprintHandler(injector)
		return handler
	}

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock handler with generate function
		handler := setup(t)
		expectedBlueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-generated-blueprint",
			},
		}
		handler.GenerateFunc = func() *blueprintv1alpha1.Blueprint {
			return expectedBlueprint
		}

		// When generating blueprint
		generated := handler.Generate()

		// Then expected blueprint should be returned
		if !reflect.DeepEqual(generated, expectedBlueprint) {
			t.Errorf("Expected blueprint = %v, got = %v", expectedBlueprint, generated)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock handler without generate function
		handler := setup(t)

		// When generating blueprint
		generated := handler.Generate()

		// Then nil should be returned
		if generated != nil {
			t.Errorf("Expected nil, got %v", generated)
		}
	})
}
