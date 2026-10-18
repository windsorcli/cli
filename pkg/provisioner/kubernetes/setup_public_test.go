package kubernetes_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"github.com/windsorcli/cli/pkg/runtime/config"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utiljson "k8s.io/apimachinery/pkg/util/json"
)

// =============================================================================
// Test Setup
// =============================================================================

var (
	namespacesGVR      = schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
	configMapsGVR      = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	secretsGVR         = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	kustomizationsGVR  = schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}
	gitRepositoriesGVR = schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}
	ociRepositoriesGVR = schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "ocirepositories"}
	helmReleasesGVR    = schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}
	deploymentsGVR     = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	statefulSetsGVR    = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	daemonSetsGVR      = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
)

// clusterScopedKinds lists the kinds the fake cluster reports as cluster-scoped.
var clusterScopedKinds = map[string]bool{"Namespace": true, "ClusterRole": true, "GatewayClass": true}

// fakeCluster is an in-memory client.KubernetesClient. It stores objects by resource, namespace,
// and name, and records each write. The fail hook returns an error for one operation on one object.
type fakeCluster struct {
	mu      sync.Mutex
	objects map[string]*unstructured.Unstructured
	deletes []string
	patches []string
	fail    func(op string, gvr schema.GroupVersionResource, namespace, name string) error
	health  func(endpoint string) error
	nodes   map[string]bool
}

// newFakeCluster returns an empty fakeCluster that fails no operation and reports a healthy API.
func newFakeCluster() *fakeCluster {
	return &fakeCluster{objects: map[string]*unstructured.Unstructured{}}
}

// fixture holds a manager wired to a fake cluster and a mock config handler.
type fixture struct {
	manager *kubernetes.BaseKubernetesManager
	cluster *fakeCluster
	config  *config.MockConfigHandler
	values  map[string]string
}

// newFixture returns a manager for the context "local" with the id "ctx-1", backed by an empty
// fake cluster. Tests change config values through f.values.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{cluster: newFakeCluster(), config: config.NewMockConfigHandler(), values: map[string]string{"id": "ctx-1"}}
	f.config.GetContextFunc = func() string { return "local" }
	f.config.GetStringFunc = func(key string, defaultValue ...string) string {
		if value, ok := f.values[key]; ok {
			return value
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	f.manager = kubernetes.NewKubernetesManager(f.cluster, f.config)
	return f
}

// objectKey identifies one object in the fake cluster.
func objectKey(gvr schema.GroupVersionResource, namespace, name string) string {
	return gvr.String() + "|" + namespace + "|" + name
}

// seed stores obj in the fake cluster under gvr, after it is normalized to JSON types.
func (c *fakeCluster) seed(gvr schema.GroupVersionResource, obj map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	u := normalize(&unstructured.Unstructured{Object: obj})
	c.objects[objectKey(gvr, u.GetNamespace(), u.GetName())] = u
}

// get returns the stored object, or nil when it is absent.
func (c *fakeCluster) get(gvr schema.GroupVersionResource, namespace, name string) *unstructured.Unstructured {
	c.mu.Lock()
	defer c.mu.Unlock()
	obj := c.objects[objectKey(gvr, namespace, name)]
	if obj == nil {
		return nil
	}
	return obj.DeepCopy()
}

// names returns the sorted names of the stored objects under gvr.
func (c *fakeCluster) names(gvr schema.GroupVersionResource) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var names []string
	for key, obj := range c.objects {
		if strings.HasPrefix(key, gvr.String()+"|") {
			names = append(names, obj.GetName())
		}
	}
	sort.Strings(names)
	return names
}

// normalize returns a copy of obj with every value converted to the type a client decodes it to.
func normalize(obj *unstructured.Unstructured) *unstructured.Unstructured {
	raw, err := json.Marshal(obj.Object)
	if err != nil {
		panic(err)
	}
	out := map[string]any{}
	if err := utiljson.Unmarshal(raw, &out); err != nil {
		panic(err)
	}
	return &unstructured.Unstructured{Object: out}
}

// injected returns the error the fail hook sets for op, or nil.
func (c *fakeCluster) injected(op string, gvr schema.GroupVersionResource, namespace, name string) error {
	if c.fail == nil {
		return nil
	}
	return c.fail(op, gvr, namespace, name)
}

// notFound returns the error an API server returns for a missing object.
func notFound(gvr schema.GroupVersionResource, name string) error {
	return apierrors.NewNotFound(gvr.GroupResource(), name)
}

func (c *fakeCluster) GetResource(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	if err := c.injected("get", gvr, namespace, name); err != nil {
		return nil, err
	}
	obj := c.get(gvr, namespace, name)
	if obj == nil {
		return nil, notFound(gvr, name)
	}
	return obj, nil
}

func (c *fakeCluster) ListResources(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
	return c.ListResourcesByLabel(gvr, namespace, "")
}

// ListResourcesByLabel supports selectors made of "key=value" and "key" terms joined by commas.
func (c *fakeCluster) ListResourcesByLabel(gvr schema.GroupVersionResource, namespace, labelSelector string) (*unstructured.UnstructuredList, error) {
	if err := c.injected("list", gvr, namespace, ""); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := make([]string, 0, len(c.objects))
	for key := range c.objects {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	list := &unstructured.UnstructuredList{}
	for _, key := range keys {
		obj := c.objects[key]
		if !strings.HasPrefix(key, gvr.String()+"|") {
			continue
		}
		if namespace != "" && obj.GetNamespace() != namespace {
			continue
		}
		if !matchesSelector(obj.GetLabels(), labelSelector) {
			continue
		}
		list.Items = append(list.Items, *obj.DeepCopy())
	}
	return list, nil
}

// matchesSelector reports whether labels satisfy every term of selector.
func matchesSelector(labels map[string]string, selector string) bool {
	if selector == "" {
		return true
	}
	for _, term := range strings.Split(selector, ",") {
		key, value, hasValue := strings.Cut(term, "=")
		actual, ok := labels[key]
		if !ok || (hasValue && actual != value) {
			return false
		}
	}
	return true
}

func (c *fakeCluster) ApplyResource(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	if err := c.injected("apply", gvr, obj.GetNamespace(), obj.GetName()); err != nil {
		return nil, err
	}
	stored := normalize(obj)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.objects[objectKey(gvr, stored.GetNamespace(), stored.GetName())] = stored
	return stored.DeepCopy(), nil
}

func (c *fakeCluster) DeleteResource(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
	if err := c.injected("delete", gvr, namespace, name); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := objectKey(gvr, namespace, name)
	if c.objects[key] == nil {
		return notFound(gvr, name)
	}
	delete(c.objects, key)
	c.deletes = append(c.deletes, gvr.Resource+"/"+name)
	return nil
}

// ResourceFor maps a kind to its lowercase plural resource, the way a RESTMapper does.
func (c *fakeCluster) ResourceFor(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	plural := strings.ToLower(gvk.Kind)
	if strings.HasSuffix(plural, "y") {
		plural = strings.TrimSuffix(plural, "y") + "ies"
	} else {
		plural += "s"
	}
	version := gvk.Version
	if version == "" {
		version = "v1"
	}
	return schema.GroupVersionResource{Group: gvk.Group, Version: version, Resource: plural}, nil
}

func (c *fakeCluster) IsNamespaced(gvk schema.GroupVersionKind) (bool, error) {
	return !clusterScopedKinds[gvk.Kind], nil
}

// PatchResource applies a JSON merge patch to a stored object.
func (c *fakeCluster) PatchResource(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
	if err := c.injected("patch", gvr, namespace, name); err != nil {
		return nil, err
	}
	var patch map[string]any
	if err := json.Unmarshal(data, &patch); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	obj := c.objects[objectKey(gvr, namespace, name)]
	if obj == nil {
		return nil, notFound(gvr, name)
	}
	mergePatch(obj.Object, patch)
	c.patches = append(c.patches, gvr.Resource+"/"+name+" "+string(data))
	return obj.DeepCopy(), nil
}

// mergePatch applies patch to target with JSON merge patch rules.
func mergePatch(target, patch map[string]any) {
	for key, value := range patch {
		if value == nil {
			delete(target, key)
			continue
		}
		nested, isMap := value.(map[string]any)
		existing, hasMap := target[key].(map[string]any)
		if isMap && hasMap {
			mergePatch(existing, nested)
			continue
		}
		target[key] = value
	}
}

func (c *fakeCluster) CheckHealth(ctx context.Context, endpoint string) error {
	if c.health == nil {
		return nil
	}
	return c.health(endpoint)
}

func (c *fakeCluster) GetNodeReadyStatus(ctx context.Context, nodeNames []string) (map[string]bool, error) {
	if err := c.injected("nodes", schema.GroupVersionResource{}, "", ""); err != nil {
		return nil, err
	}
	status := map[string]bool{}
	for _, name := range nodeNames {
		if ready, ok := c.nodes[name]; ok {
			status[name] = ready
		}
	}
	return status, nil
}

func (c *fakeCluster) IsVerbose() bool {
	return false
}

var _ client.KubernetesClient = (*fakeCluster)(nil)

// failOn returns a fail hook that returns err for op on the named object, and nil otherwise.
func failOn(op, name string, err error) func(string, schema.GroupVersionResource, string, string) error {
	return func(gotOp string, _ schema.GroupVersionResource, _, gotName string) error {
		if gotOp == op && gotName == name {
			return err
		}
		return nil
	}
}

// kustomizationObject returns a stored Flux Kustomization in system-gitops, the default gitops
// namespace. Its observedGeneration matches its generation, so a resume needs no wait. Each
// condition is a "Type=Status:Reason" string.
func kustomizationObject(name string, labels map[string]string, dependsOn []string, conditions ...string) map[string]any {
	deps := make([]any, 0, len(dependsOn))
	for _, dep := range dependsOn {
		deps = append(deps, map[string]any{"name": dep})
	}
	conds := make([]any, 0, len(conditions))
	for _, condition := range conditions {
		typeStatus, reason, _ := strings.Cut(condition, ":")
		conditionType, status, _ := strings.Cut(typeStatus, "=")
		conds = append(conds, map[string]any{"type": conditionType, "status": status, "reason": reason, "message": fmt.Sprintf("%s is %s", name, reason), "lastTransitionTime": "2026-01-01T00:00:00Z"})
	}
	labelMap := map[string]any{}
	for key, value := range labels {
		labelMap[key] = value
	}
	return map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata":   map[string]any{"name": name, "namespace": "system-gitops", "labels": labelMap},
		"spec":       map[string]any{"dependsOn": deps},
		"status":     map[string]any{"observedGeneration": int64(0), "conditions": conds},
	}
}

// withInventory adds status.inventory entries with the given flux inventory IDs to a Kustomization object.
func withInventory(obj map[string]any, ids ...string) map[string]any {
	entries := make([]any, 0, len(ids))
	for _, id := range ids {
		entries = append(entries, map[string]any{"id": id, "v": "v1"})
	}
	obj["status"].(map[string]any)["inventory"] = map[string]any{"entries": entries}
	return obj
}

// contextLabels returns the ownership labels that the manager stamps for the fixture context.
func contextLabels() map[string]string {
	return map[string]string{"windsorcli.dev/context": "local", "windsorcli.dev/context-id": "ctx-1"}
}
