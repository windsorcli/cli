package pipelines

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/cluster"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/tools"
)

// =============================================================================
// Test Setup
// =============================================================================

type CheckMocks struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
	ToolsManager  *tools.MockToolsManager
	ClusterClient *cluster.MockClusterClient
	Shims         *Shims
}

type CheckSetupOptions struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	ConfigStr     string
	Shims         *Shims
}

func setupCheckMocks(t *testing.T, opts ...*CheckSetupOptions) *CheckMocks {
	t.Helper()

	var options *CheckSetupOptions
	if len(opts) > 0 {
		options = opts[0]
	} else {
		options = &CheckSetupOptions{}
	}

	var injector di.Injector
	if options.Injector != nil {
		injector = options.Injector
	} else {
		injector = di.NewMockInjector()
	}

	var configHandler *config.MockConfigHandler
	if options.ConfigHandler != nil {
		configHandler = options.ConfigHandler
	} else {
		configHandler = config.NewMockConfigHandler()
	}

	var shims *Shims
	if options.Shims != nil {
		shims = options.Shims
	} else {
		shims = setupCheckShims(t)
	}

	mockShell := shell.NewMockShell()
	mockShell.InitializeFunc = func() error { return nil }
	mockShell.GetProjectRootFunc = func() (string, error) { return t.TempDir(), nil }

	mockToolsManager := tools.NewMockToolsManager()
	mockToolsManager.InitializeFunc = func() error { return nil }
	mockToolsManager.CheckFunc = func() error { return nil }

	mockClusterClient := cluster.NewMockClusterClient()
	mockClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodes []string, version string) error {
		return nil
	}

	configHandler.InitializeFunc = func() error { return nil }
	configHandler.LoadConfigFunc = func(path string) error { return nil }
	configHandler.IsLoadedFunc = func() bool { return true }

	if options.ConfigStr != "" {
		if err := configHandler.LoadConfigString(options.ConfigStr); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}
	}

	injector.Register("configHandler", configHandler)
	injector.Register("shell", mockShell)
	injector.Register("toolsManager", mockToolsManager)
	injector.Register("clusterClient", mockClusterClient)

	return &CheckMocks{
		Injector:      injector,
		ConfigHandler: configHandler,
		Shell:         mockShell,
		ToolsManager:  mockToolsManager,
		ClusterClient: mockClusterClient,
		Shims:         shims,
	}
}

func setupCheckShims(t *testing.T) *Shims {
	t.Helper()
	return NewShims()
}

func setupCheckPipeline(t *testing.T, mocks *CheckMocks) *CheckPipeline {
	t.Helper()

	constructors := CheckConstructors{
		NewConfigHandler: func(di.Injector) config.ConfigHandler {
			return mocks.ConfigHandler
		},
		NewShell: func(di.Injector) shell.Shell {
			return mocks.Shell
		},
		NewToolsManager: func(di.Injector) tools.ToolsManager {
			return mocks.ToolsManager
		},
		NewClusterClient: func(di.Injector) cluster.ClusterClient {
			return mocks.ClusterClient
		},
		NewShims: func() *Shims {
			return mocks.Shims
		},
	}

	return NewCheckPipeline(constructors)
}

func checkContains(str, substr string) bool {
	return len(str) > 0 && len(substr) > 0 &&
		(str == substr || len(str) >= len(substr) &&
			(str[:len(substr)] == substr || str[len(str)-len(substr):] == substr ||
				func() bool {
					for i := 0; i <= len(str)-len(substr); i++ {
						if str[i:i+len(substr)] == substr {
							return true
						}
					}
					return false
				}()))
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewCheckPipeline(t *testing.T) {
	t.Run("CreatesWithDefaultConstructors", func(t *testing.T) {
		pipeline := NewCheckPipeline()

		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}

		if pipeline.constructors.NewConfigHandler == nil {
			t.Error("Expected NewConfigHandler constructor to be set")
		} else {
			handler := pipeline.constructors.NewConfigHandler(di.NewMockInjector())
			if handler == nil {
				t.Error("Expected NewConfigHandler to return a non-nil handler")
			}
		}

		if pipeline.constructors.NewShell == nil {
			t.Error("Expected NewShell constructor to be set")
		} else {
			shell := pipeline.constructors.NewShell(di.NewMockInjector())
			if shell == nil {
				t.Error("Expected NewShell to return a non-nil shell")
			}
		}

		if pipeline.constructors.NewToolsManager == nil {
			t.Error("Expected NewToolsManager constructor to be set")
		} else {
			manager := pipeline.constructors.NewToolsManager(di.NewMockInjector())
			if manager == nil {
				t.Error("Expected NewToolsManager to return a non-nil manager")
			}
		}

		if pipeline.constructors.NewClusterClient == nil {
			t.Error("Expected NewClusterClient constructor to be set")
		} else {
			client := pipeline.constructors.NewClusterClient(di.NewMockInjector())
			if client == nil {
				t.Error("Expected NewClusterClient to return a non-nil client")
			}
		}

		if pipeline.constructors.NewShims == nil {
			t.Error("Expected NewShims constructor to be set")
		} else {
			shims := pipeline.constructors.NewShims()
			if shims == nil {
				t.Error("Expected NewShims to return a non-nil shims")
			}
		}
	})

	t.Run("CreatesWithCustomConstructors", func(t *testing.T) {
		constructors := CheckConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return config.NewMockConfigHandler()
			},
		}

		pipeline := NewCheckPipeline(constructors)

		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}

		if pipeline.constructors.NewConfigHandler == nil {
			t.Error("Expected NewConfigHandler constructor to be set")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestCheckPipeline_Initialize(t *testing.T) {
	setup := func(t *testing.T, opts ...*CheckSetupOptions) (*CheckPipeline, *CheckMocks) {
		t.Helper()
		mocks := setupCheckMocks(t, opts...)
		pipeline := setupCheckPipeline(t, mocks)
		return pipeline, mocks
	}

	t.Run("Success", func(t *testing.T) {
		pipeline, _ := setup(t, &CheckSetupOptions{
			ConfigStr: `
contexts:
  test-context:
    tools:
      enabled: true
`,
		})

		err := pipeline.Initialize(di.NewMockInjector())

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerInitializeFails", func(t *testing.T) {
		pipeline, mocks := setup(t)
		mocks.ConfigHandler.InitializeFunc = func() error {
			return fmt.Errorf("config initialization failed")
		}

		err := pipeline.Initialize(mocks.Injector)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !checkContains(err.Error(), "failed to initialize config handler") {
			t.Errorf("Expected config handler error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenShellInitializeFails", func(t *testing.T) {
		pipeline, mocks := setup(t)
		mocks.Shell.InitializeFunc = func() error {
			return fmt.Errorf("shell initialization failed")
		}

		err := pipeline.Initialize(mocks.Injector)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !checkContains(err.Error(), "failed to initialize shell") {
			t.Errorf("Expected shell error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenToolsManagerInitializeFails", func(t *testing.T) {
		pipeline, mocks := setup(t)
		mocks.ToolsManager.InitializeFunc = func() error {
			return fmt.Errorf("tools manager initialization failed")
		}

		err := pipeline.Initialize(mocks.Injector)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !checkContains(err.Error(), "failed to initialize tools manager") {
			t.Errorf("Expected tools manager error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenLoadConfigFails", func(t *testing.T) {
		pipeline, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}

		err := pipeline.Initialize(mocks.Injector)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !checkContains(err.Error(), "failed to load config") {
			t.Errorf("Expected load config error, got: %v", err)
		}
	})

	t.Run("ReusesExistingComponentsFromDIContainer", func(t *testing.T) {
		injector := di.NewMockInjector()

		existingShell := shell.NewMockShell()
		existingShell.InitializeFunc = func() error { return nil }
		existingShell.GetProjectRootFunc = func() (string, error) { return t.TempDir(), nil }
		injector.Register("shell", existingShell)

		existingConfigHandler := config.NewMockConfigHandler()
		existingConfigHandler.InitializeFunc = func() error { return nil }
		existingConfigHandler.LoadConfigFunc = func(path string) error { return nil }
		existingConfigHandler.IsLoadedFunc = func() bool { return true }
		injector.Register("configHandler", existingConfigHandler)

		existingToolsManager := tools.NewMockToolsManager()
		existingToolsManager.InitializeFunc = func() error { return nil }
		injector.Register("toolsManager", existingToolsManager)

		existingClusterClient := cluster.NewMockClusterClient()
		injector.Register("clusterClient", existingClusterClient)

		constructorsCalled := false
		constructors := CheckConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				constructorsCalled = true
				return config.NewMockConfigHandler()
			},
			NewShell: func(di.Injector) shell.Shell {
				constructorsCalled = true
				return shell.NewMockShell()
			},
			NewToolsManager: func(di.Injector) tools.ToolsManager {
				constructorsCalled = true
				return tools.NewMockToolsManager()
			},
			NewClusterClient: func(di.Injector) cluster.ClusterClient {
				constructorsCalled = true
				return cluster.NewMockClusterClient()
			},
			NewShims: func() *Shims {
				return setupCheckShims(t)
			},
		}

		pipeline := NewCheckPipeline(constructors)

		err := pipeline.Initialize(injector)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if constructorsCalled {
			t.Error("Expected constructors not to be called when components exist in DI container")
		}

		if pipeline.shell != existingShell {
			t.Error("Expected pipeline to use existing shell from DI container")
		}
		if pipeline.configHandler != existingConfigHandler {
			t.Error("Expected pipeline to use existing config handler from DI container")
		}
		if pipeline.toolsManager != existingToolsManager {
			t.Error("Expected pipeline to use existing tools manager from DI container")
		}
		if pipeline.clusterClient != existingClusterClient {
			t.Error("Expected pipeline to use existing cluster client from DI container")
		}
	})
}

func TestCheckPipeline_Execute(t *testing.T) {
	setup := func(t *testing.T, opts ...*CheckSetupOptions) (*CheckPipeline, *CheckMocks) {
		t.Helper()
		mocks := setupCheckMocks(t, opts...)
		pipeline := setupCheckPipeline(t, mocks)
		err := pipeline.Initialize(mocks.Injector)
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}
		return pipeline, mocks
	}

	t.Run("ExecutesToolsCheckByDefault", func(t *testing.T) {
		pipeline, _ := setup(t, &CheckSetupOptions{
			ConfigStr: `
contexts:
  test-context:
    tools:
      enabled: true
`,
		})

		var outputMessage string
		outputFunc := func(msg string) {
			outputMessage = msg
		}

		ctx := context.WithValue(context.Background(), "output", outputFunc)

		err := pipeline.Execute(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if outputMessage != "All tools are up to date." {
			t.Errorf("Expected 'All tools are up to date.', got %q", outputMessage)
		}
	})

	t.Run("ExecutesToolsCheckExplicitly", func(t *testing.T) {
		pipeline, _ := setup(t, &CheckSetupOptions{
			ConfigStr: `
contexts:
  test-context:
    tools:
      enabled: true
`,
		})

		var outputMessage string
		outputFunc := func(msg string) {
			outputMessage = msg
		}

		ctx := context.WithValue(context.Background(), "operation", "tools")
		ctx = context.WithValue(ctx, "output", outputFunc)

		err := pipeline.Execute(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if outputMessage != "All tools are up to date." {
			t.Errorf("Expected 'All tools are up to date.', got %q", outputMessage)
		}
	})

	t.Run("ExecutesNodeHealthCheck", func(t *testing.T) {
		pipeline, _ := setup(t, &CheckSetupOptions{
			ConfigStr: `
contexts:
  test-context:
    cluster:
      enabled: true
`,
		})

		var outputMessage string
		outputFunc := func(msg string) {
			outputMessage = msg
		}

		ctx := context.WithValue(context.Background(), "operation", "node-health")
		ctx = context.WithValue(ctx, "nodes", []string{"10.0.0.1", "10.0.0.2"})
		ctx = context.WithValue(ctx, "output", outputFunc)

		err := pipeline.Execute(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if outputMessage != "All 2 nodes are healthy" {
			t.Errorf("Expected 'All 2 nodes are healthy', got %q", outputMessage)
		}
	})

	t.Run("ExecutesNodeHealthCheckWithVersion", func(t *testing.T) {
		pipeline, _ := setup(t, &CheckSetupOptions{
			ConfigStr: `
contexts:
  test-context:
    cluster:
      enabled: true
`,
		})

		var outputMessage string
		outputFunc := func(msg string) {
			outputMessage = msg
		}

		ctx := context.WithValue(context.Background(), "operation", "node-health")
		ctx = context.WithValue(ctx, "nodes", []string{"10.0.0.1"})
		ctx = context.WithValue(ctx, "version", "1.0.0")
		ctx = context.WithValue(ctx, "output", outputFunc)

		err := pipeline.Execute(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if outputMessage != "All 1 nodes are healthy and running version 1.0.0" {
			t.Errorf("Expected 'All 1 nodes are healthy and running version 1.0.0', got %q", outputMessage)
		}
	})

	t.Run("ReturnsErrorWhenConfigNotLoaded", func(t *testing.T) {
		pipeline, mocks := setup(t)
		mocks.ConfigHandler.IsLoadedFunc = func() bool { return false }

		err := pipeline.Execute(context.Background())

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !checkContains(err.Error(), "Nothing to check. Have you run") {
			t.Errorf("Expected config not loaded error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidOperationType", func(t *testing.T) {
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "operation", 123)

		err := pipeline.Execute(ctx)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !checkContains(err.Error(), "Invalid operation type") {
			t.Errorf("Expected invalid operation type error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForUnknownOperation", func(t *testing.T) {
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "operation", "unknown")

		err := pipeline.Execute(ctx)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !checkContains(err.Error(), "Unknown operation type: unknown") {
			t.Errorf("Expected unknown operation error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenToolsCheckFails", func(t *testing.T) {
		pipeline, mocks := setup(t)
		mocks.ToolsManager.CheckFunc = func() error {
			return fmt.Errorf("tools check failed")
		}

		err := pipeline.Execute(context.Background())

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !checkContains(err.Error(), "Error checking tools") {
			t.Errorf("Expected tools check error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenNodeHealthCheckFails", func(t *testing.T) {
		pipeline, mocks := setup(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodes []string, version string) error {
			return fmt.Errorf("node health check failed")
		}

		ctx := context.WithValue(context.Background(), "operation", "node-health")
		ctx = context.WithValue(ctx, "nodes", []string{"10.0.0.1"})

		err := pipeline.Execute(ctx)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !checkContains(err.Error(), "nodes failed health check") {
			t.Errorf("Expected node health check error, got: %v", err)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestCheckPipeline_executeToolsCheck(t *testing.T) {
	setup := func(t *testing.T, opts ...*CheckSetupOptions) (*CheckPipeline, *CheckMocks) {
		t.Helper()
		mocks := setupCheckMocks(t, opts...)
		pipeline := setupCheckPipeline(t, mocks)
		pipeline.toolsManager = mocks.ToolsManager
		return pipeline, mocks
	}

	t.Run("Success", func(t *testing.T) {
		pipeline, _ := setup(t)

		var outputMessage string
		outputFunc := func(msg string) {
			outputMessage = msg
		}

		ctx := context.WithValue(context.Background(), "output", outputFunc)

		err := pipeline.executeToolsCheck(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if outputMessage != "All tools are up to date." {
			t.Errorf("Expected 'All tools are up to date.', got %q", outputMessage)
		}
	})

	t.Run("ReturnsErrorWhenToolsManagerCheckFails", func(t *testing.T) {
		pipeline, mocks := setup(t)
		mocks.ToolsManager.CheckFunc = func() error {
			return fmt.Errorf("tools check failed")
		}

		err := pipeline.executeToolsCheck(context.Background())

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !checkContains(err.Error(), "Error checking tools") {
			t.Errorf("Expected tools check error, got: %v", err)
		}
	})

	t.Run("HandlesNoOutputFunction", func(t *testing.T) {
		pipeline, _ := setup(t)

		err := pipeline.executeToolsCheck(context.Background())

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestCheckPipeline_executeNodeHealthCheck(t *testing.T) {
	setup := func(t *testing.T, opts ...*CheckSetupOptions) (*CheckPipeline, *CheckMocks) {
		t.Helper()
		mocks := setupCheckMocks(t, opts...)
		pipeline := setupCheckPipeline(t, mocks)
		pipeline.clusterClient = mocks.ClusterClient
		return pipeline, mocks
	}

	t.Run("Success", func(t *testing.T) {
		pipeline, _ := setup(t)

		var outputMessage string
		outputFunc := func(msg string) {
			outputMessage = msg
		}

		ctx := context.WithValue(context.Background(), "nodes", []string{"10.0.0.1", "10.0.0.2"})
		ctx = context.WithValue(ctx, "output", outputFunc)

		err := pipeline.executeNodeHealthCheck(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if outputMessage != "All 2 nodes are healthy" {
			t.Errorf("Expected 'All 2 nodes are healthy', got %q", outputMessage)
		}
	})

	t.Run("SuccessWithVersion", func(t *testing.T) {
		pipeline, _ := setup(t)

		var outputMessage string
		outputFunc := func(msg string) {
			outputMessage = msg
		}

		ctx := context.WithValue(context.Background(), "nodes", []string{"10.0.0.1"})
		ctx = context.WithValue(ctx, "version", "1.0.0")
		ctx = context.WithValue(ctx, "output", outputFunc)

		err := pipeline.executeNodeHealthCheck(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if outputMessage != "All 1 nodes are healthy and running version 1.0.0" {
			t.Errorf("Expected 'All 1 nodes are healthy and running version 1.0.0', got %q", outputMessage)
		}
	})

	t.Run("SuccessWithTimeout", func(t *testing.T) {
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "nodes", []string{"10.0.0.1"})
		ctx = context.WithValue(ctx, "timeout", 5*time.Second)

		err := pipeline.executeNodeHealthCheck(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenClusterClientIsNil", func(t *testing.T) {
		pipeline, _ := setup(t)
		pipeline.clusterClient = nil

		ctx := context.WithValue(context.Background(), "nodes", []string{"10.0.0.1"})

		err := pipeline.executeNodeHealthCheck(ctx)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !checkContains(err.Error(), "No cluster client found") {
			t.Errorf("Expected cluster client error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenNoNodesSpecified", func(t *testing.T) {
		pipeline, _ := setup(t)

		err := pipeline.executeNodeHealthCheck(context.Background())

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !checkContains(err.Error(), "No nodes specified") {
			t.Errorf("Expected no nodes error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenNodesParameterIsInvalidType", func(t *testing.T) {
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "nodes", "invalid")

		err := pipeline.executeNodeHealthCheck(ctx)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !checkContains(err.Error(), "Invalid nodes parameter type") {
			t.Errorf("Expected invalid nodes parameter error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenNodesSliceIsEmpty", func(t *testing.T) {
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "nodes", []string{})

		err := pipeline.executeNodeHealthCheck(ctx)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !checkContains(err.Error(), "No nodes specified") {
			t.Errorf("Expected no nodes error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenWaitForNodesHealthyFails", func(t *testing.T) {
		pipeline, mocks := setup(t)
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodes []string, version string) error {
			return fmt.Errorf("health check failed")
		}

		ctx := context.WithValue(context.Background(), "nodes", []string{"10.0.0.1"})

		err := pipeline.executeNodeHealthCheck(ctx)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !checkContains(err.Error(), "nodes failed health check") {
			t.Errorf("Expected health check error, got: %v", err)
		}
	})

	t.Run("HandlesNoOutputFunction", func(t *testing.T) {
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "nodes", []string{"10.0.0.1"})

		err := pipeline.executeNodeHealthCheck(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}
