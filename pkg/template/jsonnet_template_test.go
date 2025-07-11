package template

import (
	"fmt"
	"strings"
	"testing"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupJsonnetTemplateMocks(t *testing.T, opts ...*SetupOptions) (*Mocks, *JsonnetTemplate) {
	t.Helper()
	mocks := setupMocks(t, opts...)
	template := NewJsonnetTemplate(mocks.Injector)
	return mocks, template
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestJsonnetTemplate_NewJsonnetTemplate(t *testing.T) {
	t.Run("CreatesTemplateWithDefaultRules", func(t *testing.T) {
		// Given an injector
		mocks := setupMocks(t)

		// When creating a new jsonnet template
		template := NewJsonnetTemplate(mocks.Injector)

		// Then the template should be properly initialized
		if template == nil {
			t.Fatal("Expected non-nil template")
		}

		// And base template should be set
		if template.BaseTemplate == nil {
			t.Error("Expected BaseTemplate to be set")
		}

		// And default rules should be configured (verified by testing Process method)
	})
}

func TestJsonnetTemplate_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*JsonnetTemplate, *Mocks) {
		t.Helper()
		mocks, template := setupJsonnetTemplateMocks(t)
		return template, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a jsonnet template
		template, _ := setup(t)

		// When calling Initialize
		err := template.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And base template dependencies should be injected
		if template.BaseTemplate.configHandler == nil {
			t.Error("Expected configHandler to be set after Initialize()")
		}
		if template.BaseTemplate.shell == nil {
			t.Error("Expected shell to be set after Initialize()")
		}
	})

	t.Run("HandlesBaseInitializeError", func(t *testing.T) {
		// Given a jsonnet template with nil injector in base
		template := NewJsonnetTemplate(nil)

		// When calling Initialize
		err := template.Initialize()

		// Then no error should be returned (base template handles nil gracefully)
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})
}

func TestJsonnetTemplate_Process(t *testing.T) {
	setup := func(t *testing.T) (*JsonnetTemplate, *Mocks) {
		t.Helper()
		mocks, template := setupJsonnetTemplateMocks(t)
		err := template.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize template: %v", err)
		}
		return template, mocks
	}

	t.Run("ProcessesBlueprintJsonnetTemplate", func(t *testing.T) {
		// Given a jsonnet template
		template, mocks := setup(t)

		// And template data containing a blueprint.jsonnet file
		templateData := map[string][]byte{
			"blueprint.jsonnet": []byte(`local context = std.extVar("context"); { kind: "Blueprint", metadata: { name: context.name } }`),
		}
		renderedData := make(map[string]any)

		// And a mock jsonnet VM that returns valid blueprint JSON
		template.shims.NewJsonnetVM = func() JsonnetVM {
			mockVM := &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					return `{"kind": "Blueprint", "metadata": {"name": "test-context"}}`, nil
				},
			}
			return mockVM
		}

		// And mock config handler returns valid YAML
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("contexts:\n  mock-context:\n    dns:\n      domain: mock.domain.com"), nil
		}

		// When processing the template data
		err := template.Process(templateData, renderedData)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And the rendered data should contain the blueprint content
		if len(renderedData) != 1 {
			t.Errorf("Expected 1 rendered item, got %d", len(renderedData))
		}

		if _, exists := renderedData["blueprint"]; !exists {
			t.Error("Expected blueprint to be rendered")
		}
	})

	t.Run("ProcessesTerraformJsonnetTemplates", func(t *testing.T) {
		// Given a jsonnet template
		template, mocks := setup(t)

		// And template data containing terraform/ .jsonnet files
		templateData := map[string][]byte{
			"terraform/main.jsonnet":    []byte(`local context = std.extVar("context"); { cluster_name: context.name }`),
			"terraform/cluster.jsonnet": []byte(`local context = std.extVar("context"); { instance_type: "t3.micro" }`),
		}
		renderedData := make(map[string]any)

		// And a mock jsonnet VM that returns valid terraform vars
		template.shims.NewJsonnetVM = func() JsonnetVM {
			mockVM := &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					if strings.Contains(filename, "main") {
						return `{"cluster_name": "test-cluster"}`, nil
					}
					return `{"instance_type": "t3.micro"}`, nil
				},
			}
			return mockVM
		}

		// And mock config handler returns valid YAML
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("contexts:\n  mock-context:\n    dns:\n      domain: mock.domain.com"), nil
		}

		// When processing the template data
		err := template.Process(templateData, renderedData)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And the rendered data should contain the terraform variables
		if len(renderedData) != 2 {
			t.Errorf("Expected 2 rendered items, got %d", len(renderedData))
		}

		if _, exists := renderedData["terraform/main"]; !exists {
			t.Error("Expected terraform/main to be rendered")
		}
		if _, exists := renderedData["terraform/cluster"]; !exists {
			t.Error("Expected terraform/cluster to be rendered")
		}
	})

	t.Run("ProcessesBothBlueprintAndTerraformTemplates", func(t *testing.T) {
		// Given a jsonnet template
		template, mocks := setup(t)

		// And template data containing both blueprint and terraform files
		templateData := map[string][]byte{
			"blueprint.jsonnet":      []byte(`{ kind: "Blueprint", metadata: { name: "test" } }`),
			"terraform/main.jsonnet": []byte(`{ cluster_name: "test-cluster" }`),
			"other.jsonnet":          []byte(`{ ignored: true }`),
		}
		renderedData := make(map[string]any)

		// And a mock jsonnet VM that returns valid JSON
		template.shims.NewJsonnetVM = func() JsonnetVM {
			mockVM := &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					if strings.Contains(filename, "blueprint") {
						return `{"kind": "Blueprint", "metadata": {"name": "test"}}`, nil
					}
					return `{"cluster_name": "test-cluster"}`, nil
				},
			}
			return mockVM
		}

		// And mock config handler returns valid YAML
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("contexts:\n  mock-context:\n    dns:\n      domain: mock.domain.com"), nil
		}

		// When processing the template data
		err := template.Process(templateData, renderedData)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And both blueprint and terraform should be processed, but not other.jsonnet
		if len(renderedData) != 2 {
			t.Errorf("Expected 2 rendered items, got %d", len(renderedData))
		}

		if _, exists := renderedData["blueprint"]; !exists {
			t.Error("Expected blueprint to be rendered")
		}
		if _, exists := renderedData["terraform/main"]; !exists {
			t.Error("Expected terraform/main to be rendered")
		}
		if _, exists := renderedData["other"]; exists {
			t.Error("Expected other.jsonnet to be ignored")
		}
	})

	t.Run("IgnoresNonMatchingTemplates", func(t *testing.T) {
		// Given a jsonnet template
		template, _ := setup(t)

		// And template data containing files that don't match any rules
		templateData := map[string][]byte{
			"other.jsonnet": []byte(`{"name": "test"}`),
			"config.yaml":   []byte(`key: value`),
			"script.js":     []byte(`console.log("hello")`),
		}
		renderedData := make(map[string]any)

		// When processing the template data
		err := template.Process(templateData, renderedData)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And no data should be processed
		if len(renderedData) != 0 {
			t.Errorf("Expected no data to be processed for non-matching templates, got %d items", len(renderedData))
		}
	})

	t.Run("HandlesJsonnetProcessingError", func(t *testing.T) {
		// Given a jsonnet template
		template, mocks := setup(t)

		// And template data containing a blueprint.jsonnet file
		templateData := map[string][]byte{
			"blueprint.jsonnet": []byte(`invalid jsonnet`),
		}
		renderedData := make(map[string]any)

		// And a mock jsonnet VM that returns an error
		template.shims.NewJsonnetVM = func() JsonnetVM {
			mockVM := &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					return "", fmt.Errorf("jsonnet evaluation error")
				},
			}
			return mockVM
		}

		// And mock config handler returns valid YAML
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("contexts:\n  mock-context:\n    dns:\n      domain: mock.domain.com"), nil
		}

		// When processing the template data
		err := template.Process(templateData, renderedData)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to process jsonnet template") {
			t.Errorf("Expected error about jsonnet template processing, got: %v", err)
		}
	})

	t.Run("ProcessesOnlyFirstMatchingRule", func(t *testing.T) {
		// Given a jsonnet template
		template, mocks := setup(t)

		// And template data containing a blueprint.jsonnet file (which matches the first rule)
		templateData := map[string][]byte{
			"blueprint.jsonnet": []byte(`{ kind: "Blueprint" }`),
		}
		renderedData := make(map[string]any)

		// And a mock jsonnet VM
		template.shims.NewJsonnetVM = func() JsonnetVM {
			mockVM := &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					return `{"kind": "Blueprint"}`, nil
				},
			}
			return mockVM
		}

		// And mock config handler returns valid YAML
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("contexts:\n  mock-context:\n    dns:\n      domain: mock.domain.com"), nil
		}

		// When processing the template data
		err := template.Process(templateData, renderedData)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And the blueprint should be processed by the first matching rule
		if len(renderedData) != 1 {
			t.Errorf("Expected 1 rendered item, got %d", len(renderedData))
		}
		if _, exists := renderedData["blueprint"]; !exists {
			t.Error("Expected blueprint to be rendered by blueprint rule")
		}
	})
}

func TestJsonnetTemplate_processJsonnetTemplate(t *testing.T) {
	setup := func(t *testing.T) (*JsonnetTemplate, *Mocks) {
		t.Helper()
		mocks, template := setupJsonnetTemplateMocks(t)
		err := template.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize template: %v", err)
		}
		return template, mocks
	}

	t.Run("ProcessesValidJsonnetTemplate", func(t *testing.T) {
		// Given a jsonnet template
		template, mocks := setup(t)

		// And a mock jsonnet VM that returns valid JSON
		template.shims.NewJsonnetVM = func() JsonnetVM {
			mockVM := &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					return `{"key": "value", "number": 42}`, nil
				},
			}
			return mockVM
		}

		// And mock config handler returns valid YAML
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("contexts:\n  mock-context:\n    dns:\n      domain: mock.domain.com"), nil
		}

		templateContent := `local context = std.extVar("context"); { key: "value", number: 42 }`

		// When processing the jsonnet template
		result, err := template.processJsonnetTemplate(templateContent)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And the result should contain the expected values
		if result["key"] != "value" {
			t.Errorf("Expected key 'value', got: %v", result["key"])
		}
		if result["number"] != float64(42) {
			t.Errorf("Expected number 42, got: %v", result["number"])
		}
	})

	t.Run("HandlesYamlMarshalError", func(t *testing.T) {
		// Given a jsonnet template
		template, mocks := setup(t)

		// And a config handler that returns an error when marshaling
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("yaml marshal error")
		}

		templateContent := `local context = std.extVar("context"); { key: "value" }`

		// When processing the jsonnet template
		_, err := template.processJsonnetTemplate(templateContent)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to marshal context to YAML") {
			t.Errorf("Expected error about marshal failure, got: %v", err)
		}
	})

	t.Run("HandlesProjectRootError", func(t *testing.T) {
		// Given a jsonnet template
		template, mocks := setup(t)

		// And a shell that returns an error for project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}

		// And mock config handler returns valid YAML
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("contexts:\n  mock-context:\n    dns:\n      domain: mock.domain.com"), nil
		}

		templateContent := `local context = std.extVar("context"); { key: "value" }`

		// When processing the jsonnet template
		_, err := template.processJsonnetTemplate(templateContent)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("Expected error about project root failure, got: %v", err)
		}
	})

	t.Run("HandlesYamlUnmarshalError", func(t *testing.T) {
		// Given a jsonnet template
		template, mocks := setup(t)

		// And mock config handler returns invalid YAML
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("invalid yaml: ["), nil
		}

		// And a mock YamlUnmarshal that returns an error
		template.shims.YamlUnmarshal = func(data []byte, v any) error {
			return fmt.Errorf("yaml unmarshal error")
		}

		templateContent := `local context = std.extVar("context"); { key: "value" }`

		// When processing the jsonnet template
		_, err := template.processJsonnetTemplate(templateContent)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to unmarshal context YAML") {
			t.Errorf("Expected error about YAML unmarshal failure, got: %v", err)
		}
	})

	t.Run("HandlesJsonMarshalError", func(t *testing.T) {
		// Given a jsonnet template
		template, mocks := setup(t)

		// And mock config handler returns valid YAML
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("contexts:\n  mock-context:\n    dns:\n      domain: mock.domain.com"), nil
		}

		// And a mock JsonMarshal that returns an error
		template.shims.JsonMarshal = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("json marshal error")
		}

		templateContent := `local context = std.extVar("context"); { key: "value" }`

		// When processing the jsonnet template
		_, err := template.processJsonnetTemplate(templateContent)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to marshal context map to JSON") {
			t.Errorf("Expected error about JSON marshal failure, got: %v", err)
		}
	})

	t.Run("HandlesJsonnetEvaluationError", func(t *testing.T) {
		// Given a jsonnet template
		template, mocks := setup(t)

		// And mock config handler returns valid YAML
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("contexts:\n  mock-context:\n    dns:\n      domain: mock.domain.com"), nil
		}

		// And a mock jsonnet VM that returns an error
		template.shims.NewJsonnetVM = func() JsonnetVM {
			mockVM := &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					return "", fmt.Errorf("jsonnet evaluation error")
				},
			}
			return mockVM
		}

		templateContent := `invalid jsonnet syntax`

		// When processing the jsonnet template
		_, err := template.processJsonnetTemplate(templateContent)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to evaluate jsonnet template") {
			t.Errorf("Expected error about jsonnet evaluation, got: %v", err)
		}
	})

	t.Run("HandlesInvalidJsonResult", func(t *testing.T) {
		// Given a jsonnet template
		template, mocks := setup(t)

		// And mock config handler returns valid YAML
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("contexts:\n  mock-context:\n    dns:\n      domain: mock.domain.com"), nil
		}

		// And a mock jsonnet VM that returns invalid JSON
		template.shims.NewJsonnetVM = func() JsonnetVM {
			mockVM := &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					return `invalid json response`, nil
				},
			}
			return mockVM
		}

		templateContent := `local context = std.extVar("context"); "not an object"`

		// When processing the jsonnet template
		_, err := template.processJsonnetTemplate(templateContent)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "jsonnet template must output valid JSON") {
			t.Errorf("Expected error about invalid JSON, got: %v", err)
		}
	})

	t.Run("HandlesContextDataInjection", func(t *testing.T) {
		// Given a jsonnet template
		template, mocks := setup(t)

		// And mock config handler returns complex YAML with nested data
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte(`
contexts:
  test-context:
    dns:
      domain: example.com
    cluster:
      name: test-cluster
      region: us-west-2
`), nil
		}

		// And mock shell returns a specific project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/path/to/test-project", nil
		}

		// And a mock jsonnet VM that captures the context data
		var capturedContext string
		template.shims.NewJsonnetVM = func() JsonnetVM {
			mockVM := &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					// Verify the template content references context
					if !strings.Contains(snippet, "std.extVar(\"context\")") {
						t.Errorf("Expected template to reference context variable")
					}
					return `{"processed": true, "contextReceived": true}`, nil
				},
			}
			// Create a custom ExtCode function to capture context
			mockVM.ExtCodeFunc = func(key, val string) {
				if key == "context" {
					capturedContext = val
				}
				// Store the call as usual
				mockVM.ExtCalls = append(mockVM.ExtCalls, struct{ Key, Val string }{key, val})
			}
			return mockVM
		}

		templateContent := `local context = std.extVar("context"); { processed: true, name: context.name, projectName: context.projectName }`

		// When processing the jsonnet template
		result, err := template.processJsonnetTemplate(templateContent)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And the result should be processed
		if result["processed"] != true {
			t.Error("Expected template to be processed")
		}

		// And context should have been injected
		if capturedContext == "" {
			t.Error("Expected context to be captured")
		}

		// And context should contain expected fields
		if !strings.Contains(capturedContext, "test-context") {
			t.Error("Expected context to contain context name")
		}
		if !strings.Contains(capturedContext, "test-project") {
			t.Error("Expected context to contain project name")
		}
	})

	t.Run("HandlesEmptyContextMap", func(t *testing.T) {
		// Given a jsonnet template
		template, mocks := setup(t)

		// And mock config handler returns minimal YAML
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("{}"), nil
		}

		// And mock config handler returns empty context
		mocks.ConfigHandler.GetContextFunc = func() string {
			return ""
		}

		// And a mock jsonnet VM
		template.shims.NewJsonnetVM = func() JsonnetVM {
			mockVM := &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					return `{"minimal": true}`, nil
				},
			}
			return mockVM
		}

		templateContent := `local context = std.extVar("context"); { minimal: true }`

		// When processing the jsonnet template
		result, err := template.processJsonnetTemplate(templateContent)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And the result should be processed
		if result["minimal"] != true {
			t.Error("Expected minimal template to be processed")
		}
	})
}

func TestJsonnetTemplate_RealShimsIntegration(t *testing.T) {
	t.Run("UsesRealShimsForJsonnetVM", func(t *testing.T) {
		// Given a jsonnet template that uses real shims (not mocked)
		mocks := setupMocks(t)
		template := NewJsonnetTemplate(mocks.Injector)
		err := template.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize template: %v", err)
		}

		// And mock config handler returns valid YAML
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("contexts:\n  test-context:\n    value: test"), nil
		}

		// When processing a simple jsonnet template using real shims
		templateContent := `local context = std.extVar("context"); { result: "success", hasContext: std.objectHas(context, "name") }`
		result, err := template.processJsonnetTemplate(templateContent)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error with real shims, got: %v", err)
		}

		// And the result should be processed correctly
		if result["result"] != "success" {
			t.Errorf("Expected result 'success', got: %v", result["result"])
		}

		// And context should be available
		if hasContext, ok := result["hasContext"].(bool); !ok || !hasContext {
			t.Error("Expected context to be available in jsonnet template")
		}
	})

	t.Run("ShimsProvideBasicFunctionality", func(t *testing.T) {
		// Given real shims
		shims := NewShims()

		// When testing basic functionality
		// Then all function fields should be set
		if shims.ReadFile == nil {
			t.Error("Expected ReadFile to be set")
		}
		if shims.JsonMarshal == nil {
			t.Error("Expected JsonMarshal to be set")
		}
		if shims.JsonUnmarshal == nil {
			t.Error("Expected JsonUnmarshal to be set")
		}
		if shims.YamlMarshal == nil {
			t.Error("Expected YamlMarshal to be set")
		}
		if shims.YamlUnmarshal == nil {
			t.Error("Expected YamlUnmarshal to be set")
		}
		if shims.NewJsonnetVM == nil {
			t.Error("Expected NewJsonnetVM to be set")
		}
		if shims.FilepathBase == nil {
			t.Error("Expected FilepathBase to be set")
		}

		// And JsonnetVM should be creatable
		vm := shims.NewJsonnetVM()
		if vm == nil {
			t.Error("Expected NewJsonnetVM to create a VM")
		}

		// And basic shim functions should work
		testData := map[string]interface{}{"test": "value"}
		jsonBytes, err := shims.JsonMarshal(testData)
		if err != nil {
			t.Errorf("Expected JsonMarshal to work, got error: %v", err)
		}

		var unmarshaledData map[string]interface{}
		err = shims.JsonUnmarshal(jsonBytes, &unmarshaledData)
		if err != nil {
			t.Errorf("Expected JsonUnmarshal to work, got error: %v", err)
		}
		if unmarshaledData["test"] != "value" {
			t.Errorf("Expected unmarshaled data to match, got: %v", unmarshaledData)
		}

		// And FilepathBase should work
		baseName := shims.FilepathBase("/path/to/file.txt")
		if baseName != "file.txt" {
			t.Errorf("Expected base name 'file.txt', got: %v", baseName)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

type mockJsonnetVM struct {
	EvaluateFunc func(filename, snippet string) (string, error)
	ExtCodeFunc  func(key, val string)
	ExtCalls     []struct{ Key, Val string }
}

func (m *mockJsonnetVM) ExtCode(key, val string) {
	if m.ExtCodeFunc != nil {
		m.ExtCodeFunc(key, val)
	} else {
		m.ExtCalls = append(m.ExtCalls, struct{ Key, Val string }{key, val})
	}
}

func (m *mockJsonnetVM) EvaluateAnonymousSnippet(filename, snippet string) (string, error) {
	if m.EvaluateFunc != nil {
		return m.EvaluateFunc(filename, snippet)
	}
	return "", nil
}
