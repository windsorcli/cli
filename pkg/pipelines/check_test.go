package pipelines

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/environment/tools"
	"github.com/windsorcli/cli/pkg/infrastructure/cluster"
	"github.com/windsorcli/cli/pkg/infrastructure/kubernetes"
	"github.com/windsorcli/cli/pkg/shell"
)

// mockFileInfo implements os.FileInfo for testing
type mockFileInfo struct{}

func (m *mockFileInfo) Name() string       { return "windsor.yaml" }
func (m *mockFileInfo) Size() int64        { return 100 }
func (m *mockFileInfo) Mode() os.FileMode  { return 0644 }
func (m *mockFileInfo) ModTime() time.Time { return time.Now() }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Sys() interface{}   { return nil }

// =============================================================================
// Test Setup Infrastructure
// =============================================================================

// CheckMocks extends the base Mocks with check-specific dependencies
type CheckMocks struct {
	*Mocks
	ToolsManager      *tools.MockToolsManager
	ClusterClient     *cluster.MockClusterClient
	KubernetesManager *kubernetes.MockKubernetesManager
}

// setupCheckMocks creates mocks for check pipeline tests
func setupCheckMocks(t *testing.T, opts ...*SetupOptions) *CheckMocks {
	t.Helper()

	// Create setup options, preserving any provided options
	setupOptions := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		setupOptions = opts[0]
	}

	// Only create a default mock config handler if one wasn't provided
	if setupOptions.ConfigHandler == nil {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error { return nil }
		mockConfigHandler.IsLoadedFunc = func() bool { return true } // Default to loaded
		setupOptions.ConfigHandler = mockConfigHandler
	}

	baseMocks := setupMocks(t, setupOptions)

	// Create check-specific mocks only if they don't already exist
	var mockToolsManager *tools.MockToolsManager
	if existing := baseMocks.Injector.Resolve("toolsManager"); existing != nil {
		if tm, ok := existing.(*tools.MockToolsManager); ok {
			mockToolsManager = tm
		} else {
			// If existing is not a MockToolsManager, create a new one
			mockToolsManager = tools.NewMockToolsManager()
			mockToolsManager.InitializeFunc = func() error { return nil }
			mockToolsManager.CheckFunc = func() error { return nil }
			baseMocks.Injector.Register("toolsManager", mockToolsManager)
		}
	} else {
		mockToolsManager = tools.NewMockToolsManager()
		mockToolsManager.InitializeFunc = func() error { return nil }
		mockToolsManager.CheckFunc = func() error { return nil }
		baseMocks.Injector.Register("toolsManager", mockToolsManager)
	}

	var mockClusterClient *cluster.MockClusterClient
	if existing := baseMocks.Injector.Resolve("clusterClient"); existing != nil {
		if cc, ok := existing.(*cluster.MockClusterClient); ok {
			mockClusterClient = cc
		} else {
			// If existing is not a MockClusterClient, create a new one
			mockClusterClient = cluster.NewMockClusterClient()
			mockClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
				return nil
			}
			baseMocks.Injector.Register("clusterClient", mockClusterClient)
		}
	} else {
		mockClusterClient = cluster.NewMockClusterClient()
		mockClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
			return nil
		}
		baseMocks.Injector.Register("clusterClient", mockClusterClient)
	}

	// Create kubernetes manager mock
	var mockKubernetesManager *kubernetes.MockKubernetesManager
	if existing := baseMocks.Injector.Resolve("kubernetesManager"); existing != nil {
		if km, ok := existing.(*kubernetes.MockKubernetesManager); ok {
			mockKubernetesManager = km
		} else {
			// If existing is not a MockKubernetesManager, create a new one
			mockKubernetesManager = kubernetes.NewMockKubernetesManager(baseMocks.Injector)
			mockKubernetesManager.InitializeFunc = func() error { return nil }
			mockKubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
				return nil
			}
			baseMocks.Injector.Register("kubernetesManager", mockKubernetesManager)
		}
	} else {
		mockKubernetesManager = kubernetes.NewMockKubernetesManager(baseMocks.Injector)
		mockKubernetesManager.InitializeFunc = func() error { return nil }
		mockKubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}
		baseMocks.Injector.Register("kubernetesManager", mockKubernetesManager)
	}

	return &CheckMocks{
		Mocks:             baseMocks,
		ToolsManager:      mockToolsManager,
		ClusterClient:     mockClusterClient,
		KubernetesManager: mockKubernetesManager,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewCheckPipeline(t *testing.T) {
	t.Run("CreatesWithDefaults", func(t *testing.T) {
		// Given creating a new check pipeline
		pipeline := NewCheckPipeline()

		// Then pipeline should not be nil
		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}
	})
}

// =============================================================================
// Test Public Methods - Initialize
// =============================================================================

func TestCheckPipeline_Initialize(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*CheckPipeline, *CheckMocks) {
		t.Helper()
		pipeline := NewCheckPipeline()
		mocks := setupCheckMocks(t, opts...)
		return pipeline, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a check pipeline
		pipeline, mocks := setup(t)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerInitializeFails", func(t *testing.T) {
		// Given a check pipeline with failing config handler initialization
		pipeline := NewCheckPipeline()

		// Create injector and register failing config handler directly
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error {
			return fmt.Errorf("config initialization failed")
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
		if err.Error() != "failed to initialize config handler: config initialization failed" {
			t.Errorf("Expected config handler error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenShellInitializeFails", func(t *testing.T) {
		// Given a check pipeline with failing shell initialization
		pipeline, mocks := setup(t)

		mocks.Shell.InitializeFunc = func() error {
			return fmt.Errorf("shell initialization failed")
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to initialize shell: shell initialization failed" {
			t.Errorf("Expected shell error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenToolsManagerInitializeFails", func(t *testing.T) {
		// Given a check pipeline with failing tools manager initialization
		pipeline := NewCheckPipeline()
		mocks := setupCheckMocks(t)

		mocks.ToolsManager.InitializeFunc = func() error {
			return fmt.Errorf("tools manager initialization failed")
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to initialize tools manager: tools manager initialization failed" {
			t.Errorf("Expected tools manager error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenLoadConfigFails", func(t *testing.T) {
		// Given a check pipeline with failing config loading
		pipeline := NewCheckPipeline()

		// Create injector and register failing config handler directly
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error { return nil }
		mockConfigHandler.LoadConfigFunc = func() error {
			return fmt.Errorf("config loading failed")
		}
		injector.Register("configHandler", mockConfigHandler)

		// Create and register basic shell
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error { return nil }
		mockShell.GetProjectRootFunc = func() (string, error) { return t.TempDir(), nil }
		injector.Register("shell", mockShell)

		// Register shims that simulate config file exists
		shims := setupShims(t)
		shims.Stat = func(name string) (os.FileInfo, error) {
			// Simulate windsor.yaml exists
			if strings.HasSuffix(name, "windsor.yaml") {
				return &mockFileInfo{}, nil
			}
			return nil, os.ErrNotExist
		}
		injector.Register("shims", shims)

		// When initializing the pipeline
		err := pipeline.Initialize(injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to load context config: config loading failed" {
			t.Errorf("Expected config loading error, got: %v", err)
		}
	})

	t.Run("ReusesExistingComponentsFromDIContainer", func(t *testing.T) {
		// Given a check pipeline with pre-registered components
		injector := di.NewInjector()
		existingToolsManager := tools.NewMockToolsManager()
		existingToolsManager.InitializeFunc = func() error { return nil }
		injector.Register("toolsManager", existingToolsManager)

		pipeline, mocks := setup(t, &SetupOptions{
			Injector: injector,
		})

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned and existing components should be reused
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		resolvedToolsManager := mocks.Injector.Resolve("toolsManager")
		if resolvedToolsManager != existingToolsManager {
			t.Error("Expected existing tools manager to be reused")
		}
	})
}

// =============================================================================
// Test Public Methods - Execute
// =============================================================================

func TestCheckPipeline_Execute(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*CheckPipeline, *CheckMocks) {
		t.Helper()
		pipeline := NewCheckPipeline()
		mocks := setupCheckMocks(t, opts...)

		// Initialize the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		return pipeline, mocks
	}

	t.Run("ExecutesToolsCheckByDefault", func(t *testing.T) {
		// Given a check pipeline
		pipeline, mocks := setup(t)

		checkCalled := false
		mocks.ToolsManager.CheckFunc = func() error {
			checkCalled = true
			return nil
		}

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And tools check should be called
		if !checkCalled {
			t.Error("Expected tools check to be called")
		}
	})

	t.Run("ExecutesToolsCheckExplicitly", func(t *testing.T) {
		// Given a check pipeline with explicit tools operation
		pipeline, mocks := setup(t)

		checkCalled := false
		mocks.ToolsManager.CheckFunc = func() error {
			checkCalled = true
			return nil
		}

		ctx := context.WithValue(context.Background(), "operation", "tools")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And tools check should be called
		if !checkCalled {
			t.Error("Expected tools check to be called")
		}
	})

	t.Run("ExecutesNodeHealthCheck", func(t *testing.T) {
		// Given a check pipeline with node health operation
		pipeline, mocks := setup(t)

		waitCalled := false
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
			waitCalled = true
			return nil
		}

		ctx := context.WithValue(context.Background(), "operation", "node-health")
		ctx = context.WithValue(ctx, "nodes", []string{"node1", "node2"})

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And node health check should be called
		if !waitCalled {
			t.Error("Expected node health check to be called")
		}
	})

	t.Run("ExecutesNodeHealthCheckWithVersion", func(t *testing.T) {
		// Given a check pipeline with node health operation and version
		pipeline, mocks := setup(t)

		var capturedVersion string
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
			capturedVersion = expectedVersion
			return nil
		}

		ctx := context.WithValue(context.Background(), "operation", "node-health")
		ctx = context.WithValue(ctx, "nodes", []string{"node1"})
		ctx = context.WithValue(ctx, "version", "v1.30.0")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And node health check should be called with version
		if capturedVersion != "v1.30.0" {
			t.Errorf("Expected version v1.30.0, got %s", capturedVersion)
		}
	})

	t.Run("ReturnsErrorWhenConfigNotLoaded", func(t *testing.T) {
		// Given a check pipeline with config not loaded
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error { return nil }
		mockConfigHandler.IsLoadedFunc = func() bool { return false }

		pipeline, _ := setup(t, &SetupOptions{
			ConfigHandler: mockConfigHandler,
		})

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		expectedMsg := "Nothing to check. Have you run \033[1mwindsor init\033[0m?"
		if err.Error() != expectedMsg {
			t.Errorf("Expected config not loaded error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidOperationType", func(t *testing.T) {
		// Given a check pipeline with invalid operation type
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "operation", 123)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Invalid operation type" {
			t.Errorf("Expected operation type error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForUnknownOperation", func(t *testing.T) {
		// Given a check pipeline with unknown operation
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "operation", "unknown")

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Unknown operation type: unknown" {
			t.Errorf("Expected unknown operation error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenToolsCheckFails", func(t *testing.T) {
		// Given a check pipeline with failing tools check
		pipeline, mocks := setup(t)

		mocks.ToolsManager.CheckFunc = func() error {
			return fmt.Errorf("tools check failed")
		}

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error checking tools: tools check failed" {
			t.Errorf("Expected tools check error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenNodeHealthCheckFails", func(t *testing.T) {
		// Given a check pipeline with failing node health check
		pipeline, mocks := setup(t)

		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
			return fmt.Errorf("node health check failed")
		}

		ctx := context.WithValue(context.Background(), "operation", "node-health")
		ctx = context.WithValue(ctx, "nodes", []string{"node1"})

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "nodes failed health check: node health check failed" {
			t.Errorf("Expected node health check error, got: %v", err)
		}
	})
}

// =============================================================================
// Test Private Methods - executeToolsCheck
// =============================================================================

func TestCheckPipeline_executeToolsCheck(t *testing.T) {
	setup := func(t *testing.T) (*CheckPipeline, *CheckMocks) {
		t.Helper()
		pipeline := NewCheckPipeline()
		mocks := setupCheckMocks(t)

		// Initialize the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		return pipeline, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a check pipeline
		pipeline, mocks := setup(t)

		checkCalled := false
		mocks.ToolsManager.CheckFunc = func() error {
			checkCalled = true
			return nil
		}

		// When executing tools check
		err := pipeline.executeToolsCheck(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And tools check should be called
		if !checkCalled {
			t.Error("Expected tools check to be called")
		}
	})

	t.Run("ReturnsErrorWhenToolsManagerCheckFails", func(t *testing.T) {
		// Given a check pipeline with failing tools manager
		pipeline, mocks := setup(t)

		mocks.ToolsManager.CheckFunc = func() error {
			return fmt.Errorf("tools check failed")
		}

		// When executing tools check
		err := pipeline.executeToolsCheck(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "Error checking tools: tools check failed" {
			t.Errorf("Expected tools check error, got: %v", err)
		}
	})

	t.Run("HandlesNoOutputFunction", func(t *testing.T) {
		// Given a check pipeline with no output function
		pipeline, _ := setup(t)

		// When executing tools check
		err := pipeline.executeToolsCheck(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// =============================================================================
// Test Private Methods - executeNodeHealthCheck
// =============================================================================

func TestCheckPipeline_executeNodeHealthCheck(t *testing.T) {
	setup := func(t *testing.T) (*CheckPipeline, *CheckMocks) {
		t.Helper()
		pipeline := NewCheckPipeline()
		mocks := setupCheckMocks(t)

		// Initialize the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		return pipeline, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a check pipeline
		pipeline, mocks := setup(t)

		waitCalled := false
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
			waitCalled = true
			return nil
		}

		ctx := context.WithValue(context.Background(), "nodes", []string{"node1", "node2"})

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And node health check should be called
		if !waitCalled {
			t.Error("Expected node health check to be called")
		}
	})

	t.Run("SuccessWithVersion", func(t *testing.T) {
		// Given a check pipeline with version specified
		pipeline, mocks := setup(t)

		var capturedVersion string
		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
			capturedVersion = expectedVersion
			return nil
		}

		ctx := context.WithValue(context.Background(), "nodes", []string{"node1"})
		ctx = context.WithValue(ctx, "version", "v1.30.0")

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And node health check should be called with version
		if capturedVersion != "v1.30.0" {
			t.Errorf("Expected version v1.30.0, got %s", capturedVersion)
		}
	})

	t.Run("SuccessWithTimeout", func(t *testing.T) {
		// Given a check pipeline with timeout specified
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "nodes", []string{"node1"})
		ctx = context.WithValue(ctx, "timeout", 30*time.Second)

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SucceedsWhenClusterClientIsNilButK8sEndpointProvided", func(t *testing.T) {
		// Given a check pipeline with nil cluster client but k8s endpoint provided
		pipeline, mocks := setup(t)
		pipeline.clusterClient = nil

		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}

		ctx := context.WithValue(context.Background(), "nodes", []string{"node1"})
		ctx = context.WithValue(ctx, "k8s-endpoint-provided", true)
		ctx = context.WithValue(ctx, "k8s-endpoint", "")
		ctx = context.WithValue(ctx, "output", func(msg string) {})

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then no error should be returned (k8s check succeeds)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenClusterClientIsNilAndNoK8sEndpoint", func(t *testing.T) {
		// Given a check pipeline with nil cluster client and no k8s endpoint
		pipeline, _ := setup(t)
		pipeline.clusterClient = nil

		ctx := context.WithValue(context.Background(), "nodes", []string{"node1"})

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "No health checks specified. Use --nodes and/or --k8s-endpoint flags to specify health checks to perform" {
			t.Errorf("Expected health checks required error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenNoHealthChecksSpecified", func(t *testing.T) {
		// Given a check pipeline with no health checks specified
		pipeline, _ := setup(t)

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "No health checks specified. Use --nodes and/or --k8s-endpoint flags to specify health checks to perform" {
			t.Errorf("Expected health checks required error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenNodesParameterIsInvalidType", func(t *testing.T) {
		// Given a check pipeline with invalid nodes parameter type
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "nodes", "invalid")

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "No health checks specified. Use --nodes and/or --k8s-endpoint flags to specify health checks to perform" {
			t.Errorf("Expected health checks required error, got: %v", err)
		}
	})

	t.Run("SucceedsWhenOnlyK8sEndpointSpecified", func(t *testing.T) {
		// Given a check pipeline with only k8s endpoint specified
		pipeline, mocks := setup(t)

		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}

		ctx := context.WithValue(context.Background(), "k8s-endpoint-provided", true)
		ctx = context.WithValue(ctx, "k8s-endpoint", "")
		ctx = context.WithValue(ctx, "output", func(msg string) {})

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenWaitForNodesHealthyFails", func(t *testing.T) {
		// Given a check pipeline with failing cluster client
		pipeline, mocks := setup(t)

		mocks.ClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
			return fmt.Errorf("node health check failed")
		}

		ctx := context.WithValue(context.Background(), "nodes", []string{"node1"})

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "nodes failed health check: node health check failed" {
			t.Errorf("Expected node health check error, got: %v", err)
		}
	})

	t.Run("HandlesNoOutputFunction", func(t *testing.T) {
		// Given a check pipeline with no output function
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "nodes", []string{"node1"})

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	// =============================================================================
	// --ready flag tests
	// =============================================================================

	t.Run("K8sEndpointOnly_NoReadyFlag", func(t *testing.T) {
		// Given a check pipeline with only k8s endpoint specified (no --ready)
		pipeline, mocks := setup(t)

		var capturedNodeNames []string
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			capturedNodeNames = nodeNames
			return nil
		}

		ctx := context.WithValue(context.Background(), "k8s-endpoint-provided", true)
		ctx = context.WithValue(ctx, "k8s-endpoint", "")
		ctx = context.WithValue(ctx, "output", func(msg string) {})

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And no node names should be passed (no readiness check)
		if len(capturedNodeNames) != 0 {
			t.Errorf("Expected no node names, got %v", capturedNodeNames)
		}
	})

	t.Run("K8sEndpointWithReadyFlag_NoSpecificNodes", func(t *testing.T) {
		// Given a check pipeline with k8s endpoint and --ready flag (no specific nodes)
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "k8s-endpoint-provided", true)
		ctx = context.WithValue(ctx, "k8s-endpoint", "")
		ctx = context.WithValue(ctx, "check-node-ready", true)
		ctx = context.WithValue(ctx, "output", func(msg string) {})

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "--ready flag requires --nodes to be specified" {
			t.Errorf("Expected error about --ready requiring --nodes, got: %v", err)
		}
	})

	t.Run("K8sEndpointWithReadyFlag_SpecificNodes", func(t *testing.T) {
		// Given a check pipeline with k8s endpoint, --ready flag, and specific nodes
		pipeline, mocks := setup(t)

		var capturedNodeNames []string
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			capturedNodeNames = nodeNames
			return nil
		}

		ctx := context.WithValue(context.Background(), "k8s-endpoint-provided", true)
		ctx = context.WithValue(ctx, "k8s-endpoint", "")
		ctx = context.WithValue(ctx, "check-node-ready", true)
		ctx = context.WithValue(ctx, "nodes", []string{"specific-node1", "specific-node2"})
		ctx = context.WithValue(ctx, "output", func(msg string) {})

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And specific node names should be passed for readiness check
		expectedNodeNames := []string{"specific-node1", "specific-node2"}
		if len(capturedNodeNames) != len(expectedNodeNames) {
			t.Errorf("Expected %d node names, got %d", len(expectedNodeNames), len(capturedNodeNames))
		}
		for i, name := range expectedNodeNames {
			if capturedNodeNames[i] != name {
				t.Errorf("Expected node name %s at index %d, got %s", name, i, capturedNodeNames[i])
			}
		}
	})

	t.Run("K8sEndpointWithNodes_NoReadyFlag", func(t *testing.T) {
		// Given a check pipeline with k8s endpoint and specific nodes but no --ready flag
		pipeline, mocks := setup(t)

		var capturedNodeNames []string
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			capturedNodeNames = nodeNames
			return nil
		}

		ctx := context.WithValue(context.Background(), "k8s-endpoint-provided", true)
		ctx = context.WithValue(ctx, "k8s-endpoint", "")
		ctx = context.WithValue(ctx, "nodes", []string{"node1", "node2"})
		ctx = context.WithValue(ctx, "output", func(msg string) {})

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And no node names should be passed (no readiness check)
		if len(capturedNodeNames) != 0 {
			t.Errorf("Expected no node names, got %v", capturedNodeNames)
		}
	})

	t.Run("ReadyFlagOnly_NoK8sEndpoint", func(t *testing.T) {
		// Given a check pipeline with only --ready flag (no k8s endpoint)
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "check-node-ready", true)

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "No health checks specified. Use --nodes and/or --k8s-endpoint flags to specify health checks to perform" {
			t.Errorf("Expected health checks required error, got: %v", err)
		}
	})

	t.Run("ReadyFlagFalse", func(t *testing.T) {
		// Given a check pipeline with k8s endpoint and --ready=false
		pipeline, mocks := setup(t)

		var capturedNodeNames []string
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			capturedNodeNames = nodeNames
			return nil
		}

		ctx := context.WithValue(context.Background(), "k8s-endpoint-provided", true)
		ctx = context.WithValue(ctx, "k8s-endpoint", "")
		ctx = context.WithValue(ctx, "check-node-ready", false)
		ctx = context.WithValue(ctx, "output", func(msg string) {})

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And no node names should be passed (ready=false)
		if len(capturedNodeNames) != 0 {
			t.Errorf("Expected no node names, got %v", capturedNodeNames)
		}
	})

	t.Run("ReadyFlagNil", func(t *testing.T) {
		// Given a check pipeline with k8s endpoint and no ready flag (nil)
		pipeline, mocks := setup(t)

		var capturedNodeNames []string
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			capturedNodeNames = nodeNames
			return nil
		}

		ctx := context.WithValue(context.Background(), "k8s-endpoint-provided", true)
		ctx = context.WithValue(ctx, "k8s-endpoint", "")
		ctx = context.WithValue(ctx, "output", func(msg string) {})

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And no node names should be passed (ready=nil)
		if len(capturedNodeNames) != 0 {
			t.Errorf("Expected no node names, got %v", capturedNodeNames)
		}
	})

	t.Run("ShowsWaitingMessageWhenReadyFlagUsed", func(t *testing.T) {
		// Given a check pipeline with --ready flag and specific nodes
		pipeline, mocks := setup(t)

		var capturedMessages []string
		outputFunc := func(msg string) {
			capturedMessages = append(capturedMessages, msg)
		}

		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}

		ctx := context.WithValue(context.Background(), "k8s-endpoint-provided", true)
		ctx = context.WithValue(ctx, "k8s-endpoint", "")
		ctx = context.WithValue(ctx, "check-node-ready", true)
		ctx = context.WithValue(ctx, "nodes", []string{"node1", "node2"})
		ctx = context.WithValue(ctx, "output", outputFunc)

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And waiting message should be shown
		expectedMessage := "Waiting for 2 nodes to be Ready..."
		found := false
		for _, msg := range capturedMessages {
			if msg == expectedMessage {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected waiting message '%s', got messages: %v", expectedMessage, capturedMessages)
		}
	})

	t.Run("PassesOutputFuncToWaitForKubernetesHealthy", func(t *testing.T) {
		// Given a check pipeline with --ready flag
		pipeline, mocks := setup(t)

		var capturedOutputFunc func(string)
		var capturedNodeNames []string
		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			capturedOutputFunc = outputFunc
			capturedNodeNames = nodeNames
			return nil
		}

		outputFunc := func(msg string) {}
		ctx := context.WithValue(context.Background(), "k8s-endpoint-provided", true)
		ctx = context.WithValue(ctx, "k8s-endpoint", "")
		ctx = context.WithValue(ctx, "check-node-ready", true)
		ctx = context.WithValue(ctx, "nodes", []string{"node1"})
		ctx = context.WithValue(ctx, "output", outputFunc)

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And output function should be passed to WaitForKubernetesHealthy
		if capturedOutputFunc == nil {
			t.Error("Expected output function to be passed to WaitForKubernetesHealthy")
		}

		// And node names should be passed
		if len(capturedNodeNames) != 1 || capturedNodeNames[0] != "node1" {
			t.Errorf("Expected node names ['node1'], got %v", capturedNodeNames)
		}
	})

	t.Run("NoImmediateNotFoundMessages", func(t *testing.T) {
		// Given a check pipeline with --ready flag
		pipeline, mocks := setup(t)

		var capturedMessages []string
		outputFunc := func(msg string) {
			capturedMessages = append(capturedMessages, msg)
		}

		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}

		ctx := context.WithValue(context.Background(), "k8s-endpoint-provided", true)
		ctx = context.WithValue(ctx, "k8s-endpoint", "")
		ctx = context.WithValue(ctx, "check-node-ready", true)
		ctx = context.WithValue(ctx, "nodes", []string{"node1"})
		ctx = context.WithValue(ctx, "output", outputFunc)

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And no immediate "NOT FOUND" messages should be shown
		for _, msg := range capturedMessages {
			if strings.Contains(msg, "NOT FOUND") {
				t.Errorf("Expected no immediate NOT FOUND messages, but found: %s", msg)
			}
		}

		// And the waiting message should be shown (along with Talos health check messages)
		waitingMessageFound := false
		for _, msg := range capturedMessages {
			if msg == "Waiting for 1 nodes to be Ready..." {
				waitingMessageFound = true
				break
			}
		}
		if !waitingMessageFound {
			t.Errorf("Expected waiting message 'Waiting for 1 nodes to be Ready...', got messages: %v", capturedMessages)
		}
	})

	t.Run("HandlesNilOutputFunc", func(t *testing.T) {
		// Given a check pipeline with --ready flag but no output function
		pipeline, mocks := setup(t)

		mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return nil
		}

		ctx := context.WithValue(context.Background(), "k8s-endpoint-provided", true)
		ctx = context.WithValue(ctx, "k8s-endpoint", "")
		ctx = context.WithValue(ctx, "check-node-ready", true)
		ctx = context.WithValue(ctx, "nodes", []string{"node1"})
		// No output function in context

		// When executing node health check
		err := pipeline.executeNodeHealthCheck(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}
