package kubernetes_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"github.com/windsorcli/cli/pkg/runtime/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseKubernetesManager_PruneBlueprint(t *testing.T) {
	// setup returns a manager over kubeClient with real unstructured conversion shims.
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		manager := newManager(setupKubernetesMocks(t), kubeClient)
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.Shims.FromUnstructured = func(obj map[string]any, target any) error {
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

	// wire returns a manager whose client lists items and records deletions in order. GetResource
	// returns not-found, so the termination wait of DeleteKustomization completes at once.
	wire := func(t *testing.T, items ...unstructured.Unstructured) (*kubernetes.BaseKubernetesManager, *[]string) {
		t.Helper()
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
		return setup(t, c), &deleted
	}

	t.Run("PrunesOrphansAndKeepsDesired", func(t *testing.T) {
		// Given a desired kustomization and an orphan, both owned by this context
		manager, deleted := wire(t,
			kustomizationObj("app", "test-context-id"),
			kustomizationObj("old-thing", "test-context-id"),
		)
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "app"}},
		}
		ctx, rec := recordProgress(t)

		// When pruning
		err := manager.PruneBlueprint(ctx, blueprint, "system-gitops")

		// Then only the orphan is deleted and a progress spinner names it
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(*deleted) != 1 || (*deleted)[0] != "old-thing" {
			t.Errorf("Expected only 'old-thing' pruned, got %v", *deleted)
		}
		found := false
		for _, s := range rec.started() {
			if strings.Contains(s, "old-thing") {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected a progress spinner naming the pruned orphan, got: %v", rec.started())
		}
	})

	t.Run("IgnoresOtherContextsAndUnlabeledKustomizations", func(t *testing.T) {
		// Given orphans owned by another context and by no context
		manager, deleted := wire(t,
			kustomizationObj("other-orphan", "different-context-id"),
			kustomizationObj("unmanaged", ""),
		)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{}}

		// When pruning
		err := manager.PruneBlueprint(context.Background(), blueprint, "system-gitops")

		// Then nothing is deleted
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(*deleted) != 0 {
			t.Errorf("Expected no deletions, got %v", *deleted)
		}
	})

	t.Run("DeletesOrphansInReverseDependencyOrder", func(t *testing.T) {
		// Given two orphans where 'leaf' dependsOn 'base'
		manager, deleted := wire(t,
			kustomizationObj("base", "test-context-id"),
			kustomizationObj("leaf", "test-context-id", "base"),
		)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{}}

		// When pruning
		err := manager.PruneBlueprint(context.Background(), blueprint, "system-gitops")

		// Then the dependent is deleted before its dependency
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(*deleted) != 2 || (*deleted)[0] != "leaf" || (*deleted)[1] != "base" {
			t.Errorf("Expected delete order [leaf base], got %v", *deleted)
		}
	})

	t.Run("DestroyOnlyKustomizationsAreNotDesired", func(t *testing.T) {
		// Given a live kustomization whose blueprint entry is DestroyOnly
		destroyOnly := true
		manager, deleted := wire(t, kustomizationObj("backup", "test-context-id"))
		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "backup", DestroyOnly: &destroyOnly}},
		}

		// When pruning
		err := manager.PruneBlueprint(context.Background(), blueprint, "system-gitops")

		// Then it is pruned as an orphan
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(*deleted) != 1 || (*deleted)[0] != "backup" {
			t.Errorf("Expected 'backup' pruned, got %v", *deleted)
		}
	})

	t.Run("NilBlueprintReturnsError", func(t *testing.T) {
		// Given a manager over an empty cluster
		manager, _ := wire(t)

		// When pruning a nil blueprint
		err := manager.PruneBlueprint(context.Background(), nil, "system-gitops")

		// Then it returns an error
		if err == nil {
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
		manager := kubernetes.NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{}}

		// When pruning
		err := manager.PruneBlueprint(context.Background(), blueprint, "system-gitops")

		// Then it returns an error
		if err == nil {
			t.Error("Expected error when context id is not set")
		}
	})

	t.Run("ListResourcesErrorIsPropagated", func(t *testing.T) {
		// Given a client whose kustomization list fails
		c := client.NewMockKubernetesClient()
		c.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return nil, fmt.Errorf("api server unreachable")
		}
		manager := setup(t, c)
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{}}

		// When pruning
		err := manager.PruneBlueprint(context.Background(), blueprint, "system-gitops")

		// Then the list error is returned
		if err == nil {
			t.Error("Expected error when listing kustomizations fails")
		}
	})
}

func TestBaseKubernetesManager_ListPrunableKustomizations(t *testing.T) {
	ctxItem := func(name string) unstructured.Unstructured {
		return unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
			"kind":       "Kustomization",
			"metadata": map[string]any{
				"name":      name,
				"namespace": "system-gitops",
				"labels":    map[string]any{"windsorcli.dev/context-id": "test-context-id"},
			},
		}}
	}

	// wire returns a manager whose client lists items.
	wire := func(t *testing.T, items ...unstructured.Unstructured) *kubernetes.BaseKubernetesManager {
		t.Helper()
		c := client.NewMockKubernetesClient()
		c.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: items}, nil
		}
		return newManager(setupKubernetesMocks(t), c)
	}

	t.Run("ReturnsOrphanNames", func(t *testing.T) {
		// Given a desired kustomization and an orphan, both owned by this context
		manager := wire(t, ctxItem("app"), ctxItem("old-thing"))
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "app"}}}

		// When listing prunable kustomizations
		got, err := manager.ListPrunableKustomizations(blueprint, "system-gitops")

		// Then only the orphan is reported
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(got) != 1 || got[0] != "old-thing" {
			t.Errorf("Expected [old-thing], got %v", got)
		}
	})

	t.Run("EmptyWhenNothingOrphaned", func(t *testing.T) {
		// Given only a desired kustomization
		manager := wire(t, ctxItem("app"))
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "app"}}}

		// When listing prunable kustomizations
		got, err := manager.ListPrunableKustomizations(blueprint, "system-gitops")

		// Then nothing is reported
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(got) != 0 {
			t.Errorf("Expected no prunable kustomizations, got %v", got)
		}
	})

	t.Run("NilBlueprintReturnsError", func(t *testing.T) {
		// Given a manager over an empty cluster
		manager := wire(t)

		// When listing prunable kustomizations for a nil blueprint
		_, err := manager.ListPrunableKustomizations(nil, "system-gitops")

		// Then it returns an error
		if err == nil {
			t.Error("Expected error for nil blueprint")
		}
	})
}
