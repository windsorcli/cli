package kubernetes

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"context"
	"reflect"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	meta "github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// KubernetesTestMocks contains all the mock dependencies for testing the KubernetesManager
type KubernetesTestMocks struct {
	Shims            *Shims
	KubernetesClient client.KubernetesClient
}

// setupKubernetesMocks creates mock components for testing the KubernetesManager with optional overrides
func setupKubernetesMocks(t *testing.T, opts ...func(*KubernetesTestMocks)) *KubernetesTestMocks {
	t.Helper()

	kubernetesClient := client.NewMockKubernetesClient()
	kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
		return obj, nil
	}
	kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
		return &unstructured.Unstructured{}, nil
	}

	mocks := &KubernetesTestMocks{
		Shims:            setupDefaultShims(),
		KubernetesClient: kubernetesClient,
	}

	// Apply any overrides
	for _, opt := range opts {
		opt(mocks)
	}

	return mocks
}

// setupDefaultShims initializes and returns shims with default test configurations
func setupDefaultShims() *Shims {
	shims := NewShims()
	shims.ToUnstructured = func(obj any) (map[string]any, error) {
		return nil, fmt.Errorf("forced conversion error")
	}
	return shims
}

func TestBaseKubernetesManager_ApplyKustomization(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
		// Use shorter timeouts for tests
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationReconcileSleep = 50 * time.Millisecond
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kustomization := kustomizev1.Kustomization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-kustomization",
				Namespace: "test-namespace",
			},
			Spec: kustomizev1.KustomizationSpec{
				Path: "./test-path",
			},
		}

		err := manager.ApplyKustomization(kustomization)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("UnstructuredConversionError", func(t *testing.T) {
		manager := setup(t)
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return nil, fmt.Errorf("forced conversion error")
		}

		kustomization := kustomizev1.Kustomization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-kustomization",
				Namespace: "test-namespace",
			},
			Spec: kustomizev1.KustomizationSpec{
				Path: "./test-path",
			},
		}

		err := manager.ApplyKustomization(kustomization)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to convert kustomization to unstructured") {
			t.Errorf("Expected conversion error, got: %v", err)
		}
	})

	t.Run("ApplyWithRetryError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("apply error")
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		kustomization := kustomizev1.Kustomization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-kustomization",
				Namespace: "test-namespace",
			},
			Spec: kustomizev1.KustomizationSpec{
				Path: "./test-path",
			},
		}

		err := manager.ApplyKustomization(kustomization)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ApplyWithRetryExistingResourceConversionError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		existingObj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
				"kind":       "Kustomization",
				"metadata": map[string]any{
					"name":            "test-kustomization",
					"namespace":       "test-namespace",
					"resourceVersion": "123",
				},
			},
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return existingObj, nil
		}
		callCount := 0
		originalToUnstructured := manager.shims.ToUnstructured
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			callCount++
			if callCount == 1 {
				return originalToUnstructured(obj)
			}
			return nil, fmt.Errorf("conversion error")
		}
		manager.client = kubernetesClient

		kustomization := kustomizev1.Kustomization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-kustomization",
				Namespace: "test-namespace",
			},
			Spec: kustomizev1.KustomizationSpec{
				Path: "./test-path",
			},
		}

		err := manager.ApplyKustomization(kustomization)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to convert existing object to unstructured") {
			t.Errorf("Expected conversion error, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_DeleteKustomization(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
		// Use shorter timeouts for tests
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationReconcileSleep = 50 * time.Millisecond
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		// Mock GetResource to return "not found" immediately to simulate successful deletion
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		err := manager.DeleteKustomization("test-kustomization", "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DeleteError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("delete error")
		}
		manager.client = kubernetesClient

		err := manager.DeleteKustomization("test-kustomization", "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("KustomizationNotFound", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		err := manager.DeleteKustomization("test-kustomization", "test-namespace")
		if err != nil {
			t.Errorf("Expected no error for not found resource, got %v", err)
		}
	})

	t.Run("TimeoutWaitingForDeletion", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		}
		manager.client = kubernetesClient
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond

		err := manager.DeleteKustomization("test-kustomization", "test-namespace")
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for kustomization") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
	})

	t.Run("ErrorCheckingDeletionStatus", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("error checking status")
		}
		manager.client = kubernetesClient
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond

		err := manager.DeleteKustomization("test-kustomization", "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error checking kustomization deletion status") {
			t.Errorf("Expected error checking status, got: %v", err)
		}
	})

	t.Run("UsesCorrectDeleteOptions", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		var capturedOptions metav1.DeleteOptions
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			capturedOptions = opts
			return nil
		}
		// Mock GetResource to return "not found" immediately to simulate successful deletion
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		err := manager.DeleteKustomization("test-kustomization", "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify the correct delete options were used
		if capturedOptions.PropagationPolicy == nil {
			t.Error("Expected PropagationPolicy to be set")
		} else if *capturedOptions.PropagationPolicy != metav1.DeletePropagationBackground {
			t.Errorf("Expected PropagationPolicy to be DeletePropagationBackground, got %s", *capturedOptions.PropagationPolicy)
		}
	})
}

func TestBaseKubernetesManager_WaitForKustomizations(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
		// Use shorter timeouts for tests
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationReconcileSleep = 50 * time.Millisecond
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
				Object: map[string]any{
					"status": map[string]any{
						"conditions": []any{
							map[string]any{
								"type":   "Ready",
								"status": "True",
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: 200 * time.Millisecond,
					},
				},
			},
		}

		err := manager.WaitForKustomizations("Waiting for kustomizations", blueprint)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
				Object: map[string]any{
					"status": map[string]any{
						"conditions": []any{
							map[string]any{
								"type":   "Ready",
								"status": "False",
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: 50 * time.Millisecond,
					},
				},
			},
		}

		err := manager.WaitForKustomizations("Waiting for kustomizations", blueprint)
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})

	t.Run("MissingStatus", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
				Object: map[string]any{},
			}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: 50 * time.Millisecond,
					},
				},
			},
		}

		err := manager.WaitForKustomizations("Waiting for kustomizations", blueprint)
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})

	t.Run("FromUnstructuredError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
				Object: map[string]any{
					"status": map[string]any{
						"conditions": []any{
							map[string]any{
								"type":   "Ready",
								"status": "True",
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return fmt.Errorf("forced conversion error")
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: 50 * time.Millisecond,
					},
				},
			},
		}

		err := manager.WaitForKustomizations("Waiting for kustomizations", blueprint)
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})

	t.Run("MissingConditions", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
				Object: map[string]any{
					"status": map[string]any{},
				},
			}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: 50 * time.Millisecond,
					},
				},
			},
		}

		err := manager.WaitForKustomizations("Waiting for kustomizations", blueprint)
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})

	t.Run("ConditionTypeNotReady", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
				Object: map[string]any{
					"status": map[string]any{
						"conditions": []any{
							map[string]any{
								"type":   "NotReady",
								"status": "True",
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: 50 * time.Millisecond,
					},
				},
			},
		}

		err := manager.WaitForKustomizations("Waiting for kustomizations", blueprint)
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})

	t.Run("ConditionReadyFalse", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
				Object: map[string]any{
					"status": map[string]any{
						"conditions": []any{
							map[string]any{
								"type":   "Ready",
								"status": "False",
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Timeout: &blueprintv1alpha1.DurationString{
						Duration: 50 * time.Millisecond,
					},
				},
			},
		}

		err := manager.WaitForKustomizations("Waiting for kustomizations", blueprint)
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})

	t.Run("WithBlueprintCalculatesTimeout", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
				Object: map[string]any{
					"status": map[string]any{
						"conditions": []any{
							map[string]any{
								"type":   "Ready",
								"status": "True",
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		timeout1 := 50 * time.Millisecond
		timeout2 := 75 * time.Millisecond
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

		err := manager.WaitForKustomizations("Waiting for kustomizations", blueprint)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		calculatedTimeout := manager.calculateTotalWaitTime(blueprint)
		expectedTimeout := timeout1 + timeout2
		if calculatedTimeout != expectedTimeout {
			t.Errorf("Expected calculated timeout %v, got %v", expectedTimeout, calculatedTimeout)
		}
	})

	t.Run("WithBlueprintNilReturnsError", func(t *testing.T) {
		manager := setup(t)
		err := manager.WaitForKustomizations("Waiting for kustomizations", nil)
		if err == nil {
			t.Error("Expected error for nil blueprint, got nil")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected 'blueprint not provided' error, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_CreateNamespace(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		err := manager.CreateNamespace("test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ApplyError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("apply error")
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		err := manager.CreateNamespace("test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})
}

func TestBaseKubernetesManager_DeleteNamespace(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		// Mock GetResource to return "not found" immediately to simulate successful deletion
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		err := manager.DeleteNamespace("test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DeleteError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("delete error")
		}
		manager.client = kubernetesClient

		err := manager.DeleteNamespace("test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("UsesCorrectDeleteOptions", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		var capturedOptions metav1.DeleteOptions
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			capturedOptions = opts
			return nil
		}
		manager.client = kubernetesClient

		err := manager.DeleteNamespace("test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify the delete options were used (no specific policy required)
		if capturedOptions.PropagationPolicy != nil {
			t.Errorf("Expected no PropagationPolicy, got %+v", capturedOptions.PropagationPolicy)
		}
	})
}

func TestBaseKubernetesManager_ApplyConfigMap(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
		// Use shorter timeouts for tests
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationReconcileSleep = 50 * time.Millisecond
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		data := map[string]string{
			"key1": "value1",
			"key2": "value2",
		}
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", data)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ImmutableConfigMap", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
				Object: map[string]any{
					"kind": "ConfigMap",
					"spec": map[string]any{
						"immutable": true,
					},
				},
			}, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		manager.client = kubernetesClient

		data := map[string]string{
			"key1": "value1",
		}
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", data)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ApplyError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("apply error")
		}
		manager.client = kubernetesClient

		data := map[string]string{
			"key1": "value1",
		}
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", data)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ValidateFieldsError_MissingData", func(t *testing.T) {
		manager := setup(t)
		// Remove data field by passing nil
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", nil)
		if err == nil {
			t.Error("Expected error for missing data, got nil")
		}
	})

	t.Run("ValidateFieldsError_MissingName", func(t *testing.T) {
		manager := setup(t)
		// Patch shims to remove name from metadata
		origToUnstructured := manager.shims.ToUnstructured
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			m, _ := origToUnstructured(obj)
			if meta, ok := m["metadata"].(map[string]any); ok {
				delete(meta, "name")
			}
			return m, nil
		}
		// Data is present, but name will be missing
		err := manager.ApplyConfigMap("", "test-namespace", map[string]string{"k": "v"})
		if err == nil {
			t.Error("Expected error for missing metadata.name, got nil")
		}
	})

	t.Run("GetResourceError_NotFound", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		manager.client = kubernetesClient
		// Should not error, just apply
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"k": "v"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("GetResourceError_Other", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("some error")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		manager.client = kubernetesClient
		// Should not error, just apply
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"k": "v"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DeleteResourceError_ImmutableConfigMap", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
				Object: map[string]any{
					"kind": "ConfigMap",
					"spec": map[string]any{"immutable": true},
				},
			}, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, ns, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("delete error")
		}
		manager.client = kubernetesClient
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"k": "v"})
		if err == nil || !strings.Contains(err.Error(), "failed to delete immutable configmap") {
			t.Errorf("Expected delete error, got %v", err)
		}
	})

	t.Run("ToUnstructuredError", func(t *testing.T) {
		manager := setup(t)
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return nil, fmt.Errorf("forced toUnstructured error")
		}
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"k": "v"})
		if err == nil || !strings.Contains(err.Error(), "failed to convert") {
			t.Errorf("Expected toUnstructured error, got %v", err)
		}
	})

	t.Run("FromUnstructuredError", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":   "Ready",
										"status": "True",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return fmt.Errorf("forced conversion error")
		}

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "forced conversion error") {
			t.Errorf("Expected error containing 'forced conversion error', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("KustomizationNotReady", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":   "Ready",
										"status": "False",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if status["k1"] {
			t.Errorf("Expected k1 to be not ready, got %v", status["k1"])
		}
	})

	t.Run("KustomizationFailed", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":    "Ready",
										"status":  "False",
										"reason":  "ReconciliationFailed",
										"message": "kustomization failed",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kustomization k1 failed: kustomization failed") {
			t.Errorf("Expected error containing 'kustomization k1 failed: kustomization failed', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("KustomizationMissing", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":   "Ready",
										"status": "True",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1", "k2"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !status["k1"] {
			t.Errorf("Expected k1 to be ready, got %v", status["k1"])
		}
		if status["k2"] {
			t.Errorf("Expected k2 to be not ready, got %v", status["k2"])
		}
	})

	t.Run("ValidateFieldsError_MissingSpec", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Deployment",
				"metadata": map[string]any{
					"name": "foo",
				},
			},
		}
		err := validateFields(obj)
		if err == nil || !strings.Contains(err.Error(), "spec is required") {
			t.Errorf("Expected error containing 'spec is required', got %v", err)
		}
	})

	t.Run("ValidateFieldsError_MissingMetadataName", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Deployment",
				"metadata":   map[string]any{},
				"spec":       map[string]any{},
			},
		}
		err := validateFields(obj)
		if err == nil || !strings.Contains(err.Error(), "metadata.name is required") {
			t.Errorf("Expected error containing 'metadata.name is required', got %v", err)
		}
	})

	t.Run("ValidateFieldsError_EmptyMetadataName", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Deployment",
				"metadata": map[string]any{
					"name": " ",
				},
				"spec": map[string]any{},
			},
		}
		err := validateFields(obj)
		if err == nil || !strings.Contains(err.Error(), "metadata.name cannot be empty") {
			t.Errorf("Expected error containing 'metadata.name cannot be empty', got %v", err)
		}
	})

	t.Run("ValidateFieldsError_ConfigMapMissingData", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "foo",
				},
			},
		}
		err := validateFields(obj)
		if err == nil || !strings.Contains(err.Error(), "data is required for ConfigMap") {
			t.Errorf("Expected error containing 'data is required for ConfigMap', got %v", err)
		}
	})

	t.Run("ValidateFieldsError_ConfigMapDataNil", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "foo",
				},
				"data": nil,
			},
		}
		err := validateFields(obj)
		if err == nil || !strings.Contains(err.Error(), "data cannot be nil for ConfigMap") {
			t.Errorf("Expected error containing 'data cannot be nil for ConfigMap', got %v", err)
		}
	})

	t.Run("ValidateFieldsError_ConfigMapDataEmptyStringMap", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "foo",
				},
				"data": map[string]string{},
			},
		}
		err := validateFields(obj)
		if err == nil || !strings.Contains(err.Error(), "data cannot be empty for ConfigMap") {
			t.Errorf("Expected error containing 'data cannot be empty for ConfigMap', got %v", err)
		}
	})

	t.Run("ValidateFieldsError_ConfigMapDataEmptyAnyMap", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "foo",
				},
				"data": map[string]any{},
			},
		}
		err := validateFields(obj)
		if err == nil || !strings.Contains(err.Error(), "data cannot be empty for ConfigMap") {
			t.Errorf("Expected error containing 'data cannot be empty for ConfigMap', got %v", err)
		}
	})

	t.Run("IsImmutableConfigMap_WrongKind", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"kind": "Deployment",
				"spec": map[string]any{"immutable": true},
			},
		}
		if isImmutableConfigMap(obj) {
			t.Error("Expected false for non-ConfigMap kind")
		}
	})

	t.Run("IsImmutableConfigMap_MissingSpec", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"kind": "ConfigMap",
			},
		}
		if isImmutableConfigMap(obj) {
			t.Error("Expected false for missing spec")
		}
	})

	t.Run("IsImmutableConfigMap_ImmutableFalse", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"kind": "ConfigMap",
				"spec": map[string]any{"immutable": false},
			},
		}
		if isImmutableConfigMap(obj) {
			t.Error("Expected false for immutable false")
		}
	})

	t.Run("IsImmutableConfigMap_ImmutableTrue", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"kind": "ConfigMap",
				"spec": map[string]any{"immutable": true},
			},
		}
		if !isImmutableConfigMap(obj) {
			t.Error("Expected true for immutable true")
		}
	})
}

func TestBaseKubernetesManager_GetHelmReleasesForKustomization(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		// Return a Kustomization with a valid HelmRelease inventory entry
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			if gvr.Group == "kustomize.toolkit.fluxcd.io" && gvr.Resource == "kustomizations" {
				return &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
						"kind":       "Kustomization",
						"status": map[string]any{
							"inventory": map[string]any{
								"entries": []any{
									map[string]any{
										"id": "test-namespace_test-release_helm.toolkit.fluxcd.io_HelmRelease",
									},
								},
							},
						},
					},
				}, nil
			}
			if gvr.Group == "helm.toolkit.fluxcd.io" && gvr.Resource == "helmreleases" {
				return &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "helm.toolkit.fluxcd.io/v2",
						"kind":       "HelmRelease",
						"metadata": map[string]any{
							"name":      "test-release",
							"namespace": "test-namespace",
						},
					},
				}, nil
			}
			return nil, fmt.Errorf("unexpected resource request")
		}
		manager.client = kubernetesClient

		releases, err := manager.GetHelmReleasesForKustomization("test-kustomization", "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(releases) != 1 {
			t.Errorf("Expected 1 release, got %d", len(releases))
		}
	})

	t.Run("GetResourceError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("get resource error")
		}
		manager.client = kubernetesClient

		releases, err := manager.GetHelmReleasesForKustomization("test-kustomization", "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if len(releases) != 0 {
			t.Errorf("Expected 0 releases, got %d", len(releases))
		}
	})

	t.Run("KustomizationNotFound", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		releases, err := manager.GetHelmReleasesForKustomization("test-kustomization", "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(releases) != 0 {
			t.Errorf("Expected 0 releases, got %d", len(releases))
		}
	})

	t.Run("FromUnstructuredError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
				Object: map[string]any{},
			}, nil
		}
		manager.client = kubernetesClient
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return fmt.Errorf("forced conversion error")
		}
		_, err := manager.GetHelmReleasesForKustomization("test-kustomization", "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})
}

func TestBaseKubernetesManager_SuspendKustomization(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.PatchResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			expectedPatch := []byte(`{"spec":{"suspend":true}}`)
			if !bytes.Equal(data, expectedPatch) {
				t.Errorf("Expected patch %s, got %s", expectedPatch, data)
			}
			return &unstructured.Unstructured{}, nil
		}
		manager.client = kubernetesClient

		err := manager.SuspendKustomization("test-kustomization", "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("PatchError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.PatchResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("patch error")
		}
		manager.client = kubernetesClient

		err := manager.SuspendKustomization("test-kustomization", "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "patch error" {
			t.Errorf("Expected error 'patch error', got %v", err)
		}
	})

	t.Run("ResourceNotFound", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.PatchResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("resource not found")
		}
		manager.client = kubernetesClient

		err := manager.SuspendKustomization("nonexistent-kustomization", "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "resource not found") {
			t.Errorf("Expected error containing 'resource not found', got %v", err)
		}
	})

	t.Run("PatchResourceError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.PatchResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("namespace not found")
		}
		manager.client = kubernetesClient

		err := manager.SuspendKustomization("test-kustomization", "nonexistent-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "namespace not found") {
			t.Errorf("Expected error containing 'namespace not found', got %v", err)
		}
	})

	t.Run("ServerCouldNotFindResource", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.PatchResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		err := manager.SuspendKustomization("observability", "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "the server could not find the requested resource") {
			t.Errorf("Expected error containing 'the server could not find the requested resource', got %v", err)
		}
	})
}

func TestBaseKubernetesManager_SuspendHelmRelease(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.PatchResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			expectedPatch := []byte(`{"spec":{"suspend":true}}`)
			if !bytes.Equal(data, expectedPatch) {
				t.Errorf("Expected patch %s, got %s", expectedPatch, data)
			}
			return &unstructured.Unstructured{}, nil
		}
		manager.client = kubernetesClient

		err := manager.SuspendHelmRelease("test-release", "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("PatchError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.PatchResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("patch error")
		}
		manager.client = kubernetesClient

		err := manager.SuspendHelmRelease("test-release", "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "patch error" {
			t.Errorf("Expected error 'patch error', got %v", err)
		}
	})

	t.Run("ResourceNotFound", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.PatchResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("resource not found")
		}
		manager.client = kubernetesClient

		err := manager.SuspendHelmRelease("nonexistent-release", "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "resource not found") {
			t.Errorf("Expected error containing 'resource not found', got %v", err)
		}
	})

	t.Run("PatchResourceError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.PatchResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("namespace not found")
		}
		manager.client = kubernetesClient

		err := manager.SuspendHelmRelease("test-release", "nonexistent-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "namespace not found") {
			t.Errorf("Expected error containing 'namespace not found', got %v", err)
		}
	})

	t.Run("ServerCouldNotFindResource", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.PatchResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		err := manager.SuspendHelmRelease("observability", "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "the server could not find the requested resource") {
			t.Errorf("Expected error containing 'the server could not find the requested resource', got %v", err)
		}
	})
}

func TestBaseKubernetesManager_ApplyGitRepository(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		repo := &sourcev1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.GitRepositorySpec{
				URL: "https://github.com/test/repo",
				Interval: metav1.Duration{
					Duration: time.Minute,
				},
			},
		}

		err := manager.ApplyGitRepository(repo)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ToUnstructuredError", func(t *testing.T) {
		manager := setup(t)
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return nil, fmt.Errorf("forced conversion error")
		}

		repo := &sourcev1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.GitRepositorySpec{
				URL: "https://github.com/test/repo",
				Interval: metav1.Duration{
					Duration: time.Minute,
				},
			},
		}

		err := manager.ApplyGitRepository(repo)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ValidateFieldsError", func(t *testing.T) {
		manager := setup(t)
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return map[string]any{
				"metadata": map[string]any{},
				"spec":     map[string]any{},
			}, nil
		}

		repo := &sourcev1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.GitRepositorySpec{
				URL: "https://github.com/test/repo",
				Interval: metav1.Duration{
					Duration: time.Minute,
				},
			},
		}

		err := manager.ApplyGitRepository(repo)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ApplyWithRetryError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("apply error")
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		repo := &sourcev1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.GitRepositorySpec{
				URL: "https://github.com/test/repo",
				Interval: metav1.Duration{
					Duration: time.Minute,
				},
			},
		}

		err := manager.ApplyGitRepository(repo)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("NilRepository", func(t *testing.T) {
		manager := setup(t)
		err := manager.ApplyGitRepository(nil)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "ToUnstructured requires a non-nil pointer to an object") {
			t.Errorf("Expected error containing 'ToUnstructured requires a non-nil pointer to an object', got %v", err)
		}
	})

	t.Run("SuccessWithSecretRef", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		var appliedObj *unstructured.Unstructured
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			appliedObj = obj
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}

		secretName := "test-secret"
		repo := &sourcev1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.GitRepositorySpec{
				URL: "https://github.com/test/repo",
				Interval: metav1.Duration{
					Duration: time.Minute,
				},
				SecretRef: &meta.LocalObjectReference{
					Name: secretName,
				},
			},
		}

		err := manager.ApplyGitRepository(repo)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if appliedObj == nil {
			t.Fatal("Expected object to be applied")
		}
		spec, ok := appliedObj.Object["spec"].(map[string]any)
		if !ok {
			t.Fatal("Expected spec to be present")
		}
		secretRef, exists := spec["secretRef"]
		if !exists {
			t.Error("Expected secretRef to be present in spec")
		} else {
			secretRefMap, ok := secretRef.(map[string]any)
			if !ok {
				t.Errorf("Expected secretRef to be a map, got %T", secretRef)
			} else if secretRefMap["name"] != secretName {
				t.Errorf("Expected secretRef name to be '%s', got '%v'", secretName, secretRefMap["name"])
			}
		}
	})

	t.Run("SuccessWithoutSecretRef", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		var appliedObj *unstructured.Unstructured
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			appliedObj = obj
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}

		repo := &sourcev1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.GitRepositorySpec{
				URL: "https://github.com/test/repo",
				Interval: metav1.Duration{
					Duration: time.Minute,
				},
				SecretRef: nil,
			},
		}

		err := manager.ApplyGitRepository(repo)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if appliedObj == nil {
			t.Fatal("Expected object to be applied")
		}
		spec, ok := appliedObj.Object["spec"].(map[string]any)
		if !ok {
			t.Fatal("Expected spec to be present")
		}
		if _, exists := spec["secretRef"]; exists {
			t.Error("Expected secretRef to not be present in spec when nil")
		}
	})
}

func TestBaseKubernetesManager_CheckGitRepositoryStatus(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "source.toolkit.fluxcd.io/v1",
							"kind":       "GitRepository",
							"metadata": map[string]any{
								"name": "repo1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":    "Ready",
										"status":  "True",
										"message": "Ready",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		err := manager.CheckGitRepositoryStatus()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ListResourcesError", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return nil, fmt.Errorf("list resources error")
		}
		manager.client = kubernetesClient

		err := manager.CheckGitRepositoryStatus()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "list resources error") {
			t.Errorf("Expected error containing 'list resources error', got %v", err)
		}
	})

	t.Run("FromUnstructuredError", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "source.toolkit.fluxcd.io/v1",
							"kind":       "GitRepository",
							"metadata": map[string]any{
								"name": "repo1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":    "Ready",
										"status":  "True",
										"message": "Ready",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return fmt.Errorf("forced conversion error")
		}

		err := manager.CheckGitRepositoryStatus()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "forced conversion error") {
			t.Errorf("Expected error containing 'forced conversion error', got %v", err)
		}
	})

	t.Run("RepositoryNotReady", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "source.toolkit.fluxcd.io/v1",
							"kind":       "GitRepository",
							"metadata": map[string]any{
								"name": "repo1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":    "Ready",
										"status":  "False",
										"message": "repo not ready",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		err := manager.CheckGitRepositoryStatus()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "repo1") || !strings.Contains(err.Error(), "repo not ready") {
			t.Errorf("Expected error containing repo name and message, got %v", err)
		}
	})

	t.Run("OCIRepositoryNotReady", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		callCount := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			callCount++
			if gvr.Resource == "gitrepositories" {
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{}}, nil
			}
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "source.toolkit.fluxcd.io/v1",
							"kind":       "OCIRepository",
							"metadata": map[string]any{
								"name": "oci-repo1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":    "Ready",
										"status":  "False",
										"message": "oci repo not ready",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		err := manager.CheckGitRepositoryStatus()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "oci-repo1") || !strings.Contains(err.Error(), "oci repo not ready") {
			t.Errorf("Expected error containing oci repo name and message, got %v", err)
		}
	})

	t.Run("OCIRepositoryListError", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		callCount := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			callCount++
			if gvr.Resource == "gitrepositories" {
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{}}, nil
			}
			return nil, fmt.Errorf("oci list error")
		}
		manager.client = kubernetesClient

		err := manager.CheckGitRepositoryStatus()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to list oci repositories") {
			t.Errorf("Expected oci list error, got %v", err)
		}
	})

	t.Run("OCIRepositoryFromUnstructuredError", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		callCount := 0
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			callCount++
			if gvr.Resource == "gitrepositories" {
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{}}, nil
			}
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "source.toolkit.fluxcd.io/v1",
							"kind":       "OCIRepository",
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return fmt.Errorf("forced conversion error")
		}

		err := manager.CheckGitRepositoryStatus()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to convert oci repository") {
			t.Errorf("Expected oci conversion error, got %v", err)
		}
	})
}

func TestBaseKubernetesManager_GetKustomizationStatus(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":   "Ready",
										"status": "True",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !status["k1"] {
			t.Errorf("Expected k1 to be ready, got %v", status["k1"])
		}
	})

	t.Run("ListResourcesError", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return nil, fmt.Errorf("list resources error")
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "list resources error") {
			t.Errorf("Expected error containing 'list resources error', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("FromUnstructuredError", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":   "Ready",
										"status": "True",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return fmt.Errorf("forced conversion error")
		}

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "forced conversion error") {
			t.Errorf("Expected error containing 'forced conversion error', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("KustomizationNotReady", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":   "Ready",
										"status": "False",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if status["k1"] {
			t.Errorf("Expected k1 to be not ready, got %v", status["k1"])
		}
	})

	t.Run("KustomizationFailed", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":    "Ready",
										"status":  "False",
										"reason":  "ReconciliationFailed",
										"message": "kustomization failed",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kustomization k1 failed: kustomization failed") {
			t.Errorf("Expected error containing 'kustomization k1 failed: kustomization failed', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("KustomizationArtifactFailed", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":    "Ready",
										"status":  "False",
										"reason":  "ArtifactFailed",
										"message": "kustomization path not found: stat /tmp/kustomization-1671333540/kustomize\\ingress\\cleanup: no such file or directory",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kustomization k1 failed: kustomization path not found") {
			t.Errorf("Expected error containing 'kustomization k1 failed: kustomization path not found', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("KustomizationMissing", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":   "Ready",
										"status": "True",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1", "k2"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !status["k1"] {
			t.Errorf("Expected k1 to be ready, got %v", status["k1"])
		}
		if status["k2"] {
			t.Errorf("Expected k2 to be not ready, got %v", status["k2"])
		}
	})
}

func TestBaseKubernetesManager_WaitForKubernetesHealthy(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
		manager.healthCheckPollInterval = 50 * time.Millisecond
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

func TestBaseKubernetesManager_ApplyOCIRepository(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		manager.client = kubernetesClient
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return map[string]any{
				"apiVersion": "source.toolkit.fluxcd.io/v1",
				"kind":       "OCIRepository",
				"metadata": map[string]any{
					"name":      "test-repo",
					"namespace": "test-namespace",
				},
				"spec": map[string]any{
					"url": "oci://test-registry.com/test-image",
				},
			}, nil
		}

		repo := &sourcev1.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.OCIRepositorySpec{
				URL: "oci://test-registry.com/test-image",
			},
		}

		err := manager.ApplyOCIRepository(repo)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ToUnstructuredError", func(t *testing.T) {
		manager := setup(t)
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return nil, fmt.Errorf("conversion error")
		}

		repo := &sourcev1.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.OCIRepositorySpec{
				URL: "oci://test-registry.com/test-image",
			},
		}

		err := manager.ApplyOCIRepository(repo)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to convert ocirepository to unstructured") {
			t.Errorf("Expected conversion error, got: %v", err)
		}
	})

	t.Run("ValidationError", func(t *testing.T) {
		manager := setup(t)
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return map[string]any{
				"metadata": map[string]any{
					"name": "",
				},
			}, nil
		}

		repo := &sourcev1.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.OCIRepositorySpec{
				URL: "oci://test-registry.com/test-image",
			},
		}

		err := manager.ApplyOCIRepository(repo)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid ocirepository fields") {
			t.Errorf("Expected validation error, got: %v", err)
		}
	})

	t.Run("ValidateFieldsError_NilObject", func(t *testing.T) {
		manager := setup(t)
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return nil, nil
		}

		repo := &sourcev1.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.OCIRepositorySpec{
				URL: "oci://test-registry.com/test-image",
			},
		}

		err := manager.ApplyOCIRepository(repo)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid ocirepository fields") {
			t.Errorf("Expected validation error, got: %v", err)
		}
	})

	t.Run("ValidateFieldsError_MissingMetadata", func(t *testing.T) {
		manager := setup(t)
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return map[string]any{
				"spec": map[string]any{
					"url": "oci://test-registry.com/test-image",
				},
			}, nil
		}

		repo := &sourcev1.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: "test-namespace",
			},
			Spec: sourcev1.OCIRepositorySpec{
				URL: "oci://test-registry.com/test-image",
			},
		}

		err := manager.ApplyOCIRepository(repo)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid ocirepository fields") {
			t.Errorf("Expected validation error, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_GetNodeReadyStatus(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
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

func TestBaseKubernetesManager_hasCleanupOperations(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		return NewKubernetesManager(mocks.KubernetesClient)
	}

	t.Run("ReturnsFalseForEmptyBlueprint", func(t *testing.T) {
		manager := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{},
		}

		result := manager.hasCleanupOperations(blueprint)
		if result {
			t.Error("Expected false for empty blueprint")
		}
	})

	t.Run("ReturnsFalseForRegularKustomizations", func(t *testing.T) {
		manager := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "regular-1"},
				{Name: "regular-2"},
			},
		}

		result := manager.hasCleanupOperations(blueprint)
		if result {
			t.Error("Expected false for regular kustomizations without cleanup")
		}
	})

	t.Run("ReturnsTrueForDestroyOnly", func(t *testing.T) {
		manager := setup(t)
		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "regular"},
				{Name: "destroy-only", DestroyOnly: &destroyOnlyTrue},
			},
		}

		result := manager.hasCleanupOperations(blueprint)
		if !result {
			t.Error("Expected true for blueprint with destroy-only kustomization")
		}
	})

	t.Run("ReturnsTrueForCleanupComponents", func(t *testing.T) {
		manager := setup(t)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "with-cleanup", Cleanup: []string{"cleanup-path"}},
			},
		}

		result := manager.hasCleanupOperations(blueprint)
		if !result {
			t.Error("Expected true for blueprint with cleanup components")
		}
	})

	t.Run("ReturnsFalseForDestroyOnlyWithDestroyFalse", func(t *testing.T) {
		manager := setup(t)
		destroyOnlyTrue := true
		destroyFalseVal := false
		destroyFalse := blueprintv1alpha1.BoolExpression{Value: &destroyFalseVal, IsExpr: false}
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "destroy-only", DestroyOnly: &destroyOnlyTrue, Destroy: &destroyFalse},
			},
		}

		result := manager.hasCleanupOperations(blueprint)
		if result {
			t.Error("Expected false when destroy-only kustomization has destroy=false")
		}
	})

	t.Run("ReturnsFalseForCleanupWithDestroyFalse", func(t *testing.T) {
		manager := setup(t)
		destroyFalseVal := false
		destroyFalse := blueprintv1alpha1.BoolExpression{Value: &destroyFalseVal, IsExpr: false}
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "with-cleanup", Cleanup: []string{"cleanup-path"}, Destroy: &destroyFalse},
			},
		}

		result := manager.hasCleanupOperations(blueprint)
		if result {
			t.Error("Expected false when cleanup kustomization has destroy=false")
		}
	})
}

func TestBaseKubernetesManager_deployCleanupSemaphore(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		return NewKubernetesManager(mocks.KubernetesClient)
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var appliedResources []string
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			appliedResources = append(appliedResources, obj.GetKind()+":"+obj.GetName())
			return obj, nil
		}
		manager.client = kubernetesClient

		err := manager.deployCleanupSemaphore()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(appliedResources) != 2 {
			t.Errorf("Expected 2 applied resources (namespace + configmap), got %d", len(appliedResources))
		}
		if appliedResources[0] != "Namespace:system-cleanup" {
			t.Errorf("Expected first resource to be 'Namespace:system-cleanup', got %s", appliedResources[0])
		}
		if appliedResources[1] != "ConfigMap:cleanup-authorized" {
			t.Errorf("Expected second resource to be 'ConfigMap:cleanup-authorized', got %s", appliedResources[1])
		}
	})

	t.Run("ErrorCreatingNamespace", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "namespaces" {
				return nil, fmt.Errorf("namespace creation failed")
			}
			return obj, nil
		}
		manager.client = kubernetesClient

		err := manager.deployCleanupSemaphore()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create cleanup namespace") {
			t.Errorf("Expected error about namespace creation, got %v", err)
		}
	})

	t.Run("ErrorCreatingConfigMap", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "configmaps" {
				return nil, fmt.Errorf("configmap creation failed")
			}
			return obj, nil
		}
		manager.client = kubernetesClient

		err := manager.deployCleanupSemaphore()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create cleanup semaphore") {
			t.Errorf("Expected error about semaphore creation, got %v", err)
		}
	})

	t.Run("ConfigMapContainsExpectedData", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var configMapData map[string]string
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "configmaps" {
				configMapData = obj.Object["data"].(map[string]string)
			}
			return obj, nil
		}
		manager.client = kubernetesClient

		err := manager.deployCleanupSemaphore()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if configMapData["authorized"] != "true" {
			t.Errorf("Expected authorized=true, got %v", configMapData["authorized"])
		}
		if _, ok := configMapData["timestamp"]; !ok {
			t.Error("Expected timestamp field in configmap data")
		}
	})
}

func TestBaseKubernetesManager_DeleteBlueprint(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationReconcileSleep = 50 * time.Millisecond
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var deleteCalls []string
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCalls = append(deleteCalls, name)
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization-1",
				},
				{
					Name: "test-kustomization-2",
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(deleteCalls) != 2 {
			t.Errorf("Expected 2 delete calls, got %d", len(deleteCalls))
		}
	})

	t.Run("SuccessSkipsDestroyFalse", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var deleteCalls []string
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCalls = append(deleteCalls, name)
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		destroyFalse := false
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization-1",
					Destroy: &blueprintv1alpha1.BoolExpression{Value: &destroyFalse, IsExpr: false},
				},
				{
					Name:    "test-kustomization-2",
					Destroy: nil,
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(deleteCalls) != 1 {
			t.Errorf("Expected 1 delete call, got %d", len(deleteCalls))
		}
		if deleteCalls[0] != "test-kustomization-2" {
			t.Errorf("Expected delete call for 'test-kustomization-2', got %s", deleteCalls[0])
		}
	})

	t.Run("SuccessWithDestroyOnlyKustomizations", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var applyCalls []string
		var deleteCalls []string
		deletedResources := make(map[string]bool)

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if name, ok := obj.Object["metadata"].(map[string]any)["name"].(string); ok {
				applyCalls = append(applyCalls, name)
			}
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCalls = append(deleteCalls, name)
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only-kustomization"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only-kustomization",
					DestroyOnly: &destroyOnlyTrue,
				},
				{
					Name: "regular-kustomization",
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(applyCalls) != 3 {
			t.Errorf("Expected 3 apply calls (namespace + semaphore + kustomization), got %d", len(applyCalls))
		}
		if len(applyCalls) >= 3 && applyCalls[2] != "destroy-only-kustomization" {
			t.Errorf("Expected third apply call for 'destroy-only-kustomization', got %s", applyCalls[2])
		}

		if len(deleteCalls) != 2 {
			t.Errorf("Expected 2 delete calls (destroy-only + regular), got %d", len(deleteCalls))
		}
	})

	t.Run("DestroyOnlyProcessedBeforeRegular", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var operations []string
		deletedResources := make(map[string]bool)

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if name, ok := obj.Object["metadata"].(map[string]any)["name"].(string); ok {
				operations = append(operations, "apply:"+name)
			}
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			operations = append(operations, "delete:"+name)
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "regular",
				},
				{
					Name:        "destroy-only",
					DestroyOnly: &destroyOnlyTrue,
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		applyDestroyOnlyIdx := -1
		deleteDestroyOnlyIdx := -1
		deleteRegularIdx := -1

		for i, op := range operations {
			switch op {
			case "apply:destroy-only":
				applyDestroyOnlyIdx = i
			case "delete:destroy-only":
				deleteDestroyOnlyIdx = i
			case "delete:regular":
				deleteRegularIdx = i
			}
		}

		if applyDestroyOnlyIdx == -1 {
			t.Error("Expected destroy-only kustomization to be applied")
		}
		if deleteDestroyOnlyIdx == -1 {
			t.Error("Expected destroy-only kustomization to be deleted")
		}
		if deleteRegularIdx == -1 {
			t.Error("Expected regular kustomization to be deleted")
		}

		if applyDestroyOnlyIdx > deleteDestroyOnlyIdx {
			t.Error("Expected destroy-only to be applied before deleted")
		}
		if deleteDestroyOnlyIdx > deleteRegularIdx {
			t.Error("Expected destroy-only to be deleted before regular kustomization")
		}
	})

	t.Run("DestroyOnlyFailsWhenNotReady", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "False"},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only",
					DestroyOnly: &destroyOnlyTrue,
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error when destroy-only kustomization does not become ready")
		}
		if !strings.Contains(err.Error(), "did not become ready within timeout") {
			t.Errorf("Expected timeout error, got %v", err)
		}
	})

	t.Run("DestroyOnlyWithSubstitutions", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var appliedConfigMaps []string
		deletedResources := make(map[string]bool)

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "configmaps" {
				appliedConfigMaps = append(appliedConfigMaps, obj.GetName())
			}
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:          "destroy-only",
					DestroyOnly:   &destroyOnlyTrue,
					Substitutions: map[string]string{"key": "value"},
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		foundValuesConfigMap := false
		for _, name := range appliedConfigMaps {
			if name == "values-destroy-only" {
				foundValuesConfigMap = true
			}
		}
		if !foundValuesConfigMap {
			t.Error("Expected ConfigMap 'values-destroy-only' to be created for substitutions")
		}
	})

	t.Run("DestroyOnlyWithBlueprintConfigMaps", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var appliedKustomizations []map[string]any
		deletedResources := make(map[string]bool)

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				if spec, ok := obj.Object["spec"].(map[string]any); ok {
					appliedKustomizations = append(appliedKustomizations, spec)
				}
			}
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata:   blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			ConfigMaps: map[string]map[string]string{"values-common": {"key": "value"}},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only",
					DestroyOnly: &destroyOnlyTrue,
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(appliedKustomizations) == 0 {
			t.Fatal("Expected at least one kustomization to be applied")
		}
		postBuild, ok := appliedKustomizations[0]["postBuild"].(map[string]any)
		if !ok {
			t.Fatal("Expected postBuild in kustomization spec")
		}
		substituteFrom, ok := postBuild["substituteFrom"].([]any)
		if !ok {
			t.Fatal("Expected substituteFrom in postBuild")
		}
		foundValuesCommon := false
		for _, ref := range substituteFrom {
			refMap := ref.(map[string]any)
			if refMap["name"] == "values-common" {
				foundValuesCommon = true
			}
		}
		if !foundValuesCommon {
			t.Error("Expected blueprint ConfigMap 'values-common' to be referenced in destroy-only kustomization")
		}
	})

	t.Run("DestroyOnlyBecomesReady", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		deletedResources := make(map[string]bool)

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only",
					DestroyOnly: &destroyOnlyTrue,
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if !deletedResources["destroy-only"] {
			t.Error("Expected destroy-only kustomization to be deleted after becoming ready")
		}
	})

	t.Run("DestroyOnlyErrorApplyingKustomization", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return nil, fmt.Errorf("apply kustomization error")
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only",
					DestroyOnly: &destroyOnlyTrue,
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply destroy-only kustomization") {
			t.Errorf("Expected error about applying destroy-only kustomization, got %v", err)
		}
	})

	t.Run("DestroyOnlyErrorDeletingKustomization", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			if name == "destroy-only" {
				return fmt.Errorf("delete kustomization error")
			}
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only",
					DestroyOnly: &destroyOnlyTrue,
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to delete destroy-only kustomization") {
			t.Errorf("Expected error about deleting destroy-only kustomization, got %v", err)
		}
	})

	t.Run("DestroyOnlyErrorCreatingConfigMap", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "configmaps" && obj.GetName() == "values-destroy-only" {
				return nil, fmt.Errorf("configmap creation error")
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:          "destroy-only",
					DestroyOnly:   &destroyOnlyTrue,
					Substitutions: map[string]string{"key": "value"},
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create ConfigMap for destroy-only kustomization") {
			t.Errorf("Expected error about ConfigMap creation, got %v", err)
		}
	})

	t.Run("SuccessWithCleanupKustomizations", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var deleteCalls []string
		var applyCalls []string
		deletedResources := make(map[string]bool)
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCalls = append(deleteCalls, name)
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "test-kustomization-cleanup"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if name, ok := obj.Object["metadata"].(map[string]any)["name"].(string); ok {
				applyCalls = append(applyCalls, name)
			}
			return obj, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization",
					Path:    "base",
					Cleanup: []string{"path"},
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(deleteCalls) < 2 {
			t.Errorf("Expected at least 2 delete calls (main + cleanup), got %d", len(deleteCalls))
		}
		if len(applyCalls) != 3 {
			t.Errorf("Expected 3 apply calls (namespace + semaphore + cleanup), got %d", len(applyCalls))
		}
		if len(applyCalls) >= 3 && applyCalls[2] != "test-kustomization-cleanup" {
			t.Errorf("Expected third apply call for 'test-kustomization-cleanup', got %s", applyCalls[2])
		}
	})

	t.Run("SuccessWithMultipleCleanupPaths", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var deleteCalls []string
		var applyCalls []string
		deletedResources := make(map[string]bool)
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCalls = append(deleteCalls, name)
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "test-kustomization-cleanup"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if name, ok := obj.Object["metadata"].(map[string]any)["name"].(string); ok {
				applyCalls = append(applyCalls, name)
			}
			return obj, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization",
					Cleanup: []string{"cleanup1", "cleanup2"},
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(applyCalls) != 3 {
			t.Errorf("Expected 3 apply calls (namespace + semaphore + cleanup), got %d", len(applyCalls))
		}
		if len(deleteCalls) < 2 {
			t.Errorf("Expected at least 2 delete calls (main + cleanup), got %d", len(deleteCalls))
		}
	})

	t.Run("CleanupInheritsParentSubstitutions", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var cleanupPostBuild map[string]any
		deletedResources := make(map[string]bool)
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "test-kustomization-cleanup"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if name, ok := obj.Object["metadata"].(map[string]any)["name"].(string); ok {
				if name == "test-kustomization-cleanup" {
					spec := obj.Object["spec"].(map[string]any)
					if pb, ok := spec["postBuild"].(map[string]any); ok {
						cleanupPostBuild = pb
					}
				}
			}
			return obj, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			ConfigMaps: map[string]map[string]string{
				"values-common": {"key": "value"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization",
					Cleanup: []string{"cleanup"},
					Substitutions: map[string]string{
						"VAR1": "value1",
					},
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if cleanupPostBuild == nil {
			t.Fatal("Expected cleanup kustomization to have postBuild, got nil")
		}

		substituteFrom, ok := cleanupPostBuild["substituteFrom"].([]any)
		if !ok {
			t.Fatal("Expected postBuild to have substituteFrom")
		}

		foundParentConfigMap := false
		foundBlueprintConfigMap := false
		for _, ref := range substituteFrom {
			refMap := ref.(map[string]any)
			name := refMap["name"].(string)
			if name == "values-test-kustomization" {
				foundParentConfigMap = true
			}
			if name == "values-common" {
				foundBlueprintConfigMap = true
			}
		}

		if !foundParentConfigMap {
			t.Error("Expected cleanup to reference parent's substitutions ConfigMap (values-test-kustomization)")
		}
		if !foundBlueprintConfigMap {
			t.Error("Expected cleanup to reference blueprint ConfigMap (values-common)")
		}
	})

	t.Run("SuccessWithOCISource", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var applyCalls []map[string]any
		deletedResources := make(map[string]bool)
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "test-kustomization-cleanup"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if spec, ok := obj.Object["spec"].(map[string]any); ok {
				if sourceRef, ok := spec["sourceRef"].(map[string]any); ok {
					applyCalls = append(applyCalls, sourceRef)
				}
			}
			return obj, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "oci-source",
					Url:  "oci://example.com/repo",
				},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization",
					Source:  "oci-source",
					Cleanup: []string{"cleanup"},
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(applyCalls) != 1 {
			t.Errorf("Expected 1 apply call with sourceRef, got %d", len(applyCalls))
		}
		if len(applyCalls) >= 1 && applyCalls[0]["kind"] != "OCIRepository" {
			t.Errorf("Expected source kind 'OCIRepository', got %v", applyCalls[0]["kind"])
		}
	})

	t.Run("SuccessWithPathNormalization", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var applyCalls []map[string]any
		deletedResources := make(map[string]bool)
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "test-kustomization-cleanup"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if spec, ok := obj.Object["spec"].(map[string]any); ok {
				applyCalls = append(applyCalls, spec)
			}
			return obj, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization",
					Path:    "base\\path",
					Cleanup: []string{"path"},
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(applyCalls) != 1 {
			t.Errorf("Expected 1 apply call with spec, got %d", len(applyCalls))
		}
		expectedPath := "kustomize/base/path/cleanup"
		if len(applyCalls) >= 1 && applyCalls[0]["path"] != expectedPath {
			t.Errorf("Expected path '%s', got %v", expectedPath, applyCalls[0]["path"])
		}
		if len(applyCalls) >= 1 {
			components := applyCalls[0]["components"].([]any)
			if len(components) != 1 || components[0] != "path" {
				t.Errorf("Expected components ['path'], got %v", components)
			}
		}
	})

	t.Run("ErrorDeleteKustomization", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("delete error")
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to delete kustomization") {
			t.Errorf("Expected error containing 'failed to delete kustomization', got %v", err)
		}
	})

	t.Run("ErrorApplyCleanupKustomization", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return nil, fmt.Errorf("apply error")
			}
			return obj, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization",
					Cleanup: []string{"cleanup"},
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply cleanup kustomization") {
			t.Errorf("Expected error containing 'failed to apply cleanup kustomization', got %v", err)
		}
	})

	t.Run("ErrorDeleteCleanupKustomization", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		deleteCallCount := 0
		deletedResources := make(map[string]bool)
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCallCount++
			if strings.Contains(name, "cleanup") {
				return fmt.Errorf("delete cleanup error")
			}
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "test-kustomization-cleanup"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization",
					Cleanup: []string{"cleanup"},
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to delete cleanup kustomization") {
			t.Errorf("Expected error containing 'failed to delete cleanup kustomization', got %v", err)
		}
	})
}

func TestBaseKubernetesManager_ApplyBlueprint(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
		manager.shims = mocks.Shims
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return runtime.DefaultUnstructuredConverter.FromUnstructured(obj, target)
		}
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "test-source",
					Url:  "https://github.com/example/repo.git",
					Ref:  blueprintv1alpha1.Reference{Branch: "main"},
				},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
				},
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessSkipsDestroyOnlyKustomizations", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var appliedKustomizations []string
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				if name, ok := obj.Object["metadata"].(map[string]any)["name"].(string); ok {
					appliedKustomizations = append(appliedKustomizations, name)
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only-kustomization",
					DestroyOnly: &destroyOnlyTrue,
				},
				{
					Name: "regular-kustomization",
				},
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(appliedKustomizations) != 1 {
			t.Errorf("Expected 1 kustomization to be applied, got %d", len(appliedKustomizations))
		}
		if len(appliedKustomizations) > 0 && appliedKustomizations[0] != "regular-kustomization" {
			t.Errorf("Expected 'regular-kustomization' to be applied, got %s", appliedKustomizations[0])
		}
	})

	t.Run("ErrorOnCreateNamespace", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "Namespace" {
				return nil, fmt.Errorf("namespace creation failed")
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create namespace") {
			t.Errorf("Expected error about namespace creation, got %v", err)
		}
	})

	t.Run("SuccessWithRepository", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/example/repo.git",
				Ref: blueprintv1alpha1.Reference{Branch: "main"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
				},
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithRepositoryAndSecretName", func(t *testing.T) {
		manager := setup(t)
		secretName := "test-secret"
		var appliedRepo *sourcev1.GitRepository
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "gitrepositories" {
				var repo sourcev1.GitRepository
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &repo); err == nil {
					appliedRepo = &repo
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return runtime.DefaultUnstructuredConverter.FromUnstructured(obj, target)
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url:        "https://github.com/example/repo.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
				SecretName: &secretName,
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if appliedRepo == nil {
			t.Fatal("Expected GitRepository to be applied")
		}
		if appliedRepo.Spec.SecretRef == nil {
			t.Error("Expected SecretRef to be set when SecretName is provided")
		} else if appliedRepo.Spec.SecretRef.Name != secretName {
			t.Errorf("Expected SecretRef.Name to be '%s', got '%s'", secretName, appliedRepo.Spec.SecretRef.Name)
		}
	})

	t.Run("SuccessWithRepositoryAndNilSecretName", func(t *testing.T) {
		manager := setup(t)
		var appliedRepo *sourcev1.GitRepository
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "gitrepositories" {
				var repo sourcev1.GitRepository
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &repo); err == nil {
					appliedRepo = &repo
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return runtime.DefaultUnstructuredConverter.FromUnstructured(obj, target)
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url:        "https://github.com/example/repo.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
				SecretName: nil,
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if appliedRepo == nil {
			t.Fatal("Expected GitRepository to be applied")
		}
		if appliedRepo.Spec.SecretRef != nil {
			t.Errorf("Expected SecretRef to be nil when SecretName is nil, got %v", appliedRepo.Spec.SecretRef)
		}
	})

	t.Run("SuccessWithRepositoryAndEmptySecretName", func(t *testing.T) {
		manager := setup(t)
		emptySecretName := ""
		var appliedRepo *sourcev1.GitRepository
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "gitrepositories" {
				var repo sourcev1.GitRepository
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &repo); err == nil {
					appliedRepo = &repo
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return runtime.DefaultUnstructuredConverter.FromUnstructured(obj, target)
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url:        "https://github.com/example/repo.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
				SecretName: &emptySecretName,
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if appliedRepo == nil {
			t.Fatal("Expected GitRepository to be applied")
		}
		if appliedRepo.Spec.SecretRef != nil {
			t.Errorf("Expected SecretRef to be nil when SecretName is empty string, got %v", appliedRepo.Spec.SecretRef)
		}
	})

	t.Run("ErrorOnApplyBlueprintRepository", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "GitRepository" {
				return nil, fmt.Errorf("git repository apply failed")
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/example/repo.git",
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply blueprint repository") {
			t.Errorf("Expected error about blueprint repository, got %v", err)
		}
	})

	t.Run("ErrorOnApplySource", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "GitRepository" {
				return nil, fmt.Errorf("git repository apply failed")
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "test-source",
					Url:  "https://github.com/example/repo.git",
				},
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply source") {
			t.Errorf("Expected error about applying source, got %v", err)
		}
	})

	t.Run("ErrorOnApplyConfigMap", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "ConfigMap" {
				return nil, fmt.Errorf("configmap apply failed")
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Substitutions: map[string]string{
						"key": "value",
					},
				},
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create ConfigMap") {
			t.Errorf("Expected error about ConfigMap, got %v", err)
		}
	})

	t.Run("ErrorOnApplyKustomization", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "Kustomization" {
				return nil, fmt.Errorf("kustomization apply failed")
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Path: "test/path",
				},
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply kustomization") {
			t.Errorf("Expected error about kustomization, got %v", err)
		}
	})

	t.Run("SuccessWithOCISource", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "oci-source",
					Url:  "oci://example.com/repo",
				},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
				},
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithBlueprintConfigMaps", func(t *testing.T) {
		manager := setup(t)
		configMapApplied := false
		kustomizationApplied := false
		var appliedKustomization kustomizev1.Kustomization
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "ConfigMap" && obj.GetName() == "values-common" {
				configMapApplied = true
			}
			if obj.GetKind() == "Kustomization" {
				kustomizationApplied = true
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &appliedKustomization); err != nil {
					t.Fatalf("Failed to convert kustomization: %v", err)
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			ConfigMaps: map[string]map[string]string{
				"values-common": {
					"domain": "example.com",
					"region": "us-west-2",
				},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Path: "test/path",
				},
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !configMapApplied {
			t.Error("Expected values-common ConfigMap to be applied")
		}
		if !kustomizationApplied {
			t.Error("Expected Kustomization to be applied")
		}
		if appliedKustomization.Spec.PostBuild == nil {
			t.Fatal("Expected PostBuild to be set when blueprint has ConfigMaps")
		}
		if len(appliedKustomization.Spec.PostBuild.SubstituteFrom) != 1 {
			t.Fatalf("Expected 1 SubstituteFrom reference (values-common), got %d", len(appliedKustomization.Spec.PostBuild.SubstituteFrom))
		}
		if appliedKustomization.Spec.PostBuild.SubstituteFrom[0].Name != "values-common" {
			t.Errorf("Expected SubstituteFrom to be values-common, got '%s'", appliedKustomization.Spec.PostBuild.SubstituteFrom[0].Name)
		}
	})

	t.Run("SuccessWithBlueprintConfigMapsAndKustomizationSubstitutions", func(t *testing.T) {
		manager := setup(t)
		commonConfigMapApplied := false
		kustomizationConfigMapApplied := false
		kustomizationApplied := false
		var appliedKustomization kustomizev1.Kustomization
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "ConfigMap" {
				if obj.GetName() == "values-common" {
					commonConfigMapApplied = true
				}
				if obj.GetName() == "values-test-kustomization" {
					kustomizationConfigMapApplied = true
				}
			}
			if obj.GetKind() == "Kustomization" {
				kustomizationApplied = true
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &appliedKustomization); err != nil {
					t.Fatalf("Failed to convert kustomization: %v", err)
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			ConfigMaps: map[string]map[string]string{
				"values-common": {
					"domain": "example.com",
				},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Path: "test/path",
					Substitutions: map[string]string{
						"key": "value",
					},
				},
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !commonConfigMapApplied {
			t.Error("Expected values-common ConfigMap to be applied")
		}
		if !kustomizationConfigMapApplied {
			t.Error("Expected values-test-kustomization ConfigMap to be applied")
		}
		if !kustomizationApplied {
			t.Error("Expected Kustomization to be applied")
		}
		if appliedKustomization.Spec.PostBuild == nil {
			t.Fatal("Expected PostBuild to be set")
		}
		if len(appliedKustomization.Spec.PostBuild.SubstituteFrom) != 2 {
			t.Fatalf("Expected 2 SubstituteFrom references (values-common and values-test-kustomization), got %d", len(appliedKustomization.Spec.PostBuild.SubstituteFrom))
		}
		foundCommon := false
		foundKustomization := false
		for _, ref := range appliedKustomization.Spec.PostBuild.SubstituteFrom {
			if ref.Name == "values-common" {
				foundCommon = true
			}
			if ref.Name == "values-test-kustomization" {
				foundKustomization = true
			}
		}
		if !foundCommon {
			t.Error("Expected values-common in SubstituteFrom")
		}
		if !foundKustomization {
			t.Error("Expected values-test-kustomization in SubstituteFrom")
		}
	})

	t.Run("SuccessWithMultipleBlueprintConfigMaps", func(t *testing.T) {
		manager := setup(t)
		configMapsApplied := make(map[string]bool)
		var appliedKustomization kustomizev1.Kustomization
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "ConfigMap" {
				configMapsApplied[obj.GetName()] = true
			}
			if obj.GetKind() == "Kustomization" {
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &appliedKustomization); err != nil {
					t.Fatalf("Failed to convert kustomization: %v", err)
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			ConfigMaps: map[string]map[string]string{
				"values-common": {
					"domain": "example.com",
				},
				"values-shared": {
					"shared_key": "shared_value",
				},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Path: "test/path",
				},
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !configMapsApplied["values-common"] {
			t.Error("Expected values-common ConfigMap to be applied")
		}
		if !configMapsApplied["values-shared"] {
			t.Error("Expected values-shared ConfigMap to be applied")
		}
		if appliedKustomization.Spec.PostBuild == nil {
			t.Fatal("Expected PostBuild to be set")
		}
		if len(appliedKustomization.Spec.PostBuild.SubstituteFrom) != 2 {
			t.Fatalf("Expected 2 SubstituteFrom references, got %d", len(appliedKustomization.Spec.PostBuild.SubstituteFrom))
		}
		foundCommon := false
		foundShared := false
		for _, ref := range appliedKustomization.Spec.PostBuild.SubstituteFrom {
			if ref.Name == "values-common" {
				foundCommon = true
			}
			if ref.Name == "values-shared" {
				foundShared = true
			}
		}
		if !foundCommon {
			t.Error("Expected values-common in SubstituteFrom")
		}
		if !foundShared {
			t.Error("Expected values-shared in SubstituteFrom")
		}
	})

	t.Run("SuccessWithKustomizationSubstitutions", func(t *testing.T) {
		manager := setup(t)
		kustomizationConfigMapApplied := false
		kustomizationApplied := false
		var appliedKustomization kustomizev1.Kustomization
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "ConfigMap" && obj.GetName() == "values-csi" {
				kustomizationConfigMapApplied = true
			}
			if obj.GetKind() == "Kustomization" {
				kustomizationApplied = true
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &appliedKustomization); err != nil {
					t.Fatalf("Failed to convert kustomization: %v", err)
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "csi",
					Path: "csi",
					Substitutions: map[string]string{
						"volume_path":   "/custom/volumes",
						"storage_class": "fast-ssd",
					},
				},
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !kustomizationConfigMapApplied {
			t.Error("Expected values-csi ConfigMap to be applied")
		}
		if !kustomizationApplied {
			t.Error("Expected Kustomization to be applied")
		}
		if appliedKustomization.Spec.PostBuild == nil {
			t.Fatal("Expected PostBuild to be set when kustomization has substitutions")
		}
		if len(appliedKustomization.Spec.PostBuild.SubstituteFrom) != 1 {
			t.Fatalf("Expected 1 SubstituteFrom reference (values-csi), got %d", len(appliedKustomization.Spec.PostBuild.SubstituteFrom))
		}
		if appliedKustomization.Spec.PostBuild.SubstituteFrom[0].Name != "values-csi" {
			t.Errorf("Expected SubstituteFrom to be values-csi, got '%s'", appliedKustomization.Spec.PostBuild.SubstituteFrom[0].Name)
		}
	})
}
