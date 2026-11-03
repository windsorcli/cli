package kubernetes_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseKubernetesManager_DeleteBlueprintLoadBalancerRemediation(t *testing.T) {
	// finishAfterRemediation lets DeleteBlueprint run to the end after load balancer remediation.
	// Remediation ends at the first resume patch. After it, each object reads as gone and each list is empty.
	finishAfterRemediation := func(c *client.MockKubernetesClient) {
		get, list, del := c.GetResourceFunc, c.ListResourcesFunc, c.DeleteResourceFunc
		remediated := false
		c.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			if strings.Contains(string(data), `"suspend":false`) {
				remediated = true
			}
			return &unstructured.Unstructured{}, nil
		}
		c.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if remediated || get == nil {
				return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
			}
			return get(gvr, namespace, name)
		}
		c.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			if remediated || list == nil {
				return &unstructured.UnstructuredList{}, nil
			}
			return list(gvr, namespace)
		}
		c.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			if remediated || del == nil {
				return nil
			}
			return del(gvr, namespace, name, opts)
		}
	}

	setup := func(t *testing.T, kubernetesClient *client.MockKubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		finishAfterRemediation(kubernetesClient)
		manager := kubernetes.NewKubernetesManager(kubernetesClient, mocks.ConfigHandler)
		manager.Timing.KustomizationWaitPollInterval = 20 * time.Millisecond
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		manager.Timing.KustomizationReconcileSleep = 20 * time.Millisecond
		manager.Timing.LoadBalancerTeardownTimeout = 100 * time.Millisecond
		return manager
	}

	destroy := func(manager *kubernetes.BaseKubernetesManager, kustomizations []blueprintv1alpha1.Kustomization) error {
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "local"},
			Kustomizations: kustomizations,
		}
		return manager.DeleteBlueprint(context.Background(), blueprint, "system-gitops")
	}

	controller := true
	gatewayOwner := metav1.OwnerReference{
		APIVersion: "gateway.networking.k8s.io/v1",
		Kind:       "Gateway",
		Name:       "external",
		Controller: &controller,
	}
	gatewayClassOwner := metav1.OwnerReference{
		APIVersion: "gateway.networking.k8s.io/v1",
		Kind:       "GatewayClass",
		Name:       "envoy",
		Controller: &controller,
	}
	servicesGVR := schema.GroupVersionResource{Version: "v1", Resource: "services"}
	gatewaysGVR := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}
	gatewayClassesGVR := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gatewayclasses"}

	// lbService builds a type=LoadBalancer Service, optionally owned by the given ownerReference.
	lbService := func(namespace, name string, owner *metav1.OwnerReference) unstructured.Unstructured {
		u := unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata":   map[string]any{"namespace": namespace, "name": name},
			"spec":       map[string]any{"type": "LoadBalancer"},
		}}
		if owner != nil {
			u.SetOwnerReferences([]metav1.OwnerReference{*owner})
		}
		return u
	}

	// listing returns a ListResources func that returns the given Services.
	listing := func(services ...unstructured.Unstructured) func(schema.GroupVersionResource, string) (*unstructured.UnstructuredList, error) {
		return func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: services}, nil
		}
	}

	// kustomizationWithInventory builds a Kustomization with the given "<namespace>_<name>_<group>_<kind>" inventory IDs.
	kustomizationWithInventory := func(ids ...string) *unstructured.Unstructured {
		entries := make([]any, 0, len(ids))
		for _, id := range ids {
			entries = append(entries, map[string]any{"id": id})
		}
		return &unstructured.Unstructured{Object: map[string]any{
			"status": map[string]any{"inventory": map[string]any{"entries": entries}},
		}}
	}

	// inventoryOnly returns a GetResource func that returns the inventory for kustomizations and NotFound otherwise.
	inventoryOnly := func(ids ...string) func(schema.GroupVersionResource, string, string) (*unstructured.Unstructured, error) {
		return func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return kustomizationWithInventory(ids...), nil
			}
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
	}

	resolveTo := func(gvr schema.GroupVersionResource) func(schema.GroupVersionKind) (schema.GroupVersionResource, error) {
		return func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
			return gvr, nil
		}
	}
	resourceForGatewayOrClass := func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
		if gvk.Kind == "GatewayClass" {
			return gatewayClassesGVR, nil
		}
		return gatewaysGVR, nil
	}
	isGatewayClassNamespaced := func(gvk schema.GroupVersionKind) (bool, error) {
		return gvk.Kind != "GatewayClass", nil
	}

	gatewayInventoryID := "system-gateway_external_gateway.networking.k8s.io_Gateway"
	gatewayClassInventoryID := "_envoy_gateway.networking.k8s.io_GatewayClass"
	eligible := []blueprintv1alpha1.Kustomization{{Name: "gateway-resources"}}
	twoKustomizations := []blueprintv1alpha1.Kustomization{{Name: "gateway-install"}, {Name: "gateway-resources"}}

	t.Run("ForegroundDeletesGatewayOwningLoadBalancer", func(t *testing.T) {
		// Given a LoadBalancer Service owned by a Gateway in the inventory
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = inventoryOnly(gatewayInventoryID)
		kubernetesClient.ListResourcesFunc = listing(lbService("system-gateway", "cilium-gateway-external", &gatewayOwner))
		kubernetesClient.ResourceForFunc = resolveTo(gatewaysGVR)
		var deletedGVR schema.GroupVersionResource
		var deletedName string
		var deletedPolicy *metav1.DeletionPropagation
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedGVR, deletedName, deletedPolicy = gvr, name, opts.PropagationPolicy
			return nil
		}
		manager := setup(t, kubernetesClient)

		// When the blueprint is deleted
		err := destroy(manager, eligible)

		// Then the owning Gateway is deleted in the foreground
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if deletedGVR != gatewaysGVR {
			t.Errorf("Expected delete on %v, got %v", gatewaysGVR, deletedGVR)
		}
		if deletedName != "external" {
			t.Errorf("Expected the owning Gateway 'external' deleted, got %q", deletedName)
		}
		if deletedPolicy == nil || *deletedPolicy != metav1.DeletePropagationForeground {
			t.Errorf("Expected foreground propagation, got %v", deletedPolicy)
		}
	})

	t.Run("ForegroundDeletesDirectlyOwnedLoadBalancerService", func(t *testing.T) {
		// Given a LoadBalancer Service that is itself in the inventory
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = inventoryOnly("system-lb_my-lb__Service")
		kubernetesClient.ListResourcesFunc = listing(lbService("system-lb", "my-lb", nil))
		var deletedGVR schema.GroupVersionResource
		var deletedName string
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedGVR, deletedName = gvr, name
			return nil
		}
		manager := setup(t, kubernetesClient)

		// When the blueprint is deleted
		err := destroy(manager, eligible)

		// Then the Service itself is deleted
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if deletedGVR != servicesGVR || deletedName != "my-lb" {
			t.Errorf("Expected delete on service my-lb, got %v %q", deletedGVR, deletedName)
		}
	})

	t.Run("ForegroundDeletesClusterScopedOwner", func(t *testing.T) {
		// Given a LoadBalancer Service owned by a cluster-scoped GatewayClass in the inventory
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = inventoryOnly(gatewayClassInventoryID)
		kubernetesClient.ListResourcesFunc = listing(lbService("system-gateway", "envoy-system-gateway-external", &gatewayClassOwner))
		kubernetesClient.ResourceForFunc = resolveTo(gatewayClassesGVR)
		kubernetesClient.IsNamespacedFunc = isGatewayClassNamespaced
		var deletedGVR schema.GroupVersionResource
		var deletedNamespace, deletedName string
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedGVR, deletedNamespace, deletedName = gvr, namespace, name
			return nil
		}
		manager := setup(t, kubernetesClient)

		// When the blueprint is deleted
		err := destroy(manager, eligible)

		// Then the GatewayClass is deleted with an empty namespace
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if deletedGVR != gatewayClassesGVR {
			t.Errorf("Expected delete on %v, got %v", gatewayClassesGVR, deletedGVR)
		}
		if deletedName != "envoy" {
			t.Errorf("Expected the owning GatewayClass 'envoy' deleted, got %q", deletedName)
		}
		if deletedNamespace != "" {
			t.Errorf("Expected cluster-scoped delete with empty namespace, got %q", deletedNamespace)
		}
	})

	t.Run("PropagatesTransientIsNamespacedError", func(t *testing.T) {
		// Given an owner whose scope lookup fails with a transient error
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = inventoryOnly(gatewayInventoryID)
		kubernetesClient.ListResourcesFunc = listing(lbService("system-gateway", "cilium-gateway-external", &gatewayOwner))
		kubernetesClient.ResourceForFunc = resolveTo(gatewaysGVR)
		kubernetesClient.IsNamespacedFunc = func(gvk schema.GroupVersionKind) (bool, error) {
			return false, fmt.Errorf("the server is currently unable to handle the request")
		}
		deleted := false
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleted = true
			return nil
		}
		manager := setup(t, kubernetesClient)

		// When the blueprint is deleted
		err := destroy(manager, eligible)

		// Then the destroy aborts and nothing is deleted
		if err == nil {
			t.Fatal("Expected a transient scope-resolution error to abort remediation, got nil")
		}
		if !strings.Contains(err.Error(), "destroy aborted") {
			t.Errorf("Expected remediation to abort the destroy, got %v", err)
		}
		if deleted {
			t.Error("Expected no delete when scope resolution failed transiently")
		}
	})

	t.Run("SkipsForeignLoadBalancer", func(t *testing.T) {
		// Given a LoadBalancer Service whose owner is not in the inventory
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = inventoryOnly("system-gateway_other_gateway.networking.k8s.io_Gateway")
		kubernetesClient.ListResourcesFunc = listing(lbService("system-gateway", "cilium-gateway-external", &gatewayOwner))
		kubernetesClient.ResourceForFunc = resolveTo(gatewaysGVR)
		deleted := false
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleted = true
			return nil
		}
		manager := setup(t, kubernetesClient)

		// When the blueprint is deleted
		err := destroy(manager, eligible)

		// Then remediation deletes nothing
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if deleted {
			t.Error("Expected no delete for a LoadBalancer whose owner is not in our inventory")
		}
	})

	t.Run("PropagatesTransientResourceForError", func(t *testing.T) {
		// Given an owner whose resource lookup fails with a transient error
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = inventoryOnly(gatewayInventoryID)
		kubernetesClient.ListResourcesFunc = listing(lbService("system-gateway", "cilium-gateway-external", &gatewayOwner))
		kubernetesClient.ResourceForFunc = func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
			return schema.GroupVersionResource{}, fmt.Errorf("the server is currently unable to handle the request")
		}
		deleted := false
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleted = true
			return nil
		}
		manager := setup(t, kubernetesClient)

		// When the blueprint is deleted
		err := destroy(manager, eligible)

		// Then the destroy aborts and nothing is deleted
		if err == nil {
			t.Fatal("Expected a transient discovery error to abort remediation, got nil")
		}
		if !strings.Contains(err.Error(), "destroy aborted") {
			t.Errorf("Expected remediation to abort the destroy, got %v", err)
		}
		if deleted {
			t.Error("Expected no delete when owner resolution failed transiently")
		}
	})

	t.Run("SkipsWhenOwnerKindHasNoMatch", func(t *testing.T) {
		// Given an owner kind that discovery does not know
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = inventoryOnly(gatewayInventoryID)
		kubernetesClient.ListResourcesFunc = listing(lbService("system-gateway", "cilium-gateway-external", &gatewayOwner))
		kubernetesClient.ResourceForFunc = func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
			return schema.GroupVersionResource{}, &apimeta.NoKindMatchError{GroupKind: gvk.GroupKind()}
		}
		deleted := false
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleted = true
			return nil
		}
		manager := setup(t, kubernetesClient)

		// When the blueprint is deleted
		err := destroy(manager, eligible)

		// Then remediation deletes nothing and returns no error
		if err != nil {
			t.Fatalf("Expected no error when the owner kind has no match, got %v", err)
		}
		if deleted {
			t.Error("Expected no delete when the owner kind is unresolvable")
		}
	})

	t.Run("NoOpWhenNoLoadBalancerServices", func(t *testing.T) {
		// Given only a ClusterIP Service
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = inventoryOnly(gatewayInventoryID)
		kubernetesClient.ListResourcesFunc = listing(unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]any{"namespace": "system-gateway", "name": "internal"},
			"spec":     map[string]any{"type": "ClusterIP"},
		}})
		deleted := false
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleted = true
			return nil
		}
		manager := setup(t, kubernetesClient)

		// When the blueprint is deleted
		err := destroy(manager, eligible)

		// Then remediation deletes nothing
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if deleted {
			t.Error("Expected no delete when there are no type=LoadBalancer services")
		}
	})

	t.Run("NoOpWhenInventoryEmpty", func(t *testing.T) {
		// Given eligible kustomizations with no inventory
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		listed := false
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			listed = true
			return &unstructured.UnstructuredList{}, nil
		}
		manager := setup(t, kubernetesClient)

		// When the blueprint is deleted
		err := destroy(manager, eligible)

		// Then remediation does not list Services
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if listed {
			t.Error("Expected no service scan when the eligible inventory is empty")
		}
	})

	t.Run("ErrorsWhenLoadBalancerServiceNeverReleases", func(t *testing.T) {
		// Given a LoadBalancer Service that never disappears
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return kustomizationWithInventory(gatewayInventoryID), nil
			}
			if gvr.Resource == "services" {
				return &unstructured.Unstructured{Object: map[string]any{
					"metadata": map[string]any{"namespace": namespace, "name": name},
				}}, nil
			}
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		kubernetesClient.ListResourcesFunc = listing(lbService("system-gateway", "cilium-gateway-external", &gatewayOwner))
		kubernetesClient.ResourceForFunc = resolveTo(gatewaysGVR)
		manager := setup(t, kubernetesClient)

		// When the blueprint is deleted
		err := destroy(manager, eligible)

		// Then the timeout error names the stuck Service
		if err == nil {
			t.Fatal("Expected a timeout error when the load balancer service never releases")
		}
		if !strings.Contains(err.Error(), "destroy aborted") {
			t.Errorf("Expected remediation to abort the destroy, got %v", err)
		}
		if !strings.Contains(err.Error(), "cilium-gateway-external") {
			t.Errorf("Expected error to name the stuck service, got %v", err)
		}
	})

	t.Run("UsesLoadBalancerTeardownTimeoutNotKustomizationReconcileTimeout", func(t *testing.T) {
		// Given a long reconcile timeout, a short teardown timeout, and a Service that never disappears
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return kustomizationWithInventory(gatewayInventoryID), nil
			}
			if gvr.Resource == "services" {
				return &unstructured.Unstructured{Object: map[string]any{
					"metadata": map[string]any{"namespace": namespace, "name": name},
				}}, nil
			}
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		kubernetesClient.ListResourcesFunc = listing(lbService("system-gateway", "cilium-gateway-external", &gatewayOwner))
		kubernetesClient.ResourceForFunc = resolveTo(gatewaysGVR)
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = time.Hour
		manager.Timing.LoadBalancerTeardownTimeout = 100 * time.Millisecond

		// When the blueprint is deleted
		err := destroy(manager, eligible)

		// Then the destroy times out on the teardown timeout
		if err == nil {
			t.Fatal("Expected a timeout error bounded by loadBalancerTeardownTimeout, got nil")
		}
		if !strings.Contains(err.Error(), "destroy aborted") {
			t.Errorf("Expected remediation to abort the destroy, got %v", err)
		}
	})

	t.Run("ErrorsWhenOwnedRootStaysTerminatingAfterServiceReleases", func(t *testing.T) {
		// Given a Service that disappears and an owning Gateway that never does
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return kustomizationWithInventory(gatewayInventoryID), nil
			}
			if gvr == gatewaysGVR {
				return &unstructured.Unstructured{Object: map[string]any{
					"metadata": map[string]any{"name": name},
				}}, nil
			}
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		kubernetesClient.ListResourcesFunc = listing(lbService("system-gateway", "cilium-gateway-external", &gatewayOwner))
		kubernetesClient.ResourceForFunc = resolveTo(gatewaysGVR)
		manager := setup(t, kubernetesClient)

		// When the blueprint is deleted
		err := destroy(manager, eligible)

		// Then the timeout error names the stuck owner
		if err == nil {
			t.Fatal("Expected a timeout error when the owned root never releases, got nil")
		}
		if !strings.Contains(err.Error(), "destroy aborted") {
			t.Errorf("Expected remediation to abort the destroy, got %v", err)
		}
		if !strings.Contains(err.Error(), "external") {
			t.Errorf("Expected error to name the stuck owner %q, got %v", "external", err)
		}
	})

	t.Run("ErrorsAndNamesBothStuckObjects", func(t *testing.T) {
		// Given a Service and an owning Gateway that never disappear
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return kustomizationWithInventory(gatewayInventoryID), nil
			}
			if gvr.Resource == "services" || gvr == gatewaysGVR {
				return &unstructured.Unstructured{Object: map[string]any{
					"metadata": map[string]any{"namespace": namespace, "name": name},
				}}, nil
			}
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		kubernetesClient.ListResourcesFunc = listing(lbService("system-gateway", "cilium-gateway-external", &gatewayOwner))
		kubernetesClient.ResourceForFunc = resolveTo(gatewaysGVR)
		manager := setup(t, kubernetesClient)

		// When the blueprint is deleted
		err := destroy(manager, eligible)

		// Then the timeout error names both the Service and the owner
		if err == nil {
			t.Fatal("Expected a timeout error when neither the service nor the owner releases, got nil")
		}
		if !strings.Contains(err.Error(), "destroy aborted") {
			t.Errorf("Expected remediation to abort the destroy, got %v", err)
		}
		if !strings.Contains(err.Error(), "cilium-gateway-external") {
			t.Errorf("Expected error to name the stuck service, got %v", err)
		}
		if !strings.Contains(err.Error(), "gateways external") {
			t.Errorf("Expected error to name the stuck owner distinctly from the service, got %v", err)
		}
	})

	t.Run("ClearsBlockingGatewayFromAnotherKustomizationBeforeDeletingGatewayClass", func(t *testing.T) {
		// Given a GatewayClass-owned Service and a Gateway of that class in another kustomization
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				if name == "gateway-install" {
					return kustomizationWithInventory(gatewayClassInventoryID), nil
				}
				return kustomizationWithInventory(gatewayInventoryID), nil
			}
			if gvr == gatewaysGVR {
				return &unstructured.Unstructured{Object: map[string]any{
					"metadata": map[string]any{"namespace": "system-gateway", "name": "external"},
					"spec":     map[string]any{"gatewayClassName": "envoy"},
				}}, nil
			}
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		kubernetesClient.ListResourcesFunc = listing(lbService("system-gateway", "envoy-system-gateway-external", &gatewayClassOwner))
		kubernetesClient.ResourceForFunc = resourceForGatewayOrClass
		kubernetesClient.IsNamespacedFunc = isGatewayClassNamespaced
		var deleted []string
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleted = append(deleted, gvr.Resource+"/"+name)
			return nil
		}
		manager := setup(t, kubernetesClient)

		// When the blueprint is deleted
		err := destroy(manager, twoKustomizations)

		// Then the blocking Gateway and the GatewayClass are both deleted
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !slices.Contains(deleted, "gateways/external") {
			t.Errorf("Expected the blocking gateway 'external' to be deleted, got %v", deleted)
		}
		if !slices.Contains(deleted, "gatewayclasses/envoy") {
			t.Errorf("Expected the gatewayclass 'envoy' to be deleted, got %v", deleted)
		}
	})

	t.Run("LeavesGatewayForADifferentGatewayClassUntouched", func(t *testing.T) {
		// Given a Gateway in the inventory that names another GatewayClass
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				if name == "gateway-install" {
					return kustomizationWithInventory(gatewayClassInventoryID), nil
				}
				return kustomizationWithInventory(gatewayInventoryID), nil
			}
			if gvr == gatewaysGVR {
				return &unstructured.Unstructured{Object: map[string]any{
					"metadata": map[string]any{"namespace": "system-gateway", "name": "external"},
					"spec":     map[string]any{"gatewayClassName": "other-class"},
				}}, nil
			}
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		kubernetesClient.ListResourcesFunc = listing(lbService("system-gateway", "envoy-system-gateway-external", &gatewayClassOwner))
		kubernetesClient.ResourceForFunc = resourceForGatewayOrClass
		kubernetesClient.IsNamespacedFunc = isGatewayClassNamespaced
		var deleted []string
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleted = append(deleted, gvr.Resource+"/"+name)
			return nil
		}
		manager := setup(t, kubernetesClient)

		// When the blueprint is deleted
		err := destroy(manager, twoKustomizations)

		// Then the Gateway of the other class is not deleted
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if slices.Contains(deleted, "gateways/external") {
			t.Errorf("Expected gateway 'external' (different class) to be left untouched, got %v", deleted)
		}
	})

	t.Run("SkipsGatewayClearingForNonGatewayClassOwner", func(t *testing.T) {
		// Given a LoadBalancer Service whose owned root is a Gateway
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = inventoryOnly(gatewayInventoryID)
		kubernetesClient.ListResourcesFunc = listing(lbService("system-gateway", "cilium-gateway-external", &gatewayOwner))
		kubernetesClient.ResourceForFunc = resolveTo(gatewaysGVR)
		deleteCount := 0
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCount++
			return nil
		}
		manager := setup(t, kubernetesClient)

		// When the blueprint is deleted
		err := destroy(manager, eligible)

		// Then remediation deletes only the Gateway
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if deleteCount != 1 {
			t.Errorf("Expected exactly 1 delete, got %d", deleteCount)
		}
	})
}
