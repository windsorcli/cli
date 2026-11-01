package kubernetes_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"github.com/windsorcli/cli/pkg/runtime/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseKubernetesManager_ApplyConfigMap(t *testing.T) {
	setup := func(t *testing.T, kubernetesClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		if kubernetesClient == nil {
			kubernetesClient = mocks.KubernetesClient
		}
		manager := kubernetes.NewKubernetesManager(kubernetesClient, mocks.ConfigHandler)
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		manager.Timing.KustomizationReconcileSleep = 50 * time.Millisecond
		return manager
	}

	// existingConfigMap returns a client whose GetResource returns obj and that records each delete.
	existingConfigMap := func(obj map[string]any) (*client.MockKubernetesClient, *bool) {
		deleted := false
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: obj}, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleted = true
			return nil
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		return kubernetesClient, &deleted
	}

	t.Run("AppliesNewConfigMap", func(t *testing.T) {
		// Given a client with no existing ConfigMap
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		// When a ConfigMap is applied
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"key1": "value1", "key2": "value2"})

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("RefusesUnresolvedExpression", func(t *testing.T) {
		// Given a client that records whether anything is applied
		kubernetesClient := client.NewMockKubernetesClient()
		var applied bool
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			applied = true
			return obj, nil
		}
		manager := setup(t, kubernetesClient)

		// When a ConfigMap with an unresolved expression is applied
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{
			"key1": "${terraform_output(\"cluster\", \"endpoint\")}",
		})

		// Then it fails and applies nothing
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unresolved expression") {
			t.Errorf("Expected unresolved-expression error, got: %v", err)
		}
		if applied {
			t.Error("Expected the ConfigMap not to be applied when a value is unresolved")
		}
	})

	t.Run("DeletesAndRecreatesImmutableConfigMap", func(t *testing.T) {
		// Given an existing immutable ConfigMap
		kubernetesClient, deleted := existingConfigMap(map[string]any{"kind": "ConfigMap", "immutable": true})
		manager := setup(t, kubernetesClient)

		// When the ConfigMap is applied
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"key1": "value1"})

		// Then the existing ConfigMap is deleted and the apply succeeds
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !*deleted {
			t.Error("Expected the immutable ConfigMap to be deleted")
		}
	})

	t.Run("KeepsExistingObjectOfOtherKind", func(t *testing.T) {
		// Given an existing object of another kind with immutable set to true
		kubernetesClient, deleted := existingConfigMap(map[string]any{"kind": "Deployment", "immutable": true})
		manager := setup(t, kubernetesClient)

		// When the ConfigMap is applied
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"k": "v"})

		// Then nothing is deleted
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if *deleted {
			t.Error("Expected no delete for a non-ConfigMap kind")
		}
	})

	t.Run("KeepsConfigMapWithoutImmutableField", func(t *testing.T) {
		// Given an existing ConfigMap with no immutable field
		kubernetesClient, deleted := existingConfigMap(map[string]any{"kind": "ConfigMap"})
		manager := setup(t, kubernetesClient)

		// When the ConfigMap is applied
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"k": "v"})

		// Then nothing is deleted
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if *deleted {
			t.Error("Expected no delete for a missing immutable field")
		}
	})

	t.Run("KeepsConfigMapWithImmutableFalse", func(t *testing.T) {
		// Given an existing ConfigMap with immutable set to false
		kubernetesClient, deleted := existingConfigMap(map[string]any{"kind": "ConfigMap", "immutable": false})
		manager := setup(t, kubernetesClient)

		// When the ConfigMap is applied
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"k": "v"})

		// Then nothing is deleted
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if *deleted {
			t.Error("Expected no delete for immutable false")
		}
	})

	t.Run("ApplyError", func(t *testing.T) {
		// Given a client whose apply fails
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("apply error")
		}
		manager := setup(t, kubernetesClient)

		// When the ConfigMap is applied
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"key1": "value1"})

		// Then the apply error is returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("RejectsNilData", func(t *testing.T) {
		// Given a manager
		manager := setup(t, nil)

		// When a ConfigMap with nil data is applied
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", nil)

		// Then a validation error is returned
		if err == nil || !strings.Contains(err.Error(), "data cannot be empty for ConfigMap") {
			t.Errorf("Expected error containing 'data cannot be empty for ConfigMap', got %v", err)
		}
	})

	t.Run("RejectsEmptyData", func(t *testing.T) {
		// Given a manager
		manager := setup(t, nil)

		// When a ConfigMap with an empty data map is applied
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{})

		// Then a validation error is returned
		if err == nil || !strings.Contains(err.Error(), "data cannot be empty for ConfigMap") {
			t.Errorf("Expected error containing 'data cannot be empty for ConfigMap', got %v", err)
		}
	})

	t.Run("RejectsEmptyName", func(t *testing.T) {
		// Given a manager
		manager := setup(t, nil)

		// When a ConfigMap with an empty name is applied
		err := manager.ApplyConfigMap("", "test-namespace", map[string]string{"k": "v"})

		// Then a validation error is returned
		if err == nil || !strings.Contains(err.Error(), "metadata.name cannot be empty") {
			t.Errorf("Expected error containing 'metadata.name cannot be empty', got %v", err)
		}
	})

	t.Run("RejectsBlankName", func(t *testing.T) {
		// Given a manager
		manager := setup(t, nil)

		// When a ConfigMap with a blank name is applied
		err := manager.ApplyConfigMap(" ", "test-namespace", map[string]string{"k": "v"})

		// Then a validation error is returned
		if err == nil || !strings.Contains(err.Error(), "metadata.name cannot be empty") {
			t.Errorf("Expected error containing 'metadata.name cannot be empty', got %v", err)
		}
	})

	t.Run("AppliesWhenGetReturnsNotFound", func(t *testing.T) {
		// Given a client whose get returns a not-found error
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		manager := setup(t, kubernetesClient)

		// When the ConfigMap is applied
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"k": "v"})

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("AppliesWhenGetReturnsOtherError", func(t *testing.T) {
		// Given a client whose get returns another error
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("some error")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		manager := setup(t, kubernetesClient)

		// When the ConfigMap is applied
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"k": "v"})

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DeleteErrorOnImmutableConfigMap", func(t *testing.T) {
		// Given an existing immutable ConfigMap whose delete fails
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{"kind": "ConfigMap", "immutable": true}}, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, ns, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("delete error")
		}
		manager := setup(t, kubernetesClient)

		// When the ConfigMap is applied
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"k": "v"})

		// Then the delete error is returned
		if err == nil || !strings.Contains(err.Error(), "failed to delete immutable configmap") {
			t.Errorf("Expected delete error, got %v", err)
		}
	})

	t.Run("ToUnstructuredError", func(t *testing.T) {
		// Given an existing ConfigMap and a conversion that fails
		manager := setup(t, nil)
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return nil, fmt.Errorf("forced toUnstructured error")
		}

		// When the ConfigMap is applied
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"k": "v"})

		// Then the conversion error is returned
		if err == nil || !strings.Contains(err.Error(), "failed to convert") {
			t.Errorf("Expected toUnstructured error, got %v", err)
		}
	})
}

func TestBaseKubernetesManager_GetKustomizationStatusReadiness(t *testing.T) {
	// listing returns a client whose kustomization list holds one k1 item with the given Ready condition.
	listing := func(condition map[string]any) *client.MockKubernetesClient {
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{{
					Object: map[string]any{
						"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
						"kind":       "Kustomization",
						"metadata":   map[string]any{"name": "k1"},
						"status":     map[string]any{"conditions": []any{condition}},
					},
				}},
			}, nil
		}
		return kubernetesClient
	}

	t.Run("FromUnstructuredError", func(t *testing.T) {
		// Given a ready kustomization and a conversion that fails
		mocks := setupKubernetesMocks(t)
		manager := kubernetes.NewKubernetesManager(listing(map[string]any{"type": "Ready", "status": "True"}), mocks.ConfigHandler)
		manager.Shims.FromUnstructured = func(obj map[string]any, target any) error {
			return fmt.Errorf("forced conversion error")
		}

		// When the status is read
		status, err := manager.GetKustomizationStatus([]string{"k1"})

		// Then the conversion error is returned with no status
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "forced conversion error") {
			t.Errorf("Expected error containing 'forced conversion error', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("KustomizationNotReady", func(t *testing.T) {
		// Given a kustomization whose Ready condition is False
		mocks := setupKubernetesMocks(t)
		manager := kubernetes.NewKubernetesManager(listing(map[string]any{"type": "Ready", "status": "False"}), mocks.ConfigHandler)

		// When the status is read
		status, err := manager.GetKustomizationStatus([]string{"k1"})

		// Then the kustomization reports not ready
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if status["k1"] {
			t.Errorf("Expected k1 to be not ready, got %v", status["k1"])
		}
	})

	t.Run("KustomizationFailed", func(t *testing.T) {
		// Given a kustomization whose reconcile failed
		mocks := setupKubernetesMocks(t)
		manager := kubernetes.NewKubernetesManager(listing(map[string]any{
			"type":    "Ready",
			"status":  "False",
			"reason":  "ReconciliationFailed",
			"message": "kustomization failed",
		}), mocks.ConfigHandler)

		// When the status is read
		status, err := manager.GetKustomizationStatus([]string{"k1"})

		// Then the failure is returned with no status
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kustomization k1 failed: kustomization failed") {
			t.Errorf("Expected error containing 'kustomization k1 failed: kustomization failed', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("KustomizationMissing", func(t *testing.T) {
		// Given a list that holds k1 but not k2
		mocks := setupKubernetesMocks(t)
		manager := kubernetes.NewKubernetesManager(listing(map[string]any{"type": "Ready", "status": "True"}), mocks.ConfigHandler)

		// When the status of k1 and k2 is read
		status, err := manager.GetKustomizationStatus([]string{"k1", "k2"})

		// Then k1 is ready and k2 is not
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !status["k1"] {
			t.Errorf("Expected k1 to be ready, got %v", status["k1"])
		}
		if status["k2"] {
			t.Errorf("Expected k2 to be not ready, got %v", status["k2"])
		}
	})
}

func TestBaseKubernetesManager_ApplySecret(t *testing.T) {
	setup := func(t *testing.T, kubernetesClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := kubernetes.NewKubernetesManager(kubernetesClient, mocks.ConfigHandler)
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		manager.Timing.KustomizationReconcileSleep = 50 * time.Millisecond
		return manager
	}

	// recording returns a client with no existing Secret that records the last applied object.
	recording := func() (*client.MockKubernetesClient, **unstructured.Unstructured) {
		var applied *unstructured.Unstructured
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			applied = obj
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		return kubernetesClient, &applied
	}

	// existingSecret returns a client whose GetResource returns obj, that records deletes and the last apply.
	existingSecret := func(obj map[string]any) (*client.MockKubernetesClient, *bool, **unstructured.Unstructured) {
		var deleted bool
		var applied *unstructured.Unstructured
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: obj}, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, ns, name string, opts metav1.DeleteOptions) error {
			deleted = true
			return nil
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			applied = obj
			return obj, nil
		}
		return kubernetesClient, &deleted, &applied
	}

	dockerCredentials := map[string]string{
		"docker-username": "octocat",
		"docker-password": "hunter2",
	}

	t.Run("AppliesOpaqueSecretWithStringData", func(t *testing.T) {
		// Given a client that records the applied object and its resource
		var appliedGVR schema.GroupVersionResource
		kubernetesClient, applied := recording()
		recordApply := kubernetesClient.ApplyResourceFunc
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			appliedGVR = gvr
			return recordApply(gvr, obj, opts)
		}
		manager := setup(t, kubernetesClient)

		// When a secret is applied
		err := manager.ApplySecret("cloudflare-creds", "system-dns", "", map[string]string{"api_token": "PLAINTEXT"}, "dns-install")

		// Then an Opaque Secret with the plaintext in stringData and the secret-owner label is applied
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if appliedGVR.Resource != "secrets" {
			t.Errorf("Expected secrets GVR, got %v", appliedGVR)
		}
		if (*applied).GetKind() != "Secret" {
			t.Errorf("Expected kind Secret, got %q", (*applied).GetKind())
		}
		if (*applied).Object["type"] != "Opaque" {
			t.Errorf("Expected type Opaque, got %v", (*applied).Object["type"])
		}
		stringData, _ := (*applied).Object["stringData"].(map[string]string)
		if stringData["api_token"] != "PLAINTEXT" {
			t.Errorf("Expected stringData api_token=PLAINTEXT, got %v", (*applied).Object["stringData"])
		}
		metadata, _ := (*applied).Object["metadata"].(map[string]any)
		if metadata["name"] != "cloudflare-creds" || metadata["namespace"] != "system-dns" {
			t.Errorf("Unexpected metadata: %v", (*applied).Object["metadata"])
		}
		if (*applied).GetLabels()["windsorcli.dev/secret-owner"] != "dns-install" {
			t.Errorf("Expected secret-owner label dns-install, got %v", (*applied).GetLabels())
		}
	})

	t.Run("EmptyNameFails", func(t *testing.T) {
		// Given a manager
		manager := setup(t, client.NewMockKubernetesClient())

		// When a secret with no name is applied
		err := manager.ApplySecret("", "system-dns", "", map[string]string{"k": "v"}, "dns-install")

		// Then a validation error is returned
		if err == nil {
			t.Error("Expected error for empty name")
		}
	})

	t.Run("SynthesizesDockerConfigJSONFromDockerCredentials", func(t *testing.T) {
		// Given a client that records the applied object
		kubernetesClient, applied := recording()
		manager := setup(t, kubernetesClient)

		// When a secret with docker-username, docker-password, and docker-server is applied
		err := manager.ApplySecret("ghcr-credentials", "pme", "", map[string]string{
			"docker-username": "octocat",
			"docker-password": "hunter2",
			"docker-server":   "ghcr.io",
		}, "addon-pme")

		// Then a dockerconfigjson Secret replaces the docker-* keys with a synthesized config
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if (*applied).Object["type"] != "kubernetes.io/dockerconfigjson" {
			t.Errorf("Expected type kubernetes.io/dockerconfigjson, got %v", (*applied).Object["type"])
		}
		stringData, _ := (*applied).Object["stringData"].(map[string]string)
		for _, key := range []string{"docker-username", "docker-password", "docker-server"} {
			if _, exists := stringData[key]; exists {
				t.Errorf("Expected %s to be dropped", key)
			}
		}
		var decoded struct {
			Auths map[string]struct {
				Username string `json:"username"`
				Password string `json:"password"`
				Auth     string `json:"auth"`
			} `json:"auths"`
		}
		if err := json.Unmarshal([]byte(stringData[".dockerconfigjson"]), &decoded); err != nil {
			t.Fatalf("Expected valid .dockerconfigjson, got error: %v", err)
		}
		entry, ok := decoded.Auths["ghcr.io"]
		if !ok {
			t.Fatalf("Expected auths entry for ghcr.io, got %v", decoded.Auths)
		}
		if entry.Username != "octocat" || entry.Password != "hunter2" {
			t.Errorf("Expected username=octocat password=hunter2, got %+v", entry)
		}
		wantAuth := base64.StdEncoding.EncodeToString([]byte("octocat:hunter2"))
		if entry.Auth != wantAuth {
			t.Errorf("Expected auth=%q, got %q", wantAuth, entry.Auth)
		}
	})

	t.Run("SynthesizesDockerConfigJSONWithDefaultServer", func(t *testing.T) {
		// Given a client that records the applied object
		kubernetesClient, applied := recording()
		manager := setup(t, kubernetesClient)

		// When a secret with docker credentials and no docker-server is applied
		err := manager.ApplySecret("ghcr-credentials", "pme", "", dockerCredentials, "addon-pme")

		// Then the synthesized config uses ghcr.io
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		stringData, _ := (*applied).Object["stringData"].(map[string]string)
		var decoded struct {
			Auths map[string]any `json:"auths"`
		}
		if err := json.Unmarshal([]byte(stringData[".dockerconfigjson"]), &decoded); err != nil {
			t.Fatalf("Expected valid .dockerconfigjson, got error: %v", err)
		}
		if _, ok := decoded.Auths["ghcr.io"]; !ok {
			t.Errorf("Expected default server ghcr.io, got %v", decoded.Auths)
		}
	})

	t.Run("PassesThroughExistingDockerConfigJSON", func(t *testing.T) {
		// Given a client that records the applied object
		kubernetesClient, applied := recording()
		manager := setup(t, kubernetesClient)
		preEncoded := `{"auths":{"ghcr.io":{"username":"u","password":"p","auth":"dTpw"}}}`

		// When a secret that already holds a .dockerconfigjson is applied
		err := manager.ApplySecret("ghcr-credentials", "pme", "", map[string]string{".dockerconfigjson": preEncoded}, "addon-pme")

		// Then the Secret is a dockerconfigjson Secret with the value unchanged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if (*applied).Object["type"] != "kubernetes.io/dockerconfigjson" {
			t.Errorf("Expected type kubernetes.io/dockerconfigjson, got %v", (*applied).Object["type"])
		}
		stringData, _ := (*applied).Object["stringData"].(map[string]string)
		if stringData[".dockerconfigjson"] != preEncoded {
			t.Errorf("Expected .dockerconfigjson to pass through unchanged, got %q", stringData[".dockerconfigjson"])
		}
	})

	t.Run("StaysOpaqueWhenOnlyDockerUsernameSet", func(t *testing.T) {
		// Given a client that records the applied object
		kubernetesClient, applied := recording()
		manager := setup(t, kubernetesClient)

		// When a secret with docker-username and no docker-password is applied
		err := manager.ApplySecret("incomplete-creds", "pme", "", map[string]string{"docker-username": "octocat"}, "addon-pme")

		// Then the Secret stays Opaque with docker-username unchanged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if (*applied).Object["type"] != "Opaque" {
			t.Errorf("Expected type Opaque, got %v", (*applied).Object["type"])
		}
		stringData, _ := (*applied).Object["stringData"].(map[string]string)
		if stringData["docker-username"] != "octocat" {
			t.Errorf("Expected docker-username to pass through unchanged, got %v", stringData)
		}
	})

	t.Run("DoesNotMutateInputStringDataMap", func(t *testing.T) {
		// Given a caller-owned stringData map with docker credentials
		kubernetesClient, _ := recording()
		manager := setup(t, kubernetesClient)
		input := map[string]string{
			"docker-username": "octocat",
			"docker-password": "hunter2",
		}

		// When the secret is applied
		err := manager.ApplySecret("ghcr-credentials", "pme", "", input, "addon-pme")

		// Then the caller's map is unchanged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(input) != 2 || input["docker-username"] != "octocat" || input["docker-password"] != "hunter2" {
			t.Errorf("Expected input map to be unmodified, got %v", input)
		}
	})

	t.Run("DeletesAndRecreatesWhenResolvedTypeChanges", func(t *testing.T) {
		// Given an existing Opaque Secret
		kubernetesClient, deleted, applied := existingSecret(map[string]any{"kind": "Secret", "type": "Opaque"})
		manager := setup(t, kubernetesClient)

		// When a secret whose docker-* keys resolve to dockerconfigjson is applied
		err := manager.ApplySecret("ghcr-credentials", "pme", "", dockerCredentials, "addon-pme")

		// Then the existing Secret is deleted before the new type is applied
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !*deleted {
			t.Error("Expected the existing Opaque Secret to be deleted before re-applying as dockerconfigjson")
		}
		if (*applied).Object["type"] != "kubernetes.io/dockerconfigjson" {
			t.Errorf("Expected type kubernetes.io/dockerconfigjson, got %v", (*applied).Object["type"])
		}
	})

	t.Run("SkipsDeleteWhenResolvedTypeUnchanged", func(t *testing.T) {
		// Given an existing dockerconfigjson Secret
		kubernetesClient, deleted, _ := existingSecret(map[string]any{"kind": "Secret", "type": "kubernetes.io/dockerconfigjson"})
		manager := setup(t, kubernetesClient)

		// When a secret whose docker-* keys resolve to dockerconfigjson is applied
		err := manager.ApplySecret("ghcr-credentials", "pme", "", dockerCredentials, "addon-pme")

		// Then the existing Secret is not deleted
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if *deleted {
			t.Error("Expected no delete when the resolved type matches the existing Secret's type")
		}
	})

	t.Run("SkipsDeleteWhenExistingTypeMissing", func(t *testing.T) {
		// Given an existing Secret with no type
		kubernetesClient, deleted, _ := existingSecret(map[string]any{"kind": "Secret"})
		manager := setup(t, kubernetesClient)

		// When an Opaque secret is applied
		err := manager.ApplySecret("plain", "pme", "", map[string]string{"k": "v"}, "addon-pme")

		// Then the existing Secret is not deleted
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if *deleted {
			t.Error("Expected no delete when the existing object has no type")
		}
	})

	t.Run("DeleteErrorOnTypeChangeIsPropagated", func(t *testing.T) {
		// Given an existing Opaque Secret whose delete fails
		kubernetesClient, _, _ := existingSecret(map[string]any{"kind": "Secret", "type": "Opaque"})
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, ns, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("delete failed")
		}
		manager := setup(t, kubernetesClient)

		// When a secret whose resolved type changes is applied
		err := manager.ApplySecret("ghcr-credentials", "pme", "", dockerCredentials, "addon-pme")

		// Then the delete error is returned
		if err == nil {
			t.Fatal("Expected error when delete fails, got nil")
		}
		if !strings.Contains(err.Error(), "failed to delete secret with changed type") {
			t.Errorf("Expected delete-failure error, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_PruneSecrets(t *testing.T) {
	// secretObj builds a CLI-placed Secret list item with the context id and the secret-owner label.
	secretObj := func(name, namespace string) unstructured.Unstructured {
		return unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]any{
					"windsorcli.dev/context-id":   "test-context-id",
					"windsorcli.dev/secret-owner": "pki-install",
				},
			},
		}}
	}

	// setup returns a manager whose client lists items and records the selector and each delete.
	setup := func(t *testing.T, items ...unstructured.Unstructured) (*kubernetes.BaseKubernetesManager, *[]string, *string) {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		deleted := []string{}
		var selector string
		c := client.NewMockKubernetesClient()
		c.ListResourcesByLabelFunc = func(gvr schema.GroupVersionResource, ns, labelSelector string) (*unstructured.UnstructuredList, error) {
			selector = labelSelector
			return &unstructured.UnstructuredList{Items: items}, nil
		}
		c.DeleteResourceFunc = func(gvr schema.GroupVersionResource, ns, name string, opts metav1.DeleteOptions) error {
			deleted = append(deleted, ns+"/"+name)
			return nil
		}
		return kubernetes.NewKubernetesManager(c, mocks.ConfigHandler), &deleted, &selector
	}

	t.Run("DeletesOrphansAndKeepsDesired", func(t *testing.T) {
		// Given the same secret placed in two namespaces
		manager, deleted, selector := setup(t,
			secretObj("hetzner-dns", "system-pki"),
			secretObj("hetzner-dns", "system-dns"),
		)

		// When secrets are pruned to a desired set that keeps only the system-pki copy
		err := manager.PruneSecrets(map[string]map[string]bool{"system-pki": {"hetzner-dns": true}})

		// Then only the system-dns copy is deleted, through a context and owner scoped selector
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(*deleted) != 1 || (*deleted)[0] != "system-dns/hetzner-dns" {
			t.Errorf("Expected only system-dns/hetzner-dns deleted, got %v", *deleted)
		}
		if !strings.Contains(*selector, "windsorcli.dev/context-id=test-context-id") || !strings.Contains(*selector, "windsorcli.dev/secret-owner") {
			t.Errorf("Expected context+owner-scoped selector, got %q", *selector)
		}
	})

	t.Run("EmptyDesiredReclaimsEveryCLISecret", func(t *testing.T) {
		// Given two CLI-placed secrets
		manager, deleted, _ := setup(t, secretObj("a", "ns1"), secretObj("b", "ns2"))

		// When secrets are pruned to an empty desired set
		err := manager.PruneSecrets(map[string]map[string]bool{})

		// Then both secrets are deleted
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(*deleted) != 2 {
			t.Errorf("Expected both CLI secrets reclaimed, got %v", *deleted)
		}
	})

	t.Run("EmptyContextIDFailsClosed", func(t *testing.T) {
		// Given a config handler with no context id
		mocks := setupKubernetesMocks(t, func(m *KubernetesTestMocks) {
			m.ConfigHandler.(*config.MockConfigHandler).GetStringFunc = func(key string, defaultValue ...string) string {
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		})
		manager := kubernetes.NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)

		// When secrets are pruned
		err := manager.PruneSecrets(nil)

		// Then an error is returned
		if err == nil {
			t.Error("Expected error when context id is not set")
		}
	})

	t.Run("InvalidContextIDFailsClosed", func(t *testing.T) {
		// Given a context id that is not a valid label value
		mocks := setupKubernetesMocks(t, func(m *KubernetesTestMocks) {
			m.ConfigHandler.(*config.MockConfigHandler).GetStringFunc = func(key string, defaultValue ...string) string {
				if key == "id" {
					return "bad,id"
				}
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		})
		listed := false
		c := client.NewMockKubernetesClient()
		c.ListResourcesByLabelFunc = func(gvr schema.GroupVersionResource, ns, labelSelector string) (*unstructured.UnstructuredList, error) {
			listed = true
			return &unstructured.UnstructuredList{}, nil
		}
		manager := kubernetes.NewKubernetesManager(c, mocks.ConfigHandler)

		// When secrets are pruned
		err := manager.PruneSecrets(nil)

		// Then the error names the id and the cluster is not queried
		if err == nil || !strings.Contains(err.Error(), "bad,id") {
			t.Errorf("Expected error naming the invalid context id, got %v", err)
		}
		if listed {
			t.Error("Expected no list call when context id is invalid")
		}
	})

	t.Run("DeleteErrorIsPropagated", func(t *testing.T) {
		// Given one CLI-placed secret whose delete fails
		mocks := setupKubernetesMocks(t)
		c := client.NewMockKubernetesClient()
		c.ListResourcesByLabelFunc = func(gvr schema.GroupVersionResource, ns, labelSelector string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{secretObj("x", "ns")}}, nil
		}
		c.DeleteResourceFunc = func(gvr schema.GroupVersionResource, ns, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("api down")
		}
		manager := kubernetes.NewKubernetesManager(c, mocks.ConfigHandler)

		// When secrets are pruned to an empty desired set
		err := manager.PruneSecrets(map[string]map[string]bool{})

		// Then the delete error is returned
		if err == nil || !strings.Contains(err.Error(), "api down") {
			t.Errorf("Expected delete error propagated, got %v", err)
		}
	})
}

func TestBaseKubernetesManager_RollWorkloadsForSecret(t *testing.T) {
	const secretName = "alertmanager-slack"
	const annotationKey = "checksum.windsorcli.dev/" + secretName

	// workload builds an apps/v1 workload with the given pod spec and pod-template annotations.
	workload := func(kind, name string, podSpec map[string]any, annotations map[string]any) unstructured.Unstructured {
		tmplMeta := map[string]any{}
		if annotations != nil {
			tmplMeta["annotations"] = annotations
		}
		return unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       kind,
			"metadata":   map[string]any{"name": name, "namespace": "system-telemetry"},
			"spec": map[string]any{
				"template": map[string]any{"metadata": tmplMeta, "spec": podSpec},
			},
		}}
	}
	envRefTo := func(name string) map[string]any {
		return map[string]any{"containers": []any{map[string]any{"name": "app", "env": []any{
			map[string]any{"name": "URL", "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": name, "key": "url"}}},
		}}}}
	}
	envRef := envRefTo(secretName)
	volumeRef := map[string]any{"volumes": []any{map[string]any{"name": "s", "secret": map[string]any{"secretName": secretName}}}}
	envFromRef := map[string]any{"containers": []any{map[string]any{"name": "app", "envFrom": []any{
		map[string]any{"secretRef": map[string]any{"name": secretName}},
	}}}}
	noRef := envRefTo("some-other-secret")

	setup := func(t *testing.T, lists map[string][]unstructured.Unstructured) (*kubernetes.BaseKubernetesManager, *map[string]string) {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: lists[gvr.Resource]}, nil
		}
		patched := map[string]string{}
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, ns, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			patched[gvr.Resource+"/"+name] = string(data)
			return nil, nil
		}
		return kubernetes.NewKubernetesManager(kubernetesClient, mocks.ConfigHandler), &patched
	}

	// patchedAnnotationKey returns the single pod-template annotation key in a rollout patch.
	patchedAnnotationKey := func(t *testing.T, patch string) string {
		t.Helper()
		var decoded struct {
			Spec struct {
				Template struct {
					Metadata struct {
						Annotations map[string]string `json:"annotations"`
					} `json:"metadata"`
				} `json:"template"`
			} `json:"spec"`
		}
		if err := json.Unmarshal([]byte(patch), &decoded); err != nil {
			t.Fatalf("Expected a JSON patch, got %q: %v", patch, err)
		}
		if len(decoded.Spec.Template.Metadata.Annotations) != 1 {
			t.Fatalf("Expected one annotation in the patch, got %v", decoded.Spec.Template.Metadata.Annotations)
		}
		for key := range decoded.Spec.Template.Metadata.Annotations {
			return key
		}
		return ""
	}

	t.Run("RollsOnlyReferencingWorkloadsAcrossKinds", func(t *testing.T) {
		// Given one referencing workload of each kind, each through another path, and a bystander
		manager, patched := setup(t, map[string][]unstructured.Unstructured{
			"deployments":  {workload("Deployment", "env-consumer", envRef, nil), workload("Deployment", "bystander", noRef, nil)},
			"statefulsets": {workload("StatefulSet", "volume-consumer", volumeRef, nil)},
			"daemonsets":   {workload("DaemonSet", "envfrom-consumer", envFromRef, nil)},
		})

		// When workloads are rolled for the secret
		err := manager.RollWorkloadsForSecret(context.Background(), "system-telemetry", secretName, "digest-1")

		// Then each referencing workload gets the digest annotation and the bystander is not patched
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		for _, key := range []string{"deployments/env-consumer", "statefulsets/volume-consumer", "daemonsets/envfrom-consumer"} {
			p, ok := (*patched)[key]
			if !ok {
				t.Errorf("Expected %s to be rolled", key)
				continue
			}
			if !strings.Contains(p, annotationKey) || !strings.Contains(p, "digest-1") {
				t.Errorf("Expected patch of %s to carry the digest annotation, got %s", key, p)
			}
		}
		if _, ok := (*patched)["deployments/bystander"]; ok {
			t.Error("Expected non-referencing workload to be left untouched")
		}
	})

	t.Run("IdempotentWhenDigestUnchanged", func(t *testing.T) {
		// Given a referencing workload that already has the current digest
		manager, patched := setup(t, map[string][]unstructured.Unstructured{
			"deployments": {workload("Deployment", "env-consumer", envRef, map[string]any{annotationKey: "digest-1"})},
		})

		// When workloads are rolled with the same digest
		err := manager.RollWorkloadsForSecret(context.Background(), "system-telemetry", secretName, "digest-1")

		// Then nothing is patched
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(*patched) != 0 {
			t.Errorf("Expected no patch for an unchanged digest, got %v", *patched)
		}
	})

	t.Run("UsesShortSecretNameVerbatimInAnnotationKey", func(t *testing.T) {
		// Given a referencing workload
		manager, patched := setup(t, map[string][]unstructured.Unstructured{
			"deployments": {workload("Deployment", "env-consumer", envRef, nil)},
		})

		// When workloads are rolled for the secret
		err := manager.RollWorkloadsForSecret(context.Background(), "system-telemetry", secretName, "digest-1")

		// Then the annotation key is the prefix plus the secret name
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if got := patchedAnnotationKey(t, (*patched)["deployments/env-consumer"]); got != "checksum.windsorcli.dev/alertmanager-slack" {
			t.Errorf("unexpected key %q", got)
		}
	})

	t.Run("LongSecretNameKeyStaysWithinLimitDeterministicAndUnique", func(t *testing.T) {
		// Given workloads that consume two distinct secret names longer than 63 characters
		long := strings.Repeat("a", 80)
		other := strings.Repeat("a", 79) + "b"
		manager, patched := setup(t, map[string][]unstructured.Unstructured{
			"deployments": {workload("Deployment", "long-a", envRefTo(long), nil), workload("Deployment", "long-b", envRefTo(other), nil)},
		})

		// When workloads are rolled twice for the first name and once for the other
		var keys []string
		for _, name := range []string{long, long, other} {
			if err := manager.RollWorkloadsForSecret(context.Background(), "system-telemetry", name, "digest-1"); err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			workloadName := "long-a"
			if name == other {
				workloadName = "long-b"
			}
			keys = append(keys, patchedAnnotationKey(t, (*patched)["deployments/"+workloadName]))
		}

		// Then the name segment fits in 63 characters, repeats for one name, and differs across names
		segment := strings.TrimPrefix(keys[0], "checksum.windsorcli.dev/")
		if len(segment) > 63 {
			t.Errorf("segment %q exceeds 63 chars (%d)", segment, len(segment))
		}
		if keys[0] != keys[1] {
			t.Error("expected a deterministic key for the same name")
		}
		if keys[0] == keys[2] {
			t.Error("expected distinct long names to produce distinct keys")
		}
	})

	t.Run("ListErrorSurfaces", func(t *testing.T) {
		// Given a client whose workload list fails
		mocks := setupKubernetesMocks(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return nil, fmt.Errorf("api down")
		}
		manager := kubernetes.NewKubernetesManager(kubernetesClient, mocks.ConfigHandler)

		// When workloads are rolled
		err := manager.RollWorkloadsForSecret(context.Background(), "system-telemetry", secretName, "digest-1")

		// Then the list error is returned
		if err == nil {
			t.Error("Expected error when listing workloads fails")
		}
	})
}
