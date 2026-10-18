package kubernetes_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestKubernetesManager_ApplyKustomization(t *testing.T) {
	kustomization := func(name string) kustomizev1.Kustomization {
		return kustomizev1.Kustomization{
			TypeMeta:   metav1.TypeMeta{APIVersion: "kustomize.toolkit.fluxcd.io/v1", Kind: "Kustomization"},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "system-gitops"},
			Spec:       kustomizev1.KustomizationSpec{Path: "kustomize/cluster"},
		}
	}

	t.Run("WritesTheKustomization", func(t *testing.T) {
		// Given an empty cluster
		f := newFixture(t)

		// When a Kustomization is applied
		err := f.manager.ApplyKustomization(kustomization("cluster"))

		// Then the cluster holds it with its path
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		path, _, _ := unstructured.NestedString(f.cluster.get(kustomizationsGVR, "system-gitops", "cluster").Object, "spec", "path")
		if path != "kustomize/cluster" {
			t.Errorf("expected the path to be stored, got %q", path)
		}
	})

	t.Run("RejectsAKustomizationWithoutAName", func(t *testing.T) {
		// Given an empty cluster
		f := newFixture(t)

		// When a Kustomization with an empty name is applied
		err := f.manager.ApplyKustomization(kustomization(""))

		// Then a field validation error is returned
		if err == nil || !strings.Contains(err.Error(), "invalid kustomization fields") {
			t.Errorf("expected a validation error, got %v", err)
		}
	})

	t.Run("ReportsWhetherAKustomizationExists", func(t *testing.T) {
		// Given a cluster with one Kustomization
		f := newFixture(t)
		f.cluster.seed(kustomizationsGVR, kustomizationObject("cluster", nil, nil))

		// When a present and an absent Kustomization are checked
		present, errPresent := f.manager.KustomizationExists("cluster", "system-gitops")
		absent, errAbsent := f.manager.KustomizationExists("dns", "system-gitops")

		// Then only the present one exists and neither check fails
		if !present || absent || errPresent != nil || errAbsent != nil {
			t.Errorf("expected (true, false), got (%v, %v) with errors %v, %v", present, absent, errPresent, errAbsent)
		}
	})
}

func TestKubernetesManager_KustomizationStatus(t *testing.T) {
	t.Run("ReportsReadinessForEachNameAndFalseForAMissingOne", func(t *testing.T) {
		// Given one ready and one progressing Kustomization
		f := newFixture(t)
		f.cluster.seed(kustomizationsGVR, kustomizationObject("cluster", nil, nil, "Ready=True:ReconciliationSucceeded"))
		f.cluster.seed(kustomizationsGVR, kustomizationObject("dns", nil, nil, "Ready=Unknown:Progressing"))

		// When the status of three names is read
		status, err := f.manager.GetKustomizationStatus([]string{"cluster", "dns", "missing"})

		// Then each name maps to its readiness
		want := map[string]bool{"cluster": true, "dns": false, "missing": false}
		if err != nil || !reflect.DeepEqual(status, want) {
			t.Errorf("expected %v, got %v (%v)", want, status, err)
		}
	})

	t.Run("FailsOnAFailedKustomization", func(t *testing.T) {
		// Given a Kustomization whose build failed
		f := newFixture(t)
		f.cluster.seed(kustomizationsGVR, kustomizationObject("dns", nil, nil, "Ready=False:BuildFailed"))

		// When its status is read
		_, err := f.manager.GetKustomizationStatus([]string{"dns"})

		// Then the error names the Kustomization and carries its condition message
		if err == nil || err.Error() != "kustomization dns failed: dns is BuildFailed" {
			t.Errorf("expected a failure error, got %v", err)
		}
	})

	t.Run("ReadinessReportsAFailedKustomizationAsNotReady", func(t *testing.T) {
		// Given one ready and one failed Kustomization
		f := newFixture(t)
		f.cluster.seed(kustomizationsGVR, kustomizationObject("cluster", nil, nil, "Ready=True:ReconciliationSucceeded"))
		f.cluster.seed(kustomizationsGVR, kustomizationObject("dns", nil, nil, "Ready=False:BuildFailed"))

		// When their readiness is read
		ready, err := f.manager.GetKustomizationReadiness([]string{"cluster", "dns", "missing"})

		// Then the failed one is not ready and no error is returned
		want := map[string]bool{"cluster": true, "dns": false, "missing": false}
		if err != nil || !reflect.DeepEqual(ready, want) {
			t.Errorf("expected %v, got %v (%v)", want, ready, err)
		}
	})

	t.Run("ReadsTheConfiguredGitopsNamespace", func(t *testing.T) {
		// Given a ready Kustomization in a custom gitops namespace
		f := newFixture(t)
		f.values["gitops.namespace"] = "gitops"
		obj := kustomizationObject("cluster", nil, nil, "Ready=True:ReconciliationSucceeded")
		obj["metadata"].(map[string]any)["namespace"] = "gitops"
		f.cluster.seed(kustomizationsGVR, obj)

		// When its readiness is read
		ready, err := f.manager.GetKustomizationReadiness([]string{"cluster"})

		// Then it is found in the custom namespace
		if err != nil || !ready["cluster"] {
			t.Errorf("expected cluster ready, got %v (%v)", ready, err)
		}
	})

	t.Run("ReturnsTheListError", func(t *testing.T) {
		// Given a cluster whose list fails
		f := newFixture(t)
		f.cluster.fail = failOn("list", "", errors.New("forbidden"))

		// When status and readiness are read
		_, statusErr := f.manager.GetKustomizationStatus([]string{"cluster"})
		_, readyErr := f.manager.GetKustomizationReadiness([]string{"cluster"})

		// Then both return the wrapped list error
		for _, err := range []error{statusErr, readyErr} {
			if err == nil || err.Error() != "failed to list kustomizations: forbidden" {
				t.Errorf("expected a list error, got %v", err)
			}
		}
	})
}

func TestKubernetesManager_GetKustomizationInventory(t *testing.T) {
	t.Run("ReturnsNilForAMissingKustomization", func(t *testing.T) {
		// Given an empty cluster
		f := newFixture(t)

		// When the inventory is read
		entries, err := f.manager.GetKustomizationInventory("cluster", "system-gitops")

		// Then nil is returned without an error
		if entries != nil || err != nil {
			t.Errorf("expected (nil, nil), got (%v, %v)", entries, err)
		}
	})

	t.Run("ReturnsAnEmptyListBeforeTheFirstReconcile", func(t *testing.T) {
		// Given a Kustomization with no inventory
		f := newFixture(t)
		f.cluster.seed(kustomizationsGVR, kustomizationObject("cluster", nil, nil))

		// When the inventory is read
		entries, err := f.manager.GetKustomizationInventory("cluster", "system-gitops")

		// Then an empty, non-nil list is returned
		if entries == nil || len(entries) != 0 || err != nil {
			t.Errorf("expected an empty list, got (%v, %v)", entries, err)
		}
	})

	t.Run("DecodesEntriesAndDropsMalformedOnes", func(t *testing.T) {
		// Given an inventory with a namespaced entry, a cluster-scoped RBAC entry, and a malformed one
		f := newFixture(t)
		f.cluster.seed(kustomizationsGVR, withInventory(kustomizationObject("cluster", nil, nil),
			"apps_web_apps_Deployment",
			"_system__controller_rbac.authorization.k8s.io_ClusterRole",
			"broken",
		))

		// When the inventory is read
		entries, err := f.manager.GetKustomizationInventory("cluster", "system-gitops")

		// Then the two valid entries decode, with the RBAC name colon restored
		want := []kubernetes.InventoryEntry{
			{Namespace: "apps", Name: "web", Group: "apps", Kind: "Deployment"},
			{Namespace: "", Name: "system:controller", Group: "rbac.authorization.k8s.io", Kind: "ClusterRole"},
		}
		if err != nil || !reflect.DeepEqual(entries, want) {
			t.Errorf("expected %v, got %v (%v)", want, entries, err)
		}
	})
}

func TestKubernetesManager_GetHelmReleasesForKustomization(t *testing.T) {
	t.Run("ReturnsTheHelmReleasesInTheInventory", func(t *testing.T) {
		// Given a Kustomization whose inventory holds a HelmRelease and a Deployment
		f := newFixture(t)
		f.cluster.seed(kustomizationsGVR, withInventory(kustomizationObject("ingress", nil, nil),
			"system-ingress_nginx_helm.toolkit.fluxcd.io_HelmRelease",
			"apps_web_apps_Deployment",
		))
		f.cluster.seed(helmReleasesGVR, map[string]any{"apiVersion": "helm.toolkit.fluxcd.io/v2", "kind": "HelmRelease", "metadata": map[string]any{"name": "nginx", "namespace": "system-ingress"}})

		// When its HelmReleases are read
		releases, err := f.manager.GetHelmReleasesForKustomization("ingress", "system-gitops")

		// Then only the HelmRelease is returned
		if err != nil || len(releases) != 1 || releases[0].Name != "nginx" {
			t.Errorf("expected the nginx release, got %v (%v)", releases, err)
		}
	})

	t.Run("ReturnsAnEmptyListForAMissingKustomization", func(t *testing.T) {
		// Given an empty cluster
		f := newFixture(t)

		// When HelmReleases are read
		releases, err := f.manager.GetHelmReleasesForKustomization("ingress", "system-gitops")

		// Then an empty list is returned without an error
		if releases == nil || len(releases) != 0 || err != nil {
			t.Errorf("expected an empty list, got (%v, %v)", releases, err)
		}
	})

	t.Run("FailsWhenAListedHelmReleaseIsMissing", func(t *testing.T) {
		// Given an inventory that names a HelmRelease the cluster does not hold
		f := newFixture(t)
		f.cluster.seed(kustomizationsGVR, withInventory(kustomizationObject("ingress", nil, nil), "system-ingress_nginx_helm.toolkit.fluxcd.io_HelmRelease"))

		// When HelmReleases are read
		_, err := f.manager.GetHelmReleasesForKustomization("ingress", "system-gitops")

		// Then the lookup error is returned
		if err == nil || !strings.Contains(err.Error(), "failed to get helm release") {
			t.Errorf("expected a lookup error, got %v", err)
		}
	})
}

func TestKubernetesManager_DeleteKustomization(t *testing.T) {
	t.Run("ReturnsNilForAMissingKustomization", func(t *testing.T) {
		// Given an empty cluster
		f := newFixture(t)

		// When a missing Kustomization is deleted
		err := f.manager.DeleteKustomization(context.Background(), "cluster", "system-gitops")

		// Then no error is returned
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("DeletesAKustomizationThatDisappears", func(t *testing.T) {
		// Given a Kustomization with no inventory
		f := newFixture(t)
		f.cluster.seed(kustomizationsGVR, kustomizationObject("cluster", nil, nil))

		// When it is deleted
		err := f.manager.DeleteKustomization(context.Background(), "cluster", "system-gitops")

		// Then it is gone and no error is returned
		if err != nil || f.cluster.get(kustomizationsGVR, "system-gitops", "cluster") != nil {
			t.Errorf("expected a clean delete, got %v", err)
		}
	})

	t.Run("ReturnsTheDeleteError", func(t *testing.T) {
		// Given a Kustomization whose delete fails
		f := newFixture(t)
		f.cluster.seed(kustomizationsGVR, kustomizationObject("cluster", nil, nil))
		f.cluster.fail = failOn("delete", "cluster", errors.New("forbidden"))

		// When it is deleted
		err := f.manager.DeleteKustomization(context.Background(), "cluster", "system-gitops")

		// Then the error names the Kustomization
		if err == nil || err.Error() != "error deleting kustomization system-gitops/cluster: forbidden" {
			t.Errorf("expected a delete error, got %v", err)
		}
	})
}

func TestKubernetesManager_WaitForKustomizations(t *testing.T) {
	t.Run("RejectsANilBlueprint", func(t *testing.T) {
		// Given a manager
		f := newFixture(t)

		// When it waits without a blueprint
		err := f.manager.WaitForKustomizations(context.Background(), "Waiting", nil)

		// Then an error is returned
		if err == nil || err.Error() != "blueprint not provided" {
			t.Errorf("expected a missing-blueprint error, got %v", err)
		}
	})

	t.Run("ReturnsTheContextErrorWhenCancelled", func(t *testing.T) {
		// Given a cancelled context
		f := newFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "cluster"}}}

		// When it waits
		err := f.manager.WaitForKustomizations(ctx, "Waiting", blueprint)

		// Then the context error is returned
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})
}

func TestKubernetesManager_PruneBlueprint(t *testing.T) {
	seedContext := func(f *fixture) {
		f.cluster.seed(kustomizationsGVR, kustomizationObject("cluster", contextLabels(), nil))
		f.cluster.seed(kustomizationsGVR, kustomizationObject("old-base", contextLabels(), nil))
		f.cluster.seed(kustomizationsGVR, kustomizationObject("old-app", contextLabels(), []string{"old-base"}))
		f.cluster.seed(kustomizationsGVR, kustomizationObject("hook", contextLabels(), nil))
		f.cluster.seed(kustomizationsGVR, kustomizationObject("other-context", map[string]string{"windsorcli.dev/context-id": "ctx-2"}, nil))
		f.cluster.seed(kustomizationsGVR, kustomizationObject("unlabeled", nil, nil))
	}
	destroyOnly := true
	blueprint := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
		{Name: "cluster"},
		{Name: "hook", DestroyOnly: &destroyOnly},
	}}

	t.Run("ListsThisContextsOrphansDependentsFirst", func(t *testing.T) {
		// Given this context's Kustomizations, one from another context, and an unlabeled one
		f := newFixture(t)
		seedContext(f)

		// When the prunable Kustomizations are listed
		names, err := f.manager.ListPrunableKustomizations(blueprint, "system-gitops")

		// Then this context's undeclared ones are listed, the dependent before its dependency
		want := []string{"old-app", "old-base", "hook"}
		if err != nil || !reflect.DeepEqual(names, want) {
			t.Errorf("expected %v, got %v (%v)", want, names, err)
		}
		if len(f.cluster.deletes) != 0 {
			t.Errorf("expected no deletes, got %v", f.cluster.deletes)
		}
	})

	t.Run("DeletesThisContextsOrphansDependentsFirst", func(t *testing.T) {
		// Given this context's Kustomizations, one from another context, and an unlabeled one
		f := newFixture(t)
		seedContext(f)

		// When the blueprint is pruned
		err := f.manager.PruneBlueprint(context.Background(), blueprint, "system-gitops")

		// Then the orphans are deleted in reverse-dependency order and the rest remain
		want := []string{"kustomizations/old-app", "kustomizations/old-base", "kustomizations/hook"}
		if err != nil || !reflect.DeepEqual(f.cluster.deletes, want) {
			t.Errorf("expected %v, got %v (%v)", want, f.cluster.deletes, err)
		}
		if got := f.cluster.names(kustomizationsGVR); !reflect.DeepEqual(got, []string{"cluster", "other-context", "unlabeled"}) {
			t.Errorf("unexpected remaining kustomizations %v", got)
		}
	})

	t.Run("CollectsEachDeleteFailure", func(t *testing.T) {
		// Given orphans whose deletes fail for one of them
		f := newFixture(t)
		seedContext(f)
		f.cluster.fail = failOn("delete", "old-app", errors.New("forbidden"))

		// When the blueprint is pruned
		err := f.manager.PruneBlueprint(context.Background(), blueprint, "system-gitops")

		// Then the failure is reported and the other orphans are still deleted
		if err == nil || !strings.Contains(err.Error(), `failed to prune kustomization "old-app"`) {
			t.Errorf("expected a prune error, got %v", err)
		}
		if !reflect.DeepEqual(f.cluster.deletes, []string{"kustomizations/old-base", "kustomizations/hook"}) {
			t.Errorf("unexpected deletes %v", f.cluster.deletes)
		}
	})

	t.Run("FailsWithoutAContextID", func(t *testing.T) {
		// Given a context without an id
		f := newFixture(t)
		f.values["id"] = ""

		// When the blueprint is pruned and listed
		pruneErr := f.manager.PruneBlueprint(context.Background(), blueprint, "system-gitops")
		_, listErr := f.manager.ListPrunableKustomizations(blueprint, "system-gitops")

		// Then both fail closed
		for _, err := range []error{pruneErr, listErr} {
			if err == nil || !strings.Contains(err.Error(), "context id not set") {
				t.Errorf("expected a missing-id error, got %v", err)
			}
		}
	})

	t.Run("RejectsANilBlueprint", func(t *testing.T) {
		// Given a manager
		f := newFixture(t)

		// When it prunes and lists without a blueprint
		pruneErr := f.manager.PruneBlueprint(context.Background(), nil, "system-gitops")
		_, listErr := f.manager.ListPrunableKustomizations(nil, "system-gitops")

		// Then both return a missing-blueprint error
		for _, err := range []error{pruneErr, listErr} {
			if err == nil || err.Error() != "blueprint not provided" {
				t.Errorf("expected a missing-blueprint error, got %v", err)
			}
		}
	})
}
