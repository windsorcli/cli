package template

import (
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMockTemplate_NewMockTemplate(t *testing.T) {
	t.Run("CreatesTemplateWithInjector", func(t *testing.T) {
		// Given an injector
		injector := di.NewInjector()

		// When creating a new mock template
		template := NewMockTemplate(injector)

		// Then the template should be properly initialized
		if template == nil {
			t.Fatal("Expected non-nil template")
		}
	})
}

func TestMockTemplate_Initialize(t *testing.T) {
	setup := func(t *testing.T) *MockTemplate {
		t.Helper()
		injector := di.NewInjector()
		return NewMockTemplate(injector)
	}

	t.Run("DefaultBehavior", func(t *testing.T) {
		// Given a mock template without initialize function
		template := setup(t)

		// When calling Initialize
		err := template.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("CustomInitializeFunc", func(t *testing.T) {
		// Given a mock template with custom initialize function
		template := setup(t)
		template.InitializeFunc = func() error {
			return fmt.Errorf("custom error")
		}

		// When calling Initialize
		err := template.Initialize()

		// Then the custom error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "custom error" {
			t.Errorf("Expected 'custom error', got: %v", err)
		}
	})
}

func TestMockTemplate_Process(t *testing.T) {
	setup := func(t *testing.T) *MockTemplate {
		t.Helper()
		injector := di.NewInjector()
		return NewMockTemplate(injector)
	}

	t.Run("DefaultBehavior", func(t *testing.T) {
		// Given a mock template without process function
		template := setup(t)

		// And template data
		templateData := map[string][]byte{
			"test.jsonnet": []byte(`{"key": "value"}`),
		}
		renderedData := make(map[string]any)

		// When calling Process
		err := template.Process(templateData, renderedData)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And rendered data should remain empty
		if len(renderedData) != 0 {
			t.Errorf("Expected empty rendered data, got %d items", len(renderedData))
		}
	})

	t.Run("CustomProcessFunc", func(t *testing.T) {
		// Given a mock template with custom process function
		template := setup(t)
		template.ProcessFunc = func(templateData map[string][]byte, renderedData map[string]any) error {
			for key := range templateData {
				renderedData[key] = "processed"
			}
			return nil
		}

		// And template data
		templateData := map[string][]byte{
			"test.jsonnet": []byte(`{"key": "value"}`),
		}
		renderedData := make(map[string]any)

		// When calling Process
		err := template.Process(templateData, renderedData)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And rendered data should contain the processed result
		if len(renderedData) != 1 {
			t.Errorf("Expected 1 rendered item, got %d", len(renderedData))
		}
		if renderedData["test.jsonnet"] != "processed" {
			t.Errorf("Expected 'processed', got: %v", renderedData["test.jsonnet"])
		}
	})

	t.Run("ProcessFuncReturnsError", func(t *testing.T) {
		// Given a mock template with error-returning process function
		template := setup(t)
		template.ProcessFunc = func(templateData map[string][]byte, renderedData map[string]any) error {
			return fmt.Errorf("process error")
		}

		// And template data
		templateData := map[string][]byte{
			"test.jsonnet": []byte(`{"key": "value"}`),
		}
		renderedData := make(map[string]any)

		// When calling Process
		err := template.Process(templateData, renderedData)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "process error" {
			t.Errorf("Expected 'process error', got: %v", err)
		}
	})
}
