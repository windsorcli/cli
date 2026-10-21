package kubernetes

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

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
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		// Use shorter timeouts for tests
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationReconcileSleep = 50 * time.Millisecond
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		data := map[string]string{
			"key1": "value1",
			"key2": "value2",
		}
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", data)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("RefusesUnresolvedExpression", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		var applied bool
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			applied = true
			return obj, nil
		}
		manager.client = kubernetesClient

		data := map[string]string{
			"key1": "${terraform_output(\"cluster\", \"endpoint\")}",
		}
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", data)
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

	t.Run("ImmutableConfigMap", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
				Object: map[string]any{
					"kind":      "ConfigMap",
					"immutable": true,
				},
			}, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		manager.client = kubernetesClient

		data := map[string]string{
			"key1": "value1",
		}
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", data)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ApplyError", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("apply error")
		}
		manager.client = kubernetesClient

		data := map[string]string{
			"key1": "value1",
		}
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", data)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ValidateFieldsError_MissingData", func(t *testing.T) {
		manager := setup(t)
		// Remove data field by passing nil
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", nil)
		if err == nil {
			t.Error("Expected error for missing data, got nil")
		}
	})

	t.Run("ValidateFieldsError_MissingName", func(t *testing.T) {
		manager := setup(t)
		// Patch shims to remove name from metadata
		origToUnstructured := manager.shims.ToUnstructured
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			m, _ := origToUnstructured(obj)
			if meta, ok := m["metadata"].(map[string]any); ok {
				delete(meta, "name")
			}
			return m, nil
		}
		// Data is present, but name will be missing
		err := manager.ApplyConfigMap("", "test-namespace", map[string]string{"k": "v"})
		if err == nil {
			t.Error("Expected error for missing metadata.name, got nil")
		}
	})

	t.Run("GetResourceError_NotFound", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		manager.client = kubernetesClient
		// Should not error, just apply
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"k": "v"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("GetResourceError_Other", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("some error")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		manager.client = kubernetesClient
		// Should not error, just apply
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"k": "v"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DeleteResourceError_ImmutableConfigMap", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
				Object: map[string]any{
					"kind":      "ConfigMap",
					"immutable": true,
				},
			}, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, ns, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("delete error")
		}
		manager.client = kubernetesClient
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"k": "v"})
		if err == nil || !strings.Contains(err.Error(), "failed to delete immutable configmap") {
			t.Errorf("Expected delete error, got %v", err)
		}
	})

	t.Run("ToUnstructuredError", func(t *testing.T) {
		manager := setup(t)
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return nil, fmt.Errorf("forced toUnstructured error")
		}
		err := manager.ApplyConfigMap("test-configmap", "test-namespace", map[string]string{"k": "v"})
		if err == nil || !strings.Contains(err.Error(), "failed to convert") {
			t.Errorf("Expected toUnstructured error, got %v", err)
		}
	})

	t.Run("FromUnstructuredError", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":   "Ready",
										"status": "True",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return fmt.Errorf("forced conversion error")
		}

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "forced conversion error") {
			t.Errorf("Expected error containing 'forced conversion error', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("KustomizationNotReady", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":   "Ready",
										"status": "False",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if status["k1"] {
			t.Errorf("Expected k1 to be not ready, got %v", status["k1"])
		}
	})

	t.Run("KustomizationFailed", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":    "Ready",
										"status":  "False",
										"reason":  "ReconciliationFailed",
										"message": "kustomization failed",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1"})
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "kustomization k1 failed: kustomization failed") {
			t.Errorf("Expected error containing 'kustomization k1 failed: kustomization failed', got %v", err)
		}
		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})

	t.Run("KustomizationMissing", func(t *testing.T) {
		manager := func(t *testing.T) *BaseKubernetesManager {
			mocks := setupKubernetesMocks(t)
			manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
			return manager
		}(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata": map[string]any{
								"name": "k1",
							},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{
										"type":   "Ready",
										"status": "True",
									},
								},
							},
						},
					},
				},
			}, nil
		}
		manager.client = kubernetesClient

		status, err := manager.GetKustomizationStatus([]string{"k1", "k2"})
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

	t.Run("ValidateFieldsError_MissingSpec", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Deployment",
				"metadata": map[string]any{
					"name": "foo",
				},
			},
		}
		err := validateFields(obj)
		if err == nil || !strings.Contains(err.Error(), "spec is required") {
			t.Errorf("Expected error containing 'spec is required', got %v", err)
		}
	})

	t.Run("ValidateFieldsError_MissingMetadataName", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Deployment",
				"metadata":   map[string]any{},
				"spec":       map[string]any{},
			},
		}
		err := validateFields(obj)
		if err == nil || !strings.Contains(err.Error(), "metadata.name is required") {
			t.Errorf("Expected error containing 'metadata.name is required', got %v", err)
		}
	})

	t.Run("ValidateFieldsError_EmptyMetadataName", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Deployment",
				"metadata": map[string]any{
					"name": " ",
				},
				"spec": map[string]any{},
			},
		}
		err := validateFields(obj)
		if err == nil || !strings.Contains(err.Error(), "metadata.name cannot be empty") {
			t.Errorf("Expected error containing 'metadata.name cannot be empty', got %v", err)
		}
	})

	t.Run("ValidateFieldsError_ConfigMapMissingData", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "foo",
				},
			},
		}
		err := validateFields(obj)
		if err == nil || !strings.Contains(err.Error(), "data is required for ConfigMap") {
			t.Errorf("Expected error containing 'data is required for ConfigMap', got %v", err)
		}
	})

	t.Run("ValidateFieldsError_ConfigMapDataNil", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "foo",
				},
				"data": nil,
			},
		}
		err := validateFields(obj)
		if err == nil || !strings.Contains(err.Error(), "data cannot be nil for ConfigMap") {
			t.Errorf("Expected error containing 'data cannot be nil for ConfigMap', got %v", err)
		}
	})

	t.Run("ValidateFieldsError_ConfigMapDataEmptyStringMap", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "foo",
				},
				"data": map[string]string{},
			},
		}
		err := validateFields(obj)
		if err == nil || !strings.Contains(err.Error(), "data cannot be empty for ConfigMap") {
			t.Errorf("Expected error containing 'data cannot be empty for ConfigMap', got %v", err)
		}
	})

	t.Run("ValidateFieldsError_ConfigMapDataEmptyAnyMap", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "foo",
				},
				"data": map[string]any{},
			},
		}
		err := validateFields(obj)
		if err == nil || !strings.Contains(err.Error(), "data cannot be empty for ConfigMap") {
			t.Errorf("Expected error containing 'data cannot be empty for ConfigMap', got %v", err)
		}
	})

	t.Run("IsImmutableConfigMap_WrongKind", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"kind":      "Deployment",
				"immutable": true,
			},
		}
		if isImmutableConfigMap(obj) {
			t.Error("Expected false for non-ConfigMap kind")
		}
	})

	t.Run("IsImmutableConfigMap_MissingField", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"kind": "ConfigMap",
			},
		}
		if isImmutableConfigMap(obj) {
			t.Error("Expected false for a missing immutable field")
		}
	})

	t.Run("IsImmutableConfigMap_ImmutableFalse", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"kind":      "ConfigMap",
				"immutable": false,
			},
		}
		if isImmutableConfigMap(obj) {
			t.Error("Expected false for immutable false")
		}
	})

	t.Run("IsImmutableConfigMap_ImmutableTrue", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"kind":      "ConfigMap",
				"immutable": true,
			},
		}
		if !isImmutableConfigMap(obj) {
			t.Error("Expected true for immutable true")
		}
	})
}

func TestBaseKubernetesManager_ApplySecret(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationReconcileSleep = 50 * time.Millisecond
		return manager
	}

	t.Run("AppliesOpaqueSecretWithStringData", func(t *testing.T) {
		// Given a manager and a client that records the applied object
		manager := setup(t)
		var applied *unstructured.Unstructured
		var appliedGVR schema.GroupVersionResource
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			applied = obj
			appliedGVR = gvr
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		// When applying a secret
		err := manager.ApplySecret("cloudflare-creds", "system-dns", map[string]string{"api_token": "PLAINTEXT"}, "dns-install")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it applies an Opaque Secret via the secrets GVR with the plaintext under stringData
		if appliedGVR.Resource != "secrets" {
			t.Errorf("Expected secrets GVR, got %v", appliedGVR)
		}
		if applied.GetKind() != "Secret" {
			t.Errorf("Expected kind Secret, got %q", applied.GetKind())
		}
		if applied.Object["type"] != "Opaque" {
			t.Errorf("Expected type Opaque, got %v", applied.Object["type"])
		}
		stringData, _ := applied.Object["stringData"].(map[string]string)
		if stringData["api_token"] != "PLAINTEXT" {
			t.Errorf("Expected stringData api_token=PLAINTEXT, got %v", applied.Object["stringData"])
		}
		metadata, _ := applied.Object["metadata"].(map[string]any)
		if metadata["name"] != "cloudflare-creds" || metadata["namespace"] != "system-dns" {
			t.Errorf("Unexpected metadata: %v", applied.Object["metadata"])
		}

		// And it carries the secret-owner marker so PruneSecrets can reclaim it later
		if applied.GetLabels()[secretOwnerLabel] != "dns-install" {
			t.Errorf("Expected secret-owner label dns-install, got %v", applied.GetLabels())
		}
	})

	t.Run("EmptyNameFails", func(t *testing.T) {
		// Given a manager
		manager := setup(t)
		manager.client = client.NewMockKubernetesClient()

		// When applying a secret with no name, it fails validation
		if err := manager.ApplySecret("", "system-dns", map[string]string{"k": "v"}, "dns-install"); err == nil {
			t.Error("Expected error for empty name")
		}
	})

	t.Run("SynthesizesDockerConfigJSONFromDockerCredentials", func(t *testing.T) {
		// Given a manager and a client that records the applied object
		manager := setup(t)
		var applied *unstructured.Unstructured
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			applied = obj
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		// When applying a secret with docker-username/docker-password/docker-server
		err := manager.ApplySecret("ghcr-credentials", "pme", map[string]string{
			"docker-username": "octocat",
			"docker-password": "hunter2",
			"docker-server":   "ghcr.io",
		}, "addon-pme")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the Secret is typed kubernetes.io/dockerconfigjson
		if applied.Object["type"] != "kubernetes.io/dockerconfigjson" {
			t.Errorf("Expected type kubernetes.io/dockerconfigjson, got %v", applied.Object["type"])
		}

		// And the docker-* keys are dropped, replaced by a synthesized .dockerconfigjson
		stringData, _ := applied.Object["stringData"].(map[string]string)
		if _, exists := stringData["docker-username"]; exists {
			t.Error("Expected docker-username to be dropped")
		}
		if _, exists := stringData["docker-password"]; exists {
			t.Error("Expected docker-password to be dropped")
		}
		if _, exists := stringData["docker-server"]; exists {
			t.Error("Expected docker-server to be dropped")
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
		// Given a manager and a client that records the applied object
		manager := setup(t)
		var applied *unstructured.Unstructured
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			applied = obj
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		// When applying a secret with docker-username/docker-password but no docker-server
		err := manager.ApplySecret("ghcr-credentials", "pme", map[string]string{
			"docker-username": "octocat",
			"docker-password": "hunter2",
		}, "addon-pme")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the synthesized config defaults to ghcr.io
		stringData, _ := applied.Object["stringData"].(map[string]string)
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
		// Given a manager and a client that records the applied object
		manager := setup(t)
		var applied *unstructured.Unstructured
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			applied = obj
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		// When applying a secret that already carries a .dockerconfigjson
		preEncoded := `{"auths":{"ghcr.io":{"username":"u","password":"p","auth":"dTpw"}}}`
		err := manager.ApplySecret("ghcr-credentials", "pme", map[string]string{
			".dockerconfigjson": preEncoded,
		}, "addon-pme")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the Secret is typed kubernetes.io/dockerconfigjson and the value passes through unchanged
		if applied.Object["type"] != "kubernetes.io/dockerconfigjson" {
			t.Errorf("Expected type kubernetes.io/dockerconfigjson, got %v", applied.Object["type"])
		}
		stringData, _ := applied.Object["stringData"].(map[string]string)
		if stringData[".dockerconfigjson"] != preEncoded {
			t.Errorf("Expected .dockerconfigjson to pass through unchanged, got %q", stringData[".dockerconfigjson"])
		}
	})

	t.Run("StaysOpaqueWhenOnlyDockerUsernameSet", func(t *testing.T) {
		// Given a manager and a client that records the applied object
		manager := setup(t)
		var applied *unstructured.Unstructured
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			applied = obj
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		// When applying a secret with only docker-username set (no docker-password)
		err := manager.ApplySecret("incomplete-creds", "pme", map[string]string{
			"docker-username": "octocat",
		}, "addon-pme")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the Secret stays Opaque and the docker-username key is untouched
		if applied.Object["type"] != "Opaque" {
			t.Errorf("Expected type Opaque, got %v", applied.Object["type"])
		}
		stringData, _ := applied.Object["stringData"].(map[string]string)
		if stringData["docker-username"] != "octocat" {
			t.Errorf("Expected docker-username to pass through unchanged, got %v", stringData)
		}
	})

	t.Run("DoesNotMutateInputStringDataMap", func(t *testing.T) {
		// Given a manager, a client, and a caller-owned stringData map
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient
		input := map[string]string{
			"docker-username": "octocat",
			"docker-password": "hunter2",
		}

		// When applying a secret that triggers docker config synthesis
		if err := manager.ApplySecret("ghcr-credentials", "pme", input, "addon-pme"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the caller's original map is untouched
		if len(input) != 2 || input["docker-username"] != "octocat" || input["docker-password"] != "hunter2" {
			t.Errorf("Expected input map to be unmodified, got %v", input)
		}
	})

	t.Run("DeletesAndRecreatesWhenResolvedTypeChanges", func(t *testing.T) {
		// Given a manager and an existing Opaque Secret
		manager := setup(t)
		var deleted bool
		var applied *unstructured.Unstructured
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
				Object: map[string]any{
					"kind": "Secret",
					"type": "Opaque",
				},
			}, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, ns, name string, opts metav1.DeleteOptions) error {
			deleted = true
			return nil
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			applied = obj
			return obj, nil
		}
		manager.client = kubernetesClient

		// When applying a secret whose docker-* keys now resolve to dockerconfigjson
		err := manager.ApplySecret("ghcr-credentials", "pme", map[string]string{
			"docker-username": "octocat",
			"docker-password": "hunter2",
		}, "addon-pme")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the existing Secret is deleted before the new type is applied
		if !deleted {
			t.Error("Expected the existing Opaque Secret to be deleted before re-applying as dockerconfigjson")
		}
		if applied.Object["type"] != "kubernetes.io/dockerconfigjson" {
			t.Errorf("Expected type kubernetes.io/dockerconfigjson, got %v", applied.Object["type"])
		}
	})

	t.Run("SkipsDeleteWhenResolvedTypeUnchanged", func(t *testing.T) {
		// Given a manager and an existing dockerconfigjson Secret
		manager := setup(t)
		var deleted bool
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
				Object: map[string]any{
					"kind": "Secret",
					"type": "kubernetes.io/dockerconfigjson",
				},
			}, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, ns, name string, opts metav1.DeleteOptions) error {
			deleted = true
			return nil
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		manager.client = kubernetesClient

		// When applying a secret whose docker-* keys still resolve to dockerconfigjson
		err := manager.ApplySecret("ghcr-credentials", "pme", map[string]string{
			"docker-username": "octocat",
			"docker-password": "hunter2",
		}, "addon-pme")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the existing Secret is left in place (SSA-merged, not deleted)
		if deleted {
			t.Error("Expected no delete when the resolved type matches the existing Secret's type")
		}
	})

	t.Run("DeleteErrorOnTypeChangeIsPropagated", func(t *testing.T) {
		// Given a manager, an existing Opaque Secret, and a delete that fails
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{
				Object: map[string]any{
					"kind": "Secret",
					"type": "Opaque",
				},
			}, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, ns, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("delete failed")
		}
		manager.client = kubernetesClient

		// When applying a secret whose resolved type would change
		err := manager.ApplySecret("ghcr-credentials", "pme", map[string]string{
			"docker-username": "octocat",
			"docker-password": "hunter2",
		}, "addon-pme")

		// Then the delete error is surfaced rather than attempting the apply
		if err == nil {
			t.Fatal("Expected error when delete fails, got nil")
		}
		if !strings.Contains(err.Error(), "failed to delete secret with changed type") {
			t.Errorf("Expected delete-failure error, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_PruneSecrets(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		return NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
	}

	// secretObj builds a CLI-placed Secret list item in a namespace, carrying this context's id and the
	// secret-owner marker — what ListResourcesByLabel would return.
	secretObj := func(name, namespace string) unstructured.Unstructured {
		return unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]any{
					"windsorcli.dev/context-id": "test-context-id",
					secretOwnerLabel:            "pki-install",
				},
			},
		}}
	}

	// wire makes the manager return the given secrets from the label-scoped list and capture deletions.
	wire := func(manager *BaseKubernetesManager, items ...unstructured.Unstructured) (*[]string, *string) {
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
		manager.client = c
		return &deleted, &selector
	}

	t.Run("DeletesOrphansAndKeepsDesired", func(t *testing.T) {
		// Given the same secret placed in two namespaces (a fan-out)
		manager := setup(t)
		deleted, selector := wire(manager,
			secretObj("hetzner-dns", "system-pki"),
			secretObj("hetzner-dns", "system-dns"),
		)

		// When reconciling to a desired set that keeps only the system-pki copy (fan-out shrank)
		desired := map[string]map[string]bool{"system-pki": {"hetzner-dns": true}}
		if err := manager.PruneSecrets(desired); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then only the dropped-namespace copy is deleted
		if len(*deleted) != 1 || (*deleted)[0] != "system-dns/hetzner-dns" {
			t.Errorf("Expected only system-dns/hetzner-dns deleted, got %v", *deleted)
		}
		// And the list is scoped to this context and the CLI secret-owner marker
		if !strings.Contains(*selector, "windsorcli.dev/context-id=test-context-id") || !strings.Contains(*selector, secretOwnerLabel) {
			t.Errorf("Expected context+owner-scoped selector, got %q", *selector)
		}
	})

	t.Run("EmptyDesiredReclaimsEveryCLISecret", func(t *testing.T) {
		// Given CLI-placed secrets but a blueprint that now declares none
		manager := setup(t)
		deleted, _ := wire(manager, secretObj("a", "ns1"), secretObj("b", "ns2"))

		// When reconciling to an empty desired set, every CLI-placed secret is reclaimed
		if err := manager.PruneSecrets(map[string]map[string]bool{}); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(*deleted) != 2 {
			t.Errorf("Expected both CLI secrets reclaimed, got %v", *deleted)
		}
	})

	t.Run("EmptyContextIDFailsClosed", func(t *testing.T) {
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

		// When pruning without a context id, the guard rejects it before deleting anything
		if err := manager.PruneSecrets(nil); err == nil {
			t.Error("Expected error when context id is not set")
		}
	})

	t.Run("InvalidContextIDFailsClosed", func(t *testing.T) {
		// Given a context id that is not a valid label value (a comma would build a
		// malformed selector the API server rejects)
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
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		listed := false
		c := client.NewMockKubernetesClient()
		c.ListResourcesByLabelFunc = func(gvr schema.GroupVersionResource, ns, labelSelector string) (*unstructured.UnstructuredList, error) {
			listed = true
			return &unstructured.UnstructuredList{}, nil
		}
		manager.client = c

		// When pruning, the guard rejects the id up front, names it, and never queries the cluster
		err := manager.PruneSecrets(nil)
		if err == nil || !strings.Contains(err.Error(), "bad,id") {
			t.Errorf("Expected error naming the invalid context id, got %v", err)
		}
		if listed {
			t.Error("Expected no list call when context id is invalid")
		}
	})

	t.Run("DeleteErrorIsPropagated", func(t *testing.T) {
		// Given the delete call fails
		manager := setup(t)
		c := client.NewMockKubernetesClient()
		c.ListResourcesByLabelFunc = func(gvr schema.GroupVersionResource, ns, labelSelector string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{secretObj("x", "ns")}}, nil
		}
		c.DeleteResourceFunc = func(gvr schema.GroupVersionResource, ns, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("api down")
		}
		manager.client = c

		// When reconciling, the delete failure surfaces
		if err := manager.PruneSecrets(map[string]map[string]bool{}); err == nil || !strings.Contains(err.Error(), "api down") {
			t.Errorf("Expected delete error propagated, got %v", err)
		}
	})
}

func TestBaseKubernetesManager_RollWorkloadsForSecret(t *testing.T) {
	const secretName = "alertmanager-slack"
	annotationKey := secretChecksumAnnotationPrefix + secretName

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
	envRef := map[string]any{"containers": []any{map[string]any{"name": "app", "env": []any{
		map[string]any{"name": "URL", "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": secretName, "key": "url"}}},
	}}}}
	volumeRef := map[string]any{"volumes": []any{map[string]any{"name": "s", "secret": map[string]any{"secretName": secretName}}}}
	envFromRef := map[string]any{"containers": []any{map[string]any{"name": "app", "envFrom": []any{
		map[string]any{"secretRef": map[string]any{"name": secretName}},
	}}}}
	noRef := map[string]any{"containers": []any{map[string]any{"name": "app", "env": []any{
		map[string]any{"name": "OTHER", "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "some-other-secret", "key": "k"}}},
	}}}}

	setup := func(t *testing.T, lists map[string][]unstructured.Unstructured) (*BaseKubernetesManager, *map[string]string) {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: lists[gvr.Resource]}, nil
		}
		patched := map[string]string{}
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, ns, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			patched[gvr.Resource+"/"+name] = string(data)
			return nil, nil
		}
		manager.client = kubernetesClient
		return manager, &patched
	}

	t.Run("RollsOnlyReferencingWorkloadsAcrossKinds", func(t *testing.T) {
		// Given one referencing and one non-referencing workload of each kind, referencing via a different path each
		manager, patched := setup(t, map[string][]unstructured.Unstructured{
			"deployments":  {workload("Deployment", "env-consumer", envRef, nil), workload("Deployment", "bystander", noRef, nil)},
			"statefulsets": {workload("StatefulSet", "volume-consumer", volumeRef, nil)},
			"daemonsets":   {workload("DaemonSet", "envfrom-consumer", envFromRef, nil)},
		})

		// When rolling workloads for the secret
		if err := manager.RollWorkloadsForSecret(context.Background(), "system-telemetry", secretName, "digest-1"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then every referencing workload is patched with the digest annotation and the bystander is left alone
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
		// Given a referencing workload that already carries the current digest
		manager, patched := setup(t, map[string][]unstructured.Unstructured{
			"deployments": {workload("Deployment", "env-consumer", envRef, map[string]any{annotationKey: "digest-1"})},
		})

		// When rolling with that same digest
		if err := manager.RollWorkloadsForSecret(context.Background(), "system-telemetry", secretName, "digest-1"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it is not patched, so unchanged content does not churn pods
		if len(*patched) != 0 {
			t.Errorf("Expected no patch for an unchanged digest, got %v", *patched)
		}
	})

	t.Run("ListErrorSurfaces", func(t *testing.T) {
		// Given a client whose workload list fails
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, error) {
			return nil, fmt.Errorf("api down")
		}
		manager.client = kubernetesClient

		// When rolling workloads, the API failure surfaces rather than being swallowed
		if err := manager.RollWorkloadsForSecret(context.Background(), "system-telemetry", secretName, "digest-1"); err == nil {
			t.Error("Expected error when listing workloads fails")
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestSecretTypeChanged(t *testing.T) {
	t.Run("ReportsChangeWhenTypesDiffer", func(t *testing.T) {
		obj := &unstructured.Unstructured{Object: map[string]any{"type": "Opaque"}}
		if !secretTypeChanged(obj, "kubernetes.io/dockerconfigjson") {
			t.Error("Expected change to be reported when types differ")
		}
	})

	t.Run("ReportsNoChangeWhenTypesMatch", func(t *testing.T) {
		obj := &unstructured.Unstructured{Object: map[string]any{"type": "kubernetes.io/dockerconfigjson"}}
		if secretTypeChanged(obj, "kubernetes.io/dockerconfigjson") {
			t.Error("Expected no change to be reported when types match")
		}
	})

	t.Run("ReportsNoChangeWhenExistingTypeMissing", func(t *testing.T) {
		obj := &unstructured.Unstructured{Object: map[string]any{}}
		if secretTypeChanged(obj, "Opaque") {
			t.Error("Expected no change to be reported when the existing object has no type")
		}
	})
}

func TestSecretChecksumAnnotationKey(t *testing.T) {
	t.Run("ShortNameUsedVerbatim", func(t *testing.T) {
		// Given a normal-length secret name, the key stays readable
		if got := secretChecksumAnnotationKey("alertmanager-slack"); got != "checksum.windsorcli.dev/alertmanager-slack" {
			t.Errorf("unexpected key %q", got)
		}
	})

	t.Run("LongNameStaysWithinLimitDeterministicAndUnique", func(t *testing.T) {
		// Given a secret name longer than the 63-char annotation name-segment limit
		long := strings.Repeat("a", 80)
		got := secretChecksumAnnotationKey(long)

		// Then the segment after the prefix is a valid length, deterministic, and distinct per name
		segment := strings.TrimPrefix(got, secretChecksumAnnotationPrefix)
		if len(segment) > 63 {
			t.Errorf("segment %q exceeds 63 chars (%d)", segment, len(segment))
		}
		if got != secretChecksumAnnotationKey(long) {
			t.Error("expected a deterministic key for the same name")
		}
		if got == secretChecksumAnnotationKey(strings.Repeat("a", 79)+"b") {
			t.Error("expected distinct long names to produce distinct keys")
		}
	})
}
