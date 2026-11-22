// The kubernetes_manager_nodes file holds the Node object checks of the BaseKubernetesManager.
// It finds Node objects by address and removes the stale Node object that a renamed node leaves behind.
// It also finds pods that cannot terminate because their node is unreachable.

package kubernetes

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/windsorcli/cli/internal/werror"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// =============================================================================
// Constants
// =============================================================================

var (
	nodeGVR      = schema.GroupVersionResource{Version: "v1", Resource: "nodes"}
	podGVR       = schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	namespaceGVR = schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
)

// =============================================================================
// Types
// =============================================================================

// nodeState is the part of a Node object that the stale node checks use.
type nodeState struct {
	name      string
	ready     bool
	addresses []string
}

// unreachableNodePods describes pods that cannot terminate because their nodes are not Ready.
type unreachableNodePods struct {
	namespaces []string
	nodes      []string
	podCount   int
}

// =============================================================================
// Public Methods
// =============================================================================

// GetNodeNamesByAddress returns the names of the Node objects that have address as an InternalIP,
// an ExternalIP, or a name. The result is empty when no Node object matches.
func (k *BaseKubernetesManager) GetNodeNamesByAddress(_ context.Context, address string) ([]string, error) {
	if k.client == nil {
		return nil, fmt.Errorf("kubernetes client not initialized")
	}
	nodes, err := k.listNodeStates()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, node := range nodes {
		if node.matches(address) {
			names = append(names, node.name)
		}
	}
	return names, nil
}

// RemoveStaleNodes deletes the Node objects that a renamed node left behind. previousNames are the
// names that Node objects at address had before the node restarted. A Node object is deleted only when
// it is in previousNames, is not Ready, and a Ready Node object at address has a name that is not in
// previousNames. It never deletes a Ready Node object. It does nothing when the node kept its name.
// The replacement can register after the node reports healthy, so it polls until the replacement
// appears or Timing.NodeRegistrationTimeout ends. It returns the names it deleted.
func (k *BaseKubernetesManager) RemoveStaleNodes(ctx context.Context, address string, previousNames []string) ([]string, error) {
	if k.client == nil {
		return nil, fmt.Errorf("kubernetes client not initialized")
	}
	if len(previousNames) == 0 {
		return nil, nil
	}

	deadline := k.Shims.TimeNow().Add(k.Timing.NodeRegistrationTimeout)
	for {
		nodes, err := k.listNodeStates()
		if err != nil {
			return nil, err
		}
		stale, settled := staleNodes(nodes, address, previousNames)
		if len(stale) > 0 {
			var deleted []string
			for _, name := range stale {
				err := k.client.DeleteResource(nodeGVR, "", name, metav1.DeleteOptions{})
				if err != nil && !isNotFoundError(err) {
					return deleted, werror.Wrap(err, "error deleting stale node %s", name)
				}
				deleted = append(deleted, name)
			}
			return deleted, nil
		}
		if settled || !k.Shims.TimeNow().Before(deadline) {
			return nil, nil
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		k.Shims.TimeSleep(k.Timing.NodeReadyPollInterval)
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// listNodeStates lists the Node objects with their Ready condition and addresses.
func (k *BaseKubernetesManager) listNodeStates() ([]nodeState, error) {
	list, err := k.client.ListResources(nodeGVR, "")
	if err != nil {
		return nil, werror.Wrap(err, "failed to list nodes")
	}
	nodes := make([]nodeState, 0, len(list.Items))
	for i := range list.Items {
		nodes = append(nodes, nodeStateFrom(&list.Items[i]))
	}
	return nodes, nil
}

// findUnreachableNodePods checks the namespaces that an inventory lists. It returns a result when
// every such namespace is Terminating and every pod left in it is being deleted on a Node object that is
// not Ready. It returns nil when any namespace has other content, has no pods, or cannot be read.
// A pod on a Node object that does not exist is ignored here, because pod garbage collection removes it.
func (k *BaseKubernetesManager) findUnreachableNodePods(entries []InventoryEntry) *unreachableNodePods {
	var namespaces []string
	for _, entry := range entries {
		if entry.Group == "" && entry.Kind == "Namespace" {
			namespaces = append(namespaces, entry.Name)
		}
	}
	if len(namespaces) == 0 {
		return nil
	}
	nodes, err := k.listNodeStates()
	if err != nil {
		return nil
	}
	notReady := map[string]bool{}
	for _, node := range nodes {
		if !node.ready {
			notReady[node.name] = true
		}
	}

	result := &unreachableNodePods{}
	nodeSet := map[string]bool{}
	for _, namespace := range namespaces {
		ns, err := k.client.GetResource(namespaceGVR, "", namespace)
		if err != nil || ns.GetDeletionTimestamp() == nil {
			return nil
		}
		pods, err := k.client.ListResources(podGVR, namespace)
		if err != nil || len(pods.Items) == 0 {
			return nil
		}
		for i := range pods.Items {
			pod := &pods.Items[i]
			nodeName, _, _ := unstructured.NestedString(pod.Object, "spec", "nodeName")
			if pod.GetDeletionTimestamp() == nil || !notReady[nodeName] {
				return nil
			}
			nodeSet[nodeName] = true
			result.podCount++
		}
		result.namespaces = append(result.namespaces, namespace)
	}
	for name := range nodeSet {
		result.nodes = append(result.nodes, name)
	}
	sort.Strings(result.nodes)
	return result
}

// =============================================================================
// Helpers
// =============================================================================

// matches reports whether value is the name or an address of the node.
func (n nodeState) matches(value string) bool {
	return n.name == value || slices.Contains(n.addresses, value)
}

// nodeStateFrom reads the name, Ready condition, and InternalIP and ExternalIP addresses of a Node object.
func nodeStateFrom(obj *unstructured.Unstructured) nodeState {
	state := nodeState{name: obj.GetName()}
	conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	for _, c := range conditions {
		condition, ok := c.(map[string]any)
		if ok && condition["type"] == "Ready" {
			state.ready = condition["status"] == "True"
		}
	}
	addresses, _, _ := unstructured.NestedSlice(obj.Object, "status", "addresses")
	for _, a := range addresses {
		address, ok := a.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := address["type"].(string)
		value, _ := address["address"].(string)
		if value != "" && (kind == "InternalIP" || kind == "ExternalIP") {
			state.addresses = append(state.addresses, value)
		}
	}
	return state
}

// staleNodes returns the names of the Node objects to delete and whether the check is settled.
// A Node object is stale when it is in previousNames, matches address, is not Ready, and a Ready
// Node object that matches address is not in previousNames. The check is settled, with nothing to
// delete, when a Ready Node object in previousNames matches address, because the node kept its name.
// It is not settled while no replacement has registered.
func staleNodes(nodes []nodeState, address string, previousNames []string) ([]string, bool) {
	hasReplacement := false
	keptName := false
	for _, node := range nodes {
		if !node.matches(address) || !node.ready {
			continue
		}
		if slices.Contains(previousNames, node.name) {
			keptName = true
		} else {
			hasReplacement = true
		}
	}
	if keptName && !hasReplacement {
		return nil, true
	}
	if !hasReplacement {
		return nil, false
	}
	var stale []string
	for _, node := range nodes {
		if node.matches(address) && !node.ready && slices.Contains(previousNames, node.name) {
			stale = append(stale, node.name)
		}
	}
	return stale, true
}

// describe returns the diagnosis text for pods that wait on unreachable nodes.
func (u *unreachableNodePods) describe() (string, string) {
	nodes := strings.Join(u.nodes, ", ")
	message := fmt.Sprintf("%d pod(s) in terminating namespace(s) %s cannot finish deleting. They run on node(s) %s, which are not Ready, so no kubelet can confirm the deletion", u.podCount, strings.Join(u.namespaces, ", "), nodes)
	hint := fmt.Sprintf("If the machines are gone, run `kubectl delete node %s` and retry.", strings.Join(u.nodes, " "))
	return message, hint
}
