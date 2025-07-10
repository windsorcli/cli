package pipelines

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/kubernetes"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/stack"
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
		ConfigHandler:     baseMocks.ConfigHandler,
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

	t.Run("ReturnsErrorWhenProcessContextTemplatesFails", func(t *testing.T) {
		// Given a pipeline with failing context template processing
		pipeline, mocks := setup(t)

		// Override blueprint handler to fail on process context templates
		mocks.BlueprintHandler.ProcessContextTemplatesFunc = func(contextName string, reset ...bool) error {
			return fmt.Errorf("process context templates failed")
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error processing context templates: process context templates failed" {
			t.Errorf("Expected process context templates error, got: %v", err)
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
		if err.Error() != "Error reloading blueprint config: blueprint load config failed" {
			t.Errorf("Expected blueprint load config error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSaveConfigFails", func(t *testing.T) {
		// Given a pipeline with failing config save
		// Use a simple mock config handler that fails on SaveConfig
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SaveConfigFunc = func(path string, overwrite ...bool) error {
			return fmt.Errorf("save config failed")
		}

		setupOptions := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		pipeline, _ := setup(t, setupOptions)

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

	t.Run("ReturnsErrorWhenSetContextFails", func(t *testing.T) {
		// Given a pipeline with failing set context
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SetContextFunc = func(contextName string) error {
			return fmt.Errorf("set context failed")
		}

		setupOptions := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		pipeline, _ := setup(t, setupOptions)

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error setting context value: set context failed" {
			t.Errorf("Expected set context error, got: %v", err)
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
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GenerateContextIDFunc = func() error {
			return fmt.Errorf("generate context ID failed")
		}

		setupOptions := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		pipeline, _ := setup(t, setupOptions)

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

func TestInitPipeline_configureVMDriver(t *testing.T) {
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

	t.Run("UsesExistingVMDriverFromConfig", func(t *testing.T) {
		// Given a pipeline with VM driver already set in config
		pipeline, mocks := setup(t)

		// Set an existing VM driver
		err := mocks.ConfigHandler.SetContextValue("vm.driver", "existing-driver")
		if err != nil {
			t.Fatalf("Failed to set vm.driver: %v", err)
		}

		// When configureVMDriver is called
		err = pipeline.configureVMDriver(context.Background(), "local")

		// Then no error should be returned and existing driver should be kept
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify the driver is still set to the existing value
		driver := mocks.ConfigHandler.GetString("vm.driver")
		if driver != "existing-driver" {
			t.Errorf("Expected vm.driver to remain 'existing-driver', got %q", driver)
		}
	})

	t.Run("SetsDockerDesktopForDarwinLocal", func(t *testing.T) {
		// Given a pipeline with no VM driver set on local context for darwin
		pipeline, mocks := setup(t)

		// When configureVMDriver is called for local context
		err := pipeline.configureVMDriver(context.Background(), "local")

		// Then docker-desktop should be set for darwin
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify the VM driver was set
		driver := mocks.ConfigHandler.GetString("vm.driver")
		if driver == "" {
			t.Error("Expected vm.driver to be set")
		}
		// Note: The actual value depends on runtime.GOOS, but we can verify it was set
	})

	t.Run("SetsVMDriverForLocalPrefixedContext", func(t *testing.T) {
		// Given a pipeline with no VM driver set on local-prefixed context
		pipeline, mocks := setup(t)

		// When configureVMDriver is called for local-prefixed context
		err := pipeline.configureVMDriver(context.Background(), "local-dev")

		// Then VM driver should be set
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify the VM driver was set
		driver := mocks.ConfigHandler.GetString("vm.driver")
		if driver == "" {
			t.Error("Expected vm.driver to be set")
		}
	})

	t.Run("DoesNotSetVMDriverForNonLocalContext", func(t *testing.T) {
		// Given a pipeline with no VM driver set on non-local context
		pipeline, mocks := setup(t)

		// When configureVMDriver is called for non-local context
		err := pipeline.configureVMDriver(context.Background(), "production")

		// Then no VM driver should be set
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify no VM driver was set
		driver := mocks.ConfigHandler.GetString("vm.driver")
		if driver != "" {
			t.Errorf("Expected no vm.driver to be set, got %q", driver)
		}
	})

	t.Run("ReturnsErrorWhenSetContextValueFails", func(t *testing.T) {
		// Given a pipeline with failing SetContextValue
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return "" // No existing driver
		}
		mockConfigHandler.SetContextValueFunc = func(key string, value any) error {
			return fmt.Errorf("set context value failed")
		}

		options := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}

		pipeline, _ := setup(t, options)

		// When configureVMDriver is called
		err := pipeline.configureVMDriver(context.Background(), "local")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "error setting vm.driver: set context value failed" {
			t.Errorf("Expected vm.driver error, got: %v", err)
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
			if key == "vm.driver" {
				return "docker-desktop"
			}
			return ""
		}
		mockConfigHandler.SetDefaultFunc = func(context v1alpha1.Context) error {
			return fmt.Errorf("set default failed")
		}

		options := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}

		pipeline, _ := setup(t, options)

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
			if key == "vm.driver" {
				return "colima"
			}
			return ""
		}
		mockConfigHandler.SetDefaultFunc = func(context v1alpha1.Context) error {
			return fmt.Errorf("set default failed")
		}

		options := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}

		pipeline, _ := setup(t, options)

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
			if key == "vm.driver" {
				return "unknown-driver"
			}
			return ""
		}
		mockConfigHandler.SetDefaultFunc = func(context v1alpha1.Context) error {
			return fmt.Errorf("set default failed")
		}

		options := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}

		pipeline, _ := setup(t, options)

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

	t.Run("ReturnsErrorWhenSetContextValueFails", func(t *testing.T) {
		// Given a pipeline with failing SetContextValue
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "platform" {
				return "aws"
			}
			return ""
		}
		mockConfigHandler.SetContextValueFunc = func(key string, value any) error {
			return fmt.Errorf("set context value failed")
		}

		options := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}

		pipeline, _ := setup(t, options)

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
}

func TestInitPipeline_saveConfiguration(t *testing.T) {
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

	t.Run("SavesConfigurationSuccessfully", func(t *testing.T) {
		// Given a pipeline with working components
		pipeline, _ := setup(t)

		// When saveConfiguration is called
		err := pipeline.saveConfiguration(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SavesConfigurationWithResetContext", func(t *testing.T) {
		// Given a pipeline with reset context
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "reset", true)

		// When saveConfiguration is called
		err := pipeline.saveConfiguration(ctx)

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

		// When saveConfiguration is called
		err := pipeline.saveConfiguration(context.Background())

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
				return &mockFileInfo{}, nil
			}
			return nil, fmt.Errorf("file not found")
		}

		// When saveConfiguration is called
		err := pipeline.saveConfiguration(context.Background())

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
				return &mockFileInfo{}, nil
			}
			return nil, fmt.Errorf("file not found")
		}

		// When saveConfiguration is called
		err := pipeline.saveConfiguration(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestInitPipeline_processContextTemplates(t *testing.T) {
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

	t.Run("ProcessesContextTemplatesSuccessfully", func(t *testing.T) {
		// Given a pipeline with working blueprint handler
		pipeline, _ := setup(t)

		// When processContextTemplates is called
		err := pipeline.processContextTemplates(context.Background(), "test-context")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ProcessesContextTemplatesWithResetContext", func(t *testing.T) {
		// Given a pipeline with reset context
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "reset", true)

		// When processContextTemplates is called
		err := pipeline.processContextTemplates(ctx, "test-context")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DoesNothingWhenBlueprintHandlerIsNil", func(t *testing.T) {
		// Given a pipeline with nil blueprint handler
		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t)

		// Initialize manually without blueprint handler
		if err := pipeline.BasePipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize base pipeline: %v", err)
		}

		// When processContextTemplates is called
		err := pipeline.processContextTemplates(context.Background(), "test-context")

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
		// Initialize the pipeline to set up configHandler
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}
		return pipeline, mocks
	}

	t.Run("WritesConfigurationFilesSuccessfully", func(t *testing.T) {
		// Given a pipeline with working components
		pipeline, _ := setup(t)

		// When writeConfigurationFiles is called
		err := pipeline.writeConfigurationFiles(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("WritesConfigurationFilesWithResetContext", func(t *testing.T) {
		// Given a pipeline with reset context
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "reset", true)

		// When writeConfigurationFiles is called
		err := pipeline.writeConfigurationFiles(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenServiceWriteConfigFails", func(t *testing.T) {
		// Given a pipeline with failing service WriteConfig
		pipeline, _ := setup(t)

		// Create a mock service that fails
		mockService := services.NewMockService()
		mockService.WriteConfigFunc = func() error {
			return fmt.Errorf("service write config failed")
		}
		pipeline.services = []services.Service{mockService}

		// When writeConfigurationFiles is called
		err := pipeline.writeConfigurationFiles(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "error writing service config: service write config failed" {
			t.Errorf("Expected service write config error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenContainerRuntimeWriteConfigFails", func(t *testing.T) {
		// Given a pipeline with failing container runtime WriteConfig
		pipeline, mocks := setup(t)

		// Override container runtime to fail on WriteConfig
		mocks.ContainerRuntime.WriteConfigFunc = func() error {
			return fmt.Errorf("container runtime write config failed")
		}

		// When writeConfigurationFiles is called
		err := pipeline.writeConfigurationFiles(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "error writing container runtime config: container runtime write config failed" {
			t.Errorf("Expected container runtime write config error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenGeneratorWriteFails", func(t *testing.T) {
		// Given a pipeline with failing generator Write
		pipeline, _ := setup(t)

		// Create a mock generator that fails
		mockGenerator := generators.NewMockGenerator()
		mockGenerator.WriteFunc = func(reset ...bool) error {
			return fmt.Errorf("generator write failed")
		}
		pipeline.generators = []generators.Generator{mockGenerator}

		// When writeConfigurationFiles is called
		err := pipeline.writeConfigurationFiles(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "error writing generator config: generator write failed" {
			t.Errorf("Expected generator write error, got: %v", err)
		}
	})
}
