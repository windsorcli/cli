package kubernetes_test

import (
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// =============================================================================
// Test Setup
// =============================================================================

// inventoryKustomization returns a Kustomization whose status.inventory holds one entry per ID.
func inventoryKustomization(ids ...string) *unstructured.Unstructured {
	entries := make([]any, 0, len(ids))
	for _, id := range ids {
		entries = append(entries, map[string]any{"id": id, "v": "v1"})
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"status": map[string]any{
			"inventory": map[string]any{"entries": entries},
		},
	}}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseKubernetesManager_GetKustomizationInventory(t *testing.T) {
	t.Run("DecodesEntriesFromInventoryStatus", func(t *testing.T) {
		// Given a kustomization whose inventory holds namespaced, core, and cluster-scoped entries
		m := setupKubernetesMocks(t)
		m.KubernetesClient.(*client.MockKubernetesClient).GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return inventoryKustomization(
				"monitoring_grafana_apps_Deployment",
				"monitoring_grafana-config__ConfigMap",
				"_admin_rbac.authorization.k8s.io_ClusterRole",
			), nil
		}
		manager := kubernetes.NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

		// When GetKustomizationInventory runs
		entries, err := manager.GetKustomizationInventory("monitoring", "flux-system")

		// Then each entry decodes with an empty namespace or group where the ID has none
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []kubernetes.InventoryEntry{
			{Namespace: "monitoring", Name: "grafana", Group: "apps", Kind: "Deployment"},
			{Namespace: "monitoring", Name: "grafana-config", Group: "", Kind: "ConfigMap"},
			{Namespace: "", Name: "admin", Group: "rbac.authorization.k8s.io", Kind: "ClusterRole"},
		}
		if len(entries) != len(want) {
			t.Fatalf("expected %d entries, got %d: %#v", len(want), len(entries), entries)
		}
		for i, e := range entries {
			if e != want[i] {
				t.Errorf("[%d] expected %#v, got %#v", i, want[i], e)
			}
		}
	})

	t.Run("ReturnsNilWhenKustomizationAbsent", func(t *testing.T) {
		// Given a kustomization that does not exist
		m := setupKubernetesMocks(t)
		m.KubernetesClient.(*client.MockKubernetesClient).GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("%q not found", name)
		}
		manager := kubernetes.NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

		// When GetKustomizationInventory runs
		entries, err := manager.GetKustomizationInventory("not-yet", "flux-system")

		// Then it returns nil entries and no error
		if err != nil {
			t.Fatalf("expected no error for not-found, got %v", err)
		}
		if entries != nil {
			t.Errorf("expected nil entries for not-found, got %#v", entries)
		}
	})

	t.Run("ReturnsEmptySliceWhenInventoryAbsent", func(t *testing.T) {
		// Given a kustomization that exists but has no inventory
		m := setupKubernetesMocks(t)
		m.KubernetesClient.(*client.MockKubernetesClient).GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{"status": map[string]any{}}}, nil
		}
		manager := kubernetes.NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

		// When GetKustomizationInventory runs
		entries, err := manager.GetKustomizationInventory("suspended", "flux-system")

		// Then it returns an empty, non-nil slice
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entries == nil || len(entries) != 0 {
			t.Errorf("expected empty (non-nil) slice, got %#v", entries)
		}
	})

	t.Run("DropsMalformedEntries", func(t *testing.T) {
		// Given an inventory with one malformed ID and one well-formed ID
		m := setupKubernetesMocks(t)
		m.KubernetesClient.(*client.MockKubernetesClient).GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return inventoryKustomization("garbage", "ns_name__Kind"), nil
		}
		manager := kubernetes.NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

		// When GetKustomizationInventory runs
		entries, err := manager.GetKustomizationInventory("x", "flux-system")

		// Then only the well-formed entry is returned
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 1 || entries[0].Kind != "Kind" {
			t.Errorf("expected only the well-formed entry, got %#v", entries)
		}
	})
}

func TestBaseKubernetesManager_GetKustomizationInventoryIDDecoding(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		wantOK    bool
		namespace string
		objName   string
		group     string
		kind      string
	}{
		{
			name:   "NamespacedCoreObject",
			id:     "system-pki_cert-manager__ServiceAccount",
			wantOK: true, namespace: "system-pki", objName: "cert-manager", group: "", kind: "ServiceAccount",
		},
		{
			name:   "ClusterScopedGroupedObject",
			id:     "_cilium_rbac.authorization.k8s.io_ClusterRole",
			wantOK: true, namespace: "", objName: "cilium", group: "rbac.authorization.k8s.io", kind: "ClusterRole",
		},
		{
			name:   "ClusterScopedNameHoldingAColon",
			id:     "_cert-manager-webhook__subjectaccessreviews_rbac.authorization.k8s.io_ClusterRole",
			wantOK: true, namespace: "", objName: "cert-manager-webhook:subjectaccessreviews",
			group: "rbac.authorization.k8s.io", kind: "ClusterRole",
		},
		{
			name:   "NamespacedNameHoldingAColon",
			id:     "system-pki-trust_trust-manager__leaderelection_rbac.authorization.k8s.io_Role",
			wantOK: true, namespace: "system-pki-trust", objName: "trust-manager:leaderelection",
			group: "rbac.authorization.k8s.io", kind: "Role",
		},
		{
			name:   "NameHoldingTwoColons",
			id:     "_system__controller__bootstrap-signer_rbac.authorization.k8s.io_ClusterRole",
			wantOK: true, namespace: "", objName: "system:controller:bootstrap-signer",
			group: "rbac.authorization.k8s.io", kind: "ClusterRole",
		},
		{
			name:   "TooFewFields",
			id:     "system-pki_cert-manager_ServiceAccount",
			wantOK: false,
		},
		{
			name:   "EmptyName",
			id:     "system-pki__rbac.authorization.k8s.io_Role",
			wantOK: false,
		},
		{
			name:   "EmptyKind",
			id:     "system-pki_cert-manager_rbac.authorization.k8s.io_",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a kustomization whose inventory holds only this flux inventory ID
			m := setupKubernetesMocks(t)
			m.KubernetesClient.(*client.MockKubernetesClient).GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
				return inventoryKustomization(tt.id), nil
			}
			manager := kubernetes.NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

			// When the inventory is read
			entries, err := manager.GetKustomizationInventory("x", "flux-system")

			// Then the ID resolves to the object it names, or is dropped when malformed
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if !tt.wantOK {
				if len(entries) != 0 {
					t.Fatalf("Expected %q to be dropped, got %#v", tt.id, entries)
				}
				return
			}
			if len(entries) != 1 {
				t.Fatalf("Expected one entry for %q, got %#v", tt.id, entries)
			}
			entry := entries[0]
			if entry.Namespace != tt.namespace {
				t.Errorf("Expected namespace %q, got %q", tt.namespace, entry.Namespace)
			}
			if entry.Name != tt.objName {
				t.Errorf("Expected name %q, got %q", tt.objName, entry.Name)
			}
			if entry.Group != tt.group {
				t.Errorf("Expected group %q, got %q", tt.group, entry.Group)
			}
			if entry.Kind != tt.kind {
				t.Errorf("Expected kind %q, got %q", tt.kind, entry.Kind)
			}
		})
	}
}
