package pipelines

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"archive/tar"
	"bytes"
	"compress/gzip"
	"path/filepath"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/kubernetes"
	"github.com/windsorcli/cli/pkg/network"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/template"
	"github.com/windsorcli/cli/pkg/terraform"
	"github.com/windsorcli/cli/pkg/tools"
	"github.com/windsorcli/cli/pkg/virt"
)

// =============================================================================
// Test Setup
// =============================================================================

type InitMocks struct {
	Injector          di.Injector
	ConfigHandler     config.ConfigHandler
	Shell             *shell.MockShell
	BlueprintHandler  *blueprint.MockBlueprintHandler
	KubernetesManager *kubernetes.MockKubernetesManager
	ToolsManager      *tools.MockToolsManager
	Stack             *stack.MockStack
	VirtualMachine    *virt.MockVirt
	ContainerRuntime  *virt.MockVirt
	ArtifactBuilder   *artifact.MockArtifact
	Shims             *Shims
}

func setupInitMocks(t *testing.T, opts ...*SetupOptions) *InitMocks {
	t.Helper()

	// Get base mocks (uses real YamlConfigHandler by default)
	baseMocks := setupMocks(t, opts...)

	// Create and register mock config handler specifically for init tests
	mockConfigHandler := config.NewMockConfigHandler()

	// Create a map to track values set via SetContextValue
	contextValues := make(map[string]interface{})

	mockConfigHandler.InitializeFunc = func() error { return nil }
	mockConfigHandler.SetContextFunc = func(contextName string) error { return nil }
	mockConfigHandler.GenerateContextIDFunc = func() error { return nil }
	mockConfigHandler.SaveConfigFunc = func(path string, overwrite ...bool) error { return nil }

	// Enhanced GetString that returns values set via SetContextValue
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if value, exists := contextValues[key]; exists {
			if strValue, ok := value.(string); ok {
				return strValue
			}
			if boolValue, ok := value.(bool); ok {
				if boolValue {
					return "true"
				}
				return "false"
			}
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}

	// SetContextValue that stores values in our map
	mockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
		contextValues[key] = value
		return nil
	}

	baseMocks.Injector.Register("configHandler", mockConfigHandler)

	// Add init-specific shell mock behaviors
	baseMocks.Shell.WriteResetTokenFunc = func() (string, error) { return "mock-token", nil }
	baseMocks.Shell.AddCurrentDirToTrustedFileFunc = func() error { return nil }

	// Setup blueprint handler mock
	mockBlueprintHandler := blueprint.NewMockBlueprintHandler(baseMocks.Injector)
	mockBlueprintHandler.InitializeFunc = func() error { return nil }
	mockBlueprintHandler.ProcessContextTemplatesFunc = func(contextName string, reset ...bool) error { return nil }
	mockBlueprintHandler.LoadConfigFunc = func(reset ...bool) error { return nil }
	baseMocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

	// Setup kubernetes manager mock
	mockKubernetesManager := kubernetes.NewMockKubernetesManager(nil)
	mockKubernetesManager.InitializeFunc = func() error { return nil }
	baseMocks.Injector.Register("kubernetesManager", mockKubernetesManager)

	// Setup tools manager mock
	mockToolsManager := tools.NewMockToolsManager()
	mockToolsManager.InitializeFunc = func() error { return nil }
	mockToolsManager.WriteManifestFunc = func() error { return nil }
	baseMocks.Injector.Register("toolsManager", mockToolsManager)

	// Setup stack mock
	mockStack := stack.NewMockStack(baseMocks.Injector)
	mockStack.InitializeFunc = func() error { return nil }
	baseMocks.Injector.Register("stack", mockStack)

	// Setup virtual machine mock
	mockVirtualMachine := virt.NewMockVirt()
	mockVirtualMachine.WriteConfigFunc = func() error { return nil }
	baseMocks.Injector.Register("virtualMachine", mockVirtualMachine)

	// Setup container runtime mock
	mockContainerRuntime := virt.NewMockVirt()
	mockContainerRuntime.WriteConfigFunc = func() error { return nil }
	baseMocks.Injector.Register("containerRuntime", mockContainerRuntime)

	// Setup artifact builder mock
	mockArtifactBuilder := artifact.NewMockArtifact()
	mockArtifactBuilder.InitializeFunc = func(injector di.Injector) error { return nil }
	baseMocks.Injector.Register("artifactBuilder", mockArtifactBuilder)

	return &InitMocks{
		Injector:          baseMocks.Injector,
		ConfigHandler:     mockConfigHandler,
		Shell:             baseMocks.Shell,
		BlueprintHandler:  mockBlueprintHandler,
		KubernetesManager: mockKubernetesManager,
		ToolsManager:      mockToolsManager,
		Stack:             mockStack,
		VirtualMachine:    mockVirtualMachine,
		ContainerRuntime:  mockContainerRuntime,
		ArtifactBuilder:   mockArtifactBuilder,
		Shims:             baseMocks.Shims,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewInitPipeline(t *testing.T) {
	t.Run("CreatesWithDefaults", func(t *testing.T) {
		// Given creating a new init pipeline
		pipeline := NewInitPipeline()

		// Then pipeline should not be nil
		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestInitPipeline_Initialize(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*InitPipeline, *InitMocks) {
		t.Helper()
		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t, opts...)
		return pipeline, mocks
	}

	t.Run("InitializesSuccessfully", func(t *testing.T) {
		// Given an init pipeline with mock dependencies
		pipeline, mocks := setup(t)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenShellInitializeFails", func(t *testing.T) {
		// Given an init pipeline with failing shell initialization
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return fmt.Errorf("shell initialization failed")
		}

		pipeline, mocks := setup(t)
		mocks.Injector.Register("shell", mockShell)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to initialize shell: shell initialization failed" {
			t.Errorf("Expected shell init error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerInitializeFails", func(t *testing.T) {
		// Given an init pipeline with failing config handler initialization
		pipeline := NewInitPipeline()

		// Create injector and register failing config handler directly
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error {
			return fmt.Errorf("config handler failed")
		}
		injector.Register("configHandler", mockConfigHandler)

		// Create and register basic shell
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error { return nil }
		mockShell.GetProjectRootFunc = func() (string, error) { return t.TempDir(), nil }
		injector.Register("shell", mockShell)

		// Register shims
		shims := setupShims(t)
		injector.Register("shims", shims)

		// When initializing the pipeline
		err := pipeline.Initialize(injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to initialize config handler: config handler failed" {
			t.Errorf("Expected config handler init error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenBlueprintHandlerInitializeFails", func(t *testing.T) {
		// Given an init pipeline with failing blueprint handler initialization
		pipeline, mocks := setup(t)

		// Override blueprint handler to fail on initialization
		mocks.BlueprintHandler.InitializeFunc = func() error {
			return fmt.Errorf("blueprint handler failed")
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to initialize blueprint handler: blueprint handler failed" {
			t.Errorf("Expected blueprint handler init error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenKubernetesManagerInitializeFails", func(t *testing.T) {
		// Given an init pipeline with failing kubernetes manager initialization
		pipeline, mocks := setup(t)

		// Override kubernetes manager to fail on initialization
		mocks.KubernetesManager.InitializeFunc = func() error {
			return fmt.Errorf("kubernetes manager failed")
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to initialize kubernetes manager: kubernetes manager failed" {
			t.Errorf("Expected kubernetes manager init error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenToolsManagerInitializeFails", func(t *testing.T) {
		// Given an init pipeline with failing tools manager initialization
		pipeline, mocks := setup(t)

		// Override tools manager to fail on initialization
		mocks.ToolsManager.InitializeFunc = func() error {
			return fmt.Errorf("tools manager failed")
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to initialize tools manager: tools manager failed" {
			t.Errorf("Expected tools manager init error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenStackInitializeFails", func(t *testing.T) {
		// Given an init pipeline with failing stack initialization
		pipeline, mocks := setup(t)

		// Override stack to fail on initialization
		mocks.Stack.InitializeFunc = func() error {
			return fmt.Errorf("stack failed")
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to initialize stack: stack failed" {
			t.Errorf("Expected stack init error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenArtifactBuilderInitializeFails", func(t *testing.T) {
		// Given an init pipeline with failing artifact builder initialization
		pipeline, mocks := setup(t)

		// Override artifact builder to fail on initialization
		mocks.ArtifactBuilder.InitializeFunc = func(injector di.Injector) error {
			return fmt.Errorf("artifact builder failed")
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to initialize artifact builder: artifact builder failed" {
			t.Errorf("Expected artifact builder init error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenTemplateRendererInitializeFails", func(t *testing.T) {
		// Given an init pipeline with failing template renderer initialization
		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t)

		// Create a mock template renderer that fails on initialization
		mockTemplate := template.NewMockTemplate(mocks.Injector)
		mockTemplate.InitializeFunc = func() error {
			return fmt.Errorf("template renderer initialization failed")
		}

		// Override the template renderer
		mocks.Injector.Register("templateRenderer", mockTemplate)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to initialize template renderer") {
			t.Errorf("Expected template renderer initialization error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenNetworkManagerInitializeFails", func(t *testing.T) {
		// Given an init pipeline with failing network manager initialization
		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t)

		// Create a mock network manager that fails on initialization
		mockNetworkManager := network.NewMockNetworkManager()
		mockNetworkManager.InitializeFunc = func() error {
			return fmt.Errorf("network manager initialization failed")
		}

		// Override the network manager
		mocks.Injector.Register("networkManager", mockNetworkManager)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to initialize network manager") {
			t.Errorf("Expected network manager initialization error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenContainerRuntimeInitializeFails", func(t *testing.T) {
		// Given an init pipeline with failing container runtime initialization
		pipeline, mocks := setup(t)

		// Override container runtime to fail on initialization
		mocks.ContainerRuntime.InitializeFunc = func() error {
			return fmt.Errorf("container runtime initialization failed")
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to initialize container runtime") {
			t.Errorf("Expected container runtime initialization error, got: %v", err)
		}
	})
}

func TestInitPipeline_Execute(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*InitPipeline, *InitMocks) {
		t.Helper()
		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t, opts...)

		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		return pipeline, mocks
	}

	t.Run("ExecutesSuccessfully", func(t *testing.T) {
		// Given a properly initialized InitPipeline
		pipeline, _ := setup(t)

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenBlueprintLoadConfigFails", func(t *testing.T) {
		// Given a pipeline with failing blueprint load config
		pipeline, mocks := setup(t)

		// Override blueprint handler to fail on load config
		mocks.BlueprintHandler.LoadConfigFunc = func(reset ...bool) error {
			return fmt.Errorf("blueprint load config failed")
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error reloading blueprint config after generation: blueprint load config failed" {
			t.Errorf("Expected blueprint load config error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSaveConfigFails", func(t *testing.T) {
		// Given a pipeline with failing config save
		pipeline, mocks := setup(t)

		// Override config handler to fail on save config after initialization
		if mockConfigHandler, ok := mocks.ConfigHandler.(*config.MockConfigHandler); ok {
			mockConfigHandler.SaveConfigFunc = func(path string, overwrite ...bool) error {
				return fmt.Errorf("save config failed")
			}
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "save config failed") {
			t.Errorf("Expected save config error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenVMWriteConfigFails", func(t *testing.T) {
		// Given a pipeline with failing VM write config
		pipeline, mocks := setup(t)

		// Set vm.driver to "colima" so VM gets created during initialization
		// We need to reinitialize the pipeline after setting the config
		mocks.ConfigHandler.SetContextValue("vm.driver", "colima")

		// Reinitialize to create the virtual machine
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to reinitialize pipeline: %v", err)
		}

		// Override VM to fail on write config
		mocks.VirtualMachine.WriteConfigFunc = func() error {
			return fmt.Errorf("vm write config failed")
		}

		// When Execute is called
		err = pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "error writing virtual machine config: vm write config failed" {
			t.Errorf("Expected vm write config error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenToolsWriteManifestFails", func(t *testing.T) {
		// Given a pipeline with failing tools write manifest
		pipeline, mocks := setup(t)

		// Override tools manager to fail on write manifest
		mocks.ToolsManager.WriteManifestFunc = func() error {
			return fmt.Errorf("tools write manifest failed")
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "error writing tools manifest: tools write manifest failed" {
			t.Errorf("Expected tools write manifest error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenAddCurrentDirToTrustedFileFails", func(t *testing.T) {
		// Given a pipeline with failing add current dir to trusted file
		pipeline, mocks := setup(t)

		// Override shell to fail on add current dir to trusted file
		mocks.Shell.AddCurrentDirToTrustedFileFunc = func() error {
			return fmt.Errorf("add current dir failed")
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error adding current directory to trusted file: add current dir failed" {
			t.Errorf("Expected add current dir error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenWriteResetTokenFails", func(t *testing.T) {
		// Given a pipeline with failing write reset token
		pipeline, mocks := setup(t)

		// Override shell to fail on write reset token
		mocks.Shell.WriteResetTokenFunc = func() (string, error) {
			return "", fmt.Errorf("write reset token failed")
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error writing reset token: write reset token failed" {
			t.Errorf("Expected write reset token error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenGenerateContextIDFails", func(t *testing.T) {
		// Given a pipeline with failing generate context ID
		pipeline, mocks := setup(t)

		// Override config handler to fail on generate context ID after initialization
		if mockConfigHandler, ok := mocks.ConfigHandler.(*config.MockConfigHandler); ok {
			mockConfigHandler.GenerateContextIDFunc = func() error {
				return fmt.Errorf("generate context ID failed")
			}
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to generate context ID: generate context ID failed" {
			t.Errorf("Expected generate context ID error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenPrepareTemplateDataFails", func(t *testing.T) {
		// Given a pipeline with failing template data preparation
		pipeline, mocks := setup(t)

		// Mock config handler to return OCI blueprint
		if mockConfigHandler, ok := mocks.ConfigHandler.(*config.MockConfigHandler); ok {
			mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
				if key == "blueprint" {
					return "oci://test/blueprint:latest"
				}
				return ""
			}
		}

		// Set artifact builder to nil to cause error
		pipeline.artifactBuilder = nil

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to prepare template data") {
			t.Errorf("Expected prepare template data error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenTemplateRendererFails", func(t *testing.T) {
		// Given a pipeline with failing template renderer
		pipeline, mocks := setup(t)

		// Mock template renderer with error
		mockTemplateRenderer := template.NewMockTemplate(mocks.Injector)
		mockTemplateRenderer.ProcessFunc = func(templateData map[string][]byte, renderedData map[string]any) error {
			return fmt.Errorf("template processing failed")
		}
		pipeline.templateRenderer = mockTemplateRenderer

		// Setup template data using existing mocks
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return t.TempDir(), nil
		}
		pipeline.shims = &Shims{
			Stat: func(path string) (os.FileInfo, error) {
				return &mockInitFileInfo{name: "_template", isDir: true}, nil
			},
			ReadDir: func(name string) ([]os.DirEntry, error) {
				return []os.DirEntry{
					&mockInitDirEntry{name: "test.txt", isDir: false},
				}, nil
			},
			ReadFile: func(name string) ([]byte, error) {
				return []byte("test content"), nil
			},
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to process template data") {
			t.Errorf("Expected template processing error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenBlueprintGeneratorFails", func(t *testing.T) {
		// Given a pipeline with failing blueprint generator
		pipeline, mocks := setup(t)

		// Mock template renderer to produce blueprint data
		mockTemplateRenderer := template.NewMockTemplate(mocks.Injector)
		mockTemplateRenderer.ProcessFunc = func(templateData map[string][]byte, renderedData map[string]any) error {
			renderedData["blueprint"] = map[string]any{"name": "test-blueprint"}
			return nil
		}
		pipeline.templateRenderer = mockTemplateRenderer

		// Mock blueprint generator with error
		mockBlueprintGenerator := generators.NewMockGenerator()
		mockBlueprintGenerator.GenerateFunc = func(data map[string]any, overwrite ...bool) error {
			return fmt.Errorf("blueprint generation failed")
		}
		mocks.Injector.Register("blueprintGenerator", mockBlueprintGenerator)

		// Setup template data using existing mocks
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return t.TempDir(), nil
		}
		pipeline.shims = &Shims{
			Stat: func(path string) (os.FileInfo, error) {
				return &mockInitFileInfo{name: "_template", isDir: true}, nil
			},
			ReadDir: func(name string) ([]os.DirEntry, error) {
				return []os.DirEntry{
					&mockInitDirEntry{name: "test.txt", isDir: false},
				}, nil
			},
			ReadFile: func(name string) ([]byte, error) {
				return []byte("test content"), nil
			},
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to generate blueprint from template data") {
			t.Errorf("Expected blueprint generation error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenTerraformResolverFails", func(t *testing.T) {
		// Given a pipeline with failing terraform resolver
		pipeline, _ := setup(t)

		// Mock terraform resolver with error
		mockTerraformResolver := terraform.NewMockModuleResolver(di.NewInjector())
		mockTerraformResolver.ProcessModulesFunc = func() error {
			return fmt.Errorf("terraform resolver failed")
		}
		pipeline.terraformResolvers = []terraform.ModuleResolver{mockTerraformResolver}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to process terraform modules") {
			t.Errorf("Expected terraform modules error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenFinalGeneratorFails", func(t *testing.T) {
		// Given a pipeline with failing final generator
		pipeline, mocks := setup(t)

		// Mock template renderer to produce rendered data
		mockTemplateRenderer := template.NewMockTemplate(di.NewInjector())
		mockTemplateRenderer.ProcessFunc = func(templateData map[string][]byte, renderedData map[string]any) error {
			renderedData["test"] = "data"
			return nil
		}
		pipeline.templateRenderer = mockTemplateRenderer

		// Mock generator with error
		mockGenerator := generators.NewMockGenerator()
		mockGenerator.GenerateFunc = func(data map[string]any, overwrite ...bool) error {
			return fmt.Errorf("final generation failed")
		}
		pipeline.generators = []generators.Generator{mockGenerator}

		// Setup template data using existing mocks
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return t.TempDir(), nil
		}
		pipeline.shims = &Shims{
			Stat: func(path string) (os.FileInfo, error) {
				return &mockInitFileInfo{name: "_template", isDir: true}, nil
			},
			ReadDir: func(name string) ([]os.DirEntry, error) {
				return []os.DirEntry{
					&mockInitDirEntry{name: "test.txt", isDir: false},
				}, nil
			},
			ReadFile: func(name string) ([]byte, error) {
				return []byte("test content"), nil
			},
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to generate from template data") {
			t.Errorf("Expected final generation error, got: %v", err)
		}
	})

	t.Run("ProcessesResetFlagFromContext", func(t *testing.T) {
		// Given a pipeline with reset flag in context
		pipeline, mocks := setup(t)
		ctx := context.WithValue(context.Background(), "reset", true)

		// Mock template renderer to produce rendered data
		mockTemplateRenderer := template.NewMockTemplate(di.NewInjector())
		mockTemplateRenderer.ProcessFunc = func(templateData map[string][]byte, renderedData map[string]any) error {
			renderedData["test"] = "data"
			return nil
		}
		pipeline.templateRenderer = mockTemplateRenderer

		// Mock generator to verify reset flag is passed
		mockGenerator := generators.NewMockGenerator()
		var resetReceived bool
		mockGenerator.GenerateFunc = func(data map[string]any, overwrite ...bool) error {
			resetReceived = len(overwrite) > 0 && overwrite[0]
			return nil
		}
		pipeline.generators = []generators.Generator{mockGenerator}

		// Setup template data using existing mocks
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return t.TempDir(), nil
		}
		pipeline.shims = &Shims{
			Stat: func(path string) (os.FileInfo, error) {
				return &mockInitFileInfo{name: "_template", isDir: true}, nil
			},
			ReadDir: func(name string) ([]os.DirEntry, error) {
				return []os.DirEntry{
					&mockInitDirEntry{name: "test.txt", isDir: false},
				}, nil
			},
			ReadFile: func(name string) ([]byte, error) {
				return []byte("test content"), nil
			},
		}

		// When Execute is called
		err := pipeline.Execute(ctx)

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And reset flag should be passed to generator
		if !resetReceived {
			t.Error("Expected reset flag to be true")
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestInitPipeline_determineContextName(t *testing.T) {
	t.Run("ReturnsContextNameFromContext", func(t *testing.T) {
		// Given a pipeline and context with contextName
		pipeline := NewInitPipeline()
		ctx := context.WithValue(context.Background(), "contextName", "test-context")

		// When determineContextName is called
		contextName := pipeline.determineContextName(ctx)

		// Then the context name should be returned
		expected := "test-context"
		if contextName != expected {
			t.Errorf("Expected context name %q, got %q", expected, contextName)
		}
	})

	t.Run("ReturnsLocalWhenNoContextName", func(t *testing.T) {
		// Given a pipeline and context without contextName
		pipeline := NewInitPipeline()
		ctx := context.Background()

		// When determineContextName is called
		contextName := pipeline.determineContextName(ctx)

		// Then "local" should be returned
		expected := "local"
		if contextName != expected {
			t.Errorf("Expected context name %q, got %q", expected, contextName)
		}
	})

	t.Run("ReturnsLocalWhenContextNameNotString", func(t *testing.T) {
		// Given a pipeline and context with non-string contextName
		pipeline := NewInitPipeline()
		ctx := context.WithValue(context.Background(), "contextName", 123)

		// When determineContextName is called
		contextName := pipeline.determineContextName(ctx)

		// Then "local" should be returned
		expected := "local"
		if contextName != expected {
			t.Errorf("Expected context name %q, got %q", expected, contextName)
		}
	})
}

func TestInitPipeline_setDefaultConfiguration(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*InitPipeline, *InitMocks) {
		t.Helper()
		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t, opts...)
		// Initialize the pipeline to set up configHandler
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}
		return pipeline, mocks
	}

	t.Run("SetsDefaultConfigLocalhostForDockerDesktop", func(t *testing.T) {
		// Given a pipeline with docker-desktop VM driver
		pipeline, mocks := setup(t)

		// Set the VM driver to docker-desktop
		err := mocks.ConfigHandler.SetContextValue("vm.driver", "docker-desktop")
		if err != nil {
			t.Fatalf("Failed to set vm.driver: %v", err)
		}

		// When setDefaultConfiguration is called
		err = pipeline.setDefaultConfiguration(context.Background(), "local")

		// Then DefaultConfig_Localhost should be set
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SetsDefaultConfigFullForColima", func(t *testing.T) {
		// Given a pipeline with colima VM driver
		pipeline, mocks := setup(t)

		// Set the VM driver to colima
		err := mocks.ConfigHandler.SetContextValue("vm.driver", "colima")
		if err != nil {
			t.Fatalf("Failed to set vm.driver: %v", err)
		}

		// When setDefaultConfiguration is called
		err = pipeline.setDefaultConfiguration(context.Background(), "local")

		// Then DefaultConfig_Full should be set
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SetsDefaultConfigFullForDocker", func(t *testing.T) {
		// Given a pipeline with docker VM driver
		pipeline, mocks := setup(t)

		// Set the VM driver to docker
		err := mocks.ConfigHandler.SetContextValue("vm.driver", "docker")
		if err != nil {
			t.Fatalf("Failed to set vm.driver: %v", err)
		}

		// When setDefaultConfiguration is called
		err = pipeline.setDefaultConfiguration(context.Background(), "local")

		// Then DefaultConfig_Full should be set
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SetsDefaultConfigForUnknownDriver", func(t *testing.T) {
		// Given a pipeline with unknown VM driver
		pipeline, mocks := setup(t)

		// Set the VM driver to unknown value
		err := mocks.ConfigHandler.SetContextValue("vm.driver", "unknown-driver")
		if err != nil {
			t.Fatalf("Failed to set vm.driver: %v", err)
		}

		// When setDefaultConfiguration is called
		err = pipeline.setDefaultConfiguration(context.Background(), "local")

		// Then DefaultConfig should be set
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSetDefaultFails", func(t *testing.T) {
		// Given a pipeline with failing SetDefault
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "vm.driver":
				return "docker-desktop"
			case "network.cidr_block":
				return "10.0.0.0/24"
			case "network.loadbalancer_ips.start":
				return "10.0.0.100"
			case "network.loadbalancer_ips.end":
				return "10.0.0.200"
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}
		mockConfigHandler.SetDefaultFunc = func(context v1alpha1.Context) error {
			return fmt.Errorf("set default failed")
		}
		mockConfigHandler.SetContextFunc = func(contextName string) error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "mock-context"
		}

		options := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}

		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t, options)

		// Initialize the pipeline manually to set up configHandler
		pipeline.injector = mocks.Injector
		pipeline.configHandler = mockConfigHandler

		// When setDefaultConfiguration is called
		err := pipeline.setDefaultConfiguration(context.Background(), "local")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error setting default config: set default failed" {
			t.Errorf("Expected set default error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSetDefaultFailsForColima", func(t *testing.T) {
		// Given a pipeline with failing SetDefault for colima
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "vm.driver":
				return "colima"
			case "network.cidr_block":
				return "10.0.0.0/24"
			case "network.loadbalancer_ips.start":
				return "10.0.0.100"
			case "network.loadbalancer_ips.end":
				return "10.0.0.200"
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}
		mockConfigHandler.SetDefaultFunc = func(context v1alpha1.Context) error {
			return fmt.Errorf("set default failed")
		}
		mockConfigHandler.SetContextFunc = func(contextName string) error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "mock-context"
		}

		options := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}

		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t, options)

		// Initialize the pipeline manually to set up configHandler
		pipeline.injector = mocks.Injector
		pipeline.configHandler = mockConfigHandler

		// When setDefaultConfiguration is called
		err := pipeline.setDefaultConfiguration(context.Background(), "local")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error setting default config: set default failed" {
			t.Errorf("Expected set default error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSetDefaultFailsForUnknownDriver", func(t *testing.T) {
		// Given a pipeline with failing SetDefault for unknown driver
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "vm.driver":
				return "unknown-driver"
			case "network.cidr_block":
				return "10.0.0.0/24"
			case "network.loadbalancer_ips.start":
				return "10.0.0.100"
			case "network.loadbalancer_ips.end":
				return "10.0.0.200"
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}
		mockConfigHandler.SetDefaultFunc = func(context v1alpha1.Context) error {
			return fmt.Errorf("set default failed")
		}
		mockConfigHandler.SetContextFunc = func(contextName string) error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "mock-context"
		}

		options := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}

		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t, options)

		// Initialize the pipeline manually to set up configHandler
		pipeline.injector = mocks.Injector
		pipeline.configHandler = mockConfigHandler

		// When setDefaultConfiguration is called
		err := pipeline.setDefaultConfiguration(context.Background(), "local")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error setting default config: set default failed" {
			t.Errorf("Expected set default error, got: %v", err)
		}
	})
}

func TestInitPipeline_processPlatformConfiguration(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*InitPipeline, *InitMocks) {
		t.Helper()
		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t, opts...)
		// Initialize the pipeline to set up configHandler
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}
		return pipeline, mocks
	}

	t.Run("DoesNothingWhenNoPlatform", func(t *testing.T) {
		// Given a pipeline with no platform set
		pipeline, _ := setup(t)

		// When processPlatformConfiguration is called
		err := pipeline.processPlatformConfiguration(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ConfiguresAWSPlatform", func(t *testing.T) {
		// Given a pipeline with AWS platform
		pipeline, mocks := setup(t)

		// Set the platform to AWS
		err := mocks.ConfigHandler.SetContextValue("platform", "aws")
		if err != nil {
			t.Fatalf("Failed to set platform: %v", err)
		}

		// When processPlatformConfiguration is called
		err = pipeline.processPlatformConfiguration(context.Background())

		// Then AWS settings should be configured
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify AWS configuration was set
		awsEnabled := mocks.ConfigHandler.GetString("aws.enabled")
		if awsEnabled != "true" {
			t.Errorf("Expected aws.enabled to be 'true', got %q", awsEnabled)
		}

		clusterDriver := mocks.ConfigHandler.GetString("cluster.driver")
		if clusterDriver != "eks" {
			t.Errorf("Expected cluster.driver to be 'eks', got %q", clusterDriver)
		}
	})

	t.Run("ConfiguresAzurePlatform", func(t *testing.T) {
		// Given a pipeline with Azure platform
		pipeline, mocks := setup(t)

		// Set the platform to Azure
		err := mocks.ConfigHandler.SetContextValue("platform", "azure")
		if err != nil {
			t.Fatalf("Failed to set platform: %v", err)
		}

		// When processPlatformConfiguration is called
		err = pipeline.processPlatformConfiguration(context.Background())

		// Then Azure settings should be configured
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify Azure configuration was set
		azureEnabled := mocks.ConfigHandler.GetString("azure.enabled")
		if azureEnabled != "true" {
			t.Errorf("Expected azure.enabled to be 'true', got %q", azureEnabled)
		}

		clusterDriver := mocks.ConfigHandler.GetString("cluster.driver")
		if clusterDriver != "aks" {
			t.Errorf("Expected cluster.driver to be 'aks', got %q", clusterDriver)
		}
	})

	t.Run("ConfiguresMetalPlatform", func(t *testing.T) {
		// Given a pipeline with metal platform
		pipeline, mocks := setup(t)

		// Set the platform to metal
		err := mocks.ConfigHandler.SetContextValue("platform", "metal")
		if err != nil {
			t.Fatalf("Failed to set platform: %v", err)
		}

		// When processPlatformConfiguration is called
		err = pipeline.processPlatformConfiguration(context.Background())

		// Then metal settings should be configured
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify metal configuration was set
		clusterDriver := mocks.ConfigHandler.GetString("cluster.driver")
		if clusterDriver != "talos" {
			t.Errorf("Expected cluster.driver to be 'talos', got %q", clusterDriver)
		}
	})

	t.Run("ConfiguresLocalPlatform", func(t *testing.T) {
		// Given a pipeline with local platform
		pipeline, mocks := setup(t)

		// Set the platform to local
		err := mocks.ConfigHandler.SetContextValue("platform", "local")
		if err != nil {
			t.Fatalf("Failed to set platform: %v", err)
		}

		// When processPlatformConfiguration is called
		err = pipeline.processPlatformConfiguration(context.Background())

		// Then local settings should be configured
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify local configuration was set
		clusterDriver := mocks.ConfigHandler.GetString("cluster.driver")
		if clusterDriver != "talos" {
			t.Errorf("Expected cluster.driver to be 'talos', got %q", clusterDriver)
		}
	})

	t.Run("ReturnsErrorWhenSetContextValueFailsForAWS", func(t *testing.T) {
		// Given a pipeline with failing SetContextValue for AWS
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "platform":
				return "aws"
			case "network.cidr_block":
				return "10.0.0.0/24"
			case "network.loadbalancer_ips.start":
				return "10.0.0.100"
			case "network.loadbalancer_ips.end":
				return "10.0.0.200"
			case "vm.driver":
				return ""
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}
		mockConfigHandler.SetContextValueFunc = func(key string, value any) error {
			return fmt.Errorf("set context value failed")
		}
		mockConfigHandler.SetContextFunc = func(contextName string) error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "mock-context"
		}

		options := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}

		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t, options)

		// Initialize the pipeline manually to set up configHandler
		pipeline.injector = mocks.Injector
		pipeline.configHandler = mockConfigHandler

		// When processPlatformConfiguration is called
		err := pipeline.processPlatformConfiguration(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error setting aws.enabled: set context value failed" {
			t.Errorf("Expected aws.enabled error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSetContextValueFailsForAWSClusterDriver", func(t *testing.T) {
		// Given a pipeline with failing SetContextValue for AWS cluster driver
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "platform":
				return "aws"
			case "network.cidr_block":
				return "10.0.0.0/24"
			case "network.loadbalancer_ips.start":
				return "10.0.0.100"
			case "network.loadbalancer_ips.end":
				return "10.0.0.200"
			case "vm.driver":
				return ""
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}
		callCount := 0
		mockConfigHandler.SetContextValueFunc = func(key string, value any) error {
			callCount++
			if callCount == 1 {
				return nil // First call (aws.enabled) succeeds
			}
			return fmt.Errorf("set context value failed") // Second call (cluster.driver) fails
		}
		mockConfigHandler.SetContextFunc = func(contextName string) error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "mock-context"
		}

		options := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}

		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t, options)

		// Initialize the pipeline manually to set up configHandler
		pipeline.injector = mocks.Injector
		pipeline.configHandler = mockConfigHandler

		// When processPlatformConfiguration is called
		err := pipeline.processPlatformConfiguration(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error setting cluster.driver: set context value failed" {
			t.Errorf("Expected cluster.driver error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSetContextValueFailsForAzure", func(t *testing.T) {
		// Given a pipeline with failing SetContextValue for Azure
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "platform":
				return "azure"
			case "network.cidr_block":
				return "10.0.0.0/24"
			case "network.loadbalancer_ips.start":
				return "10.0.0.100"
			case "network.loadbalancer_ips.end":
				return "10.0.0.200"
			case "vm.driver":
				return ""
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}
		mockConfigHandler.SetContextValueFunc = func(key string, value any) error {
			return fmt.Errorf("set context value failed")
		}
		mockConfigHandler.SetContextFunc = func(contextName string) error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "mock-context"
		}

		options := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}

		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t, options)

		// Initialize the pipeline manually to set up configHandler
		pipeline.injector = mocks.Injector
		pipeline.configHandler = mockConfigHandler

		// When processPlatformConfiguration is called
		err := pipeline.processPlatformConfiguration(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error setting azure.enabled: set context value failed" {
			t.Errorf("Expected azure.enabled error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSetContextValueFailsForAzureClusterDriver", func(t *testing.T) {
		// Given a pipeline with failing SetContextValue for Azure cluster driver
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "platform":
				return "azure"
			case "network.cidr_block":
				return "10.0.0.0/24"
			case "network.loadbalancer_ips.start":
				return "10.0.0.100"
			case "network.loadbalancer_ips.end":
				return "10.0.0.200"
			case "vm.driver":
				return ""
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}
		callCount := 0
		mockConfigHandler.SetContextValueFunc = func(key string, value any) error {
			callCount++
			if callCount == 1 {
				return nil // First call (azure.enabled) succeeds
			}
			return fmt.Errorf("set context value failed") // Second call (cluster.driver) fails
		}
		mockConfigHandler.SetContextFunc = func(contextName string) error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "mock-context"
		}

		options := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}

		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t, options)

		// Initialize the pipeline manually to set up configHandler
		pipeline.injector = mocks.Injector
		pipeline.configHandler = mockConfigHandler

		// When processPlatformConfiguration is called
		err := pipeline.processPlatformConfiguration(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error setting cluster.driver: set context value failed" {
			t.Errorf("Expected cluster.driver error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSetContextValueFailsForMetal", func(t *testing.T) {
		// Given a pipeline with failing SetContextValue for metal
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "platform":
				return "metal"
			case "network.cidr_block":
				return "10.0.0.0/24"
			case "network.loadbalancer_ips.start":
				return "10.0.0.100"
			case "network.loadbalancer_ips.end":
				return "10.0.0.200"
			case "vm.driver":
				return ""
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}
		mockConfigHandler.SetContextValueFunc = func(key string, value any) error {
			return fmt.Errorf("set context value failed")
		}
		mockConfigHandler.SetContextFunc = func(contextName string) error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "mock-context"
		}

		options := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}

		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t, options)

		// Initialize the pipeline manually to set up configHandler
		pipeline.injector = mocks.Injector
		pipeline.configHandler = mockConfigHandler

		// When processPlatformConfiguration is called
		err := pipeline.processPlatformConfiguration(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error setting cluster.driver: set context value failed" {
			t.Errorf("Expected cluster.driver error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSetContextValueFailsForLocal", func(t *testing.T) {
		// Given a pipeline with failing SetContextValue for local
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "platform":
				return "local"
			case "network.cidr_block":
				return "10.0.0.0/24"
			case "network.loadbalancer_ips.start":
				return "10.0.0.100"
			case "network.loadbalancer_ips.end":
				return "10.0.0.200"
			case "vm.driver":
				return ""
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}
		mockConfigHandler.SetContextValueFunc = func(key string, value any) error {
			return fmt.Errorf("set context value failed")
		}
		mockConfigHandler.SetContextFunc = func(contextName string) error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "mock-context"
		}

		options := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}

		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t, options)

		// Initialize the pipeline manually to set up configHandler
		pipeline.injector = mocks.Injector
		pipeline.configHandler = mockConfigHandler

		// When processPlatformConfiguration is called
		err := pipeline.processPlatformConfiguration(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error setting cluster.driver: set context value failed" {
			t.Errorf("Expected cluster.driver error, got: %v", err)
		}
	})
}

// TestInitPipeline_saveConfiguration verifies saveConfiguration behavior with overwrite flag and BDD-style comments.
func TestInitPipeline_saveConfiguration(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*InitPipeline, *InitMocks) {
		t.Helper()
		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t, opts...)
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}
		return pipeline, mocks
	}

	t.Run("SavesConfigurationSuccessfully", func(t *testing.T) {
		// Given a pipeline with working components
		pipeline, _ := setup(t)

		// When saveConfiguration is called with overwrite=false
		err := pipeline.saveConfiguration(false)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SavesConfigurationWithOverwrite", func(t *testing.T) {
		// Given a pipeline with working components
		pipeline, _ := setup(t)

		// When saveConfiguration is called with overwrite=true
		err := pipeline.saveConfiguration(true)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenGetProjectRootFails", func(t *testing.T) {
		// Given a pipeline with failing GetProjectRoot
		pipeline, mocks := setup(t)

		// Override shell to fail on GetProjectRoot
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("get project root failed")
		}

		// When saveConfiguration is called with overwrite=false
		err := pipeline.saveConfiguration(false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error retrieving project root: get project root failed" {
			t.Errorf("Expected get project root error, got: %v", err)
		}
	})

	t.Run("UsesYamlPathWhenItExists", func(t *testing.T) {
		// Given a pipeline with existing windsor.yaml file
		pipeline, mocks := setup(t)

		// Override shims to simulate yaml file exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, "windsor.yaml") {
				return &mockInitFileInfo{name: "windsor.yaml", isDir: false}, nil
			}
			return nil, fmt.Errorf("file not found")
		}

		// When saveConfiguration is called with overwrite=false
		err := pipeline.saveConfiguration(false)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("UsesYmlPathWhenYamlDoesNotExist", func(t *testing.T) {
		// Given a pipeline with existing windsor.yml file but no .yaml
		pipeline, mocks := setup(t)

		// Override shims to simulate only yml file exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, "windsor.yml") {
				return &mockInitFileInfo{name: "windsor.yml", isDir: false}, nil
			}
			return nil, fmt.Errorf("file not found")
		}

		// When saveConfiguration is called with overwrite=false
		err := pipeline.saveConfiguration(false)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestInitPipeline_writeConfigurationFiles(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*InitPipeline, *InitMocks) {
		t.Helper()
		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t, opts...)
		return pipeline, mocks
	}

	t.Run("WritesConfigurationFilesSuccessfully", func(t *testing.T) {
		// Given an init pipeline with mock dependencies
		pipeline, _ := setup(t)

		// When writing configuration files
		err := pipeline.writeConfigurationFiles()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenToolsManagerWriteManifestFails", func(t *testing.T) {
		// Given an init pipeline with failing tools manager
		pipeline, mocks := setup(t)

		// Override tools manager to fail on WriteManifest
		mocks.ToolsManager.WriteManifestFunc = func() error {
			return fmt.Errorf("tools manager write manifest failed")
		}

		// Ensure pipeline has the tools manager set
		pipeline.toolsManager = mocks.ToolsManager

		// When writing configuration files
		err := pipeline.writeConfigurationFiles()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "error writing tools manifest: tools manager write manifest failed" {
			t.Errorf("Expected tools manager write manifest error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenVirtualMachineWriteConfigFails", func(t *testing.T) {
		// Given an init pipeline with failing virtual machine
		pipeline, mocks := setup(t)

		// Override virtual machine to fail on WriteConfig
		mocks.VirtualMachine.WriteConfigFunc = func() error {
			return fmt.Errorf("virtual machine write config failed")
		}

		// Ensure pipeline has the virtual machine set
		pipeline.virtualMachine = mocks.VirtualMachine

		// When writing configuration files
		err := pipeline.writeConfigurationFiles()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "error writing virtual machine config: virtual machine write config failed" {
			t.Errorf("Expected virtual machine write config error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenContainerRuntimeWriteConfigFails", func(t *testing.T) {
		// Given an init pipeline with failing container runtime
		pipeline, mocks := setup(t)

		// Override container runtime to fail on WriteConfig
		mocks.ContainerRuntime.WriteConfigFunc = func() error {
			return fmt.Errorf("container runtime write config failed")
		}

		// Ensure pipeline has the container runtime set
		pipeline.containerRuntime = mocks.ContainerRuntime

		// When writing configuration files
		err := pipeline.writeConfigurationFiles()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "error writing container runtime config: container runtime write config failed" {
			t.Errorf("Expected container runtime write config error, got: %v", err)
		}
	})
}

func TestInitPipeline_prepareTemplateData(t *testing.T) {
	t.Run("WithOCIBlueprint", func(t *testing.T) {
		// Given a pipeline with OCI blueprint configuration
		pipeline := NewInitPipeline()

		// Create test tarball data
		tarballData := createTestTarball(t, map[string][]byte{
			"template1.txt":        []byte("content1"),
			"subdir/template2.txt": []byte("content2"),
		})

		// Mock artifact builder
		mockArtifact := artifact.NewMockArtifact()
		mockArtifact.PullFunc = func(refs []string) (map[string][]byte, error) {
			if len(refs) == 1 && refs[0] == "oci://test/blueprint:latest" {
				return map[string][]byte{
					"test-artifact": tarballData,
				}, nil
			}
			return nil, fmt.Errorf("unexpected ref")
		}
		pipeline.artifactBuilder = mockArtifact

		// Mock config handler
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "blueprint" {
				return "oci://test/blueprint:latest"
			}
			return ""
		}
		pipeline.configHandler = mockConfig

		// When preparing template data
		result, err := pipeline.prepareTemplateData()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And result should contain expected files
		if len(result) != 2 {
			t.Fatalf("Expected 2 template files, got: %d", len(result))
		}

		if string(result["template1.txt"]) != "content1" {
			t.Errorf("Expected template1.txt content 'content1', got: %s", string(result["template1.txt"]))
		}

		if string(result["subdir/template2.txt"]) != "content2" {
			t.Errorf("Expected subdir/template2.txt content 'content2', got: %s", string(result["subdir/template2.txt"]))
		}
	})

	t.Run("WithLocalTemplateDirectory", func(t *testing.T) {
		// Given a pipeline with local template directory
		pipeline := NewInitPipeline()
		tempDir := t.TempDir()
		templateDir := filepath.Join(tempDir, "contexts", "_template")

		// Mock shell
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}
		pipeline.shell = mockShell

		// Mock config handler (empty blueprint triggers local check)
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		pipeline.configHandler = mockConfig

		// Mock shims
		pipeline.shims = &Shims{
			Stat: func(path string) (os.FileInfo, error) {
				if path == templateDir {
					return &mockInitFileInfo{name: "_template", isDir: true}, nil
				}
				return nil, fmt.Errorf("file not found")
			},
			ReadDir: func(name string) ([]os.DirEntry, error) {
				if name == templateDir {
					return []os.DirEntry{
						&mockInitDirEntry{name: "test.txt", isDir: false},
						&mockInitDirEntry{name: "subdir", isDir: true},
					}, nil
				}
				if name == filepath.Join(templateDir, "subdir") {
					return []os.DirEntry{
						&mockInitDirEntry{name: "nested.txt", isDir: false},
					}, nil
				}
				return nil, fmt.Errorf("directory not found")
			},
			ReadFile: func(name string) ([]byte, error) {
				if name == filepath.Join(templateDir, "test.txt") {
					return []byte("test content"), nil
				}
				if name == filepath.Join(templateDir, "subdir", "nested.txt") {
					return []byte("nested content"), nil
				}
				return nil, fmt.Errorf("file not found")
			},
		}

		// When preparing template data
		result, err := pipeline.prepareTemplateData()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And result should contain expected files
		if len(result) != 2 {
			t.Fatalf("Expected 2 template files, got: %d", len(result))
		}

		if string(result["test.txt"]) != "test content" {
			t.Errorf("Expected test.txt content 'test content', got: %s", string(result["test.txt"]))
		}

		if string(result["subdir/nested.txt"]) != "nested content" {
			t.Errorf("Expected subdir/nested.txt content 'nested content', got: %s", string(result["subdir/nested.txt"]))
		}
	})

	t.Run("WithDefaultBehavior", func(t *testing.T) {
		// Given a pipeline with no template sources
		pipeline := NewInitPipeline()
		tempDir := t.TempDir()

		// Mock config handler
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		pipeline.configHandler = mockConfig

		// Mock shell
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}
		pipeline.shell = mockShell

		// Mock shims (directory doesn't exist)
		pipeline.shims = &Shims{
			Stat: func(path string) (os.FileInfo, error) {
				return nil, fmt.Errorf("file not found")
			},
		}

		// When preparing template data
		result, err := pipeline.prepareTemplateData()

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And result should be empty
		if len(result) != 0 {
			t.Fatalf("Expected empty map, got: %d items", len(result))
		}
	})

	t.Run("WithOCIBlueprintError", func(t *testing.T) {
		// Given a pipeline with failing OCI artifact builder
		pipeline := NewInitPipeline()

		// Mock artifact builder with error
		mockArtifact := artifact.NewMockArtifact()
		mockArtifact.PullFunc = func(refs []string) (map[string][]byte, error) {
			return nil, fmt.Errorf("pull failed")
		}
		pipeline.artifactBuilder = mockArtifact

		// Mock config handler
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "blueprint" {
				return "oci://test/blueprint:latest"
			}
			return ""
		}
		pipeline.configHandler = mockConfig

		// When preparing template data
		_, err := pipeline.prepareTemplateData()

		// Then error should occur
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "failed to pull OCI artifact") {
			t.Errorf("Expected error about pulling OCI artifact, got: %v", err)
		}
	})

	t.Run("WithNilArtifactBuilder", func(t *testing.T) {
		// Given a pipeline with nil artifact builder but OCI blueprint
		pipeline := NewInitPipeline()
		pipeline.artifactBuilder = nil

		// Mock config handler
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "blueprint" {
				return "oci://test/blueprint:latest"
			}
			return ""
		}
		pipeline.configHandler = mockConfig

		// When preparing template data
		_, err := pipeline.prepareTemplateData()

		// Then error should occur
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "artifact builder not available") {
			t.Errorf("Expected error about artifact builder, got: %v", err)
		}
	})

	t.Run("WithOCIBlueprintNoArtifacts", func(t *testing.T) {
		// Given a pipeline with OCI blueprint but no artifacts returned
		pipeline := NewInitPipeline()

		// Mock artifact builder that returns empty map
		mockArtifact := artifact.NewMockArtifact()
		mockArtifact.PullFunc = func(refs []string) (map[string][]byte, error) {
			return map[string][]byte{}, nil
		}
		pipeline.artifactBuilder = mockArtifact

		// Mock config handler
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "blueprint" {
				return "oci://test/blueprint:latest"
			}
			return ""
		}
		pipeline.configHandler = mockConfig

		// When preparing template data
		_, err := pipeline.prepareTemplateData()

		// Then error should occur
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "no artifacts downloaded") {
			t.Errorf("Expected error about no artifacts, got: %v", err)
		}
	})

	t.Run("WithOCIBlueprintGzipError", func(t *testing.T) {
		// Given a pipeline with OCI blueprint but invalid gzip data
		pipeline := NewInitPipeline()

		// Mock artifact builder that returns invalid gzip data
		mockArtifact := artifact.NewMockArtifact()
		mockArtifact.PullFunc = func(refs []string) (map[string][]byte, error) {
			return map[string][]byte{
				"test-artifact": []byte("invalid gzip data"),
			}, nil
		}
		pipeline.artifactBuilder = mockArtifact

		// Mock config handler
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "blueprint" {
				return "oci://test/blueprint:latest"
			}
			return ""
		}
		pipeline.configHandler = mockConfig

		// When preparing template data
		_, err := pipeline.prepareTemplateData()

		// Then error should occur
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "failed to create gzip reader") {
			t.Errorf("Expected error about gzip reader, got: %v", err)
		}
	})

	t.Run("WithLocalTemplateGetProjectRootError", func(t *testing.T) {
		// Given a pipeline with local template but failing GetProjectRoot
		pipeline := NewInitPipeline()

		// Mock config handler (empty blueprint triggers local check)
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		pipeline.configHandler = mockConfig

		// Mock shell with failing GetProjectRoot
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("get project root failed")
		}
		pipeline.shell = mockShell

		// When preparing template data
		_, err := pipeline.prepareTemplateData()

		// Then error should occur
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("Expected error about get project root, got: %v", err)
		}
	})

	t.Run("WithLocalTemplateWalkError", func(t *testing.T) {
		// Given a pipeline with local template but failing walkAndCollectTemplates
		pipeline := NewInitPipeline()
		tempDir := t.TempDir()

		// Mock config handler (empty blueprint triggers local check)
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		pipeline.configHandler = mockConfig

		// Mock shell
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}
		pipeline.shell = mockShell

		// Mock shims to simulate template directory exists but ReadDir fails
		pipeline.shims = &Shims{
			Stat: func(path string) (os.FileInfo, error) {
				return &mockInitFileInfo{name: "_template", isDir: true}, nil
			},
			ReadDir: func(name string) ([]os.DirEntry, error) {
				return nil, fmt.Errorf("read dir failed")
			},
		}

		// When preparing template data
		_, err := pipeline.prepareTemplateData()

		// Then error should occur
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "failed to collect local templates") {
			t.Errorf("Expected error about collecting local templates, got: %v", err)
		}
	})
}

func TestInitPipeline_walkAndCollectTemplates(t *testing.T) {
	t.Run("CollectsTemplatesSuccessfully", func(t *testing.T) {
		pipeline := NewInitPipeline()

		// Create temporary directory structure
		tempDir := t.TempDir()
		templateDir := filepath.Join(tempDir, "contexts", "_template")

		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create template directory: %v", err)
		}

		// Create test files
		if err := os.WriteFile(filepath.Join(templateDir, "test.txt"), []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		subDir := filepath.Join(templateDir, "subdir")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdirectory: %v", err)
		}

		if err := os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte("nested content"), 0644); err != nil {
			t.Fatalf("Failed to create nested file: %v", err)
		}

		// Mock shell to return temp directory as project root
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}
		pipeline.shell = mockShell

		// Initialize shims for file operations
		pipeline.shims = NewShims()

		templateData := make(map[string][]byte)
		err := pipeline.walkAndCollectTemplates(templateDir, templateData)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(templateData) != 2 {
			t.Fatalf("Expected 2 template files, got: %d", len(templateData))
		}

		if string(templateData["test.txt"]) != "test content" {
			t.Errorf("Expected test.txt content 'test content', got: %s", string(templateData["test.txt"]))
		}

		if string(templateData["subdir/nested.txt"]) != "nested content" {
			t.Errorf("Expected subdir/nested.txt content 'nested content', got: %s", string(templateData["subdir/nested.txt"]))
		}
	})

	t.Run("ReturnsErrorWhenReadDirFails", func(t *testing.T) {
		// Given a pipeline with failing ReadDir
		pipeline := NewInitPipeline()
		tempDir := t.TempDir()
		templateDir := filepath.Join(tempDir, "contexts", "_template")

		// Mock shell to return temp directory as project root
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}
		pipeline.shell = mockShell

		// Setup failing ReadDir
		pipeline.shims = &Shims{
			ReadDir: func(name string) ([]os.DirEntry, error) {
				return nil, fmt.Errorf("read dir failed")
			},
		}

		templateData := make(map[string][]byte)

		// When walkAndCollectTemplates is called
		err := pipeline.walkAndCollectTemplates(templateDir, templateData)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to read template directory: read dir failed" {
			t.Errorf("Expected read dir error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenReadFileFails", func(t *testing.T) {
		// Given a pipeline with failing ReadFile
		pipeline := NewInitPipeline()
		tempDir := t.TempDir()
		templateDir := filepath.Join(tempDir, "contexts", "_template")

		// Mock shell to return temp directory as project root
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}
		pipeline.shell = mockShell

		// Setup failing ReadFile
		pipeline.shims = &Shims{
			ReadDir: func(name string) ([]os.DirEntry, error) {
				return []os.DirEntry{
					&mockInitDirEntry{name: "test.txt", isDir: false},
				}, nil
			},
			ReadFile: func(name string) ([]byte, error) {
				return nil, fmt.Errorf("read file failed")
			},
		}

		templateData := make(map[string][]byte)

		// When walkAndCollectTemplates is called
		err := pipeline.walkAndCollectTemplates(templateDir, templateData)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to read template file") {
			t.Errorf("Expected read file error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenGetProjectRootFails", func(t *testing.T) {
		// Given a pipeline with failing GetProjectRoot
		pipeline := NewInitPipeline()
		templateDir := "/some/template/dir"

		// Mock shell to fail GetProjectRoot
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("get project root failed")
		}
		pipeline.shell = mockShell

		// Setup directory structure
		pipeline.shims = &Shims{
			ReadDir: func(name string) ([]os.DirEntry, error) {
				return []os.DirEntry{
					&mockInitDirEntry{name: "test.txt", isDir: false},
				}, nil
			},
			ReadFile: func(name string) ([]byte, error) {
				return []byte("test content"), nil
			},
		}

		templateData := make(map[string][]byte)

		// When walkAndCollectTemplates is called
		err := pipeline.walkAndCollectTemplates(templateDir, templateData)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to get project root: get project root failed" {
			t.Errorf("Expected get project root error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenRecursiveCallFails", func(t *testing.T) {
		// Given a pipeline with failing recursive call
		pipeline := NewInitPipeline()
		tempDir := t.TempDir()
		templateDir := filepath.Join(tempDir, "contexts", "_template")

		// Mock shell to return temp directory as project root
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}
		pipeline.shell = mockShell

		// Setup failing recursive ReadDir
		callCount := 0
		pipeline.shims = &Shims{
			ReadDir: func(name string) ([]os.DirEntry, error) {
				callCount++
				if callCount == 1 {
					// First call returns directory
					return []os.DirEntry{
						&mockInitDirEntry{name: "subdir", isDir: true},
					}, nil
				}
				// Second call (recursive) fails
				return nil, fmt.Errorf("recursive read dir failed")
			},
		}

		templateData := make(map[string][]byte)

		// When walkAndCollectTemplates is called
		err := pipeline.walkAndCollectTemplates(templateDir, templateData)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to read template directory: recursive read dir failed" {
			t.Errorf("Expected recursive read dir error, got: %v", err)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func createTestTarball(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	for path, content := range files {
		header := &tar.Header{
			Name: path,
			Mode: 0644,
			Size: int64(len(content)),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write tar header: %v", err)
		}

		if _, err := tarWriter.Write(content); err != nil {
			t.Fatalf("Failed to write tar content: %v", err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("Failed to close tar writer: %v", err)
	}

	if err := gzWriter.Close(); err != nil {
		t.Fatalf("Failed to close gzip writer: %v", err)
	}

	return buf.Bytes()
}
