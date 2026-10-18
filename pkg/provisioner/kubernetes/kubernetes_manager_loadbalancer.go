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

// servicesGVR is the core v1 Service resource.
var servicesGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}

// maxOwnerWalkDepth is the maximum number of owner references that ownedRootForService follows.
const maxOwnerWalkDepth = 8

// gatewayAPIGroup and gatewayClassResource identify a Gateway API GatewayClass.
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

// remediateLoadBalancerOwners releases each cloud load balancer while its cloud-controller-manager still runs.
// A controller can create a LoadBalancer Service, such as one for a Gateway, that is not in the inventory.
// Flux does not wait for that Service, so its finalizer can outlive the controller and leak the load balancer.
// For each LoadBalancer Service, it finds the first owner in the inventory and deletes that owner in the
// foreground. It does not touch a Service with no owner in the inventory.
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

// ownedRootForService follows the controller owner references of svc to the first object in owned.
// A Service in owned is its own target. It returns found=false when no owner is in owned.
// Each step gets its scope from IsNamespaced, so a cluster-scoped owner such as a GatewayClass
// uses an empty namespace, as the Flux inventory does.
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

// deleteBlockingGateways deletes each inventory Gateway that names the GatewayClass className.
// The gateway-exists-finalizer of the class lifts only when no Gateway names it. A Gateway of a
// Kustomization that the walk has not reached yet would otherwise block the class delete.
// It does nothing when classGVR is not a GatewayClass, and it never deletes a Gateway outside the inventory.
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

// foregroundDeleteAndWaitService deletes target in the foreground, then waits until svc and target are gone.
// It waits for target too, because some finalizers, such as gateway-exists-finalizer, do not use owner
// references. A NotFound on delete counts as gone. The wait uses loadBalancerTeardownTimeout, because
// cloud load balancer removal is slow. On timeout, the error names each object that is still present.
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
