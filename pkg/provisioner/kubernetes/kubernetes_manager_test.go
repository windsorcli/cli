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
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// SetupOptions holds test-specific options for setup
type SetupOptions struct {
	Injector di.Injector
}

// Mocks holds all mock dependencies for tests
type Mocks struct {
	Injector        di.Injector
	Shims           *Shims
	KubernetesClient client.KubernetesClient
}

// setupMocks initializes and returns mock dependencies for tests
func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()
	if opts == nil {
		opts = []*SetupOptions{{}}
	}

	// Ensure Injector is initialized
	if opts[0].Injector == nil {
		opts[0].Injector = di.NewMockInjector()
	}

	kubernetesClient := client.NewMockKubernetesClient()
	kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
		return obj, nil
	}
	kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
		return &unstructured.Unstructured{}, nil
	}

	mocks := &Mocks{
		Injector:        opts[0].Injector,
		Shims:           setupShims(t),
		KubernetesClient: kubernetesClient,
	}

	mocks.Injector.Register("kubernetesClient", kubernetesClient)

	return mocks
}

// setupShims initializes and returns shims for tests
func setupShims(t *testing.T) *Shims {
	t.Helper()
	shims := NewShims()
	shims.ToUnstructured = func(obj any) (map[string]any, error) {
		return nil, fmt.Errorf("forced conversion error")
	}
	return shims
}


func TestBaseKubernetesManager_ApplyKustomization(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupMocks(t)
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
}

func TestBaseKubernetesManager_DeleteKustomization(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupMocks(t)
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
		mocks := setupMocks(t)
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

		err := manager.WaitForKustomizations("Waiting for kustomizations", "test-kustomization")
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

		err := manager.WaitForKustomizations("Waiting for kustomizations", "test-kustomization")
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

		err := manager.WaitForKustomizations("Waiting for kustomizations", "test-kustomization")
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

		err := manager.WaitForKustomizations("Waiting for kustomizations", "test-kustomization")
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

		err := manager.WaitForKustomizations("Waiting for kustomizations", "test-kustomization")
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

		err := manager.WaitForKustomizations("Waiting for kustomizations", "test-kustomization")
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

		err := manager.WaitForKustomizations("Waiting for kustomizations", "test-kustomization")
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})
}

func TestBaseKubernetesManager_CreateNamespace(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupMocks(t)
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
		mocks := setupMocks(t)
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
		mocks := setupMocks(t)
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
			mocks := setupMocks(t)
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
			mocks := setupMocks(t)
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
			mocks := setupMocks(t)
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
			mocks := setupMocks(t)
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
		mocks := setupMocks(t)
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
		mocks := setupMocks(t)
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
		mocks := setupMocks(t)
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
		mocks := setupMocks(t)
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
}

func TestBaseKubernetesManager_CheckGitRepositoryStatus(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupMocks(t)
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
			mocks := setupMocks(t)
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
			mocks := setupMocks(t)
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
			mocks := setupMocks(t)
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
}

func TestBaseKubernetesManager_GetKustomizationStatus(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupMocks(t)
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
			mocks := setupMocks(t)
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
			mocks := setupMocks(t)
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
			mocks := setupMocks(t)
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
			mocks := setupMocks(t)
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
			mocks := setupMocks(t)
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
			mocks := setupMocks(t)
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
		mocks := setupMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
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
		kubernetesClient.CheckHealthFunc = func(ctx context.Context, endpoint string) error {
			return fmt.Errorf("health check failed")
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
}

func TestBaseKubernetesManager_ApplyOCIRepository(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupMocks(t)
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
		mocks := setupMocks(t)
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

func TestBaseKubernetesManager_waitForNodesReady(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
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

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := manager.waitForNodesReady(ctx, []string{"node1", "node2"}, nil)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout waiting for nodes to appear") {
			t.Errorf("Expected missing nodes error, got: %v", err)
		}
	})

}

func TestBaseKubernetesManager_getHelmRelease(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient)
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

func TestBaseKubernetesManager_DeleteBlueprint(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupMocks(t)
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
					Destroy: &destroyFalse,
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

	t.Run("SuccessWithCleanupKustomizations", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var deleteCalls []string
		var applyCalls []string
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCalls = append(deleteCalls, name)
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
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
					Cleanup: []string{"cleanup/path"},
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
		if len(applyCalls) != 1 {
			t.Errorf("Expected 1 apply call for cleanup kustomization, got %d", len(applyCalls))
		}
		if applyCalls[0] != "test-kustomization-cleanup-0" {
			t.Errorf("Expected apply call for 'test-kustomization-cleanup-0', got %s", applyCalls[0])
		}
	})

	t.Run("SuccessWithMultipleCleanupPaths", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var deleteCalls []string
		var applyCalls []string
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCalls = append(deleteCalls, name)
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
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

		if len(applyCalls) != 2 {
			t.Errorf("Expected 2 apply calls for cleanup kustomizations, got %d", len(applyCalls))
		}
		if len(deleteCalls) < 3 {
			t.Errorf("Expected at least 3 delete calls (main + 2 cleanup), got %d", len(deleteCalls))
		}
	})

	t.Run("SuccessWithOCISource", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var applyCalls []map[string]any
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			spec := obj.Object["spec"].(map[string]any)
			sourceRef := spec["sourceRef"].(map[string]any)
			applyCalls = append(applyCalls, sourceRef)
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
			t.Errorf("Expected 1 apply call, got %d", len(applyCalls))
		}
		if applyCalls[0]["kind"] != "OCIRepository" {
			t.Errorf("Expected source kind 'OCIRepository', got %v", applyCalls[0]["kind"])
		}
	})

	t.Run("SuccessWithPathNormalization", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var applyCalls []map[string]any
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			spec := obj.Object["spec"].(map[string]any)
			applyCalls = append(applyCalls, spec)
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
					Cleanup: []string{"cleanup\\path"},
				},
			},
		}

		err := manager.DeleteBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(applyCalls) != 1 {
			t.Errorf("Expected 1 apply call, got %d", len(applyCalls))
		}
		expectedPath := "kustomize/base/path/cleanup/path"
		if applyCalls[0]["path"] != expectedPath {
			t.Errorf("Expected path '%s', got %v", expectedPath, applyCalls[0]["path"])
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
			return nil, fmt.Errorf("apply error")
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
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCallCount++
			if strings.Contains(name, "cleanup") {
				return fmt.Errorf("delete cleanup error")
			}
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
