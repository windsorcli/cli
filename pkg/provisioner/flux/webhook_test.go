package flux

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

// =============================================================================
// Test Setup
// =============================================================================

// notifierMocks bundles everything a webhook test needs to drive the notifier
// end-to-end without touching a real cluster or real HTTP.
type notifierMocks struct {
	runtime    *runtime.Runtime
	kubeClient *client.MockKubernetesClient
	shims      *NotifierShims
	logs       *bytes.Buffer

	// requestCapture lets assertions inspect the POST the Notifier sent.
	requestCapture *capturedRequest

	// portCapture records the remote port passed to the PortForward shim so
	// tests can assert which port the notifier forwarded to.
	portCapture *int
}

type capturedRequest struct {
	mu     sync.Mutex
	called bool
	method string
	url    string
	header http.Header
	body   []byte
}

func (c *capturedRequest) record(req *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.called = true
	c.method = req.Method
	c.url = req.URL.String()
	c.header = req.Header.Clone()
	if req.Body != nil {
		c.body, _ = io.ReadAll(req.Body)
	}
}

// setupNotifierMocks returns a mock set with sane defaults: a generic receiver
// named "flux-webhook" with token "abcdef", a webhook-receiver Service, a
// Ready pod behind it, and a port-forward shim that returns a fixed localhost
// URL. Individual tests override whichever mock they need.
func setupNotifierMocks(t *testing.T) *notifierMocks {
	t.Helper()

	mockShell := shell.NewMockShell()
	cfg := config.NewMockConfigHandler()
	cfg.GetStringFunc = func(key string, defaultValue ...string) string {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}

	rt := &runtime.Runtime{
		Shell:         mockShell,
		ConfigHandler: cfg,
		ProjectRoot:   t.TempDir(),
	}

	kc := client.NewMockKubernetesClient()
	kc.RESTConfigFunc = func() (*rest.Config, error) {
		return &rest.Config{}, nil
	}

	// Default lookups that match the fixtures below. Any test can replace
	// ListResourcesFunc or GetResourceFunc to simulate edge cases.
	kc.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
		switch gvr.Resource {
		case "receivers":
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				*makeReceiver("flux-webhook", "generic", "/hook/abc", "webhook-token", true, []gitRepoRef{{"local"}}),
			}}, nil
		case "pods":
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				*makePod("webhook-receiver-abc", map[string]string{"app": "notification-controller"}, true),
			}}, nil
		}
		return &unstructured.UnstructuredList{}, nil
	}
	kc.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
		switch gvr.Resource {
		case "secrets":
			return makeSecret(name, map[string]string{"token": "abcdef"}), nil
		case "services":
			return makeService(name, map[string]string{"app": "notification-controller"}), nil
		}
		return nil, fmt.Errorf("unexpected get %s/%s", gvr.Resource, name)
	}

	capture := &capturedRequest{}
	portCapture := new(int)
	shims := &NotifierShims{
		HTTPDo: func(req *http.Request) (*http.Response, error) {
			capture.record(req)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader([]byte("ok"))),
			}, nil
		},
		PortForward: func(_ context.Context, _ *rest.Config, ns, pod string, remotePort int) (string, func() error, error) {
			*portCapture = remotePort
			return "http://127.0.0.1:54321", func() error { return nil }, nil
		},
	}

	return &notifierMocks{
		runtime:        rt,
		kubeClient:     kc,
		shims:          shims,
		logs:           &bytes.Buffer{},
		requestCapture: capture,
		portCapture:    portCapture,
	}
}

// newTestNotifier constructs a BaseNotifier pre-wired with the given mocks'
// shims and a buffered log writer so tests can assert on warnings.
func newTestNotifier(m *notifierMocks) *BaseNotifier {
	return &BaseNotifier{
		runtime:    m.runtime,
		kubeClient: m.kubeClient,
		shims:      m.shims,
		logWriter:  m.logs,
	}
}

func testNotifyBlueprint() *blueprintv1alpha1.Blueprint {
	return &blueprintv1alpha1.Blueprint{
		Metadata: blueprintv1alpha1.Metadata{Name: "local"},
	}
}

type gitRepoRef struct{ name string }

// sourceRef describes a flux source entry on a Receiver's spec.resources with
// an explicit kind, used by tests that exercise OCIRepository and Bucket
// preference matching.
type sourceRef struct {
	name string
	kind string
}

func makeReceiver(name, receiverType, webhookPath, secretName string, ready bool, resources []gitRepoRef) *unstructured.Unstructured {
	resourceList := make([]any, 0, len(resources))
	for _, r := range resources {
		resourceList = append(resourceList, map[string]any{
			"apiVersion": "source.toolkit.fluxcd.io/v1",
			"kind":       "GitRepository",
			"name":       r.name,
		})
	}
	readyStatus := "False"
	if ready {
		readyStatus = "True"
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "notification.toolkit.fluxcd.io/v1",
		"kind":       "Receiver",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "system-gitops",
		},
		"spec": map[string]any{
			"type":      receiverType,
			"resources": resourceList,
			"secretRef": map[string]any{"name": secretName},
		},
		"status": map[string]any{
			"webhookPath": webhookPath,
			"conditions": []any{
				map[string]any{"type": "Ready", "status": readyStatus},
			},
		},
	}}
}

func makeReceiverWithKinds(name, receiverType, webhookPath, secretName string, ready bool, resources []sourceRef) *unstructured.Unstructured {
	resourceList := make([]any, 0, len(resources))
	for _, r := range resources {
		resourceList = append(resourceList, map[string]any{
			"apiVersion": "source.toolkit.fluxcd.io/v1",
			"kind":       r.kind,
			"name":       r.name,
		})
	}
	readyStatus := "False"
	if ready {
		readyStatus = "True"
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "notification.toolkit.fluxcd.io/v1",
		"kind":       "Receiver",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "system-gitops",
		},
		"spec": map[string]any{
			"type":      receiverType,
			"resources": resourceList,
			"secretRef": map[string]any{"name": secretName},
		},
		"status": map[string]any{
			"webhookPath": webhookPath,
			"conditions": []any{
				map[string]any{"type": "Ready", "status": readyStatus},
			},
		},
	}}
}

func makeSecret(name string, rawData map[string]string) *unstructured.Unstructured {
	encoded := map[string]any{}
	for k, v := range rawData {
		encoded[k] = base64.StdEncoding.EncodeToString([]byte(v))
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]any{"name": name, "namespace": "system-gitops"},
		"data":       encoded,
	}}
}

func makeService(name string, selector map[string]string, ports ...int) *unstructured.Unstructured {
	sel := map[string]any{}
	for k, v := range selector {
		sel[k] = v
	}
	spec := map[string]any{"selector": sel}
	if len(ports) > 0 {
		portList := make([]any, 0, len(ports))
		for _, p := range ports {
			portList = append(portList, map[string]any{"port": int64(p)})
		}
		spec["ports"] = portList
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": name, "namespace": "system-gitops"},
		"spec":       spec,
	}}
}

func makePod(name string, labels map[string]string, ready bool) *unstructured.Unstructured {
	labelMap := map[string]any{}
	for k, v := range labels {
		labelMap[k] = v
	}
	readyStatus := "False"
	if ready {
		readyStatus = "True"
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "system-gitops",
			"labels":    labelMap,
		},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Ready", "status": readyStatus},
			},
		},
	}}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewNotifier(t *testing.T) {
	t.Run("PanicsOnNilRuntime", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for nil runtime")
			}
		}()
		NewNotifier(nil, client.NewMockKubernetesClient())
	})

	t.Run("PanicsOnNilKubeClient", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for nil kube client")
			}
		}()
		NewNotifier(&runtime.Runtime{}, nil)
	})

	t.Run("ReturnsNonNilNotifier", func(t *testing.T) {
		n := NewNotifier(&runtime.Runtime{}, client.NewMockKubernetesClient())
		if n == nil {
			t.Error("expected non-nil notifier")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestNotifier_Notify(t *testing.T) {
	ctx := context.Background()

	t.Run("NilBlueprintReturnsError", func(t *testing.T) {
		// Given a notifier with default mocks
		m := setupNotifierMocks(t)
		n := newTestNotifier(m)

		// When Notify is called with a nil blueprint
		err := n.Notify(ctx, nil)

		// Then a programmer error is returned — this is the only non-nil return
		if err == nil {
			t.Fatal("expected error for nil blueprint")
		}
	})

	t.Run("NoReceiversSilentNoOp", func(t *testing.T) {
		// Given a cluster with no receivers listed
		m := setupNotifierMocks(t)
		m.kubeClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{}, nil
		}
		n := newTestNotifier(m)

		// When Notify is called
		err := n.Notify(ctx, testNotifyBlueprint())

		// Then nil is returned, no request is made, and no warning is logged
		if err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
		if m.requestCapture.called {
			t.Error("expected no HTTP call when receivers absent")
		}
		if m.logs.Len() != 0 {
			t.Errorf("expected no log output, got %q", m.logs.String())
		}
	})

	t.Run("ReceiverNotReadyWarnsAndReturnsNil", func(t *testing.T) {
		// Given a receiver that exists but is not Ready
		m := setupNotifierMocks(t)
		m.kubeClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			if gvr.Resource == "receivers" {
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					*makeReceiver("flux-webhook", "generic", "/hook/abc", "webhook-token", false, nil),
				}}, nil
			}
			return &unstructured.UnstructuredList{}, nil
		}
		n := newTestNotifier(m)

		// When Notify is called
		err := n.Notify(ctx, testNotifyBlueprint())

		// Then nil is returned and a warning is logged
		if err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
		if !strings.Contains(m.logs.String(), "no ready flux receiver") {
			t.Errorf("expected 'no ready flux receiver' in logs, got %q", m.logs.String())
		}
		if m.requestCapture.called {
			t.Error("expected no HTTP call when receiver not Ready")
		}
	})

	t.Run("GenericReceiverPostsEmptyJSON", func(t *testing.T) {
		// Given the default (generic) receiver setup
		m := setupNotifierMocks(t)
		n := newTestNotifier(m)

		// When Notify is called
		if err := n.Notify(ctx, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then a POST is made to localhost+webhookPath with empty JSON body
		if !m.requestCapture.called {
			t.Fatal("expected HTTP POST to happen")
		}
		if m.requestCapture.method != http.MethodPost {
			t.Errorf("expected POST, got %s", m.requestCapture.method)
		}
		if !strings.HasSuffix(m.requestCapture.url, "/hook/abc") {
			t.Errorf("expected URL to end with webhookPath, got %s", m.requestCapture.url)
		}
		if string(m.requestCapture.body) != "{}" {
			t.Errorf("expected body {}, got %q", string(m.requestCapture.body))
		}
		// And no signature header for generic
		if sig := m.requestCapture.header.Get("X-Signature"); sig != "" {
			t.Errorf("expected no X-Signature for generic, got %q", sig)
		}
	})

	t.Run("GenericHmacReceiverSignsBody", func(t *testing.T) {
		// Given a generic-hmac receiver with a known token
		m := setupNotifierMocks(t)
		m.kubeClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			switch gvr.Resource {
			case "receivers":
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					*makeReceiver("flux-webhook", "generic-hmac", "/hook/xyz", "webhook-token", true, nil),
				}}, nil
			case "pods":
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					*makePod("webhook-receiver-xyz", map[string]string{"app": "notification-controller"}, true),
				}}, nil
			}
			return &unstructured.UnstructuredList{}, nil
		}
		n := newTestNotifier(m)

		// When Notify is called
		if err := n.Notify(ctx, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then X-Signature holds the expected HMAC of the body
		mac := hmac.New(sha256.New, []byte("abcdef"))
		mac.Write([]byte("{}"))
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if got := m.requestCapture.header.Get("X-Signature"); got != expected {
			t.Errorf("expected X-Signature=%s, got %s", expected, got)
		}
	})

	t.Run("UnsupportedReceiverTypeWarnsAndReturnsNil", func(t *testing.T) {
		// Given a receiver of a type the notifier does not implement
		m := setupNotifierMocks(t)
		m.kubeClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			switch gvr.Resource {
			case "receivers":
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					*makeReceiver("flux-webhook", "bitbucket", "/hook/abc", "webhook-token", true, nil),
				}}, nil
			case "pods":
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					*makePod("wr", map[string]string{"app": "notification-controller"}, true),
				}}, nil
			}
			return &unstructured.UnstructuredList{}, nil
		}
		n := newTestNotifier(m)

		// When Notify is called
		if err := n.Notify(ctx, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then the warning explains the unsupported type
		if !strings.Contains(m.logs.String(), "unsupported receiver type") {
			t.Errorf("expected unsupported-type warning, got %q", m.logs.String())
		}
	})

	t.Run("Non2xxResponseWarnsAndReturnsNil", func(t *testing.T) {
		// Given a webhook receiver that returns 500 Internal Server Error
		m := setupNotifierMocks(t)
		m.shims.HTTPDo = func(req *http.Request) (*http.Response, error) {
			m.requestCapture.record(req)
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewReader([]byte("boom"))),
			}, nil
		}
		n := newTestNotifier(m)

		// When Notify is called
		if err := n.Notify(ctx, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then the warning captures the non-2xx status and response snippet
		if !strings.Contains(m.logs.String(), "500") {
			t.Errorf("expected status code in warning, got %q", m.logs.String())
		}
	})

	t.Run("PortForwardFailureWarnsAndReturnsNil", func(t *testing.T) {
		// Given a port-forward shim that always errors
		m := setupNotifierMocks(t)
		m.shims.PortForward = func(_ context.Context, _ *rest.Config, _, _ string, _ int) (string, func() error, error) {
			return "", nil, fmt.Errorf("connection refused")
		}
		n := newTestNotifier(m)

		// When Notify is called
		if err := n.Notify(ctx, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then the warning explains the forward failed, and no POST happens
		if !strings.Contains(m.logs.String(), "port-forward failed") {
			t.Errorf("expected port-forward warning, got %q", m.logs.String())
		}
		if m.requestCapture.called {
			t.Error("expected no HTTP call when port-forward fails")
		}
	})

	t.Run("PrefersReceiverMatchingBlueprintGitRepo", func(t *testing.T) {
		// Given two Ready receivers where only the second references the
		// blueprint's GitRepository
		m := setupNotifierMocks(t)
		m.kubeClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			switch gvr.Resource {
			case "receivers":
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					*makeReceiver("other-hook", "generic", "/hook/other", "webhook-token", true, []gitRepoRef{{"something-else"}}),
					*makeReceiver("my-hook", "generic", "/hook/mine", "webhook-token", true, []gitRepoRef{{"local"}}),
				}}, nil
			case "pods":
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					*makePod("wr", map[string]string{"app": "notification-controller"}, true),
				}}, nil
			}
			return &unstructured.UnstructuredList{}, nil
		}
		n := newTestNotifier(m)

		// When Notify runs against blueprint Metadata.Name="local"
		if err := n.Notify(ctx, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then the POST targets the blueprint-matching receiver's webhookPath
		if !strings.HasSuffix(m.requestCapture.url, "/hook/mine") {
			t.Errorf("expected POST to /hook/mine, got %s", m.requestCapture.url)
		}
	})

	t.Run("NoPodBehindServiceWarnsAndReturnsNil", func(t *testing.T) {
		// Given a Ready receiver but no pod backing the webhook service
		m := setupNotifierMocks(t)
		m.kubeClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			if gvr.Resource == "receivers" {
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					*makeReceiver("flux-webhook", "generic", "/hook/abc", "webhook-token", true, nil),
				}}, nil
			}
			return &unstructured.UnstructuredList{}, nil
		}
		n := newTestNotifier(m)

		// When Notify is called
		if err := n.Notify(ctx, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then a warning is emitted and no POST happens
		if !strings.Contains(m.logs.String(), "no ready pod") {
			t.Errorf("expected no-ready-pod warning, got %q", m.logs.String())
		}
		if m.requestCapture.called {
			t.Error("expected no HTTP call when no pod is Ready")
		}
	})

	t.Run("SecretMissingTokenWarnsAndReturnsNil", func(t *testing.T) {
		// Given a Ready receiver whose secret is missing the token key
		m := setupNotifierMocks(t)
		m.kubeClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			switch gvr.Resource {
			case "secrets":
				return makeSecret(name, map[string]string{}), nil
			case "services":
				return makeService(name, map[string]string{"app": "notification-controller"}), nil
			}
			return nil, fmt.Errorf("unexpected get %s/%s", gvr.Resource, name)
		}
		n := newTestNotifier(m)

		// When Notify is called
		if err := n.Notify(ctx, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then a warning about the missing token is emitted
		if !strings.Contains(m.logs.String(), "missing 'token'") {
			t.Errorf("expected missing-token warning, got %q", m.logs.String())
		}
	})

	t.Run("PortComesFromServiceSpec", func(t *testing.T) {
		// Given a Service that declares port 9090 (non-default)
		m := setupNotifierMocks(t)
		m.kubeClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			switch gvr.Resource {
			case "secrets":
				return makeSecret(name, map[string]string{"token": "abcdef"}), nil
			case "services":
				return makeService(name, map[string]string{"app": "notification-controller"}, 9090), nil
			}
			return nil, fmt.Errorf("unexpected get %s/%s", gvr.Resource, name)
		}
		n := newTestNotifier(m)

		// When Notify is called
		if err := n.Notify(ctx, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then the port-forward targets the Service's declared port
		if *m.portCapture != 9090 {
			t.Errorf("expected port-forward to 9090, got %d", *m.portCapture)
		}
	})

	t.Run("PortDefaultsTo80WhenServiceHasNone", func(t *testing.T) {
		// Given a Service with no spec.ports (unusual, but possible)
		m := setupNotifierMocks(t)
		// Default service fixture has no ports — nothing to change.
		n := newTestNotifier(m)

		// When Notify is called
		if err := n.Notify(ctx, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then the port-forward falls back to the flux-convention default (80)
		if *m.portCapture != defaultWebhookServicePort {
			t.Errorf("expected port-forward to %d, got %d", defaultWebhookServicePort, *m.portCapture)
		}
	})

	t.Run("PortOverrideFromAnnotation", func(t *testing.T) {
		// Given a Receiver with an explicit port-override annotation that
		// disagrees with the Service's declared port
		m := setupNotifierMocks(t)
		receiver := makeReceiver("flux-webhook", "generic", "/hook/abc", "webhook-token", true, []gitRepoRef{{"local"}})
		receiver.SetAnnotations(map[string]string{"windsorcli.dev/webhook-port": "9999"})
		m.kubeClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			switch gvr.Resource {
			case "receivers":
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{*receiver}}, nil
			case "pods":
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					*makePod("wr", map[string]string{"app": "notification-controller"}, true),
				}}, nil
			}
			return &unstructured.UnstructuredList{}, nil
		}
		m.kubeClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			switch gvr.Resource {
			case "secrets":
				return makeSecret(name, map[string]string{"token": "abcdef"}), nil
			case "services":
				return makeService(name, map[string]string{"app": "notification-controller"}, 9090), nil
			}
			return nil, fmt.Errorf("unexpected get %s/%s", gvr.Resource, name)
		}
		n := newTestNotifier(m)

		// When Notify is called
		if err := n.Notify(ctx, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then the annotation wins over the Service's declared port
		if *m.portCapture != 9999 {
			t.Errorf("expected annotation port 9999 to win, got %d", *m.portCapture)
		}
	})

	t.Run("PrefersReceiverMatchingBlueprintOCIRepository", func(t *testing.T) {
		// Given two Ready receivers where only the second references the
		// blueprint's source as an OCIRepository
		m := setupNotifierMocks(t)
		m.kubeClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			switch gvr.Resource {
			case "receivers":
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					*makeReceiverWithKinds("other-hook", "generic", "/hook/other", "webhook-token", true, []sourceRef{{"something-else", "GitRepository"}}),
					*makeReceiverWithKinds("my-hook", "generic", "/hook/mine", "webhook-token", true, []sourceRef{{"local", "OCIRepository"}}),
				}}, nil
			case "pods":
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					*makePod("wr", map[string]string{"app": "notification-controller"}, true),
				}}, nil
			}
			return &unstructured.UnstructuredList{}, nil
		}
		n := newTestNotifier(m)

		// When Notify runs against blueprint Metadata.Name="local"
		if err := n.Notify(ctx, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then the POST targets the OCI-matching receiver
		if !strings.HasSuffix(m.requestCapture.url, "/hook/mine") {
			t.Errorf("expected POST to /hook/mine, got %s", m.requestCapture.url)
		}
	})

	t.Run("PrefersReceiverMatchingBlueprintBucket", func(t *testing.T) {
		// Given two Ready receivers where only the second references the
		// blueprint's source as a Bucket
		m := setupNotifierMocks(t)
		m.kubeClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			switch gvr.Resource {
			case "receivers":
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					*makeReceiverWithKinds("other-hook", "generic", "/hook/other", "webhook-token", true, []sourceRef{{"something-else", "GitRepository"}}),
					*makeReceiverWithKinds("my-hook", "generic", "/hook/mine", "webhook-token", true, []sourceRef{{"local", "Bucket"}}),
				}}, nil
			case "pods":
				return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
					*makePod("wr", map[string]string{"app": "notification-controller"}, true),
				}}, nil
			}
			return &unstructured.UnstructuredList{}, nil
		}
		n := newTestNotifier(m)

		// When Notify runs against blueprint Metadata.Name="local"
		if err := n.Notify(ctx, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then the POST targets the Bucket-matching receiver
		if !strings.HasSuffix(m.requestCapture.url, "/hook/mine") {
			t.Errorf("expected POST to /hook/mine, got %s", m.requestCapture.url)
		}
	})
}

