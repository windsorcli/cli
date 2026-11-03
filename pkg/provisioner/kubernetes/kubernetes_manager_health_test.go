package kubernetes_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseKubernetesManager_WaitForKubernetesHealthy(t *testing.T) {
	setup := func(t *testing.T, kubernetesClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := kubernetes.NewKubernetesManager(kubernetesClient, mocks.ConfigHandler)
		manager.Timing.HealthCheckPollInterval = 50 * time.Millisecond
		manager.Timing.HealthCheckSettleDuration = 120 * time.Millisecond
		manager.Timing.NodeReadyPollInterval = 50 * time.Millisecond
		return manager
	}

	t.Run("SucceedsWhenAPIHealthy", func(t *testing.T) {
		// Given an API that is healthy
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			return nil
		}
		manager := setup(t, kubernetesClient)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// When the manager waits for health
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil)

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ClientNotInitialized", func(t *testing.T) {
		// Given a manager with no client
		manager := &kubernetes.BaseKubernetesManager{Shims: kubernetes.NewShims(), Timing: kubernetes.DefaultTiming()}
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// When the manager waits for health
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil)

		// Then a client error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kubernetes client not initialized") {
			t.Errorf("Expected client not initialized error, got: %v", err)
		}
	})

	t.Run("ContextCancelled", func(t *testing.T) {
		// Given a failing health check and a cancelled context
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return fmt.Errorf("health check failed")
			}
		}
		manager := setup(t, kubernetesClient)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// When the manager waits for health
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil)

		// Then a timeout error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for Kubernetes API to be healthy") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
	})

	t.Run("HealthCheckFailsThenSucceeds", func(t *testing.T) {
		// Given a health check that fails once and then succeeds
		kubernetesClient := client.NewMockKubernetesClient()
		var mu sync.Mutex
		callCount := 0
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			mu.Lock()
			defer mu.Unlock()
			callCount++
			if callCount == 1 {
				return fmt.Errorf("health check failed")
			}
			return nil
		}
		manager := setup(t, kubernetesClient)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// When the manager waits for health
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil)

		// Then it retries and succeeds
		if err != nil {
			t.Errorf("Expected no error after retry, got %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		if callCount < 2 {
			t.Error("Expected CheckHealth to be called multiple times")
		}
	})

	t.Run("SettlesBeforeReturningHealthy", func(t *testing.T) {
		// Given an API that is healthy on the first check
		kubernetesClient := client.NewMockKubernetesClient()
		var mu sync.Mutex
		var checkTimes []time.Time
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			mu.Lock()
			checkTimes = append(checkTimes, time.Now())
			mu.Unlock()
			return nil
		}
		manager := setup(t, kubernetesClient)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// When the manager waits for health
		start := time.Now()
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil)
		elapsed := time.Since(start)

		// Then it polls through the full settle duration before it returns
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if elapsed < manager.Timing.HealthCheckSettleDuration {
			t.Errorf("Expected wait to span at least the settle duration (%v), took %v", manager.Timing.HealthCheckSettleDuration, elapsed)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(checkTimes) < 2 {
			t.Errorf("Expected CheckHealth to be called more than once to confirm the settle window, got %d calls", len(checkTimes))
		}
	})

	t.Run("FailureDuringSettleWindowResetsTheClock", func(t *testing.T) {
		// Given a health check that succeeds, fails once, and then succeeds
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
		manager := setup(t, kubernetesClient)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// When the manager waits for health
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil)

		// Then a full settle window of successful checks follows the failure
		if err != nil {
			t.Errorf("Expected no error once the restart resolves, got %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		if !failedOnce {
			t.Fatal("Expected the mid-window failure to have occurred")
		}
		successesAfterFailure := callCount - 2
		minTicksAfterFailure := int(manager.Timing.HealthCheckSettleDuration / manager.Timing.HealthCheckPollInterval)
		if successesAfterFailure < minTicksAfterFailure {
			t.Errorf("Expected at least %d successful checks after the failure to satisfy the settle window, got %d", minTicksAfterFailure, successesAfterFailure)
		}
	})

	t.Run("SettleDurationBoundedByCallerDeadline", func(t *testing.T) {
		// Given a healthy API and a context budget equal to the settle duration
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			return nil
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.HealthCheckPollInterval = 150 * time.Millisecond
		manager.Timing.HealthCheckSettleDuration = 900 * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), 900*time.Millisecond)
		defer cancel()

		// When the manager waits for health
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil)

		// Then the settle window fits inside the deadline and the wait succeeds
		if err != nil {
			t.Errorf("Expected settle duration to be bounded by the caller's deadline so a healthy cluster still succeeds, got %v", err)
		}
	})

	t.Run("TimeoutWaitingForHealth", func(t *testing.T) {
		// Given a health check that always fails
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			return fmt.Errorf("health check failed")
		}
		manager := setup(t, kubernetesClient)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// When the manager waits for health
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil)

		// Then the timeout error names the health check failure and the endpoint
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for Kubernetes API to be healthy") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "health check failed") || !strings.Contains(err.Error(), "test-endpoint:6443") {
			t.Errorf("Expected timeout error to surface the underlying cause, got: %v", err)
		}
	})

	t.Run("TimeoutDistinguishesNodeReadinessPhase", func(t *testing.T) {
		// Given a healthy API and a node that never becomes Ready
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			return nil
		}
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"worker-1": false}, nil
		}
		manager := setup(t, kubernetesClient)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// When the manager waits for health and the node
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil, "worker-1")

		// Then the timeout error names node readiness and not the endpoint
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

	t.Run("SucceedsWithReadyNodes", func(t *testing.T) {
		// Given a healthy API and two Ready nodes
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			return nil
		}
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"node1": true, "node2": true}, nil
		}
		manager := setup(t, kubernetesClient)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		var output []string

		// When the manager waits for health and the nodes
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", func(msg string) {
			output = append(output, msg)
		}, "node1", "node2")

		// Then no error is returned and the node status is reported
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(output) == 0 {
			t.Error("Expected output messages, got none")
		}
	})

	t.Run("HealthCheckSucceedsButNodesNotReady", func(t *testing.T) {
		// Given a healthy API and a node that is not Ready
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			return nil
		}
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"node1": false}, nil
		}
		manager := setup(t, kubernetesClient)
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		// When the manager waits for health and the node
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil, "node1")

		// Then a timeout error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for Kubernetes API to be healthy") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
	})

	t.Run("HealthCheckSucceedsButNodeStatusFails", func(t *testing.T) {
		// Given a healthy API and a node status call that always fails
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			return nil
		}
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return nil, fmt.Errorf("node status error")
		}
		manager := setup(t, kubernetesClient)
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		// When the manager waits for health and the node
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil, "node1")

		// Then a timeout error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for Kubernetes API to be healthy") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_WaitForKubernetesHealthyNodeReadiness(t *testing.T) {
	setup := func(t *testing.T, nodeStatus func(ctx context.Context, nodeNames []string) (map[string]bool, error)) *kubernetes.BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			return nil
		}
		kubernetesClient.GetNodeReadyStatusFunc = nodeStatus
		manager := kubernetes.NewKubernetesManager(kubernetesClient, mocks.ConfigHandler)
		manager.Timing.HealthCheckPollInterval = 50 * time.Millisecond
		manager.Timing.HealthCheckSettleDuration = 120 * time.Millisecond
		manager.Timing.NodeReadyPollInterval = 50 * time.Millisecond
		return manager
	}

	// recorder returns an output func and the messages it records.
	recorder := func() (func(string), func() []string) {
		var mu sync.Mutex
		var output []string
		return func(msg string) {
				mu.Lock()
				defer mu.Unlock()
				output = append(output, msg)
			}, func() []string {
				mu.Lock()
				defer mu.Unlock()
				return append([]string(nil), output...)
			}
	}

	t.Run("ReportsTimeoutForMissingNodes", func(t *testing.T) {
		// Given a status that lacks node2
		manager := setup(t, func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"node1": true}, nil
		})
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		// When the manager waits for node1 and node2
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil, "node1", "node2")

		// Then a missing-node or cancellation error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for nodes to appear") && !strings.Contains(err.Error(), "context cancelled") {
			t.Errorf("Expected missing nodes or context cancelled error, got: %v", err)
		}
	})

	t.Run("ReportsTimeoutForNotReadyNodes", func(t *testing.T) {
		// Given two nodes that are not Ready
		manager := setup(t, func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"node1": false, "node2": false}, nil
		})
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		// When the manager waits for node1 and node2
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil, "node1", "node2")

		// Then a not-ready or cancellation error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for nodes to be ready") && !strings.Contains(err.Error(), "context cancelled") {
			t.Errorf("Expected not ready nodes or context cancelled error, got: %v", err)
		}
	})

	t.Run("SucceedsWithoutContextDeadline", func(t *testing.T) {
		// Given two Ready nodes
		manager := setup(t, func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"node1": true, "node2": true}, nil
		})
		output, _ := recorder()

		// When the manager waits with a context that has no deadline
		err := manager.WaitForKubernetesHealthy(context.Background(), "https://test-endpoint:6443", output, "node1", "node2")

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("RetriesAfterNodeStatusError", func(t *testing.T) {
		// Given a node status call that fails once and then reports Ready nodes
		var mu sync.Mutex
		callCount := 0
		manager := setup(t, func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			mu.Lock()
			defer mu.Unlock()
			callCount++
			if callCount == 1 {
				return nil, fmt.Errorf("temporary error")
			}
			return map[string]bool{"node1": true, "node2": true}, nil
		})
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// When the manager waits for node1 and node2
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil, "node1", "node2")

		// Then it retries and succeeds
		if err != nil {
			t.Errorf("Expected no error after retry, got %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		if callCount < 2 {
			t.Error("Expected GetNodeReadyStatus to be called multiple times")
		}
	})

	t.Run("ReportsEachStatusTransition", func(t *testing.T) {
		// Given a node that is missing, then not Ready, then Ready
		var mu sync.Mutex
		callCount := 0
		manager := setup(t, func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			mu.Lock()
			defer mu.Unlock()
			callCount++
			switch callCount {
			case 1:
				return map[string]bool{}, nil
			case 2:
				return map[string]bool{"node1": false}, nil
			default:
				return map[string]bool{"node1": true}, nil
			}
		})
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		output, messages := recorder()

		// When the manager waits for node1
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", output, "node1")

		// Then the output reports NOT FOUND, NOT READY, and READY
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		foundNotFound, foundNotReady, foundReady := false, false, false
		for _, msg := range messages() {
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

	t.Run("ReportsUnchangedStatusOnce", func(t *testing.T) {
		// Given a node that stays not Ready
		manager := setup(t, func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"node1": false}, nil
		})
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		output, messages := recorder()

		// When the manager waits for node1
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", output, "node1")

		// Then the NOT READY status is reported once
		if err == nil {
			t.Error("Expected error, got nil")
		}
		notReadyCount := 0
		for _, msg := range messages() {
			if strings.Contains(msg, "NOT READY") {
				notReadyCount++
			}
		}
		if notReadyCount != 1 {
			t.Errorf("Expected 1 NOT READY message, got %d", notReadyCount)
		}
	})

	t.Run("ReportsFinalNodeStatusError", func(t *testing.T) {
		// Given a node that is not Ready, then a node status call that always fails
		var mu sync.Mutex
		callCount := 0
		manager := setup(t, func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			mu.Lock()
			defer mu.Unlock()
			callCount++
			if callCount == 1 {
				return map[string]bool{"node1": false}, nil
			}
			return nil, fmt.Errorf("final status error")
		})
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		// When the manager waits for node1
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil, "node1")

		// Then a final-status or cancellation error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get final status") && !strings.Contains(err.Error(), "context cancelled") {
			t.Errorf("Expected error about final status or context cancelled, got: %v", err)
		}
	})

	t.Run("ReportsMissingNodesBeforeNotReadyNodes", func(t *testing.T) {
		// Given node1 not Ready and node2 missing
		manager := setup(t, func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"node1": false}, nil
		})
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		// When the manager waits for node1 and node2
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil, "node1", "node2")

		// Then a missing-node or cancellation error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for nodes to appear") && !strings.Contains(err.Error(), "context cancelled") {
			t.Errorf("Expected missing nodes or context cancelled error, got: %v", err)
		}
	})

	t.Run("ReportsNodeTimeoutWhenOneNodeNeverAppears", func(t *testing.T) {
		// Given node1 Ready and node2 missing
		manager := setup(t, func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return map[string]bool{"node1": true}, nil
		})
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		// When the manager waits for node1 and node2
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", nil, "node1", "node2")

		// Then a node timeout or cancellation error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for nodes") && !strings.Contains(err.Error(), "context cancelled") {
			t.Errorf("Expected timeout or context cancelled error, got: %v", err)
		}
	})

	t.Run("ReportsNotReadyNodeAmongMixedStatus", func(t *testing.T) {
		// Given node1 not Ready on the first check and both nodes Ready afterward
		var mu sync.Mutex
		callCount := 0
		manager := setup(t, func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			mu.Lock()
			defer mu.Unlock()
			callCount++
			if callCount == 1 {
				return map[string]bool{"node1": false, "node2": true}, nil
			}
			return map[string]bool{"node1": true, "node2": true}, nil
		})
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		output, messages := recorder()

		// When the manager waits for node1 and node2
		err := manager.WaitForKubernetesHealthy(ctx, "https://test-endpoint:6443", output, "node1", "node2")

		// Then it succeeds and reports node1 as NOT READY
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		foundNotReady := false
		for _, msg := range messages() {
			if strings.Contains(msg, "node1") && strings.Contains(msg, "NOT READY") {
				foundNotReady = true
			}
		}
		if !foundNotReady {
			t.Error("Expected output message for node1 NOT READY status")
		}
	})
}

func TestBaseKubernetesManager_GetNodeReadyStatus(t *testing.T) {
	t.Run("ReturnsClientStatus", func(t *testing.T) {
		// Given a client that reports one Ready and one not Ready node
		mocks := setupKubernetesMocks(t)
		kubernetesClient := client.NewMockKubernetesClient()
		expectedStatus := map[string]bool{"node1": true, "node2": false}
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return expectedStatus, nil
		}
		manager := kubernetes.NewKubernetesManager(kubernetesClient, mocks.ConfigHandler)

		// When the node status is read
		status, err := manager.GetNodeReadyStatus(context.Background(), []string{"node1", "node2"})

		// Then the client status is returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !reflect.DeepEqual(status, expectedStatus) {
			t.Errorf("Expected status %v, got %v", expectedStatus, status)
		}
	})

	t.Run("ClientNotInitialized", func(t *testing.T) {
		// Given a manager with no client
		manager := &kubernetes.BaseKubernetesManager{Shims: kubernetes.NewShims(), Timing: kubernetes.DefaultTiming()}

		// When the node status is read
		_, err := manager.GetNodeReadyStatus(context.Background(), []string{"node1"})

		// Then a client error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kubernetes client not initialized") {
			t.Errorf("Expected client not initialized error, got: %v", err)
		}
	})

	t.Run("ClientError", func(t *testing.T) {
		// Given a client whose node status call fails
		mocks := setupKubernetesMocks(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetNodeReadyStatusFunc = func(ctx context.Context, nodeNames []string) (map[string]bool, error) {
			return nil, fmt.Errorf("client error")
		}
		manager := kubernetes.NewKubernetesManager(kubernetesClient, mocks.ConfigHandler)

		// When the node status is read
		_, err := manager.GetNodeReadyStatus(context.Background(), []string{"node1"})

		// Then the client error is returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "client error") {
			t.Errorf("Expected client error, got: %v", err)
		}
	})
}
