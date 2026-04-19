// Package flux provides Flux kustomization stack management functionality.
// This file owns the cluster-read helpers the Notifier uses to locate a
// flux Receiver, its backing Service pod, and the authentication secret
// that signs the webhook payload.

package flux

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// =============================================================================
// Private Methods
// =============================================================================

// discoverReceiver selects the best-matching Ready Receiver in the given namespace.
// Preference order: (1) a receiver whose spec.resources includes a GitRepository
// named after the blueprint, (2) a receiver named "flux-webhook", (3) any Ready
// receiver. Returns (nil, nil) when no receivers exist — the expected state for
// clusters without the gitops stack. Returns an error only when receivers exist
// but none are Ready or fields cannot be parsed; Notify converts that into a
// warning.
func (n *BaseNotifier) discoverReceiver(namespace string, blueprint *blueprintv1alpha1.Blueprint) (*webhookTarget, error) {
	list, err := n.kubeClient.ListResources(receiverGVR(), namespace)
	if err != nil {
		return nil, fmt.Errorf("list flux receivers: %w", err)
	}
	if list == nil || len(list.Items) == 0 {
		return nil, nil
	}

	blueprintSource := blueprint.Metadata.Name
	var byBlueprint, byConventionalName, firstReady *unstructured.Unstructured

	for i := range list.Items {
		r := &list.Items[i]
		if !unstructuredReadyConditionTrue(r) {
			continue
		}
		if firstReady == nil {
			firstReady = r
		}
		if r.GetName() == "flux-webhook" && byConventionalName == nil {
			byConventionalName = r
		}
		if blueprintSource != "" && receiverResourcesIncludeSource(r, blueprintSource) && byBlueprint == nil {
			byBlueprint = r
		}
	}

	chosen := byBlueprint
	if chosen == nil {
		chosen = byConventionalName
	}
	if chosen == nil {
		chosen = firstReady
	}
	if chosen == nil {
		return nil, fmt.Errorf("no ready flux receiver in namespace %q", namespace)
	}

	return n.buildTargetFromReceiver(chosen, namespace)
}

// buildTargetFromReceiver extracts the webhookPath, receiver type, secret token,
// backing service name, and optional port override. The service name and port
// come from optional annotations (windsorcli.dev/webhook-service and
// windsorcli.dev/webhook-port) that allow non-conventional deployments; when
// the port annotation is absent, a zero value is returned and the caller
// resolves the port from the Service's spec.ports.
func (n *BaseNotifier) buildTargetFromReceiver(r *unstructured.Unstructured, namespace string) (*webhookTarget, error) {
	path := unstructuredNestedString(r, "status", "webhookPath")
	if path == "" {
		return nil, fmt.Errorf("receiver %q has no status.webhookPath", r.GetName())
	}

	rType := unstructuredNestedString(r, "spec", "type")
	if rType == "" {
		return nil, fmt.Errorf("receiver %q has no spec.type", r.GetName())
	}

	secretName := unstructuredNestedString(r, "spec", "secretRef", "name")
	if secretName == "" {
		return nil, fmt.Errorf("receiver %q has no spec.secretRef.name", r.GetName())
	}
	token, err := n.readSecretToken(namespace, secretName)
	if err != nil {
		return nil, err
	}

	annotations := r.GetAnnotations()
	serviceName := defaultWebhookServiceName
	if ann := annotations["windsorcli.dev/webhook-service"]; ann != "" {
		serviceName = ann
	}

	var port int
	if ann := annotations["windsorcli.dev/webhook-port"]; ann != "" {
		if p, err := strconv.Atoi(ann); err == nil && p > 0 {
			port = p
		}
	}

	return &webhookTarget{
		receiverName: r.GetName(),
		receiverType: rType,
		webhookPath:  path,
		token:        token,
		serviceName:  serviceName,
		port:         port,
	}, nil
}

// readSecretToken returns the webhook secret's "token" data, which flux uses to
// derive the receiver's webhookPath and HMAC signatures. Missing key is treated
// as an error because every flux receiver type relies on the token in some way
// (even "generic" receivers are protected by the path which is hashed from it).
// The dynamic client surfaces Secret data values as base64-encoded strings (as
// they appear on the wire), so the token is decoded before being returned.
// Decoded tokens containing CR or LF bytes are rejected up front because the
// gitlab receiver type injects the raw token into an HTTP header value — net/http
// would silently reject such a request deep inside the post() call, which Notify
// then swallows as an opaque warning; rejecting here turns a malformed secret
// (commonly from accidental newlines in operator copy-paste) into a clear error.
func (n *BaseNotifier) readSecretToken(namespace, secretName string) ([]byte, error) {
	obj, err := n.kubeClient.GetResource(secretGVR(), namespace, secretName)
	if err != nil {
		return nil, fmt.Errorf("read secret %q: %w", secretName, err)
	}
	data, found, err := unstructured.NestedMap(obj.Object, "data")
	if err != nil {
		return nil, fmt.Errorf("secret %q data: %w", secretName, err)
	}
	if !found {
		return nil, fmt.Errorf("secret %q has no data", secretName)
	}
	raw, ok := data["token"]
	if !ok {
		return nil, fmt.Errorf("secret %q missing 'token' key", secretName)
	}
	s, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("secret %q token is not a string", secretName)
	}
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("secret %q token decode: %w", secretName, err)
	}
	if bytes.ContainsAny(decoded, "\r\n") {
		return nil, fmt.Errorf("secret %q token contains CR or LF bytes; reissue the token without embedded newlines", secretName)
	}
	return decoded, nil
}

// resolvePodForService returns the name of a Ready Pod backing the given
// Service along with the port the pod actually listens on. Port resolution
// mirrors kubectl's service-to-pod mapping: read spec.ports[0].targetPort,
// resolve a named target against the pod's containers[].ports[].name, fall
// back to spec.ports[0].port if targetPort is absent. Returning the pod's
// listen port (not the Service's external port) is essential because port
// forwarding targets a Pod directly — forwarding to the Service port would
// hit a socket the pod isn't bound to (e.g. flux's webhook-receiver declares
// port 80 but the pod binds 9292 via targetPort "http-webhook"). Returns 0
// for the port when neither targetPort nor port can be resolved so the
// caller can fall back to a convention default. Returns an error when the
// Service has no selector, no Pods match, or no matching Pod is Ready.
func (n *BaseNotifier) resolvePodForService(namespace, serviceName string) (string, int, error) {
	svc, err := n.kubeClient.GetResource(serviceGVR(), namespace, serviceName)
	if err != nil {
		return "", 0, fmt.Errorf("read service %q: %w", serviceName, err)
	}
	selector, found, err := unstructured.NestedStringMap(svc.Object, "spec", "selector")
	if err != nil {
		return "", 0, fmt.Errorf("service %q selector: %w", serviceName, err)
	}
	if !found || len(selector) == 0 {
		return "", 0, fmt.Errorf("service %q has no selector", serviceName)
	}

	namedPort, port := serviceFirstTargetPort(svc)

	pods, err := n.kubeClient.ListResources(podGVR(), namespace)
	if err != nil {
		return "", port, fmt.Errorf("list pods: %w", err)
	}
	for i := range pods.Items {
		p := &pods.Items[i]
		if !podLabelsMatch(p, selector) {
			continue
		}
		if !unstructuredReadyConditionTrue(p) {
			continue
		}
		if namedPort != "" {
			if resolved := resolveNamedContainerPort(p, namedPort); resolved > 0 {
				port = resolved
			}
		}
		return p.GetName(), port, nil
	}
	return "", port, fmt.Errorf("no ready pod backs service %q", serviceName)
}

// =============================================================================
// Helpers
// =============================================================================

// unstructuredReadyConditionTrue returns true when obj has a status.conditions
// entry of type Ready with status True. It works uniformly across Kubernetes
// kinds that follow the standard Conditions convention (Pod, Receiver, most
// flux CRDs) and centralizes the parsing so future additions (observedGeneration
// filters, stale-condition checks) land in one place.
func unstructuredReadyConditionTrue(obj *unstructured.Unstructured) bool {
	conds, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if m["type"] == "Ready" && m["status"] == "True" {
			return true
		}
	}
	return false
}

// receiverResourcesIncludeSource returns true when the receiver's spec.resources
// list includes a flux source (GitRepository, OCIRepository, or Bucket) whose
// name equals want. These are the three source kinds flux's notification
// controller watches; matching any of them lets the preference logic work
// regardless of whether the blueprint pulls from Git, OCI, or object storage.
func receiverResourcesIncludeSource(r *unstructured.Unstructured, want string) bool {
	resources, found, err := unstructured.NestedSlice(r.Object, "spec", "resources")
	if err != nil || !found {
		return false
	}
	for _, res := range resources {
		m, ok := res.(map[string]any)
		if !ok {
			continue
		}
		if m["name"] != want {
			continue
		}
		switch m["kind"] {
		case "GitRepository", "OCIRepository", "Bucket":
			return true
		}
	}
	return false
}

// serviceFirstTargetPort inspects spec.ports[0] and returns the port the
// backing Pod is expected to listen on. The first return value is a named
// targetPort (e.g. "http-webhook") which must be resolved against the pod's
// container ports; the second is a numeric port used when targetPort is
// numeric, numeric-in-string, or absent (Kubernetes defaults targetPort to
// port when unset). Returns ("", 0) when no ports are declared so the caller
// can fall back to a convention default.
func serviceFirstTargetPort(svc *unstructured.Unstructured) (string, int) {
	ports, found, err := unstructured.NestedSlice(svc.Object, "spec", "ports")
	if err != nil || !found || len(ports) == 0 {
		return "", 0
	}
	first, ok := ports[0].(map[string]any)
	if !ok {
		return "", 0
	}
	if tp, present := first["targetPort"]; present {
		switch v := tp.(type) {
		case int64:
			return "", int(v)
		case float64:
			return "", int(v)
		case int:
			return "", v
		case string:
			if n, err := strconv.Atoi(v); err == nil {
				return "", n
			}
			return v, 0
		}
	}
	switch p := first["port"].(type) {
	case int64:
		return "", int(p)
	case float64:
		return "", int(p)
	case int:
		return "", p
	}
	return "", 0
}

// resolveNamedContainerPort walks pod.spec.containers[].ports[] and returns
// the containerPort whose name matches. Returns 0 when no container declares
// the name, letting the caller keep a numeric fallback or trigger the
// convention default. Matching across all containers (rather than only the
// first) mirrors how kubelet resolves named ports for a Service's targetPort.
func resolveNamedContainerPort(pod *unstructured.Unstructured, name string) int {
	containers, found, err := unstructured.NestedSlice(pod.Object, "spec", "containers")
	if err != nil || !found {
		return 0
	}
	for _, c := range containers {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		ports, found, err := unstructured.NestedSlice(m, "ports")
		if err != nil || !found {
			continue
		}
		for _, p := range ports {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if pm["name"] != name {
				continue
			}
			switch cp := pm["containerPort"].(type) {
			case int64:
				return int(cp)
			case float64:
				return int(cp)
			case int:
				return cp
			}
		}
	}
	return 0
}

// podLabelsMatch returns true when every selector key/value is present on the pod.
func podLabelsMatch(p *unstructured.Unstructured, selector map[string]string) bool {
	labels := p.GetLabels()
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

