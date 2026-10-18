package kubernetes_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestKubernetesManager_Sources(t *testing.T) {
	sourceObject := func(kind, name, readyStatus string) map[string]any {
		return map[string]any{
			"apiVersion": "source.toolkit.fluxcd.io/v1",
			"kind":       kind,
			"metadata":   map[string]any{"name": name, "namespace": "system-gitops"},
			"status": map[string]any{"conditions": []any{map[string]any{
				"type": "Ready", "status": readyStatus, "reason": "Fetch", "message": "fetch failed", "lastTransitionTime": "2026-01-01T00:00:00Z",
			}}},
		}
	}

	t.Run("WritesAGitRepository", func(t *testing.T) {
		// Given an empty cluster
		f := newFixture(t)
		repo := &sourcev1.GitRepository{
			TypeMeta:   metav1.TypeMeta{APIVersion: "source.toolkit.fluxcd.io/v1", Kind: "GitRepository"},
			ObjectMeta: metav1.ObjectMeta{Name: "core", Namespace: "system-gitops"},
			Spec:       sourcev1.GitRepositorySpec{URL: "https://github.com/windsorcli/core"},
		}

		// When the repository is applied
		err := f.manager.ApplyGitRepository(repo)

		// Then the cluster holds it with its URL
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		url, _, _ := unstructured.NestedString(f.cluster.get(gitRepositoriesGVR, "system-gitops", "core").Object, "spec", "url")
		if url != "https://github.com/windsorcli/core" {
			t.Errorf("expected the URL to be stored, got %q", url)
		}
	})

	t.Run("RejectsAnOCIRepositoryWithoutAName", func(t *testing.T) {
		// Given an OCIRepository with no name
		f := newFixture(t)
		repo := &sourcev1.OCIRepository{TypeMeta: metav1.TypeMeta{APIVersion: "source.toolkit.fluxcd.io/v1", Kind: "OCIRepository"}}

		// When the repository is applied
		err := f.manager.ApplyOCIRepository(repo)

		// Then a field validation error is returned
		if err == nil || !strings.Contains(err.Error(), "invalid ocirepository fields") {
			t.Errorf("expected a validation error, got %v", err)
		}
	})

	t.Run("PassesWhenNoSourceReportsNotReady", func(t *testing.T) {
		// Given a ready GitRepository and a ready OCIRepository
		f := newFixture(t)
		f.cluster.seed(gitRepositoriesGVR, sourceObject("GitRepository", "core", "True"))
		f.cluster.seed(ociRepositoriesGVR, sourceObject("OCIRepository", "addons", "True"))

		// When the source status is checked
		err := f.manager.CheckGitRepositoryStatus()

		// Then no error is returned
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ReportsANotReadySourceByName", func(t *testing.T) {
		// Given a ready GitRepository and a failed OCIRepository
		f := newFixture(t)
		f.cluster.seed(gitRepositoriesGVR, sourceObject("GitRepository", "core", "True"))
		f.cluster.seed(ociRepositoriesGVR, sourceObject("OCIRepository", "addons", "False"))

		// When the source status is checked
		err := f.manager.CheckGitRepositoryStatus()

		// Then the error is the source name and its condition message
		if err == nil || err.Error() != "addons: fetch failed" {
			t.Errorf("expected the OCIRepository failure, got %v", err)
		}
	})

	t.Run("ReturnsTheListError", func(t *testing.T) {
		// Given a cluster whose GitRepository list fails
		f := newFixture(t)
		f.cluster.fail = func(op string, gvr schema.GroupVersionResource, _, _ string) error {
			if op == "list" && gvr.Resource == "gitrepositories" {
				return errors.New("forbidden")
			}
			return nil
		}

		// When the source status is checked
		err := f.manager.CheckGitRepositoryStatus()

		// Then the wrapped list error is returned
		if err == nil || err.Error() != "failed to list git repositories: forbidden" {
			t.Errorf("expected a list error, got %v", err)
		}
	})
}

func TestKubernetesManager_ApplyBlueprint(t *testing.T) {
	destroyOnly := true
	blueprint := func() *blueprintv1alpha1.Blueprint {
		return &blueprintv1alpha1.Blueprint{
			Metadata:   blueprintv1alpha1.Metadata{Name: "local"},
			Repository: blueprintv1alpha1.Repository{Url: "https://github.com/acme/infra.git", Ref: blueprintv1alpha1.Reference{Branch: "main"}},
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v1.2.0"},
				{Name: "template"},
			},
			ConfigMaps: map[string]map[string]string{"values-common": {"DOMAIN": "test"}},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "cluster", Path: "cluster", Source: "core", Substitutions: map[string]string{"REPLICAS": "2"}},
				{Name: "apps", Path: "apps", DependsOn: []string{"cluster"}},
				{Name: "cleanup", Path: "cleanup", DestroyOnly: &destroyOnly},
			},
		}
	}

	t.Run("AppliesTheNamespaceSourcesConfigMapsAndKustomizations", func(t *testing.T) {
		// Given a blueprint with a repository, a remote and a local source, and three kustomizations
		f := newFixture(t)

		// When the blueprint is applied
		err := f.manager.ApplyBlueprint(blueprint(), "system-gitops")

		// Then the namespace, both remote sources, both ConfigMaps, and the two regular kustomizations exist
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		checks := []struct {
			gvr  schema.GroupVersionResource
			want []string
		}{
			{namespacesGVR, []string{"system-gitops"}},
			{gitRepositoriesGVR, []string{"local"}},
			{ociRepositoriesGVR, []string{"core"}},
			{configMapsGVR, []string{"values-cluster", "values-common"}},
			{kustomizationsGVR, []string{"apps", "cluster"}},
		}
		for _, check := range checks {
			if got := f.cluster.names(check.gvr); !reflect.DeepEqual(got, check.want) {
				t.Errorf("%s: expected %v, got %v", check.gvr.Resource, check.want, got)
			}
		}
	})

	t.Run("SplitsTheOCITagIntoTheReference", func(t *testing.T) {
		// Given a blueprint with a tagged OCI source
		f := newFixture(t)

		// When the blueprint is applied
		if err := f.manager.ApplyBlueprint(blueprint(), "system-gitops"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the OCIRepository URL drops the tag and the reference holds it
		repo := f.cluster.get(ociRepositoriesGVR, "system-gitops", "core").Object
		url, _, _ := unstructured.NestedString(repo, "spec", "url")
		tag, _, _ := unstructured.NestedString(repo, "spec", "ref", "tag")
		if url != "oci://ghcr.io/windsorcli/core" || tag != "v1.2.0" {
			t.Errorf("expected url without tag and tag v1.2.0, got %q and %q", url, tag)
		}
	})

	t.Run("LabelsEachKustomizationAndItsManagedResources", func(t *testing.T) {
		// Given a blueprint
		f := newFixture(t)

		// When the blueprint is applied
		if err := f.manager.ApplyBlueprint(blueprint(), "system-gitops"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the Kustomization and its commonMetadata carry the context labels, and it references its source
		obj := f.cluster.get(kustomizationsGVR, "system-gitops", "cluster")
		common, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "commonMetadata", "labels")
		if !reflect.DeepEqual(obj.GetLabels(), contextLabels()) || !reflect.DeepEqual(common, contextLabels()) {
			t.Errorf("expected context labels, got %v and %v", obj.GetLabels(), common)
		}
		kind, _, _ := unstructured.NestedString(obj.Object, "spec", "sourceRef", "kind")
		name, _, _ := unstructured.NestedString(obj.Object, "spec", "sourceRef", "name")
		if kind != "OCIRepository" || name != "core" {
			t.Errorf("expected sourceRef OCIRepository/core, got %s/%s", kind, name)
		}
	})

	t.Run("StopsAtTheFirstFailedStep", func(t *testing.T) {
		// Given a cluster that rejects the OCI source
		f := newFixture(t)
		f.cluster.fail = failOn("apply", "core", errors.New("forbidden"))

		// When the blueprint is applied
		err := f.manager.ApplyBlueprint(blueprint(), "system-gitops")

		// Then the error names the source and no kustomization is written
		if err == nil || err.Error() != "failed to apply source core: forbidden" {
			t.Errorf("expected a source error, got %v", err)
		}
		if got := f.cluster.names(kustomizationsGVR); len(got) != 0 {
			t.Errorf("expected no kustomizations, got %v", got)
		}
	})

	t.Run("WrapsANamespaceFailure", func(t *testing.T) {
		// Given a cluster that rejects the namespace
		f := newFixture(t)
		f.cluster.fail = failOn("apply", "system-gitops", errors.New("forbidden"))

		// When the blueprint is applied
		err := f.manager.ApplyBlueprint(blueprint(), "system-gitops")

		// Then the namespace error is returned
		if err == nil || err.Error() != "failed to create namespace: forbidden" {
			t.Errorf("expected a namespace error, got %v", err)
		}
	})
}

func TestKubernetesManager_DeleteBlueprint(t *testing.T) {
	keep := false
	blueprint := &blueprintv1alpha1.Blueprint{
		Metadata: blueprintv1alpha1.Metadata{Name: "local"},
		Kustomizations: []blueprintv1alpha1.Kustomization{
			{Name: "cluster"},
			{Name: "ingress", DependsOn: []string{"cluster"}},
			{Name: "apps", DependsOn: []string{"ingress"}},
			{Name: "keep", Destroy: &blueprintv1alpha1.BoolExpression{Value: &keep}},
		},
	}
	seed := func(f *fixture) {
		for _, kustomization := range blueprint.Kustomizations {
			f.cluster.seed(kustomizationsGVR, kustomizationObject(kustomization.Name, contextLabels(), kustomization.DependsOn))
		}
	}

	t.Run("DeletesDependentsFirstAndSkipsAKeptKustomization", func(t *testing.T) {
		// Given a deployed blueprint with one kustomization marked to keep
		f := newFixture(t)
		seed(f)

		// When the blueprint is deleted
		err := f.manager.DeleteBlueprint(context.Background(), blueprint, "system-gitops")

		// Then the chain is deleted from the leaf up and the kept kustomization remains
		want := []string{"kustomizations/apps", "kustomizations/ingress", "kustomizations/cluster"}
		if err != nil || !reflect.DeepEqual(f.cluster.deletes, want) {
			t.Errorf("expected %v, got %v (%v)", want, f.cluster.deletes, err)
		}
		if f.cluster.get(kustomizationsGVR, "system-gitops", "keep") == nil {
			t.Error("expected the kept kustomization to remain")
		}
	})

	t.Run("SuspendsEachKustomizationThenResumesItBeforeItsDelete", func(t *testing.T) {
		// Given a deployed blueprint
		f := newFixture(t)
		seed(f)

		// When the blueprint is deleted
		if err := f.manager.DeleteBlueprint(context.Background(), blueprint, "system-gitops"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then every eligible kustomization is suspended first, then resumed one at a time
		want := []string{
			`kustomizations/cluster {"spec":{"suspend":true}}`,
			`kustomizations/ingress {"spec":{"suspend":true}}`,
			`kustomizations/apps {"spec":{"suspend":true}}`,
			`kustomizations/apps {"spec":{"suspend":false}}`,
			`kustomizations/ingress {"spec":{"suspend":false}}`,
			`kustomizations/cluster {"spec":{"suspend":false}}`,
		}
		if !reflect.DeepEqual(f.cluster.patches, want) {
			t.Errorf("expected %v, got %v", want, f.cluster.patches)
		}
	})

	t.Run("LeavesTheDependenciesOfAFailedDeleteInPlace", func(t *testing.T) {
		// Given a deployed blueprint whose ingress delete fails
		f := newFixture(t)
		seed(f)
		f.cluster.fail = failOn("delete", "ingress", errors.New("forbidden"))

		// When the blueprint is deleted
		err := f.manager.DeleteBlueprint(context.Background(), blueprint, "system-gitops")

		// Then the error counts the failure, and cluster, which ingress depends on, is not deleted
		if err == nil || !strings.HasPrefix(err.Error(), "1 kustomization(s) failed to delete.") || !strings.Contains(err.Error(), `failed to delete kustomization "ingress"`) {
			t.Errorf("expected a delete failure, got %v", err)
		}
		if !reflect.DeepEqual(f.cluster.deletes, []string{"kustomizations/apps"}) {
			t.Errorf("expected only apps deleted, got %v", f.cluster.deletes)
		}
	})

	t.Run("AbortsAndResumesWhenASuspendFails", func(t *testing.T) {
		// Given a deployed blueprint whose ingress suspend fails
		f := newFixture(t)
		seed(f)
		f.cluster.fail = func(op string, _ schema.GroupVersionResource, _, name string) error {
			if op == "patch" && name == "ingress" {
				return errors.New("forbidden")
			}
			return nil
		}

		// When the blueprint is deleted
		err := f.manager.DeleteBlueprint(context.Background(), blueprint, "system-gitops")

		// Then the destroy aborts, nothing is deleted, and the suspended kustomizations are resumed
		if err == nil || !strings.Contains(err.Error(), `destroy aborted: failed to suspend kustomization "ingress"`) {
			t.Errorf("expected an abort error, got %v", err)
		}
		if len(f.cluster.deletes) != 0 {
			t.Errorf("expected no deletes, got %v", f.cluster.deletes)
		}
		if spec := f.cluster.get(kustomizationsGVR, "system-gitops", "cluster").Object["spec"].(map[string]any); spec["suspend"] != false {
			t.Errorf("expected cluster to be resumed, got suspend=%v", spec["suspend"])
		}
	})
}

func TestKubernetesManager_Health(t *testing.T) {
	shortContext := func(t *testing.T) context.Context {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		t.Cleanup(cancel)
		return ctx
	}

	t.Run("ReturnsOnceTheAPIAndNodesAreReady", func(t *testing.T) {
		// Given a healthy API and two ready nodes
		f := newFixture(t)
		f.cluster.nodes = map[string]bool{"cp-1": true, "worker-1": true}
		var lines []string

		// When it waits for health with both nodes
		err := f.manager.WaitForKubernetesHealthy(shortContext(t), "https://10.5.0.2:6443", func(line string) { lines = append(lines, line) }, "cp-1", "worker-1")

		// Then it returns and reports each node once
		want := []string{"Node cp-1: READY", "Node worker-1: READY"}
		if err != nil || !reflect.DeepEqual(lines, want) {
			t.Errorf("expected %v, got %v (%v)", want, lines, err)
		}
	})

	t.Run("ReportsTheLastHealthCheckFailureOnTimeout", func(t *testing.T) {
		// Given an API that fails its health check
		f := newFixture(t)
		f.cluster.health = func(string) error { return errors.New("connection refused") }

		// When it waits for health
		err := f.manager.WaitForKubernetesHealthy(shortContext(t), "https://10.5.0.2:6443", nil)

		// Then the timeout error carries the last failure
		want := "timeout waiting for Kubernetes API to be healthy: health check for API endpoint https://10.5.0.2:6443 failed: connection refused"
		if err == nil || err.Error() != want {
			t.Errorf("expected %q, got %v", want, err)
		}
	})

	t.Run("ReportsAMissingNodeOnTimeout", func(t *testing.T) {
		// Given a healthy API and no nodes
		f := newFixture(t)

		// When it waits for a node that never appears
		err := f.manager.WaitForKubernetesHealthy(shortContext(t), "https://10.5.0.2:6443", nil, "cp-1")

		// Then the timeout error says the wait was cancelled
		want := "timeout waiting for Kubernetes API to be healthy: context cancelled while waiting for nodes to be ready"
		if err == nil || err.Error() != want {
			t.Errorf("expected %q, got %v", want, err)
		}
	})

	t.Run("ReturnsTheNodeReadyStatus", func(t *testing.T) {
		// Given one ready and one not-ready node
		f := newFixture(t)
		f.cluster.nodes = map[string]bool{"cp-1": true, "worker-1": false}

		// When the node status is read
		status, err := f.manager.GetNodeReadyStatus(context.Background(), []string{"cp-1", "worker-1"})

		// Then the client status is returned
		want := map[string]bool{"cp-1": true, "worker-1": false}
		if err != nil || !reflect.DeepEqual(status, want) {
			t.Errorf("expected %v, got %v (%v)", want, status, err)
		}
	})
}
