package kubernetes

import (
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseKubernetesManager_GetKustomizationInventory(t *testing.T) {
	t.Run("DecodesEntriesFromInventoryStatus", func(t *testing.T) {
		// Given a kustomization whose status carries inventory entries with the
		// flux ID encoding "<ns>_<name>_<group>_<kind>"
		m := setupKubernetesMocks(t)
		m.KubernetesClient.(*client.MockKubernetesClient).GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			obj := &unstructured.Unstructured{}
			obj.Object = map[string]any{
				"status": map[string]any{
					"inventory": map[string]any{
						"entries": []any{
							map[string]any{"id": "monitoring_grafana_apps_Deployment", "v": "v1"},
							map[string]any{"id": "monitoring_grafana-config__ConfigMap", "v": "v1"},
							map[string]any{"id": "_admin_rbac.authorization.k8s.io_ClusterRole", "v": "v1"},
						},
					},
				},
			}
			return obj, nil
		}
		manager := NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

		// When GetKustomizationInventory runs
		entries, err := manager.GetKustomizationInventory("monitoring", "flux-system")

		// Then each entry is decoded with namespace/name/group/kind set; cluster-
		// scoped resources have empty namespace; core API objects have empty group
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []InventoryEntry{
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
		// Absent kustomization is not an error in the destroy-plan context — it
		// just means "not deployed yet, nothing to destroy."
		m := setupKubernetesMocks(t)
		m.KubernetesClient.(*client.MockKubernetesClient).GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("%q not found", name)
		}
		manager := NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

		entries, err := manager.GetKustomizationInventory("not-yet", "flux-system")

		if err != nil {
			t.Fatalf("expected no error for not-found, got %v", err)
		}
		if entries != nil {
			t.Errorf("expected nil entries for not-found, got %#v", entries)
		}
	})

	t.Run("ReturnsEmptySliceWhenInventoryAbsent", func(t *testing.T) {
		// Kustomization exists but has not reconciled (suspended, or never
		// applied) — inventory is missing, but this is distinct from
		// kustomization-not-found and we surface it as an empty slice.
		m := setupKubernetesMocks(t)
		m.KubernetesClient.(*client.MockKubernetesClient).GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{"status": map[string]any{}}}, nil
		}
		manager := NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

		entries, err := manager.GetKustomizationInventory("suspended", "flux-system")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entries == nil || len(entries) != 0 {
			t.Errorf("expected empty (non-nil) slice, got %#v", entries)
		}
	})

	t.Run("DropsMalformedEntries", func(t *testing.T) {
		// IDs without exactly four underscore-separated fields are flux malformed
		// — skip rather than misrender.
		m := setupKubernetesMocks(t)
		m.KubernetesClient.(*client.MockKubernetesClient).GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			obj := &unstructured.Unstructured{}
			obj.Object = map[string]any{
				"status": map[string]any{
					"inventory": map[string]any{
						"entries": []any{
							map[string]any{"id": "garbage", "v": "v1"},
							map[string]any{"id": "ns_name__Kind", "v": "v1"},
						},
					},
				},
			}
			return obj, nil
		}
		manager := NewKubernetesManager(m.KubernetesClient, m.ConfigHandler)

		entries, err := manager.GetKustomizationInventory("x", "flux-system")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 1 || entries[0].Kind != "Kind" {
			t.Errorf("expected only the well-formed entry, got %#v", entries)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestDecodeInventoryID(t *testing.T) {
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
			// Given a flux inventory id
			// When it is decoded
			entry, ok := decodeInventoryID(tt.id)

			// Then its fields resolve to the object the id names
			if ok != tt.wantOK {
				t.Fatalf("Expected ok=%v for %q, got %v", tt.wantOK, tt.id, ok)
			}
			if !tt.wantOK {
				return
			}
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
