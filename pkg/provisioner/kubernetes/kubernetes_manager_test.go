package kubernetes

import (
	"fmt"
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"github.com/windsorcli/cli/pkg/runtime/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// =============================================================================
// Test Setup
// =============================================================================

// KubernetesTestMocks contains all the mock dependencies for testing the KubernetesManager
type KubernetesTestMocks struct {
	Shims            *Shims
	KubernetesClient client.KubernetesClient
	ConfigHandler    config.ConfigHandler
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

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetContextFunc = func() string {
		return "test-context"
	}
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if key == "id" {
			return "test-context-id"
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}

	mocks := &KubernetesTestMocks{
		Shims:            setupDefaultShims(),
		KubernetesClient: kubernetesClient,
		ConfigHandler:    mockConfigHandler,
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

// fakeClock gives a real-time polling loop a deterministic, instantly-advancing Now/Sleep pair,
// so tests asserting on elapsed time don't depend on actual CI scheduler timing.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) Sleep(d time.Duration) {
	c.now = c.now.Add(d)
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Now()}
}

// withGVRs resolves a kind to its plural resource, as a RESTMapper does. Without it every lookup
// collapses onto one empty GVR and a mock cannot tell the layers apart.
func withGVRs(c *client.MockKubernetesClient) *client.MockKubernetesClient {
	c.ResourceForFunc = func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
		if gvk.Kind == "" {
			return schema.GroupVersionResource{}, nil
		}
		return schema.GroupVersionResource{Group: gvk.Group, Version: "v1", Resource: strings.ToLower(gvk.Kind) + "s"}, nil
	}
	return c
}

// blockingObject is a live object a destroy must wait on: something still holds a finalizer.
func blockingObject() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"finalizers": []any{"example.com/cleanup"}},
	}}
}

// kustomizationListItem builds an unstructured Kustomization item for a mocked ListResources
// response, carrying one Ready condition with the given status, reason, and message.
func kustomizationListItem(name, status, reason, message string) unstructured.Unstructured {
	condition := map[string]any{"type": "Ready", "status": status}
	if reason != "" {
		condition["reason"] = reason
	}
	if message != "" {
		condition["message"] = message
	}
	return unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": name},
			"status": map[string]any{
				"conditions": []any{condition},
			},
		},
	}
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

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseKubernetesManager_CreateNamespace(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
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
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
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

func TestBaseKubernetesManager_NamespaceExists(t *testing.T) {
	t.Run("ReturnsTrueWhenExists", func(t *testing.T) {
		// Given a kubernetes manager whose client returns the namespace successfully
		m := setupKubernetesMocks(t)
		m.KubernetesClient.(*client.MockKubernetesClient).GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		}
		manager := NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

		// When NamespaceExists is called
		exists, err := manager.NamespaceExists("system-lb")

		// Then it returns true with no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !exists {
			t.Error("expected exists=true, got false")
		}
	})

	t.Run("ReturnsFalseWhenNotFound", func(t *testing.T) {
		// Given a kubernetes manager whose client returns a not-found error
		m := setupKubernetesMocks(t)
		m.KubernetesClient.(*client.MockKubernetesClient).GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("%q not found", name)
		}
		manager := NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

		// When NamespaceExists is called
		exists, err := manager.NamespaceExists("system-lb")

		// Then it returns false with no error
		if err != nil {
			t.Fatalf("expected no error for not-found, got %v", err)
		}
		if exists {
			t.Error("expected exists=false, got true")
		}
	})

	t.Run("ReturnsErrorOnOtherAPIError", func(t *testing.T) {
		// Given a kubernetes manager whose client returns an unexpected API error
		m := setupKubernetesMocks(t)
		m.KubernetesClient.(*client.MockKubernetesClient).GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("connection refused")
		}
		manager := NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

		// When NamespaceExists is called
		exists, err := manager.NamespaceExists("system-lb")

		// Then it returns the error and false
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if exists {
			t.Error("expected exists=false on error, got true")
		}
	})
}
