package kubernetes

import (
	"fmt"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"github.com/windsorcli/cli/pkg/runtime/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestBaseKubernetesManager_PruneBlueprint(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		manager.shims = mocks.Shims
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return runtime.DefaultUnstructuredConverter.FromUnstructured(obj, target)
		}
		return manager
	}

	// kustomizationObj builds a live Kustomization list item with the given context-id label and dependsOn names.
	kustomizationObj := func(name, contextID string, dependsOn ...string) unstructured.Unstructured {
		spec := map[string]any{}
		if len(dependsOn) > 0 {
			deps := make([]any, 0, len(dependsOn))
			for _, d := range dependsOn {
				deps = append(deps, map[string]any{"name": d})
			}
			spec["dependsOn"] = deps
		}
		metadata := map[string]any{"name": name, "namespace": "system-gitops"}
		if contextID != "" {
			metadata["labels"] = map[string]any{"windsorcli.dev/context-id": contextID}
		}
		return unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
			"kind":       "Kustomization",
			"metadata":   metadata,
			"spec":       spec,
		}}
	}

	// wire makes the manager list the given items and capture deletions in order; GetResource returns
	// not-found so DeleteKustomization's termination wait completes immediately.
	wire := func(manager *BaseKubernetesManager, items ...unstructured.Unstructured) *[]string {
		deleted := []string{}
		c := client.NewMockKubernetesClient()
		c.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: items}, nil
		}
		c.DeleteResourceFunc = func(gvr schema.GroupVersionResource, ns, name string, opts metav1.DeleteOptions) error {
			deleted = append(deleted, name)
			return nil
		}
		c.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = c
		return &deleted
	}

	t.Run("PrunesOrphansAndKeepsDesired", func(t *testing.T) {
		// Given a context with a desired kustomization and an orphan, both owned by this context
		manager := setup(t)
		deleted := wire(manager,
			kustomizationObj("app", "test-context-id"),
			kustomizationObj("old-thing", "test-context-id"),
		)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "app"}},
		}

		// When pruning
		if err := manager.PruneBlueprint(blueprint, "system-gitops"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then only the orphan is deleted; the desired kustomization is kept
		if len(*deleted) != 1 || (*deleted)[0] != "old-thing" {
			t.Errorf("Expected only 'old-thing' pruned, got %v", *deleted)
		}
	})

	t.Run("IgnoresOtherContextsAndUnlabeledKustomizations", func(t *testing.T) {
		// Given orphan-shaped kustomizations owned by another context and by no context
		manager := setup(t)
		deleted := wire(manager,
			kustomizationObj("other-orphan", "different-context-id"),
			kustomizationObj("unmanaged", ""),
		)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{}}

		// When pruning
		if err := manager.PruneBlueprint(blueprint, "system-gitops"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then nothing is deleted — pruning is scoped to this context's labeled objects
		if len(*deleted) != 0 {
			t.Errorf("Expected no deletions, got %v", *deleted)
		}
	})

	t.Run("DeletesOrphansInReverseDependencyOrder", func(t *testing.T) {
		// Given two orphans where 'leaf' dependsOn 'base'
		manager := setup(t)
		deleted := wire(manager,
			kustomizationObj("base", "test-context-id"),
			kustomizationObj("leaf", "test-context-id", "base"),
		)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{}}

		// When pruning
		if err := manager.PruneBlueprint(blueprint, "system-gitops"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the dependent is deleted before its dependency
		if len(*deleted) != 2 || (*deleted)[0] != "leaf" || (*deleted)[1] != "base" {
			t.Errorf("Expected delete order [leaf base], got %v", *deleted)
		}
	})

	t.Run("DestroyOnlyKustomizationsAreNotDesired", func(t *testing.T) {
		// Given a live kustomization whose blueprint entry is DestroyOnly (never applied by Install)
		manager := setup(t)
		destroyOnly := true
		deleted := wire(manager, kustomizationObj("backup", "test-context-id"))
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "backup", DestroyOnly: &destroyOnly}},
		}

		// When pruning
		if err := manager.PruneBlueprint(blueprint, "system-gitops"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it is treated as an orphan and pruned (it is not part of the applied set)
		if len(*deleted) != 1 || (*deleted)[0] != "backup" {
			t.Errorf("Expected 'backup' pruned, got %v", *deleted)
		}
	})

	t.Run("NilBlueprintReturnsError", func(t *testing.T) {
		manager := setup(t)
		wire(manager)
		if err := manager.PruneBlueprint(nil, "system-gitops"); err == nil {
			t.Error("Expected error for nil blueprint")
		}
	})

	t.Run("EmptyContextIDReturnsError", func(t *testing.T) {
		// Given a manager whose config handler reports no context id
		mocks := setupKubernetesMocks(t, func(m *KubernetesTestMocks) {
			m.ConfigHandler.(*config.MockConfigHandler).GetStringFunc = func(key string, defaultValue ...string) string {
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		})
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{}}

		// When pruning without a context id, the guard rejects it before listing anything
		if err := manager.PruneBlueprint(blueprint, "system-gitops"); err == nil {
			t.Error("Expected error when context id is not set")
		}
	})

	t.Run("ListResourcesErrorIsPropagated", func(t *testing.T) {
		// Given a client whose kustomization list fails
		manager := setup(t)
		c := client.NewMockKubernetesClient()
		c.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return nil, fmt.Errorf("api server unreachable")
		}
		manager.client = c
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{}}

		// When pruning, the list failure is surfaced rather than swallowed
		if err := manager.PruneBlueprint(blueprint, "system-gitops"); err == nil {
			t.Error("Expected error when listing kustomizations fails")
		}
	})
}
