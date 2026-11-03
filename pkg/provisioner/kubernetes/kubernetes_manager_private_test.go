package kubernetes

import (
	"fmt"
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
