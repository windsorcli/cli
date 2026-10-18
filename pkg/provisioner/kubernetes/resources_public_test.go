package kubernetes_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestKubernetesManager_Namespaces(t *testing.T) {
	t.Run("CreatesTheNamespaceWithTheManagedByLabel", func(t *testing.T) {
		// Given an empty cluster
		f := newFixture(t)

		// When a namespace is created
		err := f.manager.CreateNamespace("apps")

		// Then the namespace exists and carries the windsor-cli managed-by label
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		ns := f.cluster.get(namespacesGVR, "", "apps")
		if ns == nil || ns.GetLabels()["app.kubernetes.io/managed-by"] != "windsor-cli" {
			t.Errorf("expected a labeled namespace, got %v", ns)
		}
	})

	t.Run("ReturnsTheApplyErrorUnwrapped", func(t *testing.T) {
		// Given a cluster that rejects the apply
		f := newFixture(t)
		boom := errors.New("apply rejected")
		f.cluster.fail = failOn("apply", "apps", boom)

		// When a namespace is created
		err := f.manager.CreateNamespace("apps")

		// Then the client error is returned
		if !errors.Is(err, boom) {
			t.Errorf("expected the apply error, got %v", err)
		}
	})

	t.Run("DeletesTheNamespace", func(t *testing.T) {
		// Given a cluster with a namespace
		f := newFixture(t)
		f.cluster.seed(namespacesGVR, map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": "apps"}})

		// When the namespace is deleted
		err := f.manager.DeleteNamespace("apps")

		// Then the namespace is gone
		if err != nil || f.cluster.get(namespacesGVR, "", "apps") != nil {
			t.Errorf("expected the namespace to be deleted, got err %v", err)
		}
	})

	t.Run("ReturnsNotFoundWhenDeletingAMissingNamespace", func(t *testing.T) {
		// Given an empty cluster
		f := newFixture(t)

		// When a missing namespace is deleted
		err := f.manager.DeleteNamespace("apps")

		// Then the not-found error is returned
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected a not-found error, got %v", err)
		}
	})

	t.Run("ReportsWhetherANamespaceExists", func(t *testing.T) {
		// Given a cluster with one namespace
		f := newFixture(t)
		f.cluster.seed(namespacesGVR, map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": "apps"}})

		// When both a present and an absent namespace are checked
		present, errPresent := f.manager.NamespaceExists("apps")
		absent, errAbsent := f.manager.NamespaceExists("other")

		// Then only the present one exists and neither check fails
		if !present || absent || errPresent != nil || errAbsent != nil {
			t.Errorf("expected (true, false), got (%v, %v) with errors %v, %v", present, absent, errPresent, errAbsent)
		}
	})

	t.Run("PropagatesANamespaceReadError", func(t *testing.T) {
		// Given a cluster whose read fails
		f := newFixture(t)
		boom := errors.New("connection refused")
		f.cluster.fail = failOn("get", "apps", boom)

		// When the namespace is checked
		_, err := f.manager.NamespaceExists("apps")

		// Then the read error is returned
		if !errors.Is(err, boom) {
			t.Errorf("expected the read error, got %v", err)
		}
	})
}

func TestKubernetesManager_ApplyConfigMap(t *testing.T) {
	t.Run("WritesTheData", func(t *testing.T) {
		// Given an empty cluster
		f := newFixture(t)

		// When a ConfigMap is applied
		err := f.manager.ApplyConfigMap("values-common", "flux-system", map[string]string{"DOMAIN": "test"})

		// Then the ConfigMap holds the data
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		cm := f.cluster.get(configMapsGVR, "flux-system", "values-common")
		if value, _, _ := unstructured.NestedString(cm.Object, "data", "DOMAIN"); value != "test" {
			t.Errorf("expected DOMAIN=test, got %v", cm)
		}
	})

	t.Run("ReplacesTheDataOfAnExistingConfigMap", func(t *testing.T) {
		// Given a ConfigMap with an old key
		f := newFixture(t)
		f.cluster.seed(configMapsGVR, map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "values-common", "namespace": "flux-system"}, "data": map[string]any{"OLD": "1"}})

		// When the ConfigMap is applied with a new key
		err := f.manager.ApplyConfigMap("values-common", "flux-system", map[string]string{"NEW": "2"})

		// Then only the new key remains
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		data, _, _ := unstructured.NestedStringMap(f.cluster.get(configMapsGVR, "flux-system", "values-common").Object, "data")
		if len(data) != 1 || data["NEW"] != "2" {
			t.Errorf("expected only NEW=2, got %v", data)
		}
	})

	t.Run("RecreatesAnImmutableConfigMap", func(t *testing.T) {
		// Given an immutable ConfigMap
		f := newFixture(t)
		f.cluster.seed(configMapsGVR, map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "immutable": true, "metadata": map[string]any{"name": "values-common", "namespace": "flux-system"}, "data": map[string]any{"OLD": "1"}})

		// When the ConfigMap is applied
		err := f.manager.ApplyConfigMap("values-common", "flux-system", map[string]string{"NEW": "2"})

		// Then the old ConfigMap is deleted and the new data is written
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(f.cluster.deletes) != 1 || f.cluster.deletes[0] != "configmaps/values-common" {
			t.Errorf("expected one ConfigMap delete, got %v", f.cluster.deletes)
		}
		data, _, _ := unstructured.NestedStringMap(f.cluster.get(configMapsGVR, "flux-system", "values-common").Object, "data")
		if len(data) != 1 || data["NEW"] != "2" {
			t.Errorf("expected only NEW=2, got %v", data)
		}
	})

	t.Run("RejectsAnUnresolvedExpression", func(t *testing.T) {
		// Given data that still holds an expression
		f := newFixture(t)

		// When the ConfigMap is applied
		err := f.manager.ApplyConfigMap("values-common", "flux-system", map[string]string{"ENDPOINT": "${terraform_output('cluster', 'endpoint')}"})

		// Then the error names the ConfigMap and the key, a hint says to resolve it, and nothing is written
		we := windsorError(t, err)
		if we.Message != `ConfigMap values-common has an unresolved expression in key "ENDPOINT".` || !strings.Contains(we.Remediation, "windsor apply") {
			t.Errorf("unexpected error %q with hint %q", we.Message, we.Remediation)
		}
		if f.cluster.get(configMapsGVR, "flux-system", "values-common") != nil {
			t.Error("expected no ConfigMap to be written")
		}
	})

	t.Run("RejectsEmptyData", func(t *testing.T) {
		// Given an empty cluster
		f := newFixture(t)

		// When a ConfigMap with no data is applied
		err := f.manager.ApplyConfigMap("values-common", "flux-system", map[string]string{})

		// Then a field validation error is returned
		if err == nil || !strings.Contains(err.Error(), "invalid configmap fields") {
			t.Errorf("expected a validation error, got %v", err)
		}
	})

	t.Run("RejectsAnEmptyName", func(t *testing.T) {
		// Given an empty cluster
		f := newFixture(t)

		// When a ConfigMap with a blank name is applied
		err := f.manager.ApplyConfigMap(" ", "flux-system", map[string]string{"KEY": "value"})

		// Then a field validation error is returned
		if err == nil || !strings.Contains(err.Error(), "metadata.name cannot be empty") {
			t.Errorf("expected a name validation error, got %v", err)
		}
	})
}

func TestKubernetesManager_ApplySecret(t *testing.T) {
	t.Run("WritesAnOpaqueSecretWithOwnershipLabels", func(t *testing.T) {
		// Given an empty cluster
		f := newFixture(t)

		// When a secret is applied for the cluster kustomization
		err := f.manager.ApplySecret("api-token", "apps", map[string]string{"token": "s3cret"}, "cluster")

		// Then the secret is Opaque, holds the value, and carries the context and owner labels
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		secret := f.cluster.get(secretsGVR, "apps", "api-token")
		if secret.Object["type"] != "Opaque" {
			t.Errorf("expected type Opaque, got %v", secret.Object["type"])
		}
		if value, _, _ := unstructured.NestedString(secret.Object, "stringData", "token"); value != "s3cret" {
			t.Errorf("expected the token value, got %q", value)
		}
		labels := secret.GetLabels()
		if labels["windsorcli.dev/context"] != "local" || labels["windsorcli.dev/context-id"] != "ctx-1" || labels["windsorcli.dev/secret-owner"] != "cluster" {
			t.Errorf("unexpected labels %v", labels)
		}
	})

	t.Run("BuildsADockerConfigSecretFromDockerCredentials", func(t *testing.T) {
		// Given docker credentials without a server
		f := newFixture(t)

		// When the secret is applied
		err := f.manager.ApplySecret("pull", "apps", map[string]string{"docker-username": "me", "docker-password": "pw", "extra": "kept"}, "cluster")

		// Then it is a dockerconfigjson secret whose auth encodes the credentials, and other keys stay
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		secret := f.cluster.get(secretsGVR, "apps", "pull")
		if secret.Object["type"] != "kubernetes.io/dockerconfigjson" {
			t.Fatalf("expected a dockerconfigjson secret, got %v", secret.Object["type"])
		}
		data, _, _ := unstructured.NestedStringMap(secret.Object, "stringData")
		if _, ok := data["docker-username"]; ok || data["extra"] != "kept" {
			t.Errorf("expected docker-* keys replaced and other keys kept, got %v", data)
		}
		var config struct {
			Auths map[string]struct{ Auth string } `json:"auths"`
		}
		if err := json.Unmarshal([]byte(data[".dockerconfigjson"]), &config); err != nil || len(config.Auths) != 1 {
			t.Fatalf("expected one auth entry, got %q (%v)", data[".dockerconfigjson"], err)
		}
		for _, auth := range config.Auths {
			if auth.Auth != base64.StdEncoding.EncodeToString([]byte("me:pw")) {
				t.Errorf("unexpected auth %q", auth.Auth)
			}
		}
	})

	t.Run("RecreatesASecretWhoseTypeChanged", func(t *testing.T) {
		// Given an Opaque secret
		f := newFixture(t)
		f.cluster.seed(secretsGVR, map[string]any{"apiVersion": "v1", "kind": "Secret", "type": "Opaque", "metadata": map[string]any{"name": "pull", "namespace": "apps"}})

		// When docker credentials are applied under the same name
		err := f.manager.ApplySecret("pull", "apps", map[string]string{"docker-username": "me", "docker-password": "pw"}, "cluster")

		// Then the old secret is deleted and a dockerconfigjson secret replaces it
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(f.cluster.deletes) != 1 || f.cluster.deletes[0] != "secrets/pull" {
			t.Errorf("expected one secret delete, got %v", f.cluster.deletes)
		}
		if f.cluster.get(secretsGVR, "apps", "pull").Object["type"] != "kubernetes.io/dockerconfigjson" {
			t.Error("expected the replacement secret to be dockerconfigjson")
		}
	})
}

func TestKubernetesManager_PruneSecrets(t *testing.T) {
	secret := func(name, namespace string, labels map[string]any) map[string]any {
		return map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": name, "namespace": namespace, "labels": labels}}
	}
	owned := map[string]any{"windsorcli.dev/context-id": "ctx-1", "windsorcli.dev/secret-owner": "cluster"}

	t.Run("DeletesOnlyThisContextsPlacedSecretsOutsideTheDesiredSet", func(t *testing.T) {
		// Given placed secrets, a secret from another context, and a Flux secret with context labels
		f := newFixture(t)
		f.cluster.seed(secretsGVR, secret("keep", "apps", owned))
		f.cluster.seed(secretsGVR, secret("stale", "apps", owned))
		f.cluster.seed(secretsGVR, secret("foreign", "apps", map[string]any{"windsorcli.dev/context-id": "ctx-2", "windsorcli.dev/secret-owner": "cluster"}))
		f.cluster.seed(secretsGVR, secret("flux", "apps", map[string]any{"windsorcli.dev/context-id": "ctx-1"}))

		// When secrets are pruned to the desired set
		err := f.manager.PruneSecrets(map[string]map[string]bool{"apps": {"keep": true}})

		// Then only the stale placed secret is deleted
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(f.cluster.deletes) != 1 || f.cluster.deletes[0] != "secrets/stale" {
			t.Errorf("expected only the stale secret deleted, got %v", f.cluster.deletes)
		}
	})

	t.Run("FailsWithoutAContextID", func(t *testing.T) {
		// Given a context without an id
		f := newFixture(t)
		f.values["id"] = ""

		// When secrets are pruned
		err := f.manager.PruneSecrets(nil)

		// Then the prune fails closed with a hint to set the id
		if we := windsorError(t, err); we.Remediation != "Run windsor init to set the context id." {
			t.Errorf("unexpected hint %q", we.Remediation)
		}
	})

	t.Run("FailsOnAContextIDThatIsNotALabelValue", func(t *testing.T) {
		// Given a context id with a space
		f := newFixture(t)
		f.values["id"] = "bad id"

		// When secrets are pruned
		err := f.manager.PruneSecrets(nil)

		// Then the error names the bad id
		if we := windsorError(t, err); !strings.HasPrefix(we.Message, `Context id "bad id" is not a valid Kubernetes label value:`) || we.Remediation == "" {
			t.Errorf("unexpected error %q with hint %q", we.Message, we.Remediation)
		}
	})

	t.Run("ReturnsTheDeleteError", func(t *testing.T) {
		// Given a stale secret whose delete fails
		f := newFixture(t)
		f.cluster.seed(secretsGVR, secret("stale", "apps", owned))
		f.cluster.fail = failOn("delete", "stale", errors.New("forbidden"))

		// When secrets are pruned
		err := f.manager.PruneSecrets(nil)

		// Then the error names the secret and its namespace
		if err == nil || !strings.Contains(err.Error(), `failed to delete orphaned secret "stale" in namespace "apps"`) {
			t.Errorf("expected a delete error, got %v", err)
		}
	})
}

func TestKubernetesManager_RollWorkloadsForSecret(t *testing.T) {
	workload := func(kind, name string, podSpec map[string]any, annotations map[string]any) map[string]any {
		return map[string]any{
			"apiVersion": "apps/v1",
			"kind":       kind,
			"metadata":   map[string]any{"name": name, "namespace": "apps"},
			"spec":       map[string]any{"template": map[string]any{"metadata": map[string]any{"annotations": annotations}, "spec": podSpec}},
		}
	}
	envRef := map[string]any{"containers": []any{map[string]any{"name": "app", "env": []any{map[string]any{"name": "TOKEN", "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "api-token", "key": "token"}}}}}}}
	volumeRef := map[string]any{"volumes": []any{map[string]any{"name": "creds", "secret": map[string]any{"secretName": "api-token"}}}}
	initEnvFrom := map[string]any{"initContainers": []any{map[string]any{"name": "init", "envFrom": []any{map[string]any{"secretRef": map[string]any{"name": "api-token"}}}}}}
	unrelated := map[string]any{"containers": []any{map[string]any{"name": "app"}}}

	t.Run("StampsTheDigestOnEveryWorkloadThatReferencesTheSecret", func(t *testing.T) {
		// Given workloads that reference the secret by env, volume, and init envFrom, and one that does not
		f := newFixture(t)
		f.cluster.seed(deploymentsGVR, workload("Deployment", "web", envRef, nil))
		f.cluster.seed(statefulSetsGVR, workload("StatefulSet", "db", volumeRef, nil))
		f.cluster.seed(daemonSetsGVR, workload("DaemonSet", "agent", initEnvFrom, nil))
		f.cluster.seed(deploymentsGVR, workload("Deployment", "other", unrelated, nil))

		// When the workloads are rolled for the secret
		err := f.manager.RollWorkloadsForSecret(context.Background(), "apps", "api-token", "abc123")

		// Then each referencing workload carries the digest annotation and the other is untouched
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		for _, target := range []struct {
			gvr  schema.GroupVersionResource
			name string
		}{{deploymentsGVR, "web"}, {statefulSetsGVR, "db"}, {daemonSetsGVR, "agent"}} {
			value, _, _ := unstructured.NestedString(f.cluster.get(target.gvr, "apps", target.name).Object, "spec", "template", "metadata", "annotations", "checksum.windsorcli.dev/api-token")
			if value != "abc123" {
				t.Errorf("expected %s to carry the digest, got %q", target.name, value)
			}
		}
		if len(f.cluster.patches) != 3 {
			t.Errorf("expected 3 patches, got %v", f.cluster.patches)
		}
	})

	t.Run("LeavesAWorkloadThatAlreadyCarriesTheDigest", func(t *testing.T) {
		// Given a workload already annotated with the digest
		f := newFixture(t)
		f.cluster.seed(deploymentsGVR, workload("Deployment", "web", envRef, map[string]any{"checksum.windsorcli.dev/api-token": "abc123"}))

		// When the workloads are rolled with the same digest
		err := f.manager.RollWorkloadsForSecret(context.Background(), "apps", "api-token", "abc123")

		// Then nothing is patched
		if err != nil || len(f.cluster.patches) != 0 {
			t.Errorf("expected no patches, got %v (err %v)", f.cluster.patches, err)
		}
	})

	t.Run("ShortensTheAnnotationKeyForALongSecretName", func(t *testing.T) {
		// Given a workload that references a secret with a 70-character name
		f := newFixture(t)
		long := strings.Repeat("a", 70)
		f.cluster.seed(deploymentsGVR, workload("Deployment", "web", map[string]any{"volumes": []any{map[string]any{"secret": map[string]any{"secretName": long}}}}, nil))

		// When the workloads are rolled for that secret
		err := f.manager.RollWorkloadsForSecret(context.Background(), "apps", long, "abc123")

		// Then the annotation name segment is 63 characters and keeps a prefix of the secret name
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		annotations, _, _ := unstructured.NestedStringMap(f.cluster.get(deploymentsGVR, "apps", "web").Object, "spec", "template", "metadata", "annotations")
		if len(annotations) != 1 {
			t.Fatalf("expected one annotation, got %v", annotations)
		}
		for key := range annotations {
			segment := strings.TrimPrefix(key, "checksum.windsorcli.dev/")
			if len(segment) != 63 || !strings.HasPrefix(segment, strings.Repeat("a", 54)+"-") {
				t.Errorf("unexpected annotation key %q", key)
			}
		}
	})

	t.Run("ReturnsTheListError", func(t *testing.T) {
		// Given a cluster whose list fails
		f := newFixture(t)
		f.cluster.fail = func(op string, gvr schema.GroupVersionResource, _, _ string) error {
			if op == "list" && gvr.Resource == "statefulsets" {
				return errors.New("forbidden")
			}
			return nil
		}

		// When the workloads are rolled
		err := f.manager.RollWorkloadsForSecret(context.Background(), "apps", "api-token", "abc123")

		// Then the error names the resource and namespace
		if err == nil || !strings.Contains(err.Error(), `listing statefulsets in namespace "apps"`) {
			t.Errorf("expected a list error, got %v", err)
		}
	})
}

func TestKubernetesManager_VersionMarker(t *testing.T) {
	t.Run("ReadsBackTheMarkerItWrites", func(t *testing.T) {
		// Given a marker with one applied source
		f := newFixture(t)
		marker := kubernetes.VersionMarker{SchemaVersion: 1, Phase: "idle", AppliedSources: map[string]kubernetes.SourceRef{"core": {URL: "oci://ghcr.io/windsorcli/core", Ref: "v1.0.0"}}}

		// When the marker is written and read back
		writeErr := f.manager.ApplyVersionMarker("flux-system", marker)
		got, found, readErr := f.manager.GetVersionMarker("flux-system")

		// Then the same marker is found
		if writeErr != nil || readErr != nil || !found {
			t.Fatalf("expected a found marker, got found=%v, errors %v, %v", found, writeErr, readErr)
		}
		if got.Phase != "idle" || got.AppliedSources["core"].Ref != "v1.0.0" {
			t.Errorf("unexpected marker %+v", got)
		}
	})

	t.Run("ReportsNoMarkerOnAFreshCluster", func(t *testing.T) {
		// Given an empty cluster
		f := newFixture(t)

		// When the marker is read
		_, found, err := f.manager.GetVersionMarker("flux-system")

		// Then no marker is found and no error is returned
		if found || err != nil {
			t.Errorf("expected (false, nil), got (%v, %v)", found, err)
		}
	})

	t.Run("ReportsNoMarkerForAConfigMapWithoutData", func(t *testing.T) {
		// Given a marker ConfigMap with no data
		f := newFixture(t)
		f.cluster.seed(configMapsGVR, map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": kubernetes.VersionMarkerConfigMapName, "namespace": "flux-system"}})

		// When the marker is read
		_, found, err := f.manager.GetVersionMarker("flux-system")

		// Then no marker is found and no error is returned
		if found || err != nil {
			t.Errorf("expected (false, nil), got (%v, %v)", found, err)
		}
	})

	t.Run("ReturnsAReadError", func(t *testing.T) {
		// Given a cluster whose read fails
		f := newFixture(t)
		f.cluster.fail = failOn("get", kubernetes.VersionMarkerConfigMapName, errors.New("connection refused"))

		// When the marker is read
		_, _, err := f.manager.GetVersionMarker("flux-system")

		// Then the read error is returned
		if err == nil || !strings.Contains(err.Error(), "failed to read version marker") {
			t.Errorf("expected a read error, got %v", err)
		}
	})
}
