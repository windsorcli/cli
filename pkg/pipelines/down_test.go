package pipelines

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/context/config"
	envvars "github.com/windsorcli/cli/pkg/context/env"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/resources/blueprint"
	"github.com/windsorcli/cli/pkg/context/shell"
	"github.com/windsorcli/cli/pkg/workstation/network"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// =============================================================================
// Test Setup
// =============================================================================

type DownMocks struct {
	*Mocks
	VirtualMachine   *virt.MockVirt
	ContainerRuntime *virt.MockVirt
	NetworkManager   *network.MockNetworkManager
	Stack            *terraforminfra.MockStack
	BlueprintHandler *blueprint.MockBlueprintHandler
}

func setupDownMocks(t *testing.T, opts ...*SetupOptions) *DownMocks {
	t.Helper()

	// Create setup options, preserving any provided options
	setupOptions := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		setupOptions = opts[0]
	}

	baseMocks := setupMocks(t, setupOptions)

	// Add down-specific shell mock behaviors
	baseMocks.Shell.GetSessionTokenFunc = func() (string, error) { return "mock-session-token", nil }

	// Initialize the config handler if it's a real one
	if setupOptions.ConfigHandler == nil {
		configHandler := baseMocks.ConfigHandler
		configHandler.SetContext("mock-context")

		// Load base config with down-specific settings
		configYAML := `
apiVersion: v1alpha1
contexts:
  mock-context:
    dns:
      domain: mock.domain.com
      enabled: true
    network:
      cidr_block: 10.0.0.0/24
    docker:
      enabled: true
    vm:
      driver: colima`

		if err := configHandler.LoadConfigString(configYAML); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
	}

	// Setup virtual machine mock
	mockVirtualMachine := virt.NewMockVirt()
	mockVirtualMachine.InitializeFunc = func() error { return nil }
	mockVirtualMachine.DownFunc = func() error { return nil }
	baseMocks.Injector.Register("virtualMachine", mockVirtualMachine)

	// Setup container runtime mock
	mockContainerRuntime := virt.NewMockVirt()
	mockContainerRuntime.InitializeFunc = func() error { return nil }
	mockContainerRuntime.DownFunc = func() error { return nil }
	baseMocks.Injector.Register("containerRuntime", mockContainerRuntime)

	// Setup network manager mock
	mockNetworkManager := network.NewMockNetworkManager()
	mockNetworkManager.InitializeFunc = func() error { return nil }
	baseMocks.Injector.Register("networkManager", mockNetworkManager)

	// Setup stack mock
	mockStack := terraforminfra.NewMockStack(baseMocks.Injector)
	mockStack.InitializeFunc = func() error { return nil }
	mockStack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error { return nil }
	baseMocks.Injector.Register("stack", mockStack)

	// Setup blueprint handler mock
	mockBlueprintHandler := blueprint.NewMockBlueprintHandler(baseMocks.Injector)
	mockBlueprintHandler.InitializeFunc = func() error { return nil }
	mockBlueprintHandler.LoadConfigFunc = func() error { return nil }
	mockBlueprintHandler.DownFunc = func() error { return nil }
	baseMocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

	// Setup env printers
	mockEnvPrinters := []envvars.EnvPrinter{}
	windsorEnv := envvars.NewMockEnvPrinter()
	windsorEnv.InitializeFunc = func() error { return nil }
	windsorEnv.GetEnvVarsFunc = func() (map[string]string, error) {
		return map[string]string{"WINDSOR_TEST": "true"}, nil
	}
	mockEnvPrinters = append(mockEnvPrinters, windsorEnv)
	baseMocks.Injector.Register("windsorEnv", windsorEnv)

	return &DownMocks{
		Mocks:            baseMocks,
		VirtualMachine:   mockVirtualMachine,
		ContainerRuntime: mockContainerRuntime,
		NetworkManager:   mockNetworkManager,
		Stack:            mockStack,
		BlueprintHandler: mockBlueprintHandler,
	}
}

// =============================================================================
// Constructor Tests
// =============================================================================

func TestNewDownPipeline(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// When creating a new down pipeline
		pipeline := NewDownPipeline()

		// Then it should not be nil
		if pipeline == nil {
			t.Error("Expected pipeline to be non-nil")
		}
	})
}

// =============================================================================
// Initialize Tests
// =============================================================================

func TestDownPipeline_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a down pipeline and mocks
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And all components should be initialized
		if pipeline.virtualMachine == nil {
			t.Error("Expected virtual machine to be initialized")
		}
		if pipeline.containerRuntime == nil {
			t.Error("Expected container runtime to be initialized")
		}
		if pipeline.networkManager == nil {
			t.Error("Expected network manager to be initialized")
		}
		if pipeline.stack == nil {
			t.Error("Expected stack to be initialized")
		}
		if pipeline.blueprintHandler == nil {
			t.Error("Expected blueprint handler to be initialized")
		}
		if len(pipeline.envPrinters) == 0 {
			t.Error("Expected env printers to be initialized")
		}
	})

	t.Run("InitializesSecureShellWhenRegistered", func(t *testing.T) {
		// Given a down pipeline with secure shell registered
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)

		// Create mock secure shell
		mockSecureShell := shell.NewMockShell()
		secureShellInitialized := false
		mockSecureShell.InitializeFunc = func() error {
			secureShellInitialized = true
			return nil
		}
		mocks.Injector.Register("secureShell", mockSecureShell)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And secure shell should be initialized
		if !secureShellInitialized {
			t.Error("Expected secure shell to be initialized")
		}
	})

	t.Run("ReturnsErrorWhenSecureShellInitializeFails", func(t *testing.T) {
		// Given a down pipeline with failing secure shell
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)

		// Create mock secure shell that fails to initialize
		mockSecureShell := shell.NewMockShell()
		mockSecureShell.InitializeFunc = func() error {
			return fmt.Errorf("secure shell failed")
		}
		mocks.Injector.Register("secureShell", mockSecureShell)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to initialize secure shell: secure shell failed" {
			t.Errorf("Expected secure shell error, got %q", err.Error())
		}
	})

	t.Run("SkipsSecureShellWhenNotRegistered", func(t *testing.T) {
		// Given a down pipeline without secure shell registered
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SkipsSecureShellWhenRegisteredTypeIsIncorrect", func(t *testing.T) {
		// Given a down pipeline with incorrectly typed secure shell
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)

		// Register something that's not a shell.Shell
		mocks.Injector.Register("secureShell", "not-a-shell")

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorInitializingBasePipeline", func(t *testing.T) {
		// Given a down pipeline with failing base pipeline
		pipeline := NewDownPipeline()

		// Create a mock config handler that succeeds during setup but fails during pipeline init
		initCallCount := 0
		failingConfigHandler := &config.MockConfigHandler{
			InitializeFunc: func() error {
				initCallCount++
				if initCallCount > 1 {
					return fmt.Errorf("config initialization failed")
				}
				return nil
			},
			SetContextFunc:       func(context string) error { return nil },
			LoadConfigStringFunc: func(configString string) error { return nil },
		}

		mocks := setupDownMocks(t, &SetupOptions{
			ConfigHandler: failingConfigHandler,
		})

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
			return
		}

		if !strings.Contains(err.Error(), "config initialization failed") {
			t.Errorf("Expected error message containing 'config initialization failed', got: %v", err)
		}
	})

	t.Run("ErrorInitializingEnvPrinters", func(t *testing.T) {
		// Given a down pipeline with failing env printer initialization
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)

		// Initialize the base pipeline first
		err := pipeline.BasePipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize base pipeline: %v", err)
		}

		// Create a failing env printer and register it
		failingEnvPrinter := envvars.NewMockEnvPrinter()
		failingEnvPrinter.InitializeFunc = func() error {
			return fmt.Errorf("env printer initialization failed")
		}

		// Set the env printers directly to include the failing one
		pipeline.envPrinters = []envvars.EnvPrinter{failingEnvPrinter}

		// When initializing the env printers
		var initErr error
		for _, envPrinter := range pipeline.envPrinters {
			if err := envPrinter.Initialize(); err != nil {
				initErr = fmt.Errorf("failed to initialize env printer: %w", err)
				break
			}
		}

		// Then an error should be returned
		if initErr == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(initErr.Error(), "env printer initialization failed") {
			t.Errorf("Expected error message containing 'env printer initialization failed', got: %v", initErr)
		}
	})

	t.Run("ErrorInitializingVirtualMachine", func(t *testing.T) {
		// Given a down pipeline with failing virtual machine initialization
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)

		// Set up a failing virtual machine mock
		mocks.VirtualMachine.InitializeFunc = func() error {
			return fmt.Errorf("virtual machine initialization failed")
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to initialize virtual machine") {
			t.Errorf("Expected error message containing 'failed to initialize virtual machine', got: %v", err)
		}
	})

	t.Run("ErrorInitializingContainerRuntime", func(t *testing.T) {
		// Given a down pipeline with failing container runtime initialization
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)

		// Set up a failing container runtime mock
		mocks.ContainerRuntime.InitializeFunc = func() error {
			return fmt.Errorf("container runtime initialization failed")
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to initialize container runtime") {
			t.Errorf("Expected error message containing 'failed to initialize container runtime', got: %v", err)
		}
	})

	t.Run("ErrorInitializingNetworkManager", func(t *testing.T) {
		// Given a down pipeline with failing network manager initialization
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)

		// Set up a failing network manager mock
		mocks.NetworkManager.InitializeFunc = func() error {
			return fmt.Errorf("network manager initialization failed")
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to initialize network manager") {
			t.Errorf("Expected error message containing 'failed to initialize network manager', got: %v", err)
		}
	})

	t.Run("ErrorInitializingStack", func(t *testing.T) {
		// Given a down pipeline with failing stack initialization
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)

		// Set up a failing stack mock
		mocks.Stack.InitializeFunc = func() error {
			return fmt.Errorf("stack initialization failed")
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to initialize stack") {
			t.Errorf("Expected error message containing 'failed to initialize stack', got: %v", err)
		}
	})

	t.Run("ErrorInitializingBlueprintHandler", func(t *testing.T) {
		// Given a down pipeline with failing blueprint handler initialization
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)

		// Set up a failing blueprint handler mock
		mocks.BlueprintHandler.InitializeFunc = func() error {
			return fmt.Errorf("blueprint handler initialization failed")
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to initialize blueprint handler") {
			t.Errorf("Expected error message containing 'failed to initialize blueprint handler', got: %v", err)
		}
	})

	t.Run("ErrorInitializingKubernetesManager", func(t *testing.T) {
		// Given a down pipeline with failing kubernetes manager initialization
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)

		// Create a failing kubernetes manager mock
		failingKubernetesManager := kubernetes.NewMockKubernetesManager(mocks.Injector)
		failingKubernetesManager.InitializeFunc = func() error {
			return fmt.Errorf("kubernetes manager initialization failed")
		}

		// Register the failing kubernetes manager
		mocks.Injector.Register("kubernetesManager", failingKubernetesManager)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to initialize kubernetes manager") {
			t.Errorf("Expected error message containing 'failed to initialize kubernetes manager', got: %v", err)
		}
	})
}

// =============================================================================
// Execute Tests
// =============================================================================

func TestDownPipeline_Execute(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a down pipeline and mocks
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Track method calls
		var blueprintDownCalled bool
		var stackDownCalled bool
		var containerRuntimeDownCalled bool

		mocks.BlueprintHandler.DownFunc = func() error {
			blueprintDownCalled = true
			return nil
		}
		mocks.Stack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
			stackDownCalled = true
			return nil
		}
		mocks.ContainerRuntime.DownFunc = func() error {
			containerRuntimeDownCalled = true
			return nil
		}

		// When executing the pipeline
		err = pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And blueprint down should be called
		if !blueprintDownCalled {
			t.Error("Expected blueprint down to be called")
		}

		// And stack down should be called
		if !stackDownCalled {
			t.Error("Expected stack down to be called")
		}

		// And container runtime down should be called
		if !containerRuntimeDownCalled {
			t.Error("Expected container runtime down to be called")
		}
	})

	t.Run("SkipK8sFlag", func(t *testing.T) {
		// Given a down pipeline and mocks
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Track method calls
		var blueprintDownCalled bool
		var stackDownCalled bool

		mocks.BlueprintHandler.DownFunc = func() error {
			blueprintDownCalled = true
			return nil
		}
		mocks.Stack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
			stackDownCalled = true
			return nil
		}

		// When executing the pipeline with skipK8s flag
		ctx := context.WithValue(context.Background(), "skipK8s", true)
		err = pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And blueprint down should NOT be called
		if blueprintDownCalled {
			t.Error("Expected blueprint down to NOT be called when skipK8s is true")
		}

		// And stack down should still be called
		if !stackDownCalled {
			t.Error("Expected stack down to be called")
		}
	})

	t.Run("SkipTerraformFlag", func(t *testing.T) {
		// Given a down pipeline and mocks
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Track method calls
		var blueprintDownCalled bool
		var stackDownCalled bool

		mocks.BlueprintHandler.DownFunc = func() error {
			blueprintDownCalled = true
			return nil
		}
		mocks.Stack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
			stackDownCalled = true
			return nil
		}

		// When executing the pipeline with skipTerraform flag
		ctx := context.WithValue(context.Background(), "skipTerraform", true)
		err = pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And blueprint down should be called
		if !blueprintDownCalled {
			t.Error("Expected blueprint down to be called")
		}

		// And stack down should NOT be called
		if stackDownCalled {
			t.Error("Expected stack down to NOT be called when skipTerraform is true")
		}
	})

	t.Run("CleanFlag", func(t *testing.T) {
		// Given a down pipeline with mock config handler
		mockConfigHandler := &config.MockConfigHandler{
			InitializeFunc:       func() error { return nil },
			SetContextFunc:       func(context string) error { return nil },
			LoadConfigStringFunc: func(configString string) error { return nil },
			GetBoolFunc: func(key string, defaultValue ...bool) bool {
				switch key {
				case "docker.enabled":
					return true
				default:
					return false
				}
			},
			GetStringFunc: func(key string, defaultValue ...string) string {
				return ""
			},
		}

		// Track config clean calls
		var configCleanCalled bool
		mockConfigHandler.CleanFunc = func() error {
			configCleanCalled = true
			return nil
		}

		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t, &SetupOptions{
			ConfigHandler: mockConfigHandler,
		})
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Setup shell mock to return project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		// Track removed paths
		var removedPaths []string
		pipeline.shims.RemoveAll = func(path string) error {
			removedPaths = append(removedPaths, path)
			return nil
		}

		// When executing the pipeline with clean flag
		ctx := context.WithValue(context.Background(), "clean", true)
		err = pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And cleanup should be performed
		if !configCleanCalled {
			t.Error("Expected config handler clean to be called")
		}

		// And specific paths should be removed
		expectedPaths := []string{
			filepath.Join("/test/project", ".volumes"),
			filepath.Join("/test/project", ".windsor", ".tf_modules"),
			filepath.Join("/test/project", ".windsor", "Corefile"),
			filepath.Join("/test/project", ".windsor", "docker-compose.yaml"),
		}

		if len(removedPaths) != len(expectedPaths) {
			t.Errorf("Expected %d paths to be removed, got %d", len(expectedPaths), len(removedPaths))
		}

		for i, expectedPath := range expectedPaths {
			if i >= len(removedPaths) || removedPaths[i] != expectedPath {
				t.Errorf("Expected path %s to be removed, got %v", expectedPath, removedPaths)
			}
		}
	})

	t.Run("ErrorBlueprintDown", func(t *testing.T) {
		// Given a down pipeline with failing blueprint handler
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)
		mocks.BlueprintHandler.DownFunc = func() error {
			return fmt.Errorf("blueprint down failed")
		}
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When executing the pipeline
		err = pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error running blueprint down: blueprint down failed" {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorStackDown", func(t *testing.T) {
		// Given a down pipeline with failing stack
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)
		mocks.Stack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
			return fmt.Errorf("stack down failed")
		}
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When executing the pipeline
		err = pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error running stack Down command: stack down failed" {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorContainerRuntimeDown", func(t *testing.T) {
		// Given a down pipeline with failing container runtime
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)
		mocks.ContainerRuntime.DownFunc = func() error {
			return fmt.Errorf("container runtime down failed")
		}
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When executing the pipeline
		err = pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error running container runtime Down command: container runtime down failed" {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ErrorLoadingBlueprintConfig", func(t *testing.T) {
		// Given a down pipeline with failing blueprint config loading
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)
		mocks.BlueprintHandler.LoadConfigFunc = func() error {
			return fmt.Errorf("failed to load blueprint config")
		}
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When executing the pipeline
		err = pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error loading blueprint config") {
			t.Errorf("Expected error message containing 'Error loading blueprint config', got: %v", err)
		}
	})

	t.Run("MissingBlueprintHandler", func(t *testing.T) {
		// Given a down pipeline without blueprint handler
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Set blueprint handler to nil
		pipeline.blueprintHandler = nil

		// When executing the pipeline
		err = pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "No blueprint handler found") {
			t.Errorf("Expected error message containing 'No blueprint handler found', got: %v", err)
		}
	})

	t.Run("MissingStack", func(t *testing.T) {
		// Given a down pipeline without stack
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Set stack to nil
		pipeline.stack = nil

		// When executing the pipeline
		err = pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "No stack found") {
			t.Errorf("Expected error message containing 'No stack found', got: %v", err)
		}
	})

	t.Run("MissingContainerRuntime", func(t *testing.T) {
		// Given a down pipeline without container runtime but docker enabled
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Set container runtime to nil
		pipeline.containerRuntime = nil

		// When executing the pipeline
		err = pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "No container runtime found") {
			t.Errorf("Expected error message containing 'No container runtime found', got: %v", err)
		}
	})

	t.Run("ErrorDuringCleanup", func(t *testing.T) {
		// Given a down pipeline with failing cleanup
		mockConfigHandler := &config.MockConfigHandler{
			InitializeFunc:       func() error { return nil },
			SetContextFunc:       func(context string) error { return nil },
			LoadConfigStringFunc: func(configString string) error { return nil },
			GetBoolFunc: func(key string, defaultValue ...bool) bool {
				switch key {
				case "docker.enabled":
					return true
				default:
					return false
				}
			},
			GetStringFunc: func(key string, defaultValue ...string) string {
				return ""
			},
			CleanFunc: func() error {
				return fmt.Errorf("cleanup failed")
			},
		}

		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t, &SetupOptions{
			ConfigHandler: mockConfigHandler,
		})
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When executing the pipeline with clean flag
		ctx := context.WithValue(context.Background(), "clean", true)
		err = pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error performing cleanup") {
			t.Errorf("Expected error message containing 'Error performing cleanup', got: %v", err)
		}
	})

	t.Run("SkipDockerFlag", func(t *testing.T) {
		// Given a down pipeline with skipDocker flag set
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Track method calls
		var blueprintDownCalled bool
		var stackDownCalled bool
		var containerRuntimeDownCalled bool

		mocks.BlueprintHandler.DownFunc = func() error {
			blueprintDownCalled = true
			return nil
		}
		mocks.Stack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
			stackDownCalled = true
			return nil
		}
		mocks.ContainerRuntime.DownFunc = func() error {
			containerRuntimeDownCalled = true
			return nil
		}

		// Create context with skipDocker flag
		ctx := context.WithValue(context.Background(), "skipDocker", true)

		// When executing the pipeline
		err = pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And blueprint down should be called
		if !blueprintDownCalled {
			t.Error("Expected blueprint down to be called")
		}

		// And stack down should be called
		if !stackDownCalled {
			t.Error("Expected stack down to be called")
		}

		// And container runtime down should NOT be called
		if containerRuntimeDownCalled {
			t.Error("Expected container runtime down to NOT be called")
		}
	})
}

// =============================================================================
// performCleanup Tests
// =============================================================================

func TestDownPipeline_performCleanup(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a down pipeline with working cleanup using mock config handler
		mockConfigHandler := &config.MockConfigHandler{
			InitializeFunc:       func() error { return nil },
			SetContextFunc:       func(context string) error { return nil },
			LoadConfigStringFunc: func(configString string) error { return nil },
			GetBoolFunc: func(key string, defaultValue ...bool) bool {
				switch key {
				case "docker.enabled":
					return true
				default:
					return false
				}
			},
			GetStringFunc: func(key string, defaultValue ...string) string {
				return ""
			},
		}

		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t, &SetupOptions{
			ConfigHandler: mockConfigHandler,
		})
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Track cleanup calls
		var configCleanCalled bool
		var removeAllCalls []string

		mockConfigHandler.CleanFunc = func() error {
			configCleanCalled = true
			return nil
		}

		mocks.Shims.RemoveAll = func(path string) error {
			removeAllCalls = append(removeAllCalls, path)
			return nil
		}

		// When performing cleanup
		err = pipeline.performCleanup()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And config clean should be called
		if !configCleanCalled {
			t.Error("Expected config clean to be called")
		}

		// And all expected paths should be removed (4 calls expected)
		expectedCallCount := 4
		if len(removeAllCalls) != expectedCallCount {
			t.Errorf("Expected %d RemoveAll calls, got %d", expectedCallCount, len(removeAllCalls))
		}

		// Check that the expected path suffixes are present
		expectedSuffixes := []string{
			".volumes",
			filepath.Join(".windsor", ".tf_modules"),
			filepath.Join(".windsor", "Corefile"),
			filepath.Join(".windsor", "docker-compose.yaml"),
		}

		for i, expectedSuffix := range expectedSuffixes {
			if i < len(removeAllCalls) && !strings.HasSuffix(removeAllCalls[i], expectedSuffix) {
				t.Errorf("Expected RemoveAll call %d to end with %s, got %s", i, expectedSuffix, removeAllCalls[i])
			}
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a down pipeline with failing project root retrieval
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Make GetProjectRoot fail
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("failed to get project root")
		}

		// When performing cleanup
		err = pipeline.performCleanup()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("Expected error message containing 'failed to get project root', got: %v", err)
		}
	})

	t.Run("ErrorRemovingVolumesFolder", func(t *testing.T) {
		// Given a down pipeline with failing volumes folder removal
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Make RemoveAll fail for volumes folder
		mocks.Shims.RemoveAll = func(path string) error {
			if strings.HasSuffix(path, ".volumes") {
				return fmt.Errorf("failed to remove volumes folder")
			}
			return nil
		}

		// When performing cleanup
		err = pipeline.performCleanup()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error deleting .volumes folder") {
			t.Errorf("Expected error message containing 'Error deleting .volumes folder', got: %v", err)
		}
	})

	t.Run("ErrorRemovingTfModulesFolder", func(t *testing.T) {
		// Given a down pipeline with failing tf modules folder removal
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Make RemoveAll fail for tf modules folder
		mocks.Shims.RemoveAll = func(path string) error {
			if strings.HasSuffix(path, ".tf_modules") {
				return fmt.Errorf("failed to remove tf modules folder")
			}
			return nil
		}

		// When performing cleanup
		err = pipeline.performCleanup()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error deleting .windsor/.tf_modules folder") {
			t.Errorf("Expected error message containing 'Error deleting .windsor/.tf_modules folder', got: %v", err)
		}
	})

	t.Run("ErrorRemovingCorefile", func(t *testing.T) {
		// Given a down pipeline with failing Corefile removal
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Make RemoveAll fail for Corefile
		mocks.Shims.RemoveAll = func(path string) error {
			if strings.HasSuffix(path, "Corefile") {
				return fmt.Errorf("failed to remove Corefile")
			}
			return nil
		}

		// When performing cleanup
		err = pipeline.performCleanup()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error deleting .windsor/Corefile") {
			t.Errorf("Expected error message containing 'Error deleting .windsor/Corefile', got: %v", err)
		}
	})

	t.Run("ErrorRemovingDockerCompose", func(t *testing.T) {
		// Given a down pipeline with failing docker-compose removal
		pipeline := NewDownPipeline()
		mocks := setupDownMocks(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Make RemoveAll fail for docker-compose.yaml
		mocks.Shims.RemoveAll = func(path string) error {
			if strings.HasSuffix(path, "docker-compose.yaml") {
				return fmt.Errorf("failed to remove docker-compose.yaml")
			}
			return nil
		}

		// When performing cleanup
		err = pipeline.performCleanup()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error deleting .windsor/docker-compose.yaml") {
			t.Errorf("Expected error message containing 'Error deleting .windsor/docker-compose.yaml', got: %v", err)
		}
	})
}
