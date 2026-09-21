// Package client provides Kubernetes client functionality for resource operations.
// It implements server-side apply patterns for managing Kubernetes resources
// and provides a clean interface for Kubernetes resource management.

package client

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Constants
// =============================================================================

// requestTimeout is the per-request timeout for read and simple mutation API calls.
const requestTimeout = 30 * time.Second

// applyTimeout is the per-request timeout for server-side apply operations, which
// run admission webhooks synchronously and can take significantly longer than reads.
const applyTimeout = 5 * time.Minute

// =============================================================================
// Interfaces
// =============================================================================

// KubernetesClient defines methods for Kubernetes resource operations
type KubernetesClient interface {
	GetResource(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error)
	ListResources(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error)
	ListResourcesByLabel(gvr schema.GroupVersionResource, namespace, labelSelector string) (*unstructured.UnstructuredList, error)
	ApplyResource(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error)
	DeleteResource(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error
	ResourceFor(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error)
	IsNamespaced(gvk schema.GroupVersionKind) (bool, error)
	PatchResource(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error)
	CheckHealth(ctx context.Context, endpoint string) error
	GetNodeReadyStatus(ctx context.Context, nodeNames []string) (map[string]bool, error)
	IsVerbose() bool
}

// =============================================================================
// Types
// =============================================================================

// DynamicKubernetesClient implements KubernetesClient using dynamic client
type DynamicKubernetesClient struct {
	mu             sync.Mutex
	client         dynamic.Interface
	mapper         meta.RESTMapper
	endpoint       string
	builtEndpoint  string
	kubeconfigPath string
	kubeconfigHash [sha256.Size]byte
	shell          shell.Shell
}

// builtClient holds a freshly built client and the connection info that produced it. ensureClient
// commits it to the receiver only after a build fully succeeds.
type builtClient struct {
	client         dynamic.Interface
	mapper         meta.RESTMapper
	kubeconfigPath string
	kubeconfigHash [sha256.Size]byte
}

// =============================================================================
// Constructor
// =============================================================================

// NewDynamicKubernetesClient creates a new DynamicKubernetesClient. The shell's verbosity
// controls whether server-side API deprecation warnings are printed.
func NewDynamicKubernetesClient(shell shell.Shell) *DynamicKubernetesClient {
	return &DynamicKubernetesClient{shell: shell}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetResource gets a resource by name and namespace
func (c *DynamicKubernetesClient) GetResource(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	cli, _, err := c.ensureClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	return cli.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
}

// ListResources lists resources in a namespace
func (c *DynamicKubernetesClient) ListResources(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
	cli, _, err := c.ensureClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	return cli.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
}

// ListResourcesByLabel lists resources of the given kind narrowed to a label selector; an empty
// namespace lists across all namespaces. It lets callers ask the API server to return only the objects
// they care about (e.g. this context's CLI-placed secrets) rather than listing everything and filtering.
func (c *DynamicKubernetesClient) ListResourcesByLabel(gvr schema.GroupVersionResource, namespace, labelSelector string) (*unstructured.UnstructuredList, error) {
	cli, _, err := c.ensureClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	return cli.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
}

// ApplyResource applies a resource using server-side apply
func (c *DynamicKubernetesClient) ApplyResource(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	cli, _, err := c.ensureClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()
	return cli.Resource(gvr).Namespace(obj.GetNamespace()).Apply(ctx, obj.GetName(), obj, opts)
}

// DeleteResource deletes a resource
func (c *DynamicKubernetesClient) DeleteResource(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
	cli, _, err := c.ensureClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	return cli.Resource(gvr).Namespace(namespace).Delete(ctx, name, opts)
}

// ResourceFor resolves a GroupVersionKind to its GroupVersionResource using the API server's
// discovery data, so callers holding only an ownerReference (apiVersion + kind) can address the
// owning object with the dynamic client. Resolution is cached by the deferred discovery mapper.
func (c *DynamicKubernetesClient) ResourceFor(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	_, mapper, err := c.ensureClient()
	if err != nil {
		return schema.GroupVersionResource{}, err
	}
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}
	return mapping.Resource, nil
}

// IsNamespaced reports whether a GroupVersionKind is namespace-scoped, using the same discovery
// data as ResourceFor. Callers walking an ownerReference chain need this alongside the resolved
// GVR: a cluster-scoped owner (e.g. GatewayClass, ClusterRole) must be addressed with an empty
// namespace, never the child object's namespace.
func (c *DynamicKubernetesClient) IsNamespaced(gvk schema.GroupVersionKind) (bool, error) {
	_, mapper, err := c.ensureClient()
	if err != nil {
		return false, err
	}
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return false, err
	}
	return mapping.Scope.Name() == meta.RESTScopeNameNamespace, nil
}

// PatchResource patches a resource. The caller-supplied ctx is honoured so
// parent cancellation (e.g. Ctrl+C propagated through cmd.Context()) and short
// caller-defined deadlines (e.g. Notify's 10s notifyTimeout) actually preempt
// in-flight requests. A requestTimeout is layered on top as an upper bound so
// callers that pass context.Background() still get the pre-existing 30s cap.
func (c *DynamicKubernetesClient) PatchResource(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
	cli, _, err := c.ensureClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	return cli.Resource(gvr).Namespace(namespace).Patch(ctx, name, pt, data, opts)
}

// CheckHealth verifies Kubernetes API connectivity by listing nodes using the dynamic client.
// If an endpoint is specified, it overrides the default kubeconfig for this check.
// Returns an error if the client cannot be initialized or the API is unreachable.
func (c *DynamicKubernetesClient) CheckHealth(ctx context.Context, endpoint string) error {
	c.mu.Lock()
	c.endpoint = endpoint
	c.mu.Unlock()

	cli, _, err := c.ensureClient()
	if err != nil {
		return fmt.Errorf("failed to initialize Kubernetes client: %w", err)
	}

	nodeGVR := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "nodes",
	}

	if _, err := cli.Resource(nodeGVR).List(ctx, metav1.ListOptions{Limit: 1}); err != nil {
		return fmt.Errorf("failed to connect to Kubernetes API: %w", err)
	}

	return nil
}

// GetNodeReadyStatus returns a map of node names to their Ready condition status.
// It checks the Ready condition for each specified node using the dynamic client.
// If nodeNames is empty, all nodes are checked. Nodes not found are omitted from the result.
// Returns a map of node names to Ready status (true if Ready, false if NotReady), or an error if listing fails.
func (c *DynamicKubernetesClient) GetNodeReadyStatus(ctx context.Context, nodeNames []string) (map[string]bool, error) {
	cli, _, err := c.ensureClient()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Kubernetes client: %w", err)
	}

	nodeGVR := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "nodes",
	}

	nodes, err := cli.Resource(nodeGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	if len(nodeNames) > 0 {
		var filteredNodes []unstructured.Unstructured
		nodeNameSet := make(map[string]bool)
		for _, name := range nodeNames {
			nodeNameSet[name] = true
		}

		for _, node := range nodes.Items {
			if nodeNameSet[node.GetName()] {
				filteredNodes = append(filteredNodes, node)
			}
		}

		nodes.Items = filteredNodes
	}

	readyStatus := make(map[string]bool)
	for _, node := range nodes.Items {
		nodeName := node.GetName()
		ready := c.isNodeReady(&node)
		readyStatus[nodeName] = ready
	}

	return readyStatus, nil
}

// IsVerbose reports whether the shell backing this client is in verbose mode.
// A nil shell is treated as non-verbose.
func (c *DynamicKubernetesClient) IsVerbose() bool {
	return c.shell != nil && c.shell.IsVerbose()
}

// =============================================================================
// Private Methods
// =============================================================================

// ensureClient builds the client and mapper on first use. It rebuilds them when the endpoint or
// kubeconfig file changes, so a client built before a Terraform apply does not keep serving a
// cluster the apply has since replaced. A rebuild triggered by a kubeconfig file change falls
// back to the last known-good client on a transient failure (e.g. the file caught mid-write), but
// not when the file is confirmed missing. A rebuild triggered by a new explicit endpoint never
// falls back: the caller asked for that endpoint specifically, so silently serving the old one
// would misreport it. Returns the client and mapper as of this call. Another goroutine's call can
// reassign c.client/c.mapper at any time, so callers MUST use only the returned pair.
func (c *DynamicKubernetesClient) ensureClient() (dynamic.Interface, meta.RESTMapper, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil && !c.connectionChangedLocked() {
		return c.client, c.mapper, nil
	}

	endpointChanged := c.endpoint != c.builtEndpoint
	built, err := c.buildClient()
	if err != nil {
		keepCachedClient := c.client != nil && !endpointChanged && !errors.Is(err, os.ErrNotExist)
		if keepCachedClient {
			return c.client, c.mapper, nil
		}
		return nil, nil, err
	}

	c.client = built.client
	c.mapper = built.mapper
	c.builtEndpoint = c.endpoint
	c.kubeconfigPath = built.kubeconfigPath
	c.kubeconfigHash = built.kubeconfigHash
	return c.client, c.mapper, nil
}

// connectionChangedLocked reports whether the endpoint or kubeconfig file behind the cached
// client has changed. Caller MUST hold c.mu. Returns false when there is no file to compare (an
// endpoint or in-cluster config) or on a transient read failure. Returns true when the kubeconfig
// is confirmed missing: that is a real signal to stop trusting the cache, not a transient error.
func (c *DynamicKubernetesClient) connectionChangedLocked() bool {
	if c.endpoint != c.builtEndpoint {
		return true
	}
	if c.kubeconfigPath == "" {
		return false
	}
	hash, err := kubeconfigHash(c.kubeconfigPath)
	if err != nil {
		return errors.Is(err, os.ErrNotExist)
	}
	return hash != c.kubeconfigHash
}

// buildClient builds a dynamic client, REST mapper, and (for a kubeconfig file) its content hash.
// It does not mutate c, so a failed attempt leaves any existing client untouched. It hashes the
// kubeconfig file again here, separately from restConfig, since restConfig must use clientcmd's
// full loading rules so relative certificate/key/exec paths resolve correctly, and those rules
// have no way to hand back the raw bytes they read.
func (c *DynamicKubernetesClient) buildClient() (builtClient, error) {
	config, kubeconfigPath, err := c.restConfig()
	if err != nil {
		return builtClient{}, err
	}
	config.WarningHandler = warningHandlerFor(c.shell)

	cli, err := dynamic.NewForConfig(config)
	if err != nil {
		return builtClient{}, err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return builtClient{}, err
	}

	built := builtClient{
		client: cli,
		mapper: restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient)),
	}
	if kubeconfigPath != "" {
		built.kubeconfigPath = kubeconfigPath
		if hash, hashErr := kubeconfigHash(kubeconfigPath); hashErr == nil {
			built.kubeconfigHash = hash
		} else if kubeconfigPath == c.kubeconfigPath {
			// Same path as last time; this read failed transiently right after restConfig's own
			// read succeeded. Keep the last known hash rather than dropping tracking entirely,
			// so a real change still gets caught (at worst one extra rebuild attempt later)
			// instead of silently going unwatched until the process restarts.
			built.kubeconfigHash = c.kubeconfigHash
		}
	}
	return built, nil
}

// kubeconfigHash reads path and returns a hash of its content, for change detection.
func kubeconfigHash(path string) ([sha256.Size]byte, error) {
	data, err := os.ReadFile(path) // #nosec G304 - path is the operator's own KUBECONFIG env var, or the default ~/.kube/config
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(data), nil
}

// restConfig builds a REST config: an explicit endpoint first, then in-cluster config, then the
// KUBECONFIG (or ~/.kube/config) file via clientcmd's full loading rules, so relative
// certificate/key/exec paths resolve as they do for kubectl. The second return is the kubeconfig
// path read, empty otherwise. A missing file returns an os.ErrNotExist-wrapping error.
func (c *DynamicKubernetesClient) restConfig() (*rest.Config, string, error) {
	if c.endpoint != "" {
		return &rest.Config{Host: c.endpoint}, "", nil
	}

	config, err := rest.InClusterConfig()
	if err == nil {
		return config, "", nil
	}

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, "", err
		}
		kubeconfig = home + "/.kube/config"
	}
	config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, "", err
	}
	return config, kubeconfig, nil
}

// warningHandlerFor returns the REST config warning handler to use for the given shell.
// Suppresses server-side API deprecation warnings unless the shell is in verbose mode.
// A nil shell is treated as non-verbose.
func warningHandlerFor(sh shell.Shell) rest.WarningHandler {
	if sh == nil || !sh.IsVerbose() {
		return rest.NoWarnings{}
	}
	return nil
}

// isNodeReady checks if a node is in Ready state by examining its conditions.
// Returns true if the node has a Ready condition with status "True".
func (c *DynamicKubernetesClient) isNodeReady(node *unstructured.Unstructured) bool {
	conditions, found, err := unstructured.NestedSlice(node.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}

	for _, condition := range conditions {
		conditionMap, ok := condition.(map[string]interface{})
		if !ok {
			continue
		}

		conditionType, found, err := unstructured.NestedString(conditionMap, "type")
		if err != nil || !found || conditionType != "Ready" {
			continue
		}

		conditionStatus, found, err := unstructured.NestedString(conditionMap, "status")
		if err != nil || !found {
			continue
		}

		return conditionStatus == "True"
	}

	return false
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure DynamicKubernetesClient implements the KubernetesClient interface
var _ KubernetesClient = (*DynamicKubernetesClient)(nil)
