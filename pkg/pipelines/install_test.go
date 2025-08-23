package pipelines

import (
	"context"
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Mock Types
// =============================================================================

type MockTemplate struct {
	ProcessCalled bool
	ProcessFunc   func(templateData map[string][]byte, renderedData map[string]any) error
}

func (m *MockTemplate) Initialize() error { return nil }
func (m *MockTemplate) Process(templateData map[string][]byte, renderedData map[string]any) error {
	m.ProcessCalled = true
	if m.ProcessFunc != nil {
		return m.ProcessFunc(templateData, renderedData)
	}
	return nil
}

type MockGenerator struct {
	GenerateCalled bool
	GenerateFunc   func(data map[string]any, overwrite ...bool) error
}

func (m *MockGenerator) Initialize() error { return nil }
func (m *MockGenerator) Generate(data map[string]any, overwrite ...bool) error {
	m.GenerateCalled = true
	if m.GenerateFunc != nil {
		return m.GenerateFunc(data, overwrite...)
	}
	return nil
}

// =============================================================================
// Test Setup
// =============================================================================

type InstallMocks struct {
	*Mocks
	BlueprintHandler *blueprint.MockBlueprintHandler
}

func setupInstallMocks(t *testing.T, opts ...*SetupOptions) *InstallMocks {
	t.Helper()

	// Create setup options, preserving any provided options
	setupOptions := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		setupOptions = opts[0]
	}

	baseMocks := setupMocks(t, setupOptions)

	// Setup blueprint handler mock
	mockBlueprintHandler := blueprint.NewMockBlueprintHandler(baseMocks.Injector)
	mockBlueprintHandler.InitializeFunc = func() error { return nil }
	mockBlueprintHandler.InstallFunc = func() error { return nil }
	mockBlueprintHandler.WaitForKustomizationsFunc = func(message string, names ...string) error { return nil }
	baseMocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

	// Add artifact builder mock for generators
	artifactBuilder := artifact.NewMockArtifact()
	artifactBuilder.InitializeFunc = func(injector di.Injector) error { return nil }
	baseMocks.Injector.Register("artifactBuilder", artifactBuilder)

	return &InstallMocks{
		Mocks:            baseMocks,
		BlueprintHandler: mockBlueprintHandler,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewInstallPipeline(t *testing.T) {
	t.Run("CreatesNewInstallPipeline", func(t *testing.T) {
		// When creating a new InstallPipeline
		pipeline := NewInstallPipeline()

		// Then it should not be nil
		if pipeline == nil {
			t.Error("Expected pipeline to not be nil")
		}

		// And it should be of the correct type
		if pipeline == nil {
			t.Error("Expected pipeline to be of type *InstallPipeline")
		}
	})
}

// =============================================================================
// Test Initialize
// =============================================================================

func TestInstallPipeline_Initialize(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*InstallPipeline, *InstallMocks) {
		t.Helper()
		pipeline := NewInstallPipeline()
		mocks := setupInstallMocks(t, opts...)
		return pipeline, mocks
	}

	t.Run("InitializesSuccessfully", func(t *testing.T) {
		// Given a new InstallPipeline
		pipeline, mocks := setup(t)

		// When Initialize is called
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And blueprint handler should be set
		if pipeline.blueprintHandler == nil {
			t.Error("Expected blueprint handler to be set")
		}
	})

	t.Run("ReturnsErrorWhenBasePipelineInitializeFails", func(t *testing.T) {
		// Given a pipeline with failing base initialization
		pipeline, mocks := setup(t)

		// Override shell to return error during initialization
		mocks.Shell.InitializeFunc = func() error {
			return fmt.Errorf("shell init failed")
		}

		// When Initialize is called
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to initialize shell: shell init failed" {
			t.Errorf("Expected shell init error, got %q", err.Error())
		}
	})

	t.Run("ReturnsErrorWhenBlueprintHandlerInitializeFails", func(t *testing.T) {
		// Given a pipeline with failing blueprint handler initialization
		pipeline, mocks := setup(t)

		// Override blueprint handler to return error during initialization
		mocks.BlueprintHandler.InitializeFunc = func() error {
			return fmt.Errorf("blueprint handler init failed")
		}

		// When Initialize is called
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to initialize blueprint handler: blueprint handler init failed" {
			t.Errorf("Expected blueprint handler init error, got %q", err.Error())
		}
	})
}

// =============================================================================
// Test Execute
// =============================================================================

func TestInstallPipeline_Execute(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*InstallPipeline, *InstallMocks) {
		t.Helper()
		pipeline := NewInstallPipeline()
		mocks := setupInstallMocks(t, opts...)

		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		return pipeline, mocks
	}

	t.Run("ExecutesSuccessfully", func(t *testing.T) {
		// Given a properly initialized InstallPipeline
		pipeline, _ := setup(t)

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ExecutesWithWaitFlag", func(t *testing.T) {
		// Given a pipeline with wait flag set
		pipeline, mocks := setup(t)

		waitCalled := false
		mocks.BlueprintHandler.WaitForKustomizationsFunc = func(message string, names ...string) error {
			waitCalled = true
			return nil
		}

		ctx := context.WithValue(context.Background(), "wait", true)

		// When Execute is called
		err := pipeline.Execute(ctx)

		// Then no error should be returned and wait should be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !waitCalled {
			t.Error("Expected blueprint wait to be called")
		}
	})

	t.Run("ReturnsErrorWhenConfigNotLoaded", func(t *testing.T) {
		// Given a mock config handler that returns not loaded
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.IsLoadedFunc = func() bool { return false }
		mockConfigHandler.InitializeFunc = func() error { return nil }
		mockConfigHandler.GetContextFunc = func() string { return "mock-context" }
		mockConfigHandler.SetContextFunc = func(context string) error { return nil }

		// Setup with the not-loaded config handler
		pipeline, _ := setup(t, &SetupOptions{ConfigHandler: mockConfigHandler})

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Nothing to install. Have you run \033[1mwindsor init\033[0m?" {
			t.Errorf("Expected config not loaded error, got %q", err.Error())
		}
	})

	t.Run("ReturnsErrorWhenNoBlueprintHandler", func(t *testing.T) {
		// Given a pipeline with nil blueprint handler
		pipeline, _ := setup(t)

		// Set blueprint handler to nil
		pipeline.blueprintHandler = nil

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "No blueprint handler found" {
			t.Errorf("Expected no blueprint handler error, got %q", err.Error())
		}
	})

	t.Run("ReturnsErrorWhenBlueprintInstallFails", func(t *testing.T) {
		// Given a pipeline with failing blueprint install
		pipeline, mocks := setup(t)

		// Override blueprint handler to return error during install
		mocks.BlueprintHandler.InstallFunc = func() error {
			return fmt.Errorf("blueprint install failed")
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error installing blueprint: blueprint install failed" {
			t.Errorf("Expected blueprint install error, got %q", err.Error())
		}
	})

	t.Run("ReturnsErrorWhenBlueprintWaitFails", func(t *testing.T) {
		// Given a pipeline with failing blueprint wait
		pipeline, mocks := setup(t)

		// Override blueprint handler to return error during wait
		mocks.BlueprintHandler.WaitForKustomizationsFunc = func(message string, names ...string) error {
			return fmt.Errorf("blueprint wait failed")
		}

		ctx := context.WithValue(context.Background(), "wait", true)

		// When Execute is called
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed waiting for kustomizations: blueprint wait failed" {
			t.Errorf("Expected blueprint wait error, got %q", err.Error())
		}
	})

	t.Run("LoadsBlueprintConfigBeforeInstall", func(t *testing.T) {
		// Given a pipeline with blueprint handler
		pipeline, mocks := setup(t)

		loadConfigCalled := false
		installCalled := false
		var callOrder []string

		// Track the order of method calls
		mocks.BlueprintHandler.LoadConfigFunc = func() error {
			loadConfigCalled = true
			callOrder = append(callOrder, "LoadConfig")
			return nil
		}

		mocks.BlueprintHandler.InstallFunc = func() error {
			installCalled = true
			callOrder = append(callOrder, "Install")
			return nil
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And LoadConfig should be called before Install
		if !loadConfigCalled {
			t.Error("Expected LoadConfig to be called")
		}
		if !installCalled {
			t.Error("Expected Install to be called")
		}

		// And LoadConfig should be called before Install
		if len(callOrder) != 2 {
			t.Errorf("Expected 2 method calls, got %d", len(callOrder))
		}
		if callOrder[0] != "LoadConfig" {
			t.Errorf("Expected LoadConfig to be called first, got %s", callOrder[0])
		}
		if callOrder[1] != "Install" {
			t.Errorf("Expected Install to be called second, got %s", callOrder[1])
		}
	})

	t.Run("ReturnsErrorWhenBlueprintLoadConfigFails", func(t *testing.T) {
		// Given a pipeline with failing blueprint LoadConfig
		pipeline, mocks := setup(t)

		// Override blueprint handler to return error during LoadConfig
		mocks.BlueprintHandler.LoadConfigFunc = func() error {
			return fmt.Errorf("blueprint load config failed")
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error loading blueprint config: blueprint load config failed" {
			t.Errorf("Expected blueprint load config error, got %q", err.Error())
		}
	})

	t.Run("ProcessesTemplateDataSuccessfully", func(t *testing.T) {
		// Given a pipeline with template data
		pipeline, mocks := setup(t)

		// Mock template renderer to return test data
		mockTemplateRenderer := &MockTemplate{}
		mockTemplateRenderer.ProcessFunc = func(templateData map[string][]byte, renderedData map[string]any) error {
			t.Log("Template renderer Process called")
			renderedData["kustomize/values"] = map[string]any{
				"common": map[string]any{
					"domain": "test.com",
				},
			}
			return nil
		}
		// Register the mock template renderer in the injector BEFORE initialization
		mocks.Injector.Register("templateRenderer", mockTemplateRenderer)

		// Initialize the pipeline to set up generators
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Mock blueprint handler to return template data
		mocks.BlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			t.Log("GetLocalTemplateData called")
			return map[string][]byte{
				"kustomize/values.jsonnet": []byte(`{"common": {"domain": "test.com"}}`),
			}, nil
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And template processing should be called
		if !mockTemplateRenderer.ProcessCalled {
			t.Errorf("Expected template processing to be called. Config loaded: %v, Blueprint handler: %v", pipeline.configHandler.IsLoaded(), pipeline.blueprintHandler != nil)
		}
	})

	t.Run("ReturnsErrorWhenTemplateProcessingFails", func(t *testing.T) {
		// Given a pipeline with failing template processing
		pipeline, mocks := setup(t)

		// Mock template renderer to return error
		mockTemplateRenderer := &MockTemplate{}
		mockTemplateRenderer.ProcessFunc = func(templateData map[string][]byte, renderedData map[string]any) error {
			return fmt.Errorf("template processing failed")
		}
		// Register the mock template renderer in the injector BEFORE initialization
		mocks.Injector.Register("templateRenderer", mockTemplateRenderer)

		// Initialize the pipeline to set up generators
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Mock blueprint handler to return template data
		mocks.BlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return map[string][]byte{
				"kustomize/values.jsonnet": []byte(`{"common": {"domain": "test.com"}}`),
			}, nil
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to process template data: failed to process template data: template processing failed" {
			t.Errorf("Expected template processing error, got %q", err.Error())
		}
	})

	t.Run("GeneratesKustomizeDataSuccessfully", func(t *testing.T) {
		// Given a pipeline with rendered data
		pipeline, mocks := setup(t)

		// Mock template renderer to return test data
		mockTemplateRenderer := &MockTemplate{}
		mockTemplateRenderer.ProcessFunc = func(templateData map[string][]byte, renderedData map[string]any) error {
			renderedData["kustomize/values"] = map[string]any{
				"common": map[string]any{
					"domain": "test.com",
				},
			}
			return nil
		}
		// Register the mock template renderer in the injector BEFORE initialization
		mocks.Injector.Register("templateRenderer", mockTemplateRenderer)

		// Initialize the pipeline to set up generators
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Mock blueprint handler to return template data
		mocks.BlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return map[string][]byte{
				"kustomize/values.jsonnet": []byte(`{"common": {"domain": "test.com"}}`),
			}, nil
		}

		// Track generator calls
		generatorCalled := false
		for i := range pipeline.generators {
			mockGenerator := &MockGenerator{}
			mockGenerator.GenerateFunc = func(data map[string]any, overwrite ...bool) error {
				generatorCalled = true
				return nil
			}
			pipeline.generators[i] = mockGenerator
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And generators should be called
		if !generatorCalled {
			t.Error("Expected generators to be called")
		}
	})

	t.Run("ReturnsErrorWhenGeneratorFails", func(t *testing.T) {
		// Given a pipeline with failing generator
		pipeline, mocks := setup(t)

		// Mock template renderer to return test data
		mockTemplateRenderer := &MockTemplate{}
		mockTemplateRenderer.ProcessFunc = func(templateData map[string][]byte, renderedData map[string]any) error {
			renderedData["kustomize/values"] = map[string]any{
				"common": map[string]any{
					"domain": "test.com",
				},
			}
			return nil
		}
		// Register the mock template renderer in the injector BEFORE initialization
		mocks.Injector.Register("templateRenderer", mockTemplateRenderer)

		// Initialize the pipeline to set up generators
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Mock blueprint handler to return template data
		mocks.BlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return map[string][]byte{
				"kustomize/values.jsonnet": []byte(`{"common": {"domain": "test.com"}}`),
			}, nil
		}

		// Mock generator to return error
		for i := range pipeline.generators {
			mockGenerator := &MockGenerator{}
			mockGenerator.GenerateFunc = func(data map[string]any, overwrite ...bool) error {
				return fmt.Errorf("generator failed")
			}
			pipeline.generators[i] = mockGenerator
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to generate from template data: generator failed" {
			t.Errorf("Expected generator error, got %q", err.Error())
		}
	})

	t.Run("SkipsGeneratorWhenNoRenderedData", func(t *testing.T) {
		// Given a pipeline with no rendered data
		pipeline, mocks := setup(t)

		// Mock template renderer to return empty data
		mockTemplateRenderer := &MockTemplate{}
		mockTemplateRenderer.ProcessFunc = func(templateData map[string][]byte, renderedData map[string]any) error {
			// Don't add any data to renderedData
			return nil
		}
		// Register the mock template renderer in the injector BEFORE initialization
		mocks.Injector.Register("templateRenderer", mockTemplateRenderer)

		// Initialize the pipeline to set up generators
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Mock blueprint handler to return template data
		mocks.BlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return map[string][]byte{
				"kustomize/values.jsonnet": []byte(`{"common": {"domain": "test.com"}}`),
			}, nil
		}

		// Track generator calls
		generatorCalled := false
		for i := range pipeline.generators {
			mockGenerator := &MockGenerator{}
			mockGenerator.GenerateFunc = func(data map[string]any, overwrite ...bool) error {
				generatorCalled = true
				return nil
			}
			pipeline.generators[i] = mockGenerator
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And generators should not be called
		if generatorCalled {
			t.Error("Expected generators to not be called when no rendered data")
		}
	})

	t.Run("PassesCorrectDataToGenerators", func(t *testing.T) {
		// Given a pipeline with specific rendered data
		pipeline, mocks := setup(t)

		expectedData := map[string]any{
			"kustomize/values": map[string]any{
				"common": map[string]any{
					"domain": "test.com",
				},
			},
		}

		// Mock template renderer to return specific data
		mockTemplateRenderer := &MockTemplate{}
		mockTemplateRenderer.ProcessFunc = func(templateData map[string][]byte, renderedData map[string]any) error {
			renderedData["kustomize/values"] = expectedData["kustomize/values"]
			return nil
		}
		// Register the mock template renderer in the injector BEFORE initialization
		mocks.Injector.Register("templateRenderer", mockTemplateRenderer)

		// Initialize the pipeline to set up generators
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Mock blueprint handler to return template data
		mocks.BlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return map[string][]byte{
				"kustomize/values.jsonnet": []byte(`{"common": {"domain": "test.com"}}`),
			}, nil
		}

		// Track data passed to generators
		var receivedData map[string]any
		for i := range pipeline.generators {
			mockGenerator := &MockGenerator{}
			mockGenerator.GenerateFunc = func(data map[string]any, overwrite ...bool) error {
				receivedData = data
				return nil
			}
			pipeline.generators[i] = mockGenerator
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And correct data should be passed to generators
		if receivedData == nil {
			t.Fatal("Expected data to be passed to generators")
		}

		// Check that the expected data structure is passed
		if kustomizeValues, exists := receivedData["kustomize/values"]; !exists {
			t.Error("Expected kustomize/values to be in passed data")
		} else if commonValues, exists := kustomizeValues.(map[string]any)["common"]; !exists {
			t.Error("Expected common values to be in kustomize/values")
		} else if domain, exists := commonValues.(map[string]any)["domain"]; !exists || domain != "test.com" {
			t.Errorf("Expected domain to be 'test.com', got %v", domain)
		}
	})

	t.Run("PassesCorrectOverwriteFlagToGenerators", func(t *testing.T) {
		// Given a pipeline with rendered data
		pipeline, mocks := setup(t)

		// Mock template renderer to return test data
		mockTemplateRenderer := &MockTemplate{}
		mockTemplateRenderer.ProcessFunc = func(templateData map[string][]byte, renderedData map[string]any) error {
			renderedData["kustomize/values"] = map[string]any{
				"common": map[string]any{
					"domain": "test.com",
				},
			}
			return nil
		}
		// Register the mock template renderer in the injector BEFORE initialization
		mocks.Injector.Register("templateRenderer", mockTemplateRenderer)

		// Initialize the pipeline to set up generators
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Mock blueprint handler to return template data
		mocks.BlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return map[string][]byte{
				"kustomize/values.jsonnet": []byte(`{"common": {"domain": "test.com"}}`),
			}, nil
		}

		// Track overwrite flag passed to generators
		var receivedOverwrite []bool
		for i := range pipeline.generators {
			mockGenerator := &MockGenerator{}
			mockGenerator.GenerateFunc = func(data map[string]any, overwrite ...bool) error {
				receivedOverwrite = overwrite
				return nil
			}
			pipeline.generators[i] = mockGenerator
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And overwrite flag should be false
		if len(receivedOverwrite) != 1 {
			t.Errorf("Expected 1 overwrite flag, got %d", len(receivedOverwrite))
		}
		if receivedOverwrite[0] != false {
			t.Errorf("Expected overwrite flag to be false, got %v", receivedOverwrite[0])
		}
	})
}

// =============================================================================
// processTemplateData Tests
// =============================================================================

func TestInstallPipeline_processTemplateData(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*InstallPipeline, *InstallMocks) {
		t.Helper()
		pipeline := NewInstallPipeline()
		mocks := setupInstallMocks(t, opts...)
		return pipeline, mocks
	}

	t.Run("ProcessesTemplateDataSuccessfully", func(t *testing.T) {
		// Given a pipeline with template renderer and template data
		pipeline, _ := setup(t)
		mockTemplateRenderer := &MockTemplate{
			ProcessFunc: func(templateData map[string][]byte, renderedData map[string]any) error {
				renderedData["test"] = "processed"
				return nil
			},
		}
		pipeline.templateRenderer = mockTemplateRenderer

		templateData := map[string][]byte{
			"test.jsonnet": []byte(`{"key": "value"}`),
		}

		// When processTemplateData is called
		result, err := pipeline.processTemplateData(templateData)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And template processing should be called
		if !mockTemplateRenderer.ProcessCalled {
			t.Error("Expected template processing to be called")
		}

		// And result should contain processed data
		if result["test"] != "processed" {
			t.Errorf("Expected processed data, got %v", result)
		}
	})

	t.Run("ReturnsErrorWhenTemplateProcessingFails", func(t *testing.T) {
		// Given a pipeline with failing template renderer
		pipeline, _ := setup(t)
		mockTemplateRenderer := &MockTemplate{
			ProcessFunc: func(templateData map[string][]byte, renderedData map[string]any) error {
				return fmt.Errorf("template processing failed")
			},
		}
		pipeline.templateRenderer = mockTemplateRenderer

		templateData := map[string][]byte{
			"test.jsonnet": []byte(`{"key": "value"}`),
		}

		// When processTemplateData is called
		result, err := pipeline.processTemplateData(templateData)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to process template data: template processing failed" {
			t.Errorf("Expected template processing error, got %q", err.Error())
		}

		// And result should be nil
		if result != nil {
			t.Errorf("Expected nil result, got %v", result)
		}
	})

	t.Run("ReturnsNilWhenNoTemplateRenderer", func(t *testing.T) {
		// Given a pipeline with no template renderer
		pipeline, _ := setup(t)
		pipeline.templateRenderer = nil

		templateData := map[string][]byte{
			"test.jsonnet": []byte(`{"key": "value"}`),
		}

		// When processTemplateData is called
		result, err := pipeline.processTemplateData(templateData)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And result should be nil
		if result != nil {
			t.Errorf("Expected nil result, got %v", result)
		}
	})

	t.Run("ReturnsNilWhenNoTemplateData", func(t *testing.T) {
		// Given a pipeline with template renderer but no template data
		pipeline, _ := setup(t)
		mockTemplateRenderer := &MockTemplate{}
		pipeline.templateRenderer = mockTemplateRenderer

		templateData := make(map[string][]byte)

		// When processTemplateData is called
		result, err := pipeline.processTemplateData(templateData)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And result should be nil
		if result != nil {
			t.Errorf("Expected nil result, got %v", result)
		}

		// And template processing should not be called
		if mockTemplateRenderer.ProcessCalled {
			t.Error("Expected template processing to not be called")
		}
	})

	t.Run("ReturnsNilWhenEmptyTemplateData", func(t *testing.T) {
		// Given a pipeline with template renderer but empty template data
		pipeline, _ := setup(t)
		mockTemplateRenderer := &MockTemplate{}
		pipeline.templateRenderer = mockTemplateRenderer

		templateData := map[string][]byte{}

		// When processTemplateData is called
		result, err := pipeline.processTemplateData(templateData)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And result should be nil
		if result != nil {
			t.Errorf("Expected nil result, got %v", result)
		}

		// And template processing should not be called
		if mockTemplateRenderer.ProcessCalled {
			t.Error("Expected template processing to not be called")
		}
	})
}
