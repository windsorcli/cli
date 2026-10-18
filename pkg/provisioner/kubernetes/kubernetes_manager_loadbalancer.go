// The kubernetes_manager_loadbalancer file holds the load balancer cleanup of the BaseKubernetesManager.
// It provides the release of cloud load balancers before their controller is deleted.
// It follows the owner references of each LoadBalancer Service to an object in the inventory.
// It deletes that owner in the foreground and waits until the Service and the owner are gone.

package kubernetes

import (
	"fmt"
	"strings"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// =============================================================================
// Constants
// =============================================================================

// servicesGVR is the core v1 Services resource, scanned during destroy to find cloud
// LoadBalancers that must be released before their controller is torn down.
var servicesGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}

// maxOwnerWalkDepth bounds the ownerReference ascent when resolving the inventory-owned root of a
// LoadBalancer Service, guarding against cyclic or pathologically deep owner chains.
const maxOwnerWalkDepth = 8

// gatewayAPIGroup and gatewayClassResource identify a Gateway API GatewayClass, so
// deleteBlockingGateways knows when an owned root needs its blocking Gateways cleared.
const (
	gatewayAPIGroup      = "gateway.networking.k8s.io"
	gatewayClassResource = "gatewayclasses"
)

// =============================================================================
// Types
// =============================================================================

// ownedTarget identifies an inventory-owned resource to foreground-delete during load balancer
// remediation, addressed by its resolved GVR, namespace, and name.
type ownedTarget struct {
	gvr       schema.GroupVersionResource
	namespace string
	name      string
}

// =============================================================================
// Private Methods
// =============================================================================

// remediateLoadBalancerOwners releases cloud LoadBalancers that would otherwise be orphaned when
// their controller (a cloud-controller-manager) is torn down before the LoadBalancer Service's
// cloud finalizer runs. Flux prunes with background propagation and waits only on its own
// inventory, so a controller-generated type=LoadBalancer Service — a child of an inventory object
// such as a Gateway, never itself in the inventory — is garbage-collected asynchronously and its
// cloud-LB finalizer can outlive the controller, leaking the LB and wedging the terraform network
// delete. Called while every controller is still alive (after suspend, before the teardown walk),
// it lists live type=LoadBalancer Services, walks each one's ownerReferences to the first ancestor
// present in the eligible kustomizations' inventory, and foreground-deletes that owned root so the
// ownerReference cascade blocks on the child Service's cloud finalizer while the CCM can still
// release the LB. Services with no inventory-owned ancestor are foreign and left untouched.
func (k *BaseKubernetesManager) remediateLoadBalancerOwners(eligible []blueprintv1alpha1.Kustomization, namespace string) error {
	owned, entries, err := k.ownedInventorySet(eligible, namespace)
	if err != nil {
		return err
	}
	if len(owned) == 0 {
		return nil
	}

	services, err := k.client.ListResources(servicesGVR, "")
	if err != nil {
		return werror.Wrap(err, "error listing services for load balancer remediation")
	}
	if services == nil {
		return nil
	}

	handled := make(map[string]bool)
	for i := range services.Items {
		svc := &services.Items[i]
		if !isLoadBalancerService(svc) {
			continue
		}
		target, found, err := k.ownedRootForService(svc, owned)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		key := target.gvr.String() + "|" + target.namespace + "|" + target.name
		if handled[key] {
			continue
		}
		handled[key] = true
		if err := k.deleteBlockingGateways(target.gvr, target.name, entries); err != nil {
			return err
		}
		if err := k.foregroundDeleteAndWaitService(target, svc); err != nil {
			return err
		}
	}
	return nil
}

// ownedInventorySet returns the set of resources managed by the eligible kustomizations, keyed by
// group/kind/namespace/name, alongside the raw entries, from each kustomization's Flux inventory.
// The set is the ground truth of "resources we own" that scopes load balancer remediation to our
// own LoadBalancers; the raw entries let callers filter by kind, e.g. deleteBlockingGateways.
func (k *BaseKubernetesManager) ownedInventorySet(eligible []blueprintv1alpha1.Kustomization, namespace string) (map[string]bool, []InventoryEntry, error) {
	owned := make(map[string]bool)
	var all []InventoryEntry
	for _, kustomization := range eligible {
		entries, err := k.GetKustomizationInventory(kustomization.Name, namespace)
		if err != nil {
			return nil, nil, err
		}
		for _, entry := range entries {
			owned[inventoryKey(entry.Group, entry.Kind, entry.Namespace, entry.Name)] = true
			all = append(all, entry)
		}
	}
	return owned, all, nil
}

// ownedRootForService walks a LoadBalancer Service's ownerReferences to the first ancestor present
// in the owned inventory set and returns it as the deletion target. A Service we applied directly
// is its own target; otherwise the ascent follows the controller ownerReference upward, fetching
// each owner to read its own references, until an owned ancestor is found or the chain ends.
// Returns found=false when no ancestor is ours (a foreign LoadBalancer we must not touch).
//
// Each hop resolves its own scope via IsNamespaced rather than reusing the Service's namespace
// unconditionally: a cluster-scoped owner (e.g. GatewayClass, ClusterRole) is addressed with an
// empty namespace, both for the inventory lookup and the GetResource call. Flux's inventory
// likewise encodes cluster-scoped entries with an empty namespace, so treating a cluster-scoped
// owner as if it lived in the Service's namespace makes both the inventory lookup and the
// GetResource call miss — the walk silently classifies an owned root as foreign and gives up.
func (k *BaseKubernetesManager) ownedRootForService(svc *unstructured.Unstructured, owned map[string]bool) (ownedTarget, bool, error) {
	namespace := svc.GetNamespace()
	if owned[inventoryKey("", "Service", namespace, svc.GetName())] {
		return ownedTarget{gvr: servicesGVR, namespace: namespace, name: svc.GetName()}, true, nil
	}

	current := svc
	for range maxOwnerWalkDepth {
		owner := controllerOwnerRef(current)
		if owner == nil {
			return ownedTarget{}, false, nil
		}
		gv, err := schema.ParseGroupVersion(owner.APIVersion)
		if err != nil {
			return ownedTarget{}, false, nil
		}
		gvk := gv.WithKind(owner.Kind)
		gvr, ownerNamespace, ok, err := k.resolveScopedGVR(gvk, namespace)
		if err != nil {
			return ownedTarget{}, false, werror.Wrap(err, "error resolving load balancer owner %s %q", owner.Kind, owner.Name)
		}
		if !ok {
			return ownedTarget{}, false, nil
		}
		if owned[inventoryKey(gv.Group, owner.Kind, ownerNamespace, owner.Name)] {
			return ownedTarget{gvr: gvr, namespace: ownerNamespace, name: owner.Name}, true, nil
		}
		next, err := k.client.GetResource(gvr, ownerNamespace, owner.Name)
		if err != nil {
			if isNotFoundError(err) {
				return ownedTarget{}, false, nil
			}
			return ownedTarget{}, false, err
		}
		current = next
	}
	return ownedTarget{}, false, nil
}

// deleteBlockingGateways foreground-deletes every eligible-inventory Gateway naming the given
// GatewayClass before its own foreground-delete-and-wait begins. The class's gateway-exists-
// finalizer only lifts once no Gateway anywhere still names it, and without this a Gateway
// belonging to a not-yet-reached Kustomization — one orderForDestroy would delete moments later
// anyway — can time out remediation for no reason. Only inventory Gateways are touched; a
// foreign one is left alone, matching remediation's existing scope. A no-op for any other kind
// of owned root.
func (k *BaseKubernetesManager) deleteBlockingGateways(classGVR schema.GroupVersionResource, className string, entries []InventoryEntry) error {
	if classGVR.Group != gatewayAPIGroup || classGVR.Resource != gatewayClassResource {
		return nil
	}
	gatewayGVR, err := k.client.ResourceFor(schema.GroupVersionKind{Group: classGVR.Group, Version: classGVR.Version, Kind: "Gateway"})
	if err != nil {
		if apimeta.IsNoMatchError(err) {
			return nil
		}
		return werror.Wrap(err, "error resolving gateway resource while clearing gatewayclass %s", className)
	}
	for _, entry := range entries {
		if entry.Kind != "Gateway" || entry.Group != gatewayAPIGroup {
			continue
		}
		gw, err := k.client.GetResource(gatewayGVR, entry.Namespace, entry.Name)
		if err != nil {
			if isNotFoundError(err) {
				continue
			}
			return werror.Wrap(err, "error reading gateway %s/%s while clearing gatewayclass %s", entry.Namespace, entry.Name, className)
		}
		gatewayClassName, _, _ := unstructured.NestedString(gw.Object, "spec", "gatewayClassName")
		if gatewayClassName != className {
			continue
		}
		if err := k.client.DeleteResource(gatewayGVR, entry.Namespace, entry.Name, metav1.DeleteOptions{}); err != nil && !isNotFoundError(err) {
			return werror.Wrap(err, "error deleting gateway %s/%s to release gatewayclass %s", entry.Namespace, entry.Name, className)
		}
	}
	return nil
}

// foregroundDeleteAndWaitService foreground-deletes an owned load balancer root. It waits for the
// Service to confirm the cloud-controller-manager released the LB. It also waits for the root
// itself to confirm deletion finished, since some finalizers do not use ownerReferences. The
// Gateway API's gateway-exists-finalizer on a GatewayClass is one example. That finalizer clears
// only when no Gateway still names the GatewayClass. When the target is the Service itself, the two
// waits collapse into one. A NotFound on delete counts as already gone. The wait uses
// loadBalancerTeardownTimeout, not kustomizationReconcileTimeout: cloud-provider LB deprovisioning
// is a longer, unrelated wait. On timeout the error names every object still present, since
// proceeding could wedge the terraform teardown.
func (k *BaseKubernetesManager) foregroundDeleteAndWaitService(target ownedTarget, svc *unstructured.Unstructured) error {
	policy := metav1.DeletePropagationForeground
	err := k.client.DeleteResource(target.gvr, target.namespace, target.name, metav1.DeleteOptions{PropagationPolicy: &policy})
	if err != nil && !isNotFoundError(err) {
		return werror.Wrap(err, "error foreground-deleting load balancer owner %s/%s", target.namespace, target.name)
	}

	svcNamespace, svcName := svc.GetNamespace(), svc.GetName()
	rootIsService := target.gvr == servicesGVR && target.namespace == svcNamespace && target.name == svcName
	serviceGone := false
	rootGone := rootIsService

	timeout := time.Now().Add(k.loadBalancerTeardownTimeout)
	for time.Now().Before(timeout) {
		if !serviceGone {
			gone, err := k.resourceGone(servicesGVR, svcNamespace, svcName)
			if err != nil {
				return werror.Wrap(err, "error waiting for load balancer service %s/%s deletion", svcNamespace, svcName)
			}
			serviceGone = gone
		}
		if !rootGone {
			gone, err := k.resourceGone(target.gvr, target.namespace, target.name)
			if err != nil {
				return werror.Wrap(err, "error waiting for load balancer owner %s/%s deletion", target.namespace, target.name)
			}
			rootGone = gone
		}
		if serviceGone && rootGone {
			return nil
		}
		time.Sleep(k.kustomizationWaitPollInterval)
	}

	return foregroundDeleteTimeoutError(svcNamespace, svcName, serviceGone, target, rootGone)
}

// resourceGone reports whether a resource is gone (NotFound), or a non-NotFound error if the check
// itself failed. It backs the Service and root polls in foregroundDeleteAndWaitService.
func (k *BaseKubernetesManager) resourceGone(gvr schema.GroupVersionResource, namespace, name string) (bool, error) {
	if _, err := k.client.GetResource(gvr, namespace, name); err != nil {
		if isNotFoundError(err) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// =============================================================================
// Helpers
// =============================================================================

// foregroundDeleteTimeoutError builds the timeout error for foregroundDeleteAndWaitService, naming
// every object still present — the Service, the root, or both — so an operator is never pointed at
// the wrong one.
func foregroundDeleteTimeoutError(svcNamespace, svcName string, serviceGone bool, target ownedTarget, rootGone bool) error {
	var stuck []string
	var hints []string
	if !serviceGone {
		stuck = append(stuck, fmt.Sprintf("load balancer service %s/%s", svcNamespace, svcName))
		hints = append(hints, fmt.Sprintf("`kubectl get svc %s -n %s -o yaml`", svcName, svcNamespace))
	}
	if !rootGone {
		stuck = append(stuck, fmt.Sprintf("load balancer owner %s %s", target.gvr.Resource, target.name))
		hints = append(hints, fmt.Sprintf("`kubectl get %s %s%s -o yaml`", target.gvr.Resource, target.name, namespaceFlag(target.namespace)))
	}
	return fmt.Errorf("timeout waiting for load balancer teardown. %s still present. A finalizer has not lifted. Inspect with %s", strings.Join(stuck, " and "), strings.Join(hints, " and "))
}

// isLoadBalancerService reports whether an object is a Service of spec.type LoadBalancer.
func isLoadBalancerService(svc *unstructured.Unstructured) bool {
	svcType, found, err := unstructured.NestedString(svc.Object, "spec", "type")
	return err == nil && found && svcType == "LoadBalancer"
}

// controllerOwnerRef returns the controller ownerReference of an object, falling back to the first
// ownerReference when none is marked controller. Returns nil when the object has no owners.
func controllerOwnerRef(obj *unstructured.Unstructured) *metav1.OwnerReference {
	refs := obj.GetOwnerReferences()
	for i := range refs {
		if refs[i].Controller != nil && *refs[i].Controller {
			return &refs[i]
		}
	}
	if len(refs) > 0 {
		return &refs[0]
	}
	return nil
}
