package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Types
// =============================================================================

// SetupOptions provides configuration for test setup
type SetupOptions struct {
	// Add any specific setup options if needed
}

// Mocks contains all mock implementations needed for testing
type Mocks struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
}

// =============================================================================
// Test Helpers
// =============================================================================

// setupMocks creates and configures mock dependencies for testing
func setupMocks(t *testing.T, _ ...*SetupOptions) *Mocks {
	t.Helper()

	configHandler := &config.MockConfigHandler{}
	shellService := &shell.MockShell{}

	injector := di.NewMockInjector()
	injector.Register("configHandler", configHandler)
	injector.Register("shell", shellService)

	return &Mocks{
		Injector:      injector,
		ConfigHandler: configHandler,
		Shell:         shellService,
	}
}

// setupJsonnetTemplateMocks creates mocks and a JsonnetTemplate instance
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
	t.Run("CreatesTemplateWithDependencies", func(t *testing.T) {
		// Given an injector
		mocks := setupMocks(t)

		// When creating a new jsonnet template
		template := NewJsonnetTemplate(mocks.Injector)

		// Then the template should be properly initialized
		if template == nil {
			t.Fatal("Expected non-nil template")
		}

		// And injector should be set
		if template.injector == nil {
			t.Error("Expected injector to be set")
		}

		// And shims should be set
		if template.shims == nil {
			t.Error("Expected shims to be set")
		}
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

		// And dependencies should be injected
		if template.configHandler == nil {
			t.Error("Expected configHandler to be set after Initialize()")
		}
		if template.shell == nil {
			t.Error("Expected shell to be set after Initialize()")
		}
	})

	t.Run("HandlesNilInjector", func(t *testing.T) {
		// Given a jsonnet template with nil injector
		template := NewJsonnetTemplate(nil)

		// When calling Initialize
		err := template.Initialize()

		// Then no error should be returned (handles nil gracefully)
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

	t.Run("ProcessesPatchesJsonnetTemplates", func(t *testing.T) {
		// Given a jsonnet template
		template, mocks := setup(t)

		// And template data containing blueprint and kustomize/ .jsonnet files with subdirectory structure
		templateData := map[string][]byte{
			"blueprint.jsonnet":                       []byte(`{ kustomize: [{ name: "ingress", patches: [{ path: "ingress/patches/nginx" }] }, { name: "dns", patches: [{ path: "dns/patches/coredns" }] }] }`),
			"kustomize/ingress/patches/nginx.jsonnet": []byte(`local context = std.extVar("context"); { apiVersion: "v1", kind: "ConfigMap", metadata: { name: "nginx-config" } }`),
			"kustomize/dns/patches/coredns.jsonnet":   []byte(`local context = std.extVar("context"); { apiVersion: "v1", kind: "ConfigMap", metadata: { name: "coredns-config" } }`),
		}
		renderedData := make(map[string]any)

		// And a mock jsonnet VM that returns valid manifests
		template.shims.NewJsonnetVM = func() JsonnetVM {
			mockVM := &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					if strings.Contains(snippet, `kustomize:`) && strings.Contains(snippet, `patches:`) {
						// This is the blueprint template
						return `{"kustomize": [{"name": "ingress", "patches": [{"path": "ingress/patches/nginx"}]}, {"name": "dns", "patches": [{"path": "dns/patches/coredns"}]}]}`, nil
					}
					if strings.Contains(snippet, `nginx-config`) {
						return `{"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "nginx-config"}}`, nil
					}
					return `{"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "coredns-config"}}`, nil
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

		// And the rendered data should contain the blueprint and patch manifests with preserved path structure
		if len(renderedData) != 3 {
			t.Errorf("Expected 3 rendered items (blueprint + 2 patches), got %d", len(renderedData))
		}

		// Verify that the blueprint is processed and patches field is cleaned up
		if _, exists := renderedData["blueprint"]; !exists {
			t.Error("Expected blueprint to be rendered")
		}

		// Verify that the full path structure is preserved (not flattened)
		if _, exists := renderedData["kustomize/ingress/patches/nginx"]; !exists {
			t.Error("Expected kustomize/ingress/patches/nginx to be rendered with preserved path structure")
		}

		if _, exists := renderedData["kustomize/dns/patches/coredns"]; !exists {
			t.Error("Expected kustomize/dns/patches/coredns to be rendered with preserved path structure")
		}

		// Verify the content is correctly processed
		nginxPatch, ok := renderedData["kustomize/ingress/patches/nginx"].(map[string]any)
		if !ok {
			t.Error("Expected nginx patch to be a map")
		} else {
			if nginxPatch["apiVersion"] != "v1" {
				t.Errorf("Expected apiVersion to be 'v1', got %v", nginxPatch["apiVersion"])
			}
			if nginxPatch["kind"] != "ConfigMap" {
				t.Errorf("Expected kind to be 'ConfigMap', got %v", nginxPatch["kind"])
			}
		}

		corednsPatch, ok := renderedData["kustomize/dns/patches/coredns"].(map[string]any)
		if !ok {
			t.Error("Expected coredns patch to be a map")
		} else {
			if corednsPatch["apiVersion"] != "v1" {
				t.Errorf("Expected apiVersion to be 'v1', got %v", corednsPatch["apiVersion"])
			}
			if corednsPatch["kind"] != "ConfigMap" {
				t.Errorf("Expected kind to be 'ConfigMap', got %v", corednsPatch["kind"])
			}
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

		// And template data containing files that don't match known template patterns
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
		if !strings.Contains(err.Error(), "failed to process template") {
			t.Errorf("Expected error about template processing, got: %v", err)
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
  mock-context:
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
		if !strings.Contains(capturedContext, "mock-context") {
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

func TestJsonnetTemplate_buildHelperLibrary(t *testing.T) {
	setup := func(t *testing.T) (*JsonnetTemplate, *Mocks) {
		t.Helper()
		mocks, template := setupJsonnetTemplateMocks(t)
		err := template.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize template: %v", err)
		}
		return template, mocks
	}

	t.Run("GeneratesValidJsonnetLibrary", func(t *testing.T) {
		// Given a jsonnet template
		template, _ := setup(t)

		// When building the helper library
		helperLib := template.buildHelperLibrary()

		// Then it should be a valid Jsonnet object
		if !strings.HasPrefix(helperLib, "{") {
			t.Error("Expected helper library to start with '{'")
		}
		if !strings.HasSuffix(helperLib, "}") {
			t.Error("Expected helper library to end with '}'")
		}

		// And it should contain the expected helper functions
		expectedFunctions := []string{
			// Smart helpers (handle both path-based and key-based access)
			"get(obj, path, default=null):",
			"getString(obj, path, default=\"\"):",
			"getInt(obj, path, default=0):",
			"getNumber(obj, path, default=0):",
			"getBool(obj, path, default=false):",
			"getObject(obj, path, default={}):",
			"getArray(obj, path, default=[]):",
			"has(obj, path):",

			// URL helpers
			"baseUrl(endpoint):",
		}

		for _, expectedFunc := range expectedFunctions {
			if !strings.Contains(helperLib, expectedFunc) {
				t.Errorf("Expected helper library to contain '%s'", expectedFunc)
			}
		}
	})
}

func TestJsonnetTemplate_processJsonnetTemplateWithHelpers(t *testing.T) {
	setup := func(t *testing.T) (*JsonnetTemplate, *Mocks) {
		t.Helper()
		mocks, template := setupJsonnetTemplateMocks(t)
		err := template.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize template: %v", err)
		}
		return template, mocks
	}

	t.Run("InjectsHelpersAsLibrary", func(t *testing.T) {
		// Given a jsonnet template
		template, mocks := setup(t)

		// And a mock jsonnet VM that captures all ExtCode calls
		var extCalls []struct{ Key, Val string }
		template.shims.NewJsonnetVM = func() JsonnetVM {
			mockVM := &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					return `{"success": true}`, nil
				},
			}
			mockVM.ExtCodeFunc = func(key, val string) {
				extCalls = append(extCalls, struct{ Key, Val string }{key, val})
			}
			return mockVM
		}

		// And mock config handler returns valid YAML
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("contexts:\n  mock-context:\n    dns:\n      domain: mock.domain.com"), nil
		}

		templateContent := `local helpers = std.extVar("helpers"); local context = std.extVar("context"); { result: helpers.getString(context, "dns.domain", "default") }`

		// When processing the jsonnet template
		_, err := template.processJsonnetTemplate(templateContent)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And windsor helpers should be injected as a library
		var foundHelpers, foundContext bool
		for _, call := range extCalls {
			if call.Key == "helpers" {
				foundHelpers = true
				// Verify the helpers library contains expected functions
				if !strings.Contains(call.Val, "getString") {
					t.Error("Expected helpers library to contain getString function")
				}
				if !strings.Contains(call.Val, "baseUrl") {
					t.Error("Expected helpers library to contain baseUrl function")
				}
			}
			if call.Key == "context" {
				foundContext = true
			}
		}

		if !foundHelpers {
			t.Error("Expected helpers to be injected as ExtCode")
		}
		if !foundContext {
			t.Error("Expected context to be injected as ExtCode")
		}
	})

	t.Run("test helper functions with real Jsonnet VM", func(t *testing.T) {
		templateContent := `
local helpers = std.extVar("helpers");
local context = std.extVar("context");

{
  // Test primary helpers (path-based access)
  vmDriver: helpers.getString(context, "vm.driver", "default-driver"),
  vmCores: helpers.getInt(context, "vm.cores", 2),
  haEnabled: helpers.getBool(context, "cluster.ha.enabled", false),
  
  // Test nested path access  
  nodeIp: helpers.getString(context, "cluster.nodes.master.ip", "192.168.1.1"),
  
  // Test object and array access
  cluster: helpers.getObject(context, "cluster", {}),
  tags: helpers.getArray(context, "tags", ["default"]),
  
  // Test key-based helpers (same function, different usage)
  localValue: helpers.getString({test: "value"}, "test", "fallback"),
  localInt: helpers.getInt({number: 42}, "number", 0),
  
  // Test path access with primary helpers
  pathValue: helpers.get({nested: {value: "found"}}, "nested.value", "not found"),
  
  // Test existence checking
  hasVm: helpers.has(context, "vm.driver"),
  hasNonexistent: helpers.has(context, "does.not.exist"),
}`

		// Given a jsonnet template using real shims
		template, mocks := setup(t)

		// Set up mock config for the context
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte(`
vm:
  driver: colima
  cores: 4
cluster:
  ha:
    enabled: true
  nodes:
    master:
      ip: 10.0.1.100
tags:
  - production
  - k8s
`), nil
		}

		// When processing a template that uses helper functions
		result, err := template.processJsonnetTemplate(templateContent)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And the helper functions should work correctly
		if result["vmDriver"] != "colima" {
			t.Errorf("Expected vmDriver 'colima', got: %v", result["vmDriver"])
		}
		if result["vmCores"] != float64(4) { // JSON unmarshaling converts to float64
			t.Errorf("Expected vmCores 4, got: %v", result["vmCores"])
		}
		if result["haEnabled"] != true {
			t.Errorf("Expected haEnabled true, got: %v", result["haEnabled"])
		}
		if result["nodeIp"] != "10.0.1.100" {
			t.Errorf("Expected nodeIp '10.0.1.100', got: %v", result["nodeIp"])
		}

		// Verify cluster object
		cluster, ok := result["cluster"].(map[string]any)
		if !ok {
			t.Errorf("Expected cluster to be object, got: %T", result["cluster"])
		} else {
			if cluster["ha"] == nil {
				t.Error("Expected cluster to contain ha config")
			}
		}

		// Verify tags array
		tags, ok := result["tags"].([]any)
		if !ok {
			t.Errorf("Expected tags to be array, got: %T", result["tags"])
		} else {
			if len(tags) != 2 {
				t.Errorf("Expected tags array length 2, got: %d", len(tags))
			}
		}

		// Verify generic helpers work
		if result["localValue"] != "value" {
			t.Errorf("Expected localValue 'value', got: %v", result["localValue"])
		}
		if result["localInt"] != float64(42) {
			t.Errorf("Expected localInt 42, got: %v", result["localInt"])
		}

		// Verify path access works
		if result["pathValue"] != "found" {
			t.Errorf("Expected pathValue 'found', got: %v", result["pathValue"])
		}

		// Verify existence checking
		if result["hasVm"] != true {
			t.Errorf("Expected hasVm true, got: %v", result["hasVm"])
		}
		if result["hasNonexistent"] != false {
			t.Errorf("Expected hasNonexistent false, got: %v", result["hasNonexistent"])
		}
	})

	t.Run("HelpersHandleNestedPathsCorrectly", func(t *testing.T) {
		// Given a jsonnet template
		template, mocks := setup(t)

		// And mock config handler returns nested test data
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte(`
deeply:
  nested:
    object:
      value: "found"
      number: 123
      enabled: true
partial:
  path: "exists"
`), nil
		}

		// When processing a template that tests nested path navigation
		templateContent := `
local helpers = std.extVar("helpers");
local context = std.extVar("context");
{
  deepValue: helpers.getString(context, "deeply.nested.object.value", "not-found"),
  deepNumber: helpers.getInt(context, "deeply.nested.object.number", 0),
  deepBool: helpers.getBool(context, "deeply.nested.object.enabled", false),
  partialPath: helpers.getString(context, "partial.path", "missing"),
  missingDeepPath: helpers.getString(context, "deeply.nested.missing.value", "default"),
  totallyMissing: helpers.getString(context, "not.there.at.all", "default"),
}`

		result, err := template.processJsonnetTemplate(templateContent)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And nested paths should be resolved correctly
		if result["deepValue"] != "found" {
			t.Errorf("Expected deepValue 'found', got: %v", result["deepValue"])
		}
		if result["deepNumber"] != float64(123) {
			t.Errorf("Expected deepNumber 123, got: %v", result["deepNumber"])
		}
		if result["deepBool"] != true {
			t.Errorf("Expected deepBool true, got: %v", result["deepBool"])
		}
		if result["partialPath"] != "exists" {
			t.Errorf("Expected partialPath 'exists', got: %v", result["partialPath"])
		}
		if result["missingDeepPath"] != "default" {
			t.Errorf("Expected missingDeepPath 'default', got: %v", result["missingDeepPath"])
		}
		if result["totallyMissing"] != "default" {
			t.Errorf("Expected totallyMissing 'default', got: %v", result["totallyMissing"])
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

func TestJsonnetTemplate_urlHelpers(t *testing.T) {
	setup := func(t *testing.T) (*JsonnetTemplate, *Mocks) {
		t.Helper()
		mocks, template := setupJsonnetTemplateMocks(t)
		err := template.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize template: %v", err)
		}
		return template, mocks
	}

	t.Run("ExtractBaseUrlFromHttpsEndpoint", func(t *testing.T) {
		templateContent := `
local helpers = std.extVar("helpers");
{
  baseUrl: helpers.baseUrl("https://api.example.com:6443")
}`

		template, mocks := setup(t)
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("contexts:\n  mock-context: {}"), nil
		}

		result, err := template.processJsonnetTemplate(templateContent)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["baseUrl"] != "api.example.com" {
			t.Errorf("Expected baseUrl 'api.example.com', got: %v", result["baseUrl"])
		}
	})

	t.Run("ExtractBaseUrlFromHttpEndpoint", func(t *testing.T) {
		templateContent := `
local helpers = std.extVar("helpers");
{
  baseUrl: helpers.baseUrl("http://localhost:8080")
}`

		template, mocks := setup(t)
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("contexts:\n  mock-context: {}"), nil
		}

		result, err := template.processJsonnetTemplate(templateContent)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["baseUrl"] != "localhost" {
			t.Errorf("Expected baseUrl 'localhost', got: %v", result["baseUrl"])
		}
	})

	t.Run("ExtractBaseUrlFromPlainHost", func(t *testing.T) {
		templateContent := `
local helpers = std.extVar("helpers");
{
  baseUrl: helpers.baseUrl("example.com:9000")
}`

		template, mocks := setup(t)
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("contexts:\n  mock-context: {}"), nil
		}

		result, err := template.processJsonnetTemplate(templateContent)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["baseUrl"] != "example.com" {
			t.Errorf("Expected baseUrl 'example.com', got: %v", result["baseUrl"])
		}
	})

	t.Run("ExtractBaseUrlFromHostWithoutPort", func(t *testing.T) {
		templateContent := `
local helpers = std.extVar("helpers");
{
  baseUrl: helpers.baseUrl("example.com")
}`

		template, mocks := setup(t)
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("contexts:\n  mock-context: {}"), nil
		}

		result, err := template.processJsonnetTemplate(templateContent)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["baseUrl"] != "example.com" {
			t.Errorf("Expected baseUrl 'example.com', got: %v", result["baseUrl"])
		}
	})

	t.Run("ExtractBaseUrlFromEmptyString", func(t *testing.T) {
		templateContent := `
local helpers = std.extVar("helpers");
{
  baseUrl: helpers.baseUrl("")
}`

		template, mocks := setup(t)
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("contexts:\n  mock-context: {}"), nil
		}

		result, err := template.processJsonnetTemplate(templateContent)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["baseUrl"] != "" {
			t.Errorf("Expected baseUrl '', got: %v", result["baseUrl"])
		}
	})
}

func TestJsonnetTemplate_typeValidation(t *testing.T) {
	setup := func(t *testing.T) (*JsonnetTemplate, *Mocks) {
		t.Helper()
		mocks, template := setupJsonnetTemplateMocks(t)
		err := template.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize template: %v", err)
		}
		return template, mocks
	}

	t.Run("GetStringSuccess", func(t *testing.T) {
		templateContent := `
local helpers = std.extVar("helpers");
local context = std.extVar("context");
{
  provider: helpers.getString(context, "provider", "default")
}`

		template, mocks := setup(t)
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("provider: aws"), nil
		}

		result, err := template.processJsonnetTemplate(templateContent)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["provider"] != "aws" {
			t.Errorf("Expected provider 'aws', got: %v", result["provider"])
		}
	})

	t.Run("GetStringMissingUsesDefault", func(t *testing.T) {
		templateContent := `
local helpers = std.extVar("helpers");
local context = std.extVar("context");
{
  provider: helpers.getString(context, "provider", "default")
}`

		template, mocks := setup(t)
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("{}"), nil
		}

		result, err := template.processJsonnetTemplate(templateContent)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["provider"] != "default" {
			t.Errorf("Expected provider 'default', got: %v", result["provider"])
		}
	})

	t.Run("GetStringWrongTypeThrowsError", func(t *testing.T) {
		templateContent := `
local helpers = std.extVar("helpers");
local context = std.extVar("context");
{
  provider: helpers.getString(context, "provider", "default")
}`

		template, mocks := setup(t)
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("provider: 123"), nil
		}

		_, err := template.processJsonnetTemplate(templateContent)

		if err == nil {
			t.Error("Expected error for wrong type, got none")
		}

		if !strings.Contains(err.Error(), "Expected string for 'provider' but got number") {
			t.Errorf("Expected type error message, got: %v", err)
		}
	})

	t.Run("GetIntWrongTypeThrowsError", func(t *testing.T) {
		templateContent := `
local helpers = std.extVar("helpers");
local context = std.extVar("context");
{
  cores: helpers.getInt(context, "vm.cores", 2)
}`

		template, mocks := setup(t)
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("vm:\n  cores: \"not-a-number\""), nil
		}

		_, err := template.processJsonnetTemplate(templateContent)

		if err == nil {
			t.Error("Expected error for wrong type, got none")
		}

		if !strings.Contains(err.Error(), "Expected number for 'vm.cores' but got string") {
			t.Errorf("Expected type error message, got: %v", err)
		}
	})

	t.Run("GetBoolWrongTypeThrowsError", func(t *testing.T) {
		templateContent := `
local helpers = std.extVar("helpers");
local context = std.extVar("context");
{
  enabled: helpers.getBool(context, "feature.enabled", false)
}`

		template, mocks := setup(t)
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("feature:\n  enabled: \"yes\""), nil
		}

		_, err := template.processJsonnetTemplate(templateContent)

		if err == nil {
			t.Error("Expected error for wrong type, got none")
		}

		if !strings.Contains(err.Error(), "Expected boolean for 'feature.enabled' but got string") {
			t.Errorf("Expected type error message, got: %v", err)
		}
	})

	t.Run("GetObjectWrongTypeThrowsError", func(t *testing.T) {
		templateContent := `
local helpers = std.extVar("helpers");
local context = std.extVar("context");
{
  cluster: helpers.getObject(context, "cluster", {})
}`

		template, mocks := setup(t)
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("cluster: \"not-an-object\""), nil
		}

		_, err := template.processJsonnetTemplate(templateContent)

		if err == nil {
			t.Error("Expected error for wrong type, got none")
		}

		if !strings.Contains(err.Error(), "Expected object for 'cluster' but got string") {
			t.Errorf("Expected type error message, got: %v", err)
		}
	})

	t.Run("GetArrayWrongTypeThrowsError", func(t *testing.T) {
		templateContent := `
local helpers = std.extVar("helpers");
local context = std.extVar("context");
{
  tags: helpers.getArray(context, "tags", [])
}`

		template, mocks := setup(t)
		mocks.ConfigHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return []byte("tags: \"not-an-array\""), nil
		}

		_, err := template.processJsonnetTemplate(templateContent)

		if err == nil {
			t.Error("Expected error for wrong type, got none")
		}

		if !strings.Contains(err.Error(), "Expected array for 'tags' but got string") {
			t.Errorf("Expected type error message, got: %v", err)
		}
	})
}

func TestJsonnetTemplate_extractValuesReferences(t *testing.T) {
	setup := func(t *testing.T) (*JsonnetTemplate, *Mocks) {
		t.Helper()
		mocks, template := setupJsonnetTemplateMocks(t)
		return template, mocks
	}

	t.Run("EmptyRenderedData", func(t *testing.T) {
		// Given a template and empty rendered data
		template, _ := setup(t)

		// When extracting values references
		result := template.extractValuesReferences(map[string]any{})

		// Then should return empty slice
		if len(result) != 0 {
			t.Errorf("expected empty slice, got %d items", len(result))
		}
	})

	t.Run("MissingBlueprint", func(t *testing.T) {
		// Given rendered data without blueprint
		template, _ := setup(t)
		renderedData := map[string]any{
			"other": "data",
		}

		// When extracting values references
		result := template.extractValuesReferences(renderedData)

		// Then should return empty slice
		if len(result) != 0 {
			t.Errorf("expected empty slice, got %d items", len(result))
		}
	})

	t.Run("BlueprintNotMap", func(t *testing.T) {
		// Given rendered data with blueprint as string
		template, _ := setup(t)
		renderedData := map[string]any{
			"blueprint": "not a map",
		}

		// When extracting values references
		result := template.extractValuesReferences(renderedData)

		// Then should return empty slice
		if len(result) != 0 {
			t.Errorf("expected empty slice, got %d items", len(result))
		}
	})

	t.Run("MissingKustomize", func(t *testing.T) {
		// Given rendered data with blueprint but no kustomize
		template, _ := setup(t)
		renderedData := map[string]any{
			"blueprint": map[string]any{
				"other": "data",
			},
		}

		// When extracting values references
		result := template.extractValuesReferences(renderedData)

		// Then should return empty slice
		if len(result) != 0 {
			t.Errorf("expected empty slice, got %d items", len(result))
		}
	})

	t.Run("KustomizeNotArray", func(t *testing.T) {
		// Given rendered data with kustomize as string
		template, _ := setup(t)
		renderedData := map[string]any{
			"blueprint": map[string]any{
				"kustomize": "not an array",
			},
		}

		// When extracting values references
		result := template.extractValuesReferences(renderedData)

		// Then should return empty slice
		if len(result) != 0 {
			t.Errorf("expected empty slice, got %d items", len(result))
		}
	})

	t.Run("ValidBlueprintWithKustomize", func(t *testing.T) {
		// Given valid rendered data with kustomize array
		template, mocks := setup(t)

		// Initialize the template first
		_ = template.Initialize()

		renderedData := map[string]any{
			"blueprint": map[string]any{
				"kustomize": []any{
					map[string]any{"name": "test"},
				},
			},
		}

		// Mock shell to return project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		// Mock template shims for directory operations
		template.shims.ReadDir = func(name string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				&mockDirEntry{name: "ingress", isDir: true},
				&mockDirEntry{name: "database", isDir: true},
				&mockDirEntry{name: "file.txt", isDir: false},
			}, nil
		}
		template.shims.Stat = func(name string) (os.FileInfo, error) {
			// Return success for values.jsonnet files
			if strings.HasSuffix(name, "values.jsonnet") {
				return &mockFileInfo{name: "values.jsonnet", isDir: false}, nil
			}
			return nil, fmt.Errorf("file not found")
		}

		// When extracting values references
		result := template.extractValuesReferences(renderedData)

		// Then should include global values and discovered component values
		expected := []string{
			"kustomize/values.jsonnet",
			"kustomize/ingress/values.jsonnet",
			"kustomize/database/values.jsonnet",
		}
		if len(result) != len(expected) {
			t.Errorf("expected %d items, got %d", len(expected), len(result))
		}
		for i, expectedPath := range expected {
			if i >= len(result) {
				t.Errorf("missing expected path: %s", expectedPath)
				continue
			}
			if result[i] != expectedPath {
				t.Errorf("expected path %s at index %d, got %s", expectedPath, i, result[i])
			}
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given valid rendered data but shell error
		template, mocks := setup(t)

		// Initialize the template first
		_ = template.Initialize()

		renderedData := map[string]any{
			"blueprint": map[string]any{
				"kustomize": []any{
					map[string]any{"name": "test"},
				},
			},
		}

		// Mock shell to return error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}

		// When extracting values references
		result := template.extractValuesReferences(renderedData)

		// Then should only include global values (no discovery)
		expected := []string{"kustomize/values.jsonnet"}
		if len(result) != len(expected) {
			t.Errorf("expected %d items, got %d", len(expected), len(result))
		}
		if len(result) > 0 && result[0] != expected[0] {
			t.Errorf("expected path %s, got %s", expected[0], result[0])
		}
	})

	t.Run("ReadDirError", func(t *testing.T) {
		// Given valid rendered data but directory read error
		template, mocks := setup(t)

		// Initialize the template first
		_ = template.Initialize()

		renderedData := map[string]any{
			"blueprint": map[string]any{
				"kustomize": []any{
					map[string]any{"name": "test"},
				},
			},
		}

		// Mock shell to return project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		// Mock template shims to return error for ReadDir
		template.shims.ReadDir = func(name string) ([]os.DirEntry, error) {
			return nil, fmt.Errorf("read dir error")
		}

		// When extracting values references
		result := template.extractValuesReferences(renderedData)

		// Then should only include global values (no discovery)
		expected := []string{"kustomize/values.jsonnet"}
		if len(result) != len(expected) {
			t.Errorf("expected %d items, got %d", len(expected), len(result))
		}
		if len(result) > 0 && result[0] != expected[0] {
			t.Errorf("expected path %s, got %s", expected[0], result[0])
		}
	})

	t.Run("ComponentValuesFileNotExists", func(t *testing.T) {
		// Given valid rendered data but component values file doesn't exist
		template, mocks := setup(t)

		// Initialize the template first
		_ = template.Initialize()

		renderedData := map[string]any{
			"blueprint": map[string]any{
				"kustomize": []any{
					map[string]any{"name": "test"},
				},
			},
		}

		// Mock shell to return project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		// Mock template shims for directory operations
		template.shims.ReadDir = func(name string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				&mockDirEntry{name: "ingress", isDir: true},
			}, nil
		}
		template.shims.Stat = func(name string) (os.FileInfo, error) {
			// Return error for all values.jsonnet files (they don't exist)
			return nil, fmt.Errorf("file not found")
		}

		// When extracting values references
		result := template.extractValuesReferences(renderedData)

		// Then should only include global values (no component values found)
		expected := []string{"kustomize/values.jsonnet"}
		if len(result) != len(expected) {
			t.Errorf("expected %d items, got %d", len(expected), len(result))
		}
		if len(result) > 0 && result[0] != expected[0] {
			t.Errorf("expected path %s, got %s", expected[0], result[0])
		}
	})

	t.Run("MixedComponents", func(t *testing.T) {
		// Given valid rendered data with mixed components (some with values, some without)
		template, mocks := setup(t)

		// Initialize the template first
		_ = template.Initialize()

		renderedData := map[string]any{
			"blueprint": map[string]any{
				"kustomize": []any{
					map[string]any{"name": "test"},
				},
			},
		}

		// Mock shell to return project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		// Mock template shims for directory operations
		template.shims.ReadDir = func(name string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				&mockDirEntry{name: "ingress", isDir: true},
				&mockDirEntry{name: "database", isDir: true},
				&mockDirEntry{name: "api", isDir: true},
			}, nil
		}
		template.shims.Stat = func(name string) (os.FileInfo, error) {
			// Only ingress and database have values.jsonnet files
			// Use filepath operations to handle cross-platform path separators
			if strings.Contains(name, filepath.Join("ingress", "values.jsonnet")) || strings.Contains(name, filepath.Join("database", "values.jsonnet")) {
				return &mockFileInfo{name: "values.jsonnet", isDir: false}, nil
			}
			return nil, fmt.Errorf("file not found")
		}

		// When extracting values references
		result := template.extractValuesReferences(renderedData)

		// Then should include global values and only existing component values
		expected := []string{
			"kustomize/values.jsonnet",
			"kustomize/ingress/values.jsonnet",
			"kustomize/database/values.jsonnet",
		}
		if len(result) != len(expected) {
			t.Errorf("expected %d items, got %d", len(expected), len(result))
		}
		for i, expectedPath := range expected {
			if i >= len(result) {
				t.Errorf("missing expected path: %s", expectedPath)
				continue
			}
			if result[i] != expectedPath {
				t.Errorf("expected path %s at index %d, got %s", expectedPath, i, result[i])
			}
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

type mockDirEntry struct {
	name  string
	isDir bool
}

func (m *mockDirEntry) Name() string {
	return m.name
}

func (m *mockDirEntry) IsDir() bool {
	return m.isDir
}

func (m *mockDirEntry) Type() os.FileMode {
	if m.isDir {
		return os.ModeDir
	}
	return 0
}

func (m *mockDirEntry) Info() (os.FileInfo, error) {
	return &mockFileInfo{name: m.name, isDir: m.isDir}, nil
}

type mockFileInfo struct {
	name  string
	isDir bool
}

func (m *mockFileInfo) Name() string {
	return m.name
}

func (m *mockFileInfo) Size() int64 {
	return 0
}

func (m *mockFileInfo) Mode() os.FileMode {
	if m.isDir {
		return os.ModeDir
	}
	return 0
}

func (m *mockFileInfo) ModTime() time.Time {
	return time.Now()
}

func (m *mockFileInfo) IsDir() bool {
	return m.isDir
}

func (m *mockFileInfo) Sys() interface{} {
	return nil
}

func TestJsonnetTemplate_processTemplate(t *testing.T) {
	setup := func(t *testing.T) (*JsonnetTemplate, *Mocks) {
		t.Helper()
		mocks, template := setupJsonnetTemplateMocks(t)
		_ = template.Initialize()
		return template, mocks
	}

	t.Run("BlueprintJsonnet", func(t *testing.T) {
		// Given a template and blueprint.jsonnet content
		template, _ := setup(t)
		templateData := map[string][]byte{
			"blueprint.jsonnet": []byte(`{ kustomize: [{ name: "test" }] }`),
		}
		renderedData := map[string]any{}

		// Mock jsonnet processing
		template.shims.NewJsonnetVM = func() JsonnetVM {
			return &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					return `{"kustomize":[{"name":"test"}]}`, nil
				},
			}
		}

		// When processing blueprint.jsonnet
		err := template.processTemplate("blueprint.jsonnet", templateData, renderedData)

		// Then should succeed and add blueprint data
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if renderedData["blueprint"] == nil {
			t.Error("expected blueprint data to be added")
		}
	})

	t.Run("TerraformJsonnet", func(t *testing.T) {
		// Given a template and terraform jsonnet content
		template, _ := setup(t)
		templateData := map[string][]byte{
			"terraform/main.jsonnet": []byte(`{ region: "us-west-2" }`),
		}
		renderedData := map[string]any{}

		// Mock jsonnet processing
		template.shims.NewJsonnetVM = func() JsonnetVM {
			return &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					return `{"region":"us-west-2"}`, nil
				},
			}
		}

		// When processing terraform/main.jsonnet
		err := template.processTemplate("terraform/main.jsonnet", templateData, renderedData)

		// Then should succeed and add terraform data
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if renderedData["terraform/main"] == nil {
			t.Error("expected terraform data to be added")
		}
	})

	t.Run("KustomizePatchJsonnet", func(t *testing.T) {
		// Given a template and kustomize patch content
		template, _ := setup(t)
		templateData := map[string][]byte{
			"kustomize/ingress/patch.jsonnet": []byte(`{ apiVersion: "v1", kind: "ConfigMap" }`),
		}
		renderedData := map[string]any{}

		// Mock jsonnet processing
		template.shims.NewJsonnetVM = func() JsonnetVM {
			return &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					return `{"apiVersion":"v1","kind":"ConfigMap"}`, nil
				},
			}
		}

		// When processing kustomize patch
		err := template.processTemplate("kustomize/ingress/patch.jsonnet", templateData, renderedData)

		// Then should succeed and add kustomize data
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if renderedData["kustomize/ingress/patch"] == nil {
			t.Error("expected kustomize patch data to be added")
		}
	})

	t.Run("KustomizeValuesGlobal", func(t *testing.T) {
		// Given a template and global values content
		template, _ := setup(t)
		templateData := map[string][]byte{
			"kustomize/values.jsonnet": []byte(`{ domain: "example.com" }`),
		}
		renderedData := map[string]any{}

		// Mock jsonnet processing
		template.shims.NewJsonnetVM = func() JsonnetVM {
			return &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					return `{"domain":"example.com"}`, nil
				},
			}
		}

		// When processing global values
		err := template.processTemplate("kustomize/values.jsonnet", templateData, renderedData)

		// Then should succeed and add values/global data
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if renderedData["values/global"] == nil {
			t.Error("expected values/global data to be added")
		}
	})

	t.Run("KustomizeValuesComponent", func(t *testing.T) {
		// Given a template and component values content
		template, _ := setup(t)
		templateData := map[string][]byte{
			"kustomize/ingress/values.jsonnet": []byte(`{ host: "example.com" }`),
		}
		renderedData := map[string]any{}

		// Mock jsonnet processing
		template.shims.NewJsonnetVM = func() JsonnetVM {
			return &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					return `{"host":"example.com"}`, nil
				},
			}
		}

		// When processing component values
		err := template.processTemplate("kustomize/ingress/values.jsonnet", templateData, renderedData)

		// Then should succeed and add values/ingress data
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if renderedData["values/ingress"] == nil {
			t.Error("expected values/ingress data to be added")
		}
	})

	t.Run("KustomizeValuesComponentWithValuesSubdirectory", func(t *testing.T) {
		// Given a template and component values in a "values" subdirectory
		template, _ := setup(t)
		templateData := map[string][]byte{
			"kustomize/values/values.jsonnet": []byte(`{ global: "config" }`),
		}
		renderedData := map[string]any{}

		// Mock jsonnet processing
		template.shims.NewJsonnetVM = func() JsonnetVM {
			return &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					return `{"global":"config"}`, nil
				},
			}
		}

		// When processing values subdirectory
		err := template.processTemplate("kustomize/values/values.jsonnet", templateData, renderedData)

		// Then should succeed and add values/global data (special case)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if renderedData["values/global"] == nil {
			t.Error("expected values/global data to be added")
		}
	})

	t.Run("ValuesJsonnet", func(t *testing.T) {
		// Given a template and values.jsonnet content
		template, _ := setup(t)
		templateData := map[string][]byte{
			"values/global.jsonnet": []byte(`{ config: "value" }`),
		}
		renderedData := map[string]any{}

		// Mock jsonnet processing
		template.shims.NewJsonnetVM = func() JsonnetVM {
			return &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					return `{"config":"value"}`, nil
				},
			}
		}

		// When processing values.jsonnet
		err := template.processTemplate("values/global.jsonnet", templateData, renderedData)

		// Then should succeed and add values data
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if renderedData["values/global"] == nil {
			t.Error("expected values data to be added")
		}
	})

	t.Run("UnsupportedPath", func(t *testing.T) {
		// Given a template and unsupported path
		template, _ := setup(t)
		templateData := map[string][]byte{
			"unsupported/path.jsonnet": []byte(`{ data: "value" }`),
		}
		renderedData := map[string]any{}

		// When processing unsupported path
		err := template.processTemplate("unsupported/path.jsonnet", templateData, renderedData)

		// Then should return nil (no error, just ignored)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(renderedData) != 0 {
			t.Error("expected no data to be added for unsupported path")
		}
	})

	t.Run("MissingTemplateData", func(t *testing.T) {
		// Given a template and missing template data
		template, _ := setup(t)
		templateData := map[string][]byte{}
		renderedData := map[string]any{}

		// When processing missing template
		err := template.processTemplate("blueprint.jsonnet", templateData, renderedData)

		// Then should return nil (no error, just ignored)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(renderedData) != 0 {
			t.Error("expected no data to be added for missing template")
		}
	})

	t.Run("JsonnetProcessingError", func(t *testing.T) {
		// Given a template and blueprint content
		template, _ := setup(t)
		templateData := map[string][]byte{
			"blueprint.jsonnet": []byte(`{ invalid: jsonnet }`),
		}
		renderedData := map[string]any{}

		// Mock jsonnet processing to fail
		template.shims.NewJsonnetVM = func() JsonnetVM {
			return &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					return "", fmt.Errorf("jsonnet processing error")
				},
			}
		}

		// When processing blueprint with jsonnet error
		err := template.processTemplate("blueprint.jsonnet", templateData, renderedData)

		// Then should return error
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to process template") {
			t.Errorf("expected error about template processing, got: %v", err)
		}
	})

	t.Run("KustomizeValuesComplexPath", func(t *testing.T) {
		// Given a template and complex kustomize values path
		template, _ := setup(t)
		templateData := map[string][]byte{
			"kustomize/ingress/nginx/values.jsonnet": []byte(`{ port: 80 }`),
		}
		renderedData := map[string]any{}

		// Mock jsonnet processing
		template.shims.NewJsonnetVM = func() JsonnetVM {
			return &mockJsonnetVM{
				EvaluateFunc: func(filename, snippet string) (string, error) {
					return `{"port":80}`, nil
				},
			}
		}

		// When processing complex kustomize values path
		err := template.processTemplate("kustomize/ingress/nginx/values.jsonnet", templateData, renderedData)

		// Then should succeed and add the full path as key
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if renderedData["kustomize/ingress/nginx/values"] == nil {
			t.Error("expected complex path data to be added")
		}
	})
}
