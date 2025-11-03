package cmd

import (
	"bytes"
	stdcontext "context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/context/config"
	"github.com/windsorcli/cli/pkg/context/shell"
	"github.com/windsorcli/cli/pkg/context/tools"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/provisioner/cluster"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
)

// Helper function to check if a string contains a substring
func checkContains(str, substr string) bool {
	return strings.Contains(str, substr)
}

func TestCheckCmd(t *testing.T) {
	t.Cleanup(func() {
		rootCmd.SetContext(stdcontext.Background())
	})

	setup := func(t *testing.T, withConfig bool) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()

		// Change to a temporary directory
		origDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get working directory: %v", err)
		}

		tmpDir := t.TempDir()
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("Failed to change to temp directory: %v", err)
		}

		// Cleanup to change back to original directory
		t.Cleanup(func() {
			if err := os.Chdir(origDir); err != nil {
				t.Logf("Warning: Failed to change back to original directory: %v", err)
			}
		})

		// Create config file if requested
		if withConfig {
			configContent := `contexts:
  default:
    tools:
      enabled: true`
			if err := os.WriteFile("windsor.yaml", []byte(configContent), 0644); err != nil {
				t.Fatalf("Failed to create config file: %v", err)
			}
		}

		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)
		rootCmd.SetArgs([]string{"check"})

		return stdout, stderr
	}

	t.Run("Success", func(t *testing.T) {
		// Given a directory with proper configuration
		setup(t, true)

		// Set up mocks with trusted directory
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return nil
		}
		mockConfigHandler.InitializeFunc = func() error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.IsLoadedFunc = func() bool {
			return true
		}
		mocks := setupMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})

		ctx := stdcontext.WithValue(stdcontext.Background(), injectorKey, mocks.Injector)
		rootCmd.SetContext(ctx)

		mockToolsManager := tools.NewMockToolsManager()
		mockToolsManager.InitializeFunc = func() error {
			return nil
		}
		mockToolsManager.CheckFunc = func() error {
			return nil
		}
		mocks.Injector.Register("toolsManager", mockToolsManager)

		// When executing the command
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("ConfigNotLoaded", func(t *testing.T) {
		// Given a directory with no configuration
		_, _ = setup(t, false)

		// Set up mocks with trusted directory but no config loaded
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return nil
		}
		mockConfigHandler.InitializeFunc = func() error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mocks := setupMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})

		ctx := stdcontext.WithValue(stdcontext.Background(), injectorKey, mocks.Injector)
		rootCmd.SetContext(ctx)

		// When executing the command
		err := Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain init message
		if !strings.Contains(err.Error(), "Nothing to check") {
			t.Errorf("Expected error about init, got: %v", err)
		}
	})
}

// =============================================================================
// Test Error Scenarios
// =============================================================================

func TestCheckCmd_ErrorScenarios(t *testing.T) {
	t.Cleanup(func() {
		rootCmd.SetContext(stdcontext.Background())
	})

	setup := func(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()
		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)
		return stdout, stderr
	}

	t.Run("HandlesNewContextError", func(t *testing.T) {
		setup(t)
		injector := di.NewInjector()
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}
		mockShell.InitializeFunc = func() error {
			return nil
		}
		injector.Register("shell", mockShell)

		ctx := stdcontext.WithValue(stdcontext.Background(), injectorKey, injector)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"check"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when NewContext fails")
		}

		if !strings.Contains(err.Error(), "failed to initialize context") {
			t.Errorf("Expected error about context initialization, got: %v", err)
		}
	})

	t.Run("HandlesCheckTrustedDirectoryError", func(t *testing.T) {
		setup(t)
		mocks := setupMocks(t)
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not trusted")
		}

		ctx := stdcontext.WithValue(stdcontext.Background(), injectorKey, mocks.Injector)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"check"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when CheckTrustedDirectory fails")
		}

		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected error about trusted directory, got: %v", err)
		}
	})

	t.Run("HandlesLoadConfigError", func(t *testing.T) {
		setup(t)
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return fmt.Errorf("config load failed")
		}
		mockConfigHandler.InitializeFunc = func() error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		injector.Register("configHandler", mockConfigHandler)

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return t.TempDir(), nil
		}
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}
		mockShell.InitializeFunc = func() error {
			return nil
		}
		injector.Register("shell", mockShell)

		ctx := stdcontext.WithValue(stdcontext.Background(), injectorKey, injector)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"check"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when LoadConfig fails")
		}

		if !strings.Contains(err.Error(), "config load failed") && !strings.Contains(err.Error(), "failed to load config") {
			t.Errorf("Expected error about config loading, got: %v", err)
		}
	})

	t.Run("HandlesCheckToolsError", func(t *testing.T) {
		setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return nil
		}
		mockConfigHandler.InitializeFunc = func() error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.IsLoadedFunc = func() bool {
			return true
		}
		mocks := setupMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})
		mockToolsManager := tools.NewMockToolsManager()
		mockToolsManager.InitializeFunc = func() error {
			return nil
		}
		mockToolsManager.CheckFunc = func() error {
			return fmt.Errorf("tools check failed")
		}
		mocks.Injector.Register("toolsManager", mockToolsManager)

		ctx := stdcontext.WithValue(stdcontext.Background(), injectorKey, mocks.Injector)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"check"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when CheckTools fails")
		}

		if !strings.Contains(err.Error(), "tools check failed") && !strings.Contains(err.Error(), "error checking tools") {
			t.Errorf("Expected error about tools check, got: %v", err)
		}
	})
}

func TestCheckNodeHealthCmd(t *testing.T) {
	// Cleanup: reset rootCmd context after all subtests complete
	t.Cleanup(func() {
		rootCmd.SetContext(stdcontext.Background())
	})

	setup := func(t *testing.T, withConfig bool) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()

		// Change to a temporary directory
		origDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get working directory: %v", err)
		}

		tmpDir := t.TempDir()
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("Failed to change to temp directory: %v", err)
		}

		// Cleanup to change back to original directory
		t.Cleanup(func() {
			if err := os.Chdir(origDir); err != nil {
				t.Logf("Warning: Failed to change back to original directory: %v", err)
			}
		})

		// Create config file if requested
		if withConfig {
			configContent := `contexts:
  default:
    cluster:
      enabled: true`
			if err := os.WriteFile("windsor.yaml", []byte(configContent), 0644); err != nil {
				t.Fatalf("Failed to create config file: %v", err)
			}
		}

		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)

		// Reset global command flags to avoid state leakage
		nodeHealthTimeout = 0
		nodeHealthNodes = []string{}
		nodeHealthVersion = ""

		// Reset command flags
		checkNodeHealthCmd.ResetFlags()
		checkNodeHealthCmd.Flags().DurationVar(&nodeHealthTimeout, "timeout", 0, "Maximum time to wait for nodes to be ready (default 5m)")
		checkNodeHealthCmd.Flags().StringSliceVar(&nodeHealthNodes, "nodes", []string{}, "Nodes to check (required)")
		checkNodeHealthCmd.Flags().StringVar(&nodeHealthVersion, "version", "", "Expected version to check against (optional)")

		return stdout, stderr
	}

	t.Run("ClusterClientError", func(t *testing.T) {
		// Given a directory with proper configuration
		_, _ = setup(t, true)

		// Set up mocks
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return nil
		}
		mockConfigHandler.InitializeFunc = func() error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.IsLoadedFunc = func() bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return ""
		}
		mocks := setupMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})

		mockClusterClient := cluster.NewMockClusterClient()
		mockClusterClient.WaitForNodesHealthyFunc = func(ctx stdcontext.Context, nodeAddresses []string, expectedVersion string) error {
			return fmt.Errorf("cluster health check failed")
		}
		mocks.Injector.Register("clusterClient", mockClusterClient)

		ctx := stdcontext.WithValue(stdcontext.Background(), injectorKey, mocks.Injector)
		rootCmd.SetContext(ctx)

		// Setup command args with nodes
		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1,10.0.0.2"})

		// When executing the command
		err := Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain node health check message
		if !strings.Contains(err.Error(), "error checking node health") && !strings.Contains(err.Error(), "nodes failed health check") {
			t.Errorf("Expected error about node health check, got: %v", err)
		}
	})

	t.Run("NoNodesSpecified", func(t *testing.T) {
		// Given a directory with proper configuration
		_, _ = setup(t, true)

		// Setup command args without nodes
		rootCmd.SetArgs([]string{"check", "node-health"})

		// When executing the command
		err := Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain health checks message
		expectedError := "No health checks specified. Use --nodes and/or --k8s-endpoint flags to specify health checks to perform"
		if err.Error() != expectedError {
			t.Errorf("Expected error about health checks, got: %v", err)
		}
	})

	t.Run("EmptyNodesFlag", func(t *testing.T) {
		// Given a directory with proper configuration
		_, _ = setup(t, true)

		// Setup command args with empty nodes flag
		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", ""})

		// When executing the command
		err := Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain health checks message
		expectedError := "No health checks specified. Use --nodes and/or --k8s-endpoint flags to specify health checks to perform"
		if err.Error() != expectedError {
			t.Errorf("Expected error about health checks, got: %v", err)
		}
	})
}

// =============================================================================
// Test Error Scenarios
// =============================================================================

func TestCheckNodeHealthCmd_ErrorScenarios(t *testing.T) {
	t.Cleanup(func() {
		rootCmd.SetContext(stdcontext.Background())
		nodeHealthTimeout = 0
		nodeHealthNodes = []string{}
		nodeHealthVersion = ""
		k8sEndpoint = ""
		checkNodeReady = false
	})

	setup := func(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()
		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)

		nodeHealthTimeout = 0
		nodeHealthNodes = []string{}
		nodeHealthVersion = ""
		k8sEndpoint = ""
		checkNodeReady = false

		return stdout, stderr
	}

	t.Run("HandlesNewContextError", func(t *testing.T) {
		setup(t)
		injector := di.NewInjector()
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}
		mockShell.InitializeFunc = func() error {
			return nil
		}
		injector.Register("shell", mockShell)

		ctx := stdcontext.WithValue(stdcontext.Background(), injectorKey, injector)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when NewContext fails")
		}

		if !strings.Contains(err.Error(), "failed to initialize context") {
			t.Errorf("Expected error about context initialization, got: %v", err)
		}
	})

	t.Run("HandlesCheckTrustedDirectoryError", func(t *testing.T) {
		setup(t)
		mocks := setupMocks(t)
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not trusted")
		}

		ctx := stdcontext.WithValue(stdcontext.Background(), injectorKey, mocks.Injector)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when CheckTrustedDirectory fails")
		}

		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected error about trusted directory, got: %v", err)
		}
	})

	t.Run("HandlesLoadConfigError", func(t *testing.T) {
		setup(t)
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return fmt.Errorf("config load failed")
		}
		mockConfigHandler.InitializeFunc = func() error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		injector.Register("configHandler", mockConfigHandler)

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return t.TempDir(), nil
		}
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}
		mockShell.InitializeFunc = func() error {
			return nil
		}
		injector.Register("shell", mockShell)

		ctx := stdcontext.WithValue(stdcontext.Background(), injectorKey, injector)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when LoadConfig fails")
		}

		if !strings.Contains(err.Error(), "config load failed") && !strings.Contains(err.Error(), "failed to load config") {
			t.Errorf("Expected error about config loading, got: %v", err)
		}
	})

	t.Run("HandlesCheckNodeHealthError", func(t *testing.T) {
		setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return nil
		}
		mockConfigHandler.InitializeFunc = func() error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.IsLoadedFunc = func() bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return ""
		}
		mocks := setupMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})

		mockClusterClient := cluster.NewMockClusterClient()
		mockClusterClient.WaitForNodesHealthyFunc = func(ctx stdcontext.Context, nodeAddresses []string, expectedVersion string) error {
			return fmt.Errorf("cluster health check failed")
		}
		mocks.Injector.Register("clusterClient", mockClusterClient)

		ctx := stdcontext.WithValue(stdcontext.Background(), injectorKey, mocks.Injector)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when CheckNodeHealth fails")
		}

		if !strings.Contains(err.Error(), "error checking node health") && !strings.Contains(err.Error(), "nodes failed health check") {
			t.Errorf("Expected error about node health check, got: %v", err)
		}
	})

	t.Run("HandlesKubernetesManagerError", func(t *testing.T) {
		setup(t)
		nodeHealthNodes = []string{}
		nodeHealthTimeout = 0
		nodeHealthVersion = ""
		k8sEndpoint = ""
		checkNodeReady = false

		checkNodeHealthCmd.ResetFlags()
		checkNodeHealthCmd.Flags().DurationVar(&nodeHealthTimeout, "timeout", 0, "Maximum time to wait for nodes to be ready (default 5m)")
		checkNodeHealthCmd.Flags().StringSliceVar(&nodeHealthNodes, "nodes", []string{}, "Nodes to check (optional)")
		checkNodeHealthCmd.Flags().StringVar(&nodeHealthVersion, "version", "", "Expected version to check against (optional)")
		checkNodeHealthCmd.Flags().StringVar(&k8sEndpoint, "k8s-endpoint", "", "Perform Kubernetes API health check (use --k8s-endpoint or --k8s-endpoint=https://endpoint:6443)")
		checkNodeHealthCmd.Flags().Lookup("k8s-endpoint").NoOptDefVal = "true"
		checkNodeHealthCmd.Flags().BoolVar(&checkNodeReady, "ready", false, "Check Kubernetes node readiness status")

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return nil
		}
		mockConfigHandler.InitializeFunc = func() error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.IsLoadedFunc = func() bool {
			return true
		}
		mocks := setupMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})

		mockKubernetesManager := kubernetes.NewMockKubernetesManager(mocks.Injector)
		mockKubernetesManager.InitializeFunc = func() error {
			return nil
		}
		mockKubernetesManager.WaitForKubernetesHealthyFunc = func(ctx stdcontext.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
			return fmt.Errorf("kubernetes health check failed")
		}
		mocks.Injector.Register("kubernetesManager", mockKubernetesManager)

		ctx := stdcontext.WithValue(stdcontext.Background(), injectorKey, mocks.Injector)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"check", "node-health", "--k8s-endpoint", "https://test:6443"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when Kubernetes health check fails")
		}

		if !strings.Contains(err.Error(), "error checking node health") && !strings.Contains(err.Error(), "kubernetes health check failed") {
			t.Errorf("Expected error about kubernetes health check, got: %v", err)
		}
	})
}
