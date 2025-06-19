package cmd

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/cluster"
	"github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/tools"
)

func TestCheckCmd(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*Mocks, *bytes.Buffer, *bytes.Buffer) {
		t.Helper()

		// Setup mocks with default options
		mocks := setupMocks(t, opts...)

		// Setup command args and output
		rootCmd.SetArgs([]string{"check"})
		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)

		return mocks, stdout, stderr
	}

	t.Run("Success", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, stdout, stderr := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// And mock tools manager that returns success
		mockToolsManager := tools.NewMockToolsManager()
		mockToolsManager.CheckFunc = func() error {
			return nil
		}
		mocks.Injector.Register("toolsManager", mockToolsManager)

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And output should contain success message
		output := stdout.String()
		if output != "All tools are up to date.\n" {
			t.Errorf("Expected 'All tools are up to date.', got: %q", output)
		}
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("ConfigNotLoaded", func(t *testing.T) {
		// Given a set of mocks with no configuration
		mocks, _, _ := setup(t)

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain init message
		expectedError := "Nothing to check. Have you run \033[1mwindsor init\033[0m?"
		if err.Error() != expectedError {
			t.Errorf("Expected error about init, got: %v", err)
		}
	})

	t.Run("ToolsManagerNotFound", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// And mock controller that returns nil tools manager
		mocks.Controller.ResolveToolsManagerFunc = func() tools.ToolsManager {
			return nil
		}

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain tools manager message
		if err.Error() != "No tools manager found" {
			t.Errorf("Expected error about tools manager, got: %v", err)
		}
	})

	t.Run("ToolsCheckError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// And mock tools manager that returns error
		mockToolsManager := tools.NewMockToolsManager()
		mockToolsManager.CheckFunc = func() error {
			return fmt.Errorf("tools check failed")
		}
		mocks.Injector.Register("toolsManager", mockToolsManager)

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain tools check message
		if err.Error() != "Error checking tools: tools check failed" {
			t.Errorf("Expected error about tools check, got: %v", err)
		}
	})

	t.Run("InitializeWithRequirementsError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// And mock controller that returns error on initialization
		mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error {
			return fmt.Errorf("initialization failed")
		}

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain initialization message
		if err.Error() != "Error initializing: initialization failed" {
			t.Errorf("Expected error about initialization, got: %v", err)
		}
	})
}

func TestCheckNodeHealthCmd(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*Mocks, *bytes.Buffer, *bytes.Buffer) {
		t.Helper()

		// Setup mocks with default options
		mocks := setupMocks(t, opts...)

		// Setup command args and output
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

		return mocks, stdout, stderr
	}

	t.Run("Success", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, stdout, stderr := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    cluster:
      enabled: true`,
		})

		// And mock cluster client that returns success
		mockClusterClient := cluster.NewMockClusterClient()
		mockClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
			return nil
		}
		mocks.Controller.ResolveClusterClientFunc = func() cluster.ClusterClient {
			return mockClusterClient
		}

		// Setup command args
		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1,10.0.0.2"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And output should contain success message
		output := stdout.String()
		expectedOutput := "All 2 nodes are healthy\n"
		if output != expectedOutput {
			t.Errorf("Expected %q, got: %q", expectedOutput, output)
		}
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SuccessWithVersion", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, stdout, stderr := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    cluster:
      enabled: true`,
		})

		// And mock cluster client that returns success
		mockClusterClient := cluster.NewMockClusterClient()
		mockClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
			// Verify expected version is passed correctly
			if expectedVersion != "v1.0.0" {
				return fmt.Errorf("unexpected version: %s", expectedVersion)
			}
			return nil
		}
		mocks.Controller.ResolveClusterClientFunc = func() cluster.ClusterClient {
			return mockClusterClient
		}

		// Setup command args with version
		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1", "--version", "v1.0.0"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And output should contain success message with version
		output := stdout.String()
		expectedOutput := "All 1 nodes are healthy and running version v1.0.0\n"
		if output != expectedOutput {
			t.Errorf("Expected %q, got: %q", expectedOutput, output)
		}
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SuccessWithCustomTimeout", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, stdout, stderr := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    cluster:
      enabled: true`,
		})

		// And mock cluster client that tracks timeout
		mockClusterClient := cluster.NewMockClusterClient()
		timeoutReceived := time.Duration(0)
		mockClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
			// Extract timeout from context
			deadline, ok := ctx.Deadline()
			if ok {
				timeoutReceived = time.Until(deadline)
			}
			return nil
		}
		mocks.Controller.ResolveClusterClientFunc = func() cluster.ClusterClient {
			return mockClusterClient
		}

		// Setup command args with custom timeout
		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1", "--timeout", "2m"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And timeout should be approximately 2 minutes (allow some variance)
		expectedTimeout := 2 * time.Minute
		if timeoutReceived < expectedTimeout-10*time.Second || timeoutReceived > expectedTimeout+10*time.Second {
			t.Errorf("Expected timeout around %v, got %v", expectedTimeout, timeoutReceived)
		}

		// And output should contain success message
		output := stdout.String()
		if output != "All 1 nodes are healthy\n" {
			t.Errorf("Expected success message, got: %q", output)
		}
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("ConfigNotLoaded", func(t *testing.T) {
		// Given a set of mocks with no configuration
		mocks, _, _ := setup(t)

		// Setup command args
		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain init message
		expectedError := "Nothing to check. Have you run \033[1mwindsor init\033[0m?"
		if err.Error() != expectedError {
			t.Errorf("Expected error about init, got: %v", err)
		}
	})

	t.Run("ClusterClientNotFound", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    cluster:
      enabled: true`,
		})

		// And mock controller that returns nil cluster client
		mocks.Controller.ResolveClusterClientFunc = func() cluster.ClusterClient {
			return nil
		}

		// Setup command args
		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain cluster client message
		if err.Error() != "No cluster client found" {
			t.Errorf("Expected error about cluster client, got: %v", err)
		}
	})

	t.Run("NoNodesSpecified", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    cluster:
      enabled: true`,
		})

		// And mock cluster client
		mockClusterClient := cluster.NewMockClusterClient()
		mocks.Controller.ResolveClusterClientFunc = func() cluster.ClusterClient {
			return mockClusterClient
		}

		// Setup command args without nodes
		rootCmd.SetArgs([]string{"check", "node-health"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain nodes requirement message
		expectedError := "No nodes specified. Use --nodes flag to specify nodes to check"
		if err.Error() != expectedError {
			t.Errorf("Expected error about nodes requirement, got: %v", err)
		}
	})

	t.Run("HealthCheckError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    cluster:
      enabled: true`,
		})

		// And mock cluster client that returns error
		mockClusterClient := cluster.NewMockClusterClient()
		mockClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
			return fmt.Errorf("health check failed")
		}
		mocks.Controller.ResolveClusterClientFunc = func() cluster.ClusterClient {
			return mockClusterClient
		}

		// Setup command args
		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain health check message
		expectedError := "nodes failed health check: health check failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error about health check, got: %v", err)
		}
	})

	t.Run("InitializeWithRequirementsError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    cluster:
      enabled: true`,
		})

		// And mock controller that returns error on initialization
		mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error {
			return fmt.Errorf("initialization failed")
		}

		// Setup command args
		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain initialization message
		if err.Error() != "Error initializing: initialization failed" {
			t.Errorf("Expected error about initialization, got: %v", err)
		}
	})

	t.Run("MultipleNodes", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, stdout, stderr := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    cluster:
      enabled: true`,
		})

		// And mock cluster client that validates node addresses
		mockClusterClient := cluster.NewMockClusterClient()
		mockClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
			// Verify correct node addresses are passed
			expectedNodes := []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}
			if len(nodeAddresses) != len(expectedNodes) {
				return fmt.Errorf("expected %d nodes, got %d", len(expectedNodes), len(nodeAddresses))
			}
			for i, expected := range expectedNodes {
				if nodeAddresses[i] != expected {
					return fmt.Errorf("expected node %s at index %d, got %s", expected, i, nodeAddresses[i])
				}
			}
			return nil
		}
		mocks.Controller.ResolveClusterClientFunc = func() cluster.ClusterClient {
			return mockClusterClient
		}

		// Setup command args with multiple nodes
		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1,10.0.0.2,10.0.0.3"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And output should contain success message for multiple nodes
		output := stdout.String()
		expectedOutput := "All 3 nodes are healthy\n"
		if output != expectedOutput {
			t.Errorf("Expected %q, got: %q", expectedOutput, output)
		}
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("ClusterClientCloseCalledOnSuccess", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    cluster:
      enabled: true`,
		})

		// And mock cluster client that tracks close calls
		closeCalled := false
		mockClusterClient := cluster.NewMockClusterClient()
		mockClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
			return nil
		}
		mockClusterClient.CloseFunc = func() {
			closeCalled = true
		}
		mocks.Controller.ResolveClusterClientFunc = func() cluster.ClusterClient {
			return mockClusterClient
		}

		// Setup command args
		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And Close should be called
		if !closeCalled {
			t.Error("Expected Close to be called on cluster client")
		}
	})

	t.Run("ClusterClientCloseCalledOnError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    cluster:
      enabled: true`,
		})

		// And mock cluster client that tracks close calls and returns error
		closeCalled := false
		mockClusterClient := cluster.NewMockClusterClient()
		mockClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
			return fmt.Errorf("health check failed")
		}
		mockClusterClient.CloseFunc = func() {
			closeCalled = true
		}
		mocks.Controller.ResolveClusterClientFunc = func() cluster.ClusterClient {
			return mockClusterClient
		}

		// Setup command args
		rootCmd.SetArgs([]string{"check", "node-health", "--nodes", "10.0.0.1"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And Close should still be called
		if !closeCalled {
			t.Error("Expected Close to be called on cluster client even on error")
		}
	})
}
