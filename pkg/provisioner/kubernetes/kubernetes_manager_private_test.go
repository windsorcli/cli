package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

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

func TestBaseKubernetesManager_getHelmRelease(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		expectedObj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "helm.toolkit.fluxcd.io/v2",
				"kind":       "HelmRelease",
				"metadata": map[string]any{
					"name":      "test-release",
					"namespace": "test-namespace",
				},
				"spec": map[string]any{
					"chart": map[string]any{
						"spec": map[string]any{
							"chart": "test-chart",
						},
					},
				},
			},
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return expectedObj, nil
		}
		manager.client = kubernetesClient

		release, err := manager.getHelmRelease("test-release", "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if release == nil {
			t.Error("Expected helm release, got nil")
		}
		if release.Name != "test-release" {
			t.Errorf("Expected name 'test-release', got %s", release.Name)
		}
	})

	t.Run("GetResourceError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("get resource error")
		}
		manager.client = kubernetesClient

		_, err := manager.getHelmRelease("test-release", "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get helm release") {
			t.Errorf("Expected get resource error, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_calculateTotalWaitTime(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		return manager
	}

	t.Run("EmptyBlueprint", func(t *testing.T) {
		manager := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		expected := constants.DefaultKustomizationWaitTotalTimeout
		if timeout != expected {
			t.Errorf("Expected default timeout %v, got %v", expected, timeout)
		}
	})

	t.Run("SingleKustomizationNoDependencies", func(t *testing.T) {
		manager := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "k1",
				},
			},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		expected := constants.DefaultFluxKustomizationTimeout
		if timeout != expected {
			t.Errorf("Expected default timeout %v, got %v", expected, timeout)
		}
	})

	t.Run("SingleKustomizationWithCustomTimeout", func(t *testing.T) {
		manager := setup(t)
		customTimeout := 10 * time.Minute
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "k1",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: customTimeout,
					},
				},
			},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		if timeout != customTimeout {
			t.Errorf("Expected custom timeout %v, got %v", customTimeout, timeout)
		}
	})

	t.Run("SingleKustomizationWithZeroTimeout", func(t *testing.T) {
		manager := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "k1",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: 0,
					},
				},
			},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		expected := constants.DefaultFluxKustomizationTimeout
		if timeout != expected {
			t.Errorf("Expected default timeout %v, got %v", expected, timeout)
		}
	})

	t.Run("TwoKustomizationsWithDependency", func(t *testing.T) {
		manager := setup(t)
		timeout1 := 5 * time.Minute
		timeout2 := 3 * time.Minute
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "k1",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout1,
					},
				},
				{
					Name: "k2",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout2,
					},
					DependsOn: []string{"k1"},
				},
			},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		expected := timeout1 + timeout2
		if timeout != expected {
			t.Errorf("Expected sum %v, got %v", expected, timeout)
		}
	})

	t.Run("LongDependencyChain", func(t *testing.T) {
		manager := setup(t)
		timeout1 := 2 * time.Minute
		timeout2 := 3 * time.Minute
		timeout3 := 4 * time.Minute
		timeout4 := 5 * time.Minute
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "k1",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout1,
					},
				},
				{
					Name: "k2",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout2,
					},
					DependsOn: []string{"k1"},
				},
				{
					Name: "k3",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout3,
					},
					DependsOn: []string{"k2"},
				},
				{
					Name: "k4",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout4,
					},
					DependsOn: []string{"k3"},
				},
			},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		expected := timeout1 + timeout2 + timeout3 + timeout4
		if timeout != expected {
			t.Errorf("Expected sum %v, got %v", expected, timeout)
		}
	})

	t.Run("MultipleIndependentChains", func(t *testing.T) {
		manager := setup(t)
		timeout1 := 2 * time.Minute
		timeout2 := 3 * time.Minute
		timeout3 := 10 * time.Minute
		timeout4 := 1 * time.Minute
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "k1",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout1,
					},
				},
				{
					Name: "k2",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout2,
					},
					DependsOn: []string{"k1"},
				},
				{
					Name: "k3",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout3,
					},
				},
				{
					Name: "k4",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout4,
					},
					DependsOn: []string{"k3"},
				},
			},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		chain1Total := timeout1 + timeout2
		chain2Total := timeout3 + timeout4
		expected := chain2Total
		if chain1Total > chain2Total {
			expected = chain1Total
		}
		if timeout != expected {
			t.Errorf("Expected max chain total %v, got %v", expected, timeout)
		}
	})

	t.Run("ForkedDependencyChain", func(t *testing.T) {
		manager := setup(t)
		timeout1 := 2 * time.Minute
		timeout2 := 3 * time.Minute
		timeout3 := 4 * time.Minute
		timeout4 := 5 * time.Minute
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "k1",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout1,
					},
				},
				{
					Name: "k2",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout2,
					},
					DependsOn: []string{"k1"},
				},
				{
					Name: "k3",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout3,
					},
					DependsOn: []string{"k1"},
				},
				{
					Name: "k4",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: timeout4,
					},
					DependsOn: []string{"k2", "k3"},
				},
			},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		chain1Total := timeout1 + timeout2 + timeout4
		chain2Total := timeout1 + timeout3 + timeout4
		expected := chain2Total
		if chain1Total > chain2Total {
			expected = chain1Total
		}
		if timeout != expected {
			t.Errorf("Expected max chain total %v, got %v", expected, timeout)
		}
	})

	t.Run("MixedDefaultAndCustomTimeouts", func(t *testing.T) {
		manager := setup(t)
		customTimeout := 7 * time.Minute
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "k1",
				},
				{
					Name: "k2",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: customTimeout,
					},
					DependsOn: []string{"k1"},
				},
			},
		}

		timeout := manager.calculateTotalWaitTime(blueprint)
		expected := constants.DefaultFluxKustomizationTimeout + customTimeout
		if timeout != expected {
			t.Errorf("Expected sum %v, got %v", expected, timeout)
		}
	})
}

func TestReverseTopologicalKustomizations(t *testing.T) {
	t.Run("ReturnsEmptyForEmptyInput", func(t *testing.T) {
		// Given an empty input slice
		input := []blueprintv1alpha1.Kustomization{}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then no error is returned and the output is also empty
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("expected empty output, got %d entries", len(out))
		}
	})

	t.Run("ReturnsSingleNodeUnchanged", func(t *testing.T) {
		// Given a single kustomization with no dependencies
		input := []blueprintv1alpha1.Kustomization{{Name: "only"}}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then the single node is returned unchanged
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 1 || out[0].Name != "only" {
			t.Errorf("expected [only], got %v", kustomizationNames(out))
		}
	})

	t.Run("ReversesIndependentNodesByInputOrder", func(t *testing.T) {
		// Given three independent kustomizations with no dependency relation
		input := []blueprintv1alpha1.Kustomization{{Name: "a"}, {Name: "b"}, {Name: "c"}}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then the output is the input reversed, matching naive slice-reverse
		// — preserves backward compatibility for blueprints already authored
		// in topological order.
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		want := []string{"c", "b", "a"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("PutsDependentBeforeDependencyWhenInputAlreadyTopological", func(t *testing.T) {
		// Given input in apply order: dependency b first, dependent a second
		input := []blueprintv1alpha1.Kustomization{
			{Name: "b"},
			{Name: "a", DependsOn: []string{"b"}},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then a (the dependent) destroys first, then b
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		want := []string{"a", "b"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("PutsDependentBeforeDependencyWhenInputOutOfOrder", func(t *testing.T) {
		// Given input in non-topological order: dependent a first, dependency b second
		input := []blueprintv1alpha1.Kustomization{
			{Name: "a", DependsOn: []string{"b"}},
			{Name: "b"},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then a still destroys before b, regardless of input ordering
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		want := []string{"a", "b"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("OrdersDiamondDestroyDependentsFirst", func(t *testing.T) {
		// Given a diamond DAG:
		//   d ← b ← a
		//   d ← c ← a
		// Apply order is d → b,c → a; destroy order is a → b,c → d.
		input := []blueprintv1alpha1.Kustomization{
			{Name: "d"},
			{Name: "b", DependsOn: []string{"d"}},
			{Name: "c", DependsOn: []string{"d"}},
			{Name: "a", DependsOn: []string{"b", "c"}},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then a destroys first, then b and c (in input order's reverse), then d
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		// Independent siblings (b, c) appear in reverse input order: c then b.
		want := []string{"a", "c", "b", "d"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("HandlesDisconnectedComponents", func(t *testing.T) {
		// Given two disconnected dependency graphs:
		//   y ← x   (chain 1)
		//   q ← p   (chain 2)
		input := []blueprintv1alpha1.Kustomization{
			{Name: "y"},
			{Name: "x", DependsOn: []string{"y"}},
			{Name: "q"},
			{Name: "p", DependsOn: []string{"q"}},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then each chain's dependent precedes its dependency, and the chains
		// appear in reverse of input order.
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		// Forward DFS visits y, then x (already y), then q, then p (already q)
		// → forward order [y, x, q, p]; reverse → [p, q, x, y].
		want := []string{"p", "q", "x", "y"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("IgnoresMissingDependencyName", func(t *testing.T) {
		// Given a kustomization that depends on a name not present in the slice
		// — e.g. a destroy-only kustomization referencing a regular one that
		// was filtered out before this call.
		input := []blueprintv1alpha1.Kustomization{
			{Name: "a", DependsOn: []string{"missing"}},
			{Name: "b"},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then the missing dep is silently ignored and the slice is reversed
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		want := []string{"b", "a"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("DetectsTwoNodeCycle", func(t *testing.T) {
		// Given a two-node cycle: a depends on b, b depends on a
		input := []blueprintv1alpha1.Kustomization{
			{Name: "a", DependsOn: []string{"b"}},
			{Name: "b", DependsOn: []string{"a"}},
		}

		// When computing reverse-topological order
		_, err := reverseTopologicalKustomizations(input)

		// Then an error is returned mentioning a kustomization in the cycle
		if err == nil {
			t.Fatalf("expected cycle error, got nil")
		}
		if !strings.Contains(err.Error(), "cycle") {
			t.Errorf("expected error mentioning cycle, got %q", err.Error())
		}
	})

	t.Run("DetectsSelfLoopOnSingleNodeAsNoOp", func(t *testing.T) {
		// Given a single kustomization that depends on itself, the single-
		// node fast path returns it unchanged. A self-loop on a single node
		// does not change destroy correctness — there is nothing else to
		// order against — so the short-circuit is intentional.
		input := []blueprintv1alpha1.Kustomization{
			{Name: "a", DependsOn: []string{"a"}},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then the node is returned without a cycle error
		if err != nil {
			t.Fatalf("unexpected error on single self-loop: %v", err)
		}
		if len(out) != 1 || out[0].Name != "a" {
			t.Errorf("expected [a], got %v", kustomizationNames(out))
		}
	})

	t.Run("DetectsLargerCycle", func(t *testing.T) {
		// Given a three-node cycle: a → b → c → a
		input := []blueprintv1alpha1.Kustomization{
			{Name: "a", DependsOn: []string{"b"}},
			{Name: "b", DependsOn: []string{"c"}},
			{Name: "c", DependsOn: []string{"a"}},
		}

		// When computing reverse-topological order
		_, err := reverseTopologicalKustomizations(input)

		// Then an error is returned
		if err == nil {
			t.Fatalf("expected cycle error, got nil")
		}
		if !strings.Contains(err.Error(), "cycle") {
			t.Errorf("expected error mentioning cycle, got %q", err.Error())
		}
	})

	t.Run("DoesNotMutateInput", func(t *testing.T) {
		// Given an input slice
		input := []blueprintv1alpha1.Kustomization{
			{Name: "b"},
			{Name: "a", DependsOn: []string{"b"}},
		}
		inputNames := kustomizationNames(input)

		// When computing reverse-topological order
		_, err := reverseTopologicalKustomizations(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Then the input slice is unchanged in order and identity
		gotNames := kustomizationNames(input)
		if !equalStringSlice(gotNames, inputNames) {
			t.Errorf("input mutated: was %v, now %v", inputNames, gotNames)
		}
	})
}

// kustomizationNames returns the Name field from each kustomization, in slice order.
func kustomizationNames(ks []blueprintv1alpha1.Kustomization) []string {
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = k.Name
	}
	return out
}

// equalStringSlice reports whether two string slices have identical contents in order.
func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
