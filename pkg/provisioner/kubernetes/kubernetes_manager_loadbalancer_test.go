package kubernetes

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestBaseKubernetesManager_remediateLoadBalancerOwners(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		manager.kustomizationWaitPollInterval = 20 * time.Millisecond
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.loadBalancerTeardownTimeout = 100 * time.Millisecond
		return manager
	}

	controller := true
	gatewayOwner := metav1.OwnerReference{
		APIVersion: "gateway.networking.k8s.io/v1",
		Kind:       "Gateway",
		Name:       "external",
		Controller: &controller,
	}
	gatewaysGVR := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}

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

	// kustomizationWithInventory builds a Kustomization object carrying the given inventory IDs
	// (each of the form "<namespace>_<name>_<group>_<kind>").
	kustomizationWithInventory := func(ids ...string) *unstructured.Unstructured {
		entries := make([]any, 0, len(ids))
		for _, id := range ids {
			entries = append(entries, map[string]any{"id": id})
		}
		return &unstructured.Unstructured{Object: map[string]any{
			"status": map[string]any{"inventory": map[string]any{"entries": entries}},
		}}
	}

	gatewayInventoryID := "system-gateway_external_gateway.networking.k8s.io_Gateway"
	eligible := []blueprintv1alpha1.Kustomization{{Name: "gateway-resources"}}

	t.Run("ForegroundDeletesGatewayOwningLoadBalancer", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return kustomizationWithInventory(gatewayInventoryID), nil
			}
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				lbService("system-gateway", "cilium-gateway-external", &gatewayOwner),
			}}, nil
		}
		kubernetesClient.ResourceForFunc = func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
			return gatewaysGVR, nil
		}
		var deletedGVR schema.GroupVersionResource
		var deletedName string
		var deletedPolicy *metav1.DeletionPropagation
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedGVR, deletedName, deletedPolicy = gvr, name, opts.PropagationPolicy
			return nil
		}
		manager.client = kubernetesClient

		if err := manager.remediateLoadBalancerOwners(eligible, "system-gitops"); err != nil {
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
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return kustomizationWithInventory("system-lb_my-lb__Service"), nil
			}
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				lbService("system-lb", "my-lb", nil),
			}}, nil
		}
		var deletedGVR schema.GroupVersionResource
		var deletedName string
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedGVR, deletedName = gvr, name
			return nil
		}
		manager.client = kubernetesClient

		if err := manager.remediateLoadBalancerOwners(eligible, "system-gitops"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if deletedGVR != servicesGVR || deletedName != "my-lb" {
			t.Errorf("Expected delete on service my-lb, got %v %q", deletedGVR, deletedName)
		}
	})

	t.Run("ForegroundDeletesClusterScopedOwner", func(t *testing.T) {
		// Given a LoadBalancer Service owned by a cluster-scoped GatewayClass (Envoy Gateway's
		// actual ownerReference target, cli#3160) that IS in the eligible kustomization's
		// inventory, encoded with an empty namespace segment as Flux does for cluster-scoped
		// entries
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		gatewayClassOwner := metav1.OwnerReference{
			APIVersion: "gateway.networking.k8s.io/v1",
			Kind:       "GatewayClass",
			Name:       "envoy",
			Controller: &controller,
		}
		gatewayClassesGVR := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gatewayclasses"}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return kustomizationWithInventory("_envoy_gateway.networking.k8s.io_GatewayClass"), nil
			}
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				lbService("system-gateway", "envoy-system-gateway-external", &gatewayClassOwner),
			}}, nil
		}
		kubernetesClient.ResourceForFunc = func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
			return gatewayClassesGVR, nil
		}
		kubernetesClient.IsNamespacedFunc = func(gvk schema.GroupVersionKind) (bool, error) {
			return gvk.Kind != "GatewayClass", nil
		}
		var deletedGVR schema.GroupVersionResource
		var deletedNamespace, deletedName string
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedGVR, deletedNamespace, deletedName = gvr, namespace, name
			return nil
		}
		manager.client = kubernetesClient

		// When remediation runs
		if err := manager.remediateLoadBalancerOwners(eligible, "system-gitops"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the cluster-scoped GatewayClass is recognized as owned and foreground-deleted
		// with an empty namespace, rather than being silently treated as foreign
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
		// Given a client whose scope resolution fails transiently after the owner's GVR
		// resolves successfully
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return kustomizationWithInventory(gatewayInventoryID), nil
			}
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				lbService("system-gateway", "cilium-gateway-external", &gatewayOwner),
			}}, nil
		}
		kubernetesClient.ResourceForFunc = func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
			return gatewaysGVR, nil
		}
		kubernetesClient.IsNamespacedFunc = func(gvk schema.GroupVersionKind) (bool, error) {
			return false, fmt.Errorf("the server is currently unable to handle the request")
		}
		deleted := false
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleted = true
			return nil
		}
		manager.client = kubernetesClient

		// When remediation runs
		err := manager.remediateLoadBalancerOwners(eligible, "system-gitops")

		// Then the transient error aborts remediation rather than silently treating the
		// owner as foreign
		if err == nil {
			t.Fatal("Expected a transient scope-resolution error to abort remediation, got nil")
		}
		if deleted {
			t.Error("Expected no delete when scope resolution failed transiently")
		}
	})

	t.Run("SkipsForeignLoadBalancer", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return kustomizationWithInventory("system-gateway_other_gateway.networking.k8s.io_Gateway"), nil
			}
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				lbService("system-gateway", "cilium-gateway-external", &gatewayOwner),
			}}, nil
		}
		kubernetesClient.ResourceForFunc = func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
			return gatewaysGVR, nil
		}
		deleted := false
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleted = true
			return nil
		}
		manager.client = kubernetesClient

		if err := manager.remediateLoadBalancerOwners(eligible, "system-gitops"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if deleted {
			t.Error("Expected no delete for a LoadBalancer whose owner is not in our inventory")
		}
	})

	t.Run("PropagatesTransientResourceForError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return kustomizationWithInventory(gatewayInventoryID), nil
			}
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				lbService("system-gateway", "cilium-gateway-external", &gatewayOwner),
			}}, nil
		}
		kubernetesClient.ResourceForFunc = func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
			return schema.GroupVersionResource{}, fmt.Errorf("the server is currently unable to handle the request")
		}
		deleted := false
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleted = true
			return nil
		}
		manager.client = kubernetesClient

		err := manager.remediateLoadBalancerOwners(eligible, "system-gitops")
		if err == nil {
			t.Fatal("Expected a transient discovery error to abort remediation, got nil")
		}
		if deleted {
			t.Error("Expected no delete when owner resolution failed transiently")
		}
	})

	t.Run("SkipsWhenOwnerKindHasNoMatch", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return kustomizationWithInventory(gatewayInventoryID), nil
			}
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				lbService("system-gateway", "cilium-gateway-external", &gatewayOwner),
			}}, nil
		}
		kubernetesClient.ResourceForFunc = func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
			return schema.GroupVersionResource{}, &apimeta.NoKindMatchError{GroupKind: gvk.GroupKind()}
		}
		deleted := false
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleted = true
			return nil
		}
		manager.client = kubernetesClient

		if err := manager.remediateLoadBalancerOwners(eligible, "system-gitops"); err != nil {
			t.Fatalf("Expected no error when the owner kind has no match, got %v", err)
		}
		if deleted {
			t.Error("Expected no delete when the owner kind is unresolvable")
		}
	})

	t.Run("NoOpWhenNoLoadBalancerServices", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return kustomizationWithInventory(gatewayInventoryID), nil
			}
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		clusterIPService := unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]any{"namespace": "system-gateway", "name": "internal"},
			"spec":     map[string]any{"type": "ClusterIP"},
		}}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{clusterIPService}}, nil
		}
		deleted := false
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleted = true
			return nil
		}
		manager.client = kubernetesClient

		if err := manager.remediateLoadBalancerOwners(eligible, "system-gitops"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if deleted {
			t.Error("Expected no delete when there are no type=LoadBalancer services")
		}
	})

	t.Run("NoOpWhenInventoryEmpty", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		listed := false
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			listed = true
			return &unstructured.UnstructuredList{}, nil
		}
		manager.client = kubernetesClient

		if err := manager.remediateLoadBalancerOwners(eligible, "system-gitops"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if listed {
			t.Error("Expected no service scan when the eligible inventory is empty")
		}
	})

	t.Run("ErrorsWhenLoadBalancerServiceNeverReleases", func(t *testing.T) {
		manager := setup(t)
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
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				lbService("system-gateway", "cilium-gateway-external", &gatewayOwner),
			}}, nil
		}
		kubernetesClient.ResourceForFunc = func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
			return gatewaysGVR, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		manager.client = kubernetesClient

		err := manager.remediateLoadBalancerOwners(eligible, "system-gitops")
		if err == nil {
			t.Fatal("Expected a timeout error when the load balancer service never releases")
		}
		if !strings.Contains(err.Error(), "cilium-gateway-external") {
			t.Errorf("Expected error to name the stuck service, got %v", err)
		}
	})

	t.Run("UsesLoadBalancerTeardownTimeoutNotKustomizationReconcileTimeout", func(t *testing.T) {
		// Given a manager whose kustomization reconcile timeout is long but whose dedicated
		// load balancer teardown timeout is short
		manager := setup(t)
		manager.kustomizationReconcileTimeout = time.Hour
		manager.loadBalancerTeardownTimeout = 100 * time.Millisecond
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
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				lbService("system-gateway", "cilium-gateway-external", &gatewayOwner),
			}}, nil
		}
		kubernetesClient.ResourceForFunc = func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
			return gatewaysGVR, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		manager.client = kubernetesClient

		// When the load balancer service never releases
		err := manager.remediateLoadBalancerOwners(eligible, "system-gitops")

		// Then remediation still times out on the short teardown budget, not the long
		// reconcile timeout
		if err == nil {
			t.Fatal("Expected a timeout error bounded by loadBalancerTeardownTimeout, got nil")
		}
	})

	t.Run("ErrorsWhenOwnedRootStaysTerminatingAfterServiceReleases", func(t *testing.T) {
		// Given a Service that clears quickly via normal ownerReference garbage collection, but
		// whose foreground-deleted owner (e.g. a GatewayClass held by its own
		// gateway-exists-finalizer, cli#3293) never actually disappears
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return kustomizationWithInventory(gatewayInventoryID), nil
			}
			if gvr.Resource == "services" {
				return nil, fmt.Errorf("services %q not found", name)
			}
			if gvr == gatewaysGVR {
				return &unstructured.Unstructured{Object: map[string]any{
					"metadata": map[string]any{"name": name},
				}}, nil
			}
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				lbService("system-gateway", "cilium-gateway-external", &gatewayOwner),
			}}, nil
		}
		kubernetesClient.ResourceForFunc = func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
			return gatewaysGVR, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		manager.client = kubernetesClient

		// When remediation runs
		err := manager.remediateLoadBalancerOwners(eligible, "system-gitops")

		// Then it reports the stuck owner as an error instead of silently succeeding on the
		// Service's release alone
		if err == nil {
			t.Fatal("Expected a timeout error when the owned root never releases, got nil")
		}
		if !strings.Contains(err.Error(), "external") {
			t.Errorf("Expected error to name the stuck owner %q, got %v", "external", err)
		}
	})

	t.Run("ErrorsAndNamesBothStuckObjects", func(t *testing.T) {
		// Given neither the Service nor its foreground-deleted owner ever disappears
		manager := setup(t)
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
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				lbService("system-gateway", "cilium-gateway-external", &gatewayOwner),
			}}, nil
		}
		kubernetesClient.ResourceForFunc = func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
			return gatewaysGVR, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		manager.client = kubernetesClient

		// When remediation runs
		err := manager.remediateLoadBalancerOwners(eligible, "system-gitops")

		// Then the error names both the stuck Service and the stuck owner, not just one
		if err == nil {
			t.Fatal("Expected a timeout error when neither the service nor the owner releases, got nil")
		}
		if !strings.Contains(err.Error(), "cilium-gateway-external") {
			t.Errorf("Expected error to name the stuck service, got %v", err)
		}
		if !strings.Contains(err.Error(), "gateways external") {
			t.Errorf("Expected error to name the stuck owner distinctly from the service, got %v", err)
		}
	})

	gatewayClassesGVR := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gatewayclasses"}
	resourceForGatewayOrClass := func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
		if gvk.Kind == "GatewayClass" {
			return gatewayClassesGVR, nil
		}
		return gatewaysGVR, nil
	}
	isGatewayClassNamespaced := func(gvk schema.GroupVersionKind) (bool, error) {
		return gvk.Kind != "GatewayClass", nil
	}

	t.Run("ClearsBlockingGatewayFromAnotherKustomizationBeforeDeletingGatewayClass", func(t *testing.T) {
		// Given a stray LoadBalancer Service owned by a GatewayClass (gateway-install's
		// inventory), and a Gateway naming that class in a different Kustomization's
		// inventory (gateway-resources) that the ordered walk hasn't reached yet (cli#3385)
		manager := setup(t)
		twoKustomizations := []blueprintv1alpha1.Kustomization{{Name: "gateway-install"}, {Name: "gateway-resources"}}
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				if name == "gateway-install" {
					return kustomizationWithInventory("_envoy_gateway.networking.k8s.io_GatewayClass"), nil
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
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			gatewayClassOwner := metav1.OwnerReference{
				APIVersion: "gateway.networking.k8s.io/v1",
				Kind:       "GatewayClass",
				Name:       "envoy",
				Controller: &controller,
			}
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				lbService("system-gateway", "envoy-system-gateway-external", &gatewayClassOwner),
			}}, nil
		}
		kubernetesClient.ResourceForFunc = resourceForGatewayOrClass
		kubernetesClient.IsNamespacedFunc = isGatewayClassNamespaced
		var deleted []string
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleted = append(deleted, gvr.Resource+"/"+name)
			return nil
		}
		manager.client = kubernetesClient

		// When remediation runs
		if err := manager.remediateLoadBalancerOwners(twoKustomizations, "system-gitops"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the blocking Gateway is deleted before/alongside the GatewayClass, rather than
		// remediation timing out waiting on a finalizer nothing ever cleared
		if !slices.Contains(deleted, "gateways/external") {
			t.Errorf("Expected the blocking gateway 'external' to be deleted, got %v", deleted)
		}
		if !slices.Contains(deleted, "gatewayclasses/envoy") {
			t.Errorf("Expected the gatewayclass 'envoy' to be deleted, got %v", deleted)
		}
	})

	t.Run("LeavesGatewayForADifferentGatewayClassUntouched", func(t *testing.T) {
		// Given a Gateway in inventory that names a different GatewayClass than the one
		// being remediated
		manager := setup(t)
		twoKustomizations := []blueprintv1alpha1.Kustomization{{Name: "gateway-install"}, {Name: "gateway-resources"}}
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				if name == "gateway-install" {
					return kustomizationWithInventory("_envoy_gateway.networking.k8s.io_GatewayClass"), nil
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
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			gatewayClassOwner := metav1.OwnerReference{
				APIVersion: "gateway.networking.k8s.io/v1",
				Kind:       "GatewayClass",
				Name:       "envoy",
				Controller: &controller,
			}
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				lbService("system-gateway", "envoy-system-gateway-external", &gatewayClassOwner),
			}}, nil
		}
		kubernetesClient.ResourceForFunc = resourceForGatewayOrClass
		kubernetesClient.IsNamespacedFunc = isGatewayClassNamespaced
		var deleted []string
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleted = append(deleted, gvr.Resource+"/"+name)
			return nil
		}
		manager.client = kubernetesClient

		// When remediation runs
		if err := manager.remediateLoadBalancerOwners(twoKustomizations, "system-gitops"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the gateway naming a different class is left alone
		if slices.Contains(deleted, "gateways/external") {
			t.Errorf("Expected gateway 'external' (different class) to be left untouched, got %v", deleted)
		}
	})

	t.Run("SkipsGatewayClearingForNonGatewayClassOwner", func(t *testing.T) {
		// Given an owned root that is a Gateway, not a GatewayClass
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return kustomizationWithInventory(gatewayInventoryID), nil
			}
			return nil, fmt.Errorf("%s %q not found", gvr.Resource, name)
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
				lbService("system-gateway", "cilium-gateway-external", &gatewayOwner),
			}}, nil
		}
		kubernetesClient.ResourceForFunc = func(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
			return gatewaysGVR, nil
		}
		deleteCount := 0
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCount++
			return nil
		}
		manager.client = kubernetesClient

		// When remediation runs
		if err := manager.remediateLoadBalancerOwners(eligible, "system-gitops"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then only the owned Gateway itself is deleted — no extra gateway-clearing pass runs
		// for a root that isn't a GatewayClass
		if deleteCount != 1 {
			t.Errorf("Expected exactly 1 delete, got %d", deleteCount)
		}
	})
}
