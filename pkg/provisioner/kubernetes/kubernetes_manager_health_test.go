package kubernetes

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseKubernetesManager_WaitForKubernetesHealthy(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		manager.healthCheckPollInterval = 50 * time.Millisecond
		manager.healthCheckSettleDuration = 120 * time.Millisecond
		manager.nodeReadyPollInterval = 50 * time.Millisecond
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			return nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ClientNotInitialized", func(t *testing.T) {
		manager := setup(t)
		manager.client = nil

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kubernetes client not initialized") {
			t.Errorf("Expected client not initialized error, got: %v", err)
		}
	})

	t.Run("ContextCancelled", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		callCount := 0
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			callCount++
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return fmt.Errorf("health check failed")
			}
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for Kubernetes API to be healthy") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
	})

	t.Run("HealthCheckFailsThenSucceeds", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		callCount := 0
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			callCount++
			if callCount == 1 {
				return fmt.Errorf("health check failed")
			}
			return nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil)
		if err != nil {
			t.Errorf("Expected no error after retry, got %v", err)
		}
		if callCount < 2 {
			t.Error("Expected CheckHealth to be called multiple times")
		}
	})

	t.Run("SettlesBeforeReturningHealthy", func(t *testing.T) {
		// Given an API that reports healthy on the very first check
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		var mu sync.Mutex
		var checkTimes []time.Time
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			mu.Lock()
			checkTimes = append(checkTimes, time.Now())
			mu.Unlock()
			return nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		start := time.Now()
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil)
		elapsed := time.Since(start)

		// Then it does not return on the first success: it keeps polling until
		// healthCheckSettleDuration has elapsed with no observed failure, so a
		// static-pod recreation that lands moments after the first success is
		// still caught.
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if elapsed < manager.healthCheckSettleDuration {
			t.Errorf("Expected wait to span at least the settle duration (%v), took %v", manager.healthCheckSettleDuration, elapsed)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(checkTimes) < 2 {
			t.Errorf("Expected CheckHealth to be called more than once to confirm the settle window, got %d calls", len(checkTimes))
		}
	})

	t.Run("FailureDuringSettleWindowResetsTheClock", func(t *testing.T) {
		// Given a health check that succeeds, then fails once (simulating an
		// async static-pod restart), then succeeds continuously afterward
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		var mu sync.Mutex
		callCount := 0
		failedOnce := false
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			mu.Lock()
			defer mu.Unlock()
			callCount++
			if callCount == 2 {
				failedOnce = true
				return fmt.Errorf("apiserver restarting")
			}
			return nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil)

		// Then the mid-window failure is not masked by the earlier success: the
		// wait only returns once a fresh, uninterrupted settle window completes
		// after the failure, so at least a settle-duration's worth of ticks must
		// follow it before the wait is allowed to return.
		if err != nil {
			t.Errorf("Expected no error once the restart resolves, got %v", err)
		}
		if !failedOnce {
			t.Fatal("Expected the mid-window failure to have occurred")
		}
		mu.Lock()
		successesAfterFailure := callCount - 2
		mu.Unlock()
		minTicksAfterFailure := int(manager.healthCheckSettleDuration / manager.healthCheckPollInterval)
		if successesAfterFailure < minTicksAfterFailure {
			t.Errorf("Expected at least %d successful checks after the failure to satisfy the settle window, got %d", minTicksAfterFailure, successesAfterFailure)
		}
	})

	t.Run("SettleDurationBoundedByCallerDeadline", func(t *testing.T) {
		// Given a caller whose total context budget is no larger than the
		// configured settle duration itself (e.g. windsor destroy's fail-fast
		// reachability check, which hands WaitForKubernetesHealthy a 30s context
		// against the same 30s default settle duration), and a health check that
		// always succeeds. The poll interval is widened from the suite default so
		// the real slack between the clamped settle window and the context
		// deadline (exactly one poll interval, by construction) has enough
		// absolute headroom to absorb CI scheduler jitter.
		manager := setup(t)
		manager.healthCheckPollInterval = 150 * time.Millisecond
		manager.healthCheckSettleDuration = 900 * time.Millisecond
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			return nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 900*time.Millisecond)
		defer cancel()

		// When it waits for health
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil)

		// Then it still succeeds: the settle window is bounded by the remaining
		// context deadline rather than being layered on top of it, so a healthy
		// cluster isn't guaranteed to time out just because the caller's overall
		// budget isn't comfortably larger than the settle duration.
		if err != nil {
			t.Errorf("Expected settle duration to be bounded by the caller's deadline so a healthy cluster still succeeds, got %v", err)
		}
	})

	t.Run("TimeoutWaitingForHealth", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			return fmt.Errorf("health check failed")
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for Kubernetes API to be healthy") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
		// The timeout surfaces the underlying health-check failure and the endpoint
		// so the operator isn't left guessing why the API never came up.
		if !strings.Contains(err.Error(), "health check failed") || !strings.Contains(err.Error(), "test-endpoint:6443") {
			t.Errorf("Expected timeout error to surface the underlying cause, got: %v", err)
		}
	})

	t.Run("TimeoutDistinguishesNodeReadinessPhase", func(t *testing.T) {
		// Given an API that is healthy but a node that never reaches Ready
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			return nil
		}
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"worker-1": false}, nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// When the wait times out on node readiness
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil, "worker-1")

		// Then the error attributes the failure to node readiness rather than the
		// API endpoint, so a healthy-API-but-stuck-nodes timeout is distinguishable
		// from an unreachable endpoint (both were previously the same bare message).
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for Kubernetes API to be healthy") {
			t.Errorf("Expected health timeout error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "nodes") {
			t.Errorf("Expected error to attribute the failure to node readiness, got: %v", err)
		}
		if strings.Contains(err.Error(), "endpoint") {
			t.Errorf("Did not expect the node-phase failure to mention the API endpoint, got: %v", err)
		}
	})

	t.Run("SuccessWithNodeNames", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			return nil
		}
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{
				"node1": true,
				"node2": true,
			}, nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		var output []string
		outputFunc := func(msg string) {
			output = append(output, msg)
		}

		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", outputFunc, "node1", "node2")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("HealthCheckSucceedsButNodesNotReady", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			return nil
		}
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{
				"node1": false,
			}, nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil, "node1")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for Kubernetes API to be healthy") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
	})

	t.Run("HealthCheckSucceedsButWaitForNodesReadyError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			return nil
		}
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return nil, fmt.Errorf("node status error")
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil, "node1")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for Kubernetes API to be healthy") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_GetNodeReadyStatus(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		expectedStatus := map[string]bool{
			"node1": true,
			"node2": false,
		}
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return expectedStatus, nil
		}
		manager.client = kubernetesClient

		ctx := context.Background()
		status, err := manager.GetNodeReadyStatus(ctx, []string{"node1", "node2"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !reflect.DeepEqual(status, expectedStatus) {
			t.Errorf("Expected status %v, got %v", expectedStatus, status)
		}
	})

	t.Run("ClientNotInitialized", func(t *testing.T) {
		manager := setup(t)
		manager.client = nil

		ctx := context.Background()
		_, err := manager.GetNodeReadyStatus(ctx, []string{"node1"})
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kubernetes client not initialized") {
			t.Errorf("Expected client not initialized error, got: %v", err)
		}
	})

	t.Run("ClientError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return nil, fmt.Errorf("client error")
		}
		manager.client = kubernetesClient

		ctx := context.Background()
		_, err := manager.GetNodeReadyStatus(ctx, []string{"node1"})
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "client error") {
			t.Errorf("Expected client error, got: %v", err)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestBaseKubernetesManager_waitForNodesReady(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		manager.nodeReadyPollInterval = 50 * time.Millisecond
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{
				"node1": true,
				"node2": true,
			}, nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		var output []string
		outputFunc := func(msg string) {
			output = append(output, msg)
		}

		err := manager.waitForNodesReady(ctx, []string{"node1", "node2"}, outputFunc)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(output) == 0 {
			t.Error("Expected output messages, got none")
		}
	})

	t.Run("ContextCancelled", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{
				"node1": false,
				"node2": false,
			}, nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := manager.waitForNodesReady(ctx, []string{"node1", "node2"}, nil)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "context cancelled while waiting for nodes to be ready") {
			t.Errorf("Expected context cancelled error, got: %v", err)
		}
	})

	t.Run("MissingNodes", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{
				"node1": true,
			}, nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		err := manager.waitForNodesReady(ctx, []string{"node1", "node2"}, nil)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for nodes to appear") && !strings.Contains(err.Error(), "context cancelled") {
			t.Errorf("Expected missing nodes or context cancelled error, got: %v", err)
		}
	})

	t.Run("NotReadyNodes", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{
				"node1": false,
				"node2": false,
			}, nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		err := manager.waitForNodesReady(ctx, []string{"node1", "node2"}, nil)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for nodes to be ready") && !strings.Contains(err.Error(), "context cancelled") {
			t.Errorf("Expected not ready nodes or context cancelled error, got: %v", err)
		}
	})

	t.Run("ContextWithoutDeadline", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{
				"node1": true,
				"node2": true,
			}, nil
		}
		manager.client = kubernetesClient

		ctx := context.Background()

		var output []string
		outputFunc := func(msg string) {
			output = append(output, msg)
		}

		err := manager.waitForNodesReady(ctx, []string{"node1", "node2"}, outputFunc)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("GetNodeReadyStatusErrorDuringPolling", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		callCount := 0
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			callCount++
			if callCount == 1 {
				return nil, fmt.Errorf("temporary error")
			}
			return map[string]bool{
				"node1": true,
				"node2": true,
			}, nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := manager.waitForNodesReady(ctx, []string{"node1", "node2"}, nil)
		if err != nil {
			t.Errorf("Expected no error after retry, got %v", err)
		}
		if callCount < 2 {
			t.Error("Expected GetNodeReadyStatus to be called multiple times")
		}
	})

	t.Run("OutputFuncWithStatusTransitions", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		callCount := 0
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			callCount++
			switch callCount {
			case 1:
				return map[string]bool{}, nil
			case 2:
				return map[string]bool{"node1": false}, nil
			case 3:
				return map[string]bool{"node1": true}, nil
			default:
				return map[string]bool{"node1": true}, nil
			}
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		var output []string
		outputFunc := func(msg string) {
			output = append(output, msg)
		}

		err := manager.waitForNodesReady(ctx, []string{"node1"}, outputFunc)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		foundNotFound := false
		foundNotReady := false
		foundReady := false
		for _, msg := range output {
			if strings.Contains(msg, "NOT FOUND") {
				foundNotFound = true
			}
			if strings.Contains(msg, "NOT READY") {
				foundNotReady = true
			}
			if strings.Contains(msg, "READY") {
				foundReady = true
			}
		}

		if !foundNotFound {
			t.Error("Expected output message for NOT FOUND status")
		}
		if !foundNotReady {
			t.Error("Expected output message for NOT READY status")
		}
		if !foundReady {
			t.Error("Expected output message for READY status")
		}
	})

	t.Run("OutputFuncNoStatusChange", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"node1": false}, nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		var output []string
		outputFunc := func(msg string) {
			output = append(output, msg)
		}

		err := manager.waitForNodesReady(ctx, []string{"node1"}, outputFunc)
		if err == nil {
			t.Error("Expected error, got nil")
		}

		notReadyCount := 0
		for _, msg := range output {
			if strings.Contains(msg, "NOT READY") {
				notReadyCount++
			}
		}

		if notReadyCount != 1 {
			t.Errorf("Expected 1 NOT READY message, got %d", notReadyCount)
		}
	})

	t.Run("TimeoutWithFinalGetNodeReadyStatusError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		callCount := 0
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			callCount++
			if callCount == 1 {
				return map[string]bool{"node1": false}, nil
			}
			return nil, fmt.Errorf("final status error")
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		err := manager.waitForNodesReady(ctx, []string{"node1"}, nil)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get final status") && !strings.Contains(err.Error(), "context cancelled") {
			t.Errorf("Expected error about final status or context cancelled, got: %v", err)
		}
	})

	t.Run("TimeoutWithBothMissingAndNotReadyNodes", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{
				"node1": false,
			}, nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		err := manager.waitForNodesReady(ctx, []string{"node1", "node2"}, nil)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for nodes to appear") && !strings.Contains(err.Error(), "context cancelled") {
			t.Errorf("Expected missing nodes or context cancelled error, got: %v", err)
		}
	})

	t.Run("TimeoutFallbackError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{
				"node1": true,
			}, nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		err := manager.waitForNodesReady(ctx, []string{"node1", "node2"}, nil)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for nodes") && !strings.Contains(err.Error(), "context cancelled") {
			t.Errorf("Expected timeout or context cancelled error, got: %v", err)
		}
	})

	t.Run("MultipleNodesWithMixedStatus", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		callCount := 0
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			callCount++
			if callCount == 1 {
				return map[string]bool{
					"node1": false,
					"node2": true,
				}, nil
			}
			return map[string]bool{
				"node1": true,
				"node2": true,
			}, nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var output []string
		outputFunc := func(msg string) {
			output = append(output, msg)
		}

		err := manager.waitForNodesReady(ctx, []string{"node1", "node2"}, outputFunc)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		foundNotReady := false
		for _, msg := range output {
			if strings.Contains(msg, "node1") && strings.Contains(msg, "NOT READY") {
				foundNotReady = true
			}
		}

		if !foundNotReady {
			t.Error("Expected output message for node1 NOT READY status")
		}
	})

	t.Run("EmptyNodeNames", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{}, nil
		}
		manager.client = kubernetesClient

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := manager.waitForNodesReady(ctx, []string{}, nil)
		if err != nil {
			t.Errorf("Expected no error for empty node names, got %v", err)
		}
	})

}
