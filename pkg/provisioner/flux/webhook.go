// Package flux provides Flux kustomization stack management functionality.
// The Notifier piece implements best-effort flux webhook notification. After
// a successful apply/bootstrap the Notifier discovers the cluster's flux
// Receiver, opens a port-forward to the webhook-receiver Service, and POSTs
// a payload so flux reconciles the blueprint's sources immediately instead
// of waiting for the next scheduled interval.

package flux

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"github.com/windsorcli/cli/pkg/runtime"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

// =============================================================================
// Constants
// =============================================================================

const (
	// defaultWebhookServiceName is the conventional Service that fronts the
	// flux notification-controller's webhook-receiver pod.
	defaultWebhookServiceName = "webhook-receiver"

	// defaultWebhookServicePort is the Service port exposed by webhook-receiver.
	defaultWebhookServicePort = 80

	// notifyTimeout bounds the entire discover + port-forward + POST attempt.
	notifyTimeout = 10 * time.Second

	// postTimeout bounds a single HTTP POST to the forwarded receiver.
	postTimeout = 5 * time.Second
)

// =============================================================================
// Interface
// =============================================================================

// Notifier attempts to trigger a flux reconcile by POSTing to a cluster-local
// flux Receiver. Implementations are best-effort: Notify returns nil whenever
// the cluster has no receiver configured, the receiver is not Ready, the
// receiver type is unsupported, or the POST fails. Hard errors are only
// returned for programming mistakes (nil inputs), never for cluster state.
type Notifier interface {
	Notify(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) error
}

// =============================================================================
// Types
// =============================================================================

// NotifierShims provides mockable wrappers around I/O the Notifier performs so
// unit tests can replace the real HTTP client and port-forward primitive with
// in-memory fakes that do not require a live cluster. The PortForward default
// is wired from port_forward.go so this file stays focused on orchestration.
// The stop func returned by PortForward blocks until the forward is fully torn
// down and surfaces any error observed after readyCh fired, so Notify can log
// late forwarder failures that would otherwise be swallowed.
type NotifierShims struct {
	HTTPDo      func(req *http.Request) (*http.Response, error)
	PortForward func(ctx context.Context, cfg *rest.Config, namespace, pod string, remotePort int) (localURL string, stop func() error, err error)
}

// BaseNotifier implements Notifier against a real Kubernetes cluster via the
// provided KubernetesClient. It is goroutine-safe only in that each Notify
// call constructs its own port-forward and http request; callers should not
// share a single invocation across goroutines.
type BaseNotifier struct {
	runtime    *runtime.Runtime
	kubeClient client.KubernetesClient
	shims      *NotifierShims
	logWriter  io.Writer
}

// webhookTarget captures the subset of a flux Receiver needed to POST to it.
// The port field is 0 when no windsorcli.dev/webhook-port annotation is set,
// signalling to Notify that the port should come from the backing Service
// (with a final fallback to defaultWebhookServicePort).
type webhookTarget struct {
	receiverName string
	receiverType string
	webhookPath  string
	token        []byte
	serviceName  string
	port         int
}

// =============================================================================
// Constructor
// =============================================================================

// NewNotifierShims builds NotifierShims with production implementations.
// The default HTTP client enforces postTimeout; the default port-forward
// opens a real SPDY connection through the provided rest.Config.
func NewNotifierShims() *NotifierShims {
	return &NotifierShims{
		HTTPDo:      (&http.Client{Timeout: postTimeout}).Do,
		PortForward: productionPortForward,
	}
}

// NewNotifier creates a BaseNotifier. Panics if runtime or kubeClient are nil.
// Accepts an optional override struct for injecting test shims and log writer;
// matches the opts-pattern used by NewStack.
func NewNotifier(rt *runtime.Runtime, kubeClient client.KubernetesClient, opts ...*BaseNotifier) Notifier {
	if rt == nil {
		panic("runtime is required")
	}
	if kubeClient == nil {
		panic("kubernetes client is required")
	}

	n := &BaseNotifier{
		runtime:    rt,
		kubeClient: kubeClient,
		shims:      NewNotifierShims(),
	}

	if len(opts) > 0 && opts[0] != nil {
		if opts[0].shims != nil {
			n.shims = opts[0].shims
		}
		if opts[0].logWriter != nil {
			n.logWriter = opts[0].logWriter
		}
	}

	return n
}

// =============================================================================
// Public Methods
// =============================================================================

// Notify discovers the cluster's flux Receiver, opens a port-forward to the
// backing Service, and POSTs a payload that triggers flux to reconcile the
// blueprint's sources. The blueprint parameter is used both to match the
// receiver's spec.resources against the blueprint's GitRepository (when
// multiple receivers exist) and to feed per-type payloads. Returns nil in
// every non-programmer-error case: no receiver (the common case for clusters
// without the gitops stack), receiver not Ready, no backing pod, port-forward
// failure, POST failure, unsupported receiver type. Warnings are written to
// the notifier's log writer so the user can see why a notification was
// skipped without bootstrap aborting.
func (n *BaseNotifier) Notify(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	ctx, cancel := context.WithTimeout(ctx, notifyTimeout)
	defer cancel()

	namespace := n.runtime.ConfigHandler.GetString("gitops.namespace", constants.DefaultGitopsNamespace)

	target, err := n.discoverReceiver(namespace, blueprint)
	if err != nil {
		n.logf("flux webhook notification skipped: %v", err)
		return nil
	}
	if target == nil {
		return nil
	}

	pod, svcPort, err := n.resolvePodForService(namespace, target.serviceName)
	if err != nil {
		n.logf("flux webhook notification skipped: %v", err)
		return nil
	}

	cfg, err := n.kubeClient.RESTConfig()
	if err != nil {
		n.logf("flux webhook notification skipped: rest config unavailable: %v", err)
		return nil
	}

	port := resolveWebhookPort(target.port, svcPort)
	localURL, stop, err := n.shims.PortForward(ctx, cfg, namespace, pod, port)
	if err != nil {
		n.logf("flux webhook notification skipped: port-forward failed: %v", err)
		return nil
	}
	defer func() {
		if err := stop(); err != nil {
			n.logf("flux port-forward exited with error: %v", err)
		}
	}()

	if err := n.post(ctx, localURL, target); err != nil {
		n.logf("flux webhook notification skipped: %v", err)
		return nil
	}

	n.logf("flux webhook notified (%s/%s)", namespace, target.receiverName)
	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// post sends the HTTP POST to the port-forwarded webhook endpoint using the
// signature scheme that matches the receiver's type. An unsupported type
// returns an error which Notify converts into a warning.
func (n *BaseNotifier) post(ctx context.Context, localURL string, t *webhookTarget) error {
	endpoint, err := url.JoinPath(localURL, t.webhookPath)
	if err != nil {
		return fmt.Errorf("build endpoint: %w", err)
	}

	body, headers, err := n.buildPayload(t)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := n.shims.HTTPDo(req)
	if err != nil {
		return fmt.Errorf("post webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}

// buildPayload returns the request body and headers appropriate for the
// receiver's type. The supported set matches what the core/kustomize blueprint
// templates actually deploy; anything else falls through to an error so Notify
// can log "unsupported receiver type" and move on.
func (n *BaseNotifier) buildPayload(t *webhookTarget) ([]byte, map[string]string, error) {
	headers := map[string]string{}
	switch t.receiverType {
	case "generic":
		return []byte("{}"), headers, nil
	case "generic-hmac":
		body := []byte("{}")
		mac := hmac.New(sha256.New, t.token)
		mac.Write(body)
		headers["X-Signature"] = "sha256=" + hex.EncodeToString(mac.Sum(nil))
		return body, headers, nil
	case "github":
		body, err := json.Marshal(map[string]any{"ref": "refs/heads/main"})
		if err != nil {
			return nil, nil, fmt.Errorf("marshal github payload: %w", err)
		}
		mac := hmac.New(sha256.New, t.token)
		mac.Write(body)
		headers["X-Hub-Signature-256"] = "sha256=" + hex.EncodeToString(mac.Sum(nil))
		headers["X-GitHub-Event"] = "push"
		return body, headers, nil
	case "gitlab":
		body, err := json.Marshal(map[string]any{"object_kind": "push"})
		if err != nil {
			return nil, nil, fmt.Errorf("marshal gitlab payload: %w", err)
		}
		headers["X-Gitlab-Token"] = string(t.token)
		headers["X-Gitlab-Event"] = "Push Hook"
		return body, headers, nil
	default:
		return nil, nil, fmt.Errorf("unsupported receiver type %q", t.receiverType)
	}
}

// logf writes a warning to the notifier's log writer if one is configured.
// Using a no-op default keeps Notify quiet in contexts that haven't wired
// up an explicit writer.
func (n *BaseNotifier) logf(format string, args ...any) {
	if n.logWriter == nil {
		return
	}
	fmt.Fprintf(n.logWriter, format+"\n", args...)
}

// =============================================================================
// Helpers
// =============================================================================

// receiverGVR returns the GroupVersionResource used to list flux Receivers.
func receiverGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "notification.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "receivers",
	}
}

// serviceGVR returns the GroupVersionResource used to read Services.
func serviceGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "services",
	}
}

// podGVR returns the GroupVersionResource used to list Pods.
func podGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}
}

// secretGVR returns the GroupVersionResource used to read Secrets.
func secretGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "secrets",
	}
}

// unstructuredNestedString returns a nested string or "" when missing, allowing
// call sites to stay readable when they care about presence, not error detail.
func unstructuredNestedString(obj *unstructured.Unstructured, fields ...string) string {
	v, _, _ := unstructured.NestedString(obj.Object, fields...)
	return v
}

// resolveWebhookPort picks the effective port for the webhook POST. Priority:
// an explicit windsorcli.dev/webhook-port annotation (annotated), otherwise
// the Service's declared port (service), otherwise the convention default.
// Any non-positive value is treated as "unset" so downstream callers never
// receive a zero or negative port.
func resolveWebhookPort(annotated, service int) int {
	if annotated > 0 {
		return annotated
	}
	if service > 0 {
		return service
	}
	return defaultWebhookServicePort
}
