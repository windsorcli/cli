package kubernetes

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"github.com/windsorcli/cli/pkg/runtime/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

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

// progressRecorder records presenter events for assertions on operator-facing progress.
type progressRecorder struct {
	mu     sync.Mutex
	events []presenter.Event
}

func (r *progressRecorder) Emit(_ context.Context, e presenter.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

// started returns the messages of the operations that started.
func (r *progressRecorder) started() []string {
	return r.messages(presenter.KindApplying)
}

// progress returns the progress messages.
func (r *progressRecorder) progress() []string {
	return r.messages(presenter.KindProgress)
}

// warnings returns the warn-level narrative lines.
func (r *progressRecorder) warnings() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		if e.Kind == presenter.KindMessage && e.Level == slog.LevelWarn {
			out = append(out, e.Message)
		}
	}
	return out
}

func (r *progressRecorder) messages(kind presenter.Kind) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		if e.Kind == kind {
			out = append(out, e.Message)
		}
	}
	return out
}

// recordProgress returns a context inside an open operation whose presenter records every event.
func recordProgress(t *testing.T) (context.Context, *progressRecorder) {
	t.Helper()
	rec := &progressRecorder{}
	ctx, end := presenter.Begin(presenter.NewContext(context.Background(), rec), "test", "test operation")
	t.Cleanup(func() { end(nil) })
	return ctx, rec
}
