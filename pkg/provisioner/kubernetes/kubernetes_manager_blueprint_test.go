package kubernetes_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseKubernetesManager_ApplyBlueprint(t *testing.T) {
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := kubernetes.NewKubernetesManager(kubeClient, mocks.ConfigHandler)
		manager.Shims = mocks.Shims
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.Shims.FromUnstructured = func(obj map[string]any, target any) error {
			return runtime.DefaultUnstructuredConverter.FromUnstructured(obj, target)
		}
		return manager
	}

	t.Run("AppliesASourceAndAKustomization", func(t *testing.T) {
		// Given a blueprint with one git source and one kustomization
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "test-source",
					Url:  "https://github.com/example/repo.git",
					Ref:  blueprintv1alpha1.Reference{Branch: "main"},
				},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
				},
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("PreservesGitSchemeForSourceURL", func(t *testing.T) {
		// Given a source whose URL uses the git scheme
		var appliedRepo *sourcev1.GitRepository
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "gitrepositories" {
				var repo sourcev1.GitRepository
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &repo); err == nil {
					appliedRepo = &repo
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Sources: []blueprintv1alpha1.Source{
				{Name: "test-source", Url: "git://github.com/example/repo.git", Ref: blueprintv1alpha1.Reference{Branch: "main"}},
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then the GitRepository keeps the git URL
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if appliedRepo == nil {
			t.Fatal("Expected GitRepository to be applied")
		}
		if appliedRepo.Spec.URL != "git://github.com/example/repo.git" {
			t.Errorf("Expected git:// URL to be preserved, got %s", appliedRepo.Spec.URL)
		}
	})

	t.Run("PrefixesHTTPSForInvalidSourceURLScheme", func(t *testing.T) {
		// Given a source whose URL has an invalid scheme
		var appliedRepo *sourcev1.GitRepository
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "gitrepositories" {
				var repo sourcev1.GitRepository
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &repo); err == nil {
					appliedRepo = &repo
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Sources: []blueprintv1alpha1.Source{
				{Name: "test-source", Url: "1invalid://github.com/example/repo.git", Ref: blueprintv1alpha1.Reference{Branch: "main"}},
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then the GitRepository URL gets an https prefix
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if appliedRepo == nil {
			t.Fatal("Expected GitRepository to be applied")
		}
		if appliedRepo.Spec.URL != "https://1invalid://github.com/example/repo.git" {
			t.Errorf("Expected invalid scheme to be https-prefixed, got %s", appliedRepo.Spec.URL)
		}
	})

	t.Run("SuccessSkipsLocalTemplateSource", func(t *testing.T) {
		// Given a blueprint whose only source is the local template
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/example/repo.git",
				Ref: blueprintv1alpha1.Reference{Branch: "main"},
			},
			Sources: []blueprintv1alpha1.Source{
				{Name: "template", Url: ""},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "test-kustomization"},
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error when blueprint has only local template source, got %v", err)
		}
	})

	t.Run("SuccessSkipsDestroyOnlyKustomizations", func(t *testing.T) {
		// Given a blueprint with one destroy-only and one regular kustomization
		kubernetesClient := client.NewMockKubernetesClient()

		var appliedKustomizations []string
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				if name, ok := obj.Object["metadata"].(map[string]any)["name"].(string); ok {
					appliedKustomizations = append(appliedKustomizations, name)
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only-kustomization",
					DestroyOnly: &destroyOnlyTrue,
				},
				{
					Name: "regular-kustomization",
				},
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then only the regular kustomization is applied
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(appliedKustomizations) != 1 {
			t.Errorf("Expected 1 kustomization to be applied, got %d", len(appliedKustomizations))
		}
		if len(appliedKustomizations) > 0 && appliedKustomizations[0] != "regular-kustomization" {
			t.Errorf("Expected 'regular-kustomization' to be applied, got %s", appliedKustomizations[0])
		}
	})

	t.Run("ErrorOnCreateNamespace", func(t *testing.T) {
		// Given a client that rejects the namespace
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "Namespace" {
				return nil, fmt.Errorf("namespace creation failed")
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then the error names the namespace step
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create namespace") {
			t.Errorf("Expected error about namespace creation, got %v", err)
		}
	})

	t.Run("SuccessWithRepository", func(t *testing.T) {
		// Given a blueprint with a primary repository
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/example/repo.git",
				Ref: blueprintv1alpha1.Reference{Branch: "main"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
				},
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithRepositoryAndSecretName", func(t *testing.T) {
		// Given a primary repository with a secret name
		secretName := "test-secret"
		var appliedRepo *sourcev1.GitRepository
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "gitrepositories" {
				var repo sourcev1.GitRepository
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &repo); err == nil {
					appliedRepo = &repo
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.Shims.FromUnstructured = func(obj map[string]any, target any) error {
			return runtime.DefaultUnstructuredConverter.FromUnstructured(obj, target)
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url:        "https://github.com/example/repo.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
				SecretName: &secretName,
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then the GitRepository references the secret
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if appliedRepo == nil {
			t.Fatal("Expected GitRepository to be applied")
		}
		if appliedRepo.Spec.SecretRef == nil {
			t.Error("Expected SecretRef to be set when SecretName is provided")
		} else if appliedRepo.Spec.SecretRef.Name != secretName {
			t.Errorf("Expected SecretRef.Name to be '%s', got '%s'", secretName, appliedRepo.Spec.SecretRef.Name)
		}
	})

	t.Run("SuccessWithRepositoryAndNilSecretName", func(t *testing.T) {
		// Given a primary repository with a nil secret name
		var appliedRepo *sourcev1.GitRepository
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "gitrepositories" {
				var repo sourcev1.GitRepository
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &repo); err == nil {
					appliedRepo = &repo
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.Shims.FromUnstructured = func(obj map[string]any, target any) error {
			return runtime.DefaultUnstructuredConverter.FromUnstructured(obj, target)
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url:        "https://github.com/example/repo.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
				SecretName: nil,
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then the GitRepository has no secret reference
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if appliedRepo == nil {
			t.Fatal("Expected GitRepository to be applied")
		}
		if appliedRepo.Spec.SecretRef != nil {
			t.Errorf("Expected SecretRef to be nil when SecretName is nil, got %v", appliedRepo.Spec.SecretRef)
		}
	})

	t.Run("SuccessWithRepositoryAndEmptySecretName", func(t *testing.T) {
		// Given a primary repository with an empty secret name
		emptySecretName := ""
		var appliedRepo *sourcev1.GitRepository
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "gitrepositories" {
				var repo sourcev1.GitRepository
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &repo); err == nil {
					appliedRepo = &repo
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.Shims.FromUnstructured = func(obj map[string]any, target any) error {
			return runtime.DefaultUnstructuredConverter.FromUnstructured(obj, target)
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url:        "https://github.com/example/repo.git",
				Ref:        blueprintv1alpha1.Reference{Branch: "main"},
				SecretName: &emptySecretName,
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then the GitRepository has no secret reference
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if appliedRepo == nil {
			t.Fatal("Expected GitRepository to be applied")
		}
		if appliedRepo.Spec.SecretRef != nil {
			t.Errorf("Expected SecretRef to be nil when SecretName is empty string, got %v", appliedRepo.Spec.SecretRef)
		}
	})

	t.Run("ErrorOnApplyBlueprintRepository", func(t *testing.T) {
		// Given a client that rejects the primary GitRepository
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "GitRepository" {
				return nil, fmt.Errorf("git repository apply failed")
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/example/repo.git",
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then the error names the blueprint repository
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply blueprint repository") {
			t.Errorf("Expected error about blueprint repository, got %v", err)
		}
	})

	t.Run("ErrorOnApplySource", func(t *testing.T) {
		// Given a client that rejects the source GitRepository
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "GitRepository" {
				return nil, fmt.Errorf("git repository apply failed")
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "test-source",
					Url:  "https://github.com/example/repo.git",
				},
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then the error names the source
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply source") {
			t.Errorf("Expected error about applying source, got %v", err)
		}
	})

	t.Run("ErrorOnApplyConfigMap", func(t *testing.T) {
		// Given a client that rejects ConfigMaps and a kustomization with substitutions
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "ConfigMap" {
				return nil, fmt.Errorf("configmap apply failed")
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Substitutions: map[string]string{
						"key": "value",
					},
				},
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then the error names the ConfigMap
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create ConfigMap") {
			t.Errorf("Expected error about ConfigMap, got %v", err)
		}
	})

	t.Run("ErrorOnApplyKustomization", func(t *testing.T) {
		// Given a client that rejects Kustomizations
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "Kustomization" {
				return nil, fmt.Errorf("kustomization apply failed")
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Path: "test/path",
				},
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then the error names the kustomization
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply kustomization") {
			t.Errorf("Expected error about kustomization, got %v", err)
		}
	})

	t.Run("SuccessWithOCISource", func(t *testing.T) {
		// Given a blueprint with an untagged OCI source
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "oci-source",
					Url:  "oci://example.com/repo",
				},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
				},
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithBlueprintConfigMaps", func(t *testing.T) {
		// Given a blueprint with one ConfigMap and one kustomization
		configMapApplied := false
		kustomizationApplied := false
		var appliedKustomization kustomizev1.Kustomization
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "ConfigMap" && obj.GetName() == "values-common" {
				configMapApplied = true
			}
			if obj.GetKind() == "Kustomization" {
				kustomizationApplied = true
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &appliedKustomization); err != nil {
					t.Fatalf("Failed to convert kustomization: %v", err)
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			ConfigMaps: map[string]map[string]string{
				"values-common": {
					"domain": "example.com",
					"region": "us-west-2",
				},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Path: "test/path",
				},
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then the ConfigMap is applied and the kustomization substitutes from it
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !configMapApplied {
			t.Error("Expected values-common ConfigMap to be applied")
		}
		if !kustomizationApplied {
			t.Error("Expected Kustomization to be applied")
		}
		if appliedKustomization.Spec.PostBuild == nil {
			t.Fatal("Expected PostBuild to be set when blueprint has ConfigMaps")
		}
		if len(appliedKustomization.Spec.PostBuild.SubstituteFrom) != 1 {
			t.Fatalf("Expected 1 SubstituteFrom reference (values-common), got %d", len(appliedKustomization.Spec.PostBuild.SubstituteFrom))
		}
		if appliedKustomization.Spec.PostBuild.SubstituteFrom[0].Name != "values-common" {
			t.Errorf("Expected SubstituteFrom to be values-common, got '%s'", appliedKustomization.Spec.PostBuild.SubstituteFrom[0].Name)
		}
	})

	t.Run("SuccessWithBlueprintConfigMapsAndKustomizationSubstitutions", func(t *testing.T) {
		// Given a blueprint ConfigMap and a kustomization with its own substitutions
		commonConfigMapApplied := false
		kustomizationConfigMapApplied := false
		kustomizationApplied := false
		var appliedKustomization kustomizev1.Kustomization
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "ConfigMap" {
				if obj.GetName() == "values-common" {
					commonConfigMapApplied = true
				}
				if obj.GetName() == "values-test-kustomization" {
					kustomizationConfigMapApplied = true
				}
			}
			if obj.GetKind() == "Kustomization" {
				kustomizationApplied = true
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &appliedKustomization); err != nil {
					t.Fatalf("Failed to convert kustomization: %v", err)
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			ConfigMaps: map[string]map[string]string{
				"values-common": {
					"domain": "example.com",
				},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Path: "test/path",
					Substitutions: map[string]string{
						"key": "value",
					},
				},
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then the kustomization substitutes from both ConfigMaps
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !commonConfigMapApplied {
			t.Error("Expected values-common ConfigMap to be applied")
		}
		if !kustomizationConfigMapApplied {
			t.Error("Expected values-test-kustomization ConfigMap to be applied")
		}
		if !kustomizationApplied {
			t.Error("Expected Kustomization to be applied")
		}
		if appliedKustomization.Spec.PostBuild == nil {
			t.Fatal("Expected PostBuild to be set")
		}
		if len(appliedKustomization.Spec.PostBuild.SubstituteFrom) != 2 {
			t.Fatalf("Expected 2 SubstituteFrom references (values-common and values-test-kustomization), got %d", len(appliedKustomization.Spec.PostBuild.SubstituteFrom))
		}
		foundCommon := false
		foundKustomization := false
		for _, ref := range appliedKustomization.Spec.PostBuild.SubstituteFrom {
			if ref.Name == "values-common" {
				foundCommon = true
			}
			if ref.Name == "values-test-kustomization" {
				foundKustomization = true
			}
		}
		if !foundCommon {
			t.Error("Expected values-common in SubstituteFrom")
		}
		if !foundKustomization {
			t.Error("Expected values-test-kustomization in SubstituteFrom")
		}
	})

	t.Run("SuccessWithMultipleBlueprintConfigMaps", func(t *testing.T) {
		// Given a blueprint with two ConfigMaps
		configMapsApplied := make(map[string]bool)
		var appliedKustomization kustomizev1.Kustomization
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "ConfigMap" {
				configMapsApplied[obj.GetName()] = true
			}
			if obj.GetKind() == "Kustomization" {
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &appliedKustomization); err != nil {
					t.Fatalf("Failed to convert kustomization: %v", err)
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			ConfigMaps: map[string]map[string]string{
				"values-common": {
					"domain": "example.com",
				},
				"values-shared": {
					"shared_key": "shared_value",
				},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Path: "test/path",
				},
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then the kustomization substitutes from both ConfigMaps
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !configMapsApplied["values-common"] {
			t.Error("Expected values-common ConfigMap to be applied")
		}
		if !configMapsApplied["values-shared"] {
			t.Error("Expected values-shared ConfigMap to be applied")
		}
		if appliedKustomization.Spec.PostBuild == nil {
			t.Fatal("Expected PostBuild to be set")
		}
		if len(appliedKustomization.Spec.PostBuild.SubstituteFrom) != 2 {
			t.Fatalf("Expected 2 SubstituteFrom references, got %d", len(appliedKustomization.Spec.PostBuild.SubstituteFrom))
		}
		foundCommon := false
		foundShared := false
		for _, ref := range appliedKustomization.Spec.PostBuild.SubstituteFrom {
			if ref.Name == "values-common" {
				foundCommon = true
			}
			if ref.Name == "values-shared" {
				foundShared = true
			}
		}
		if !foundCommon {
			t.Error("Expected values-common in SubstituteFrom")
		}
		if !foundShared {
			t.Error("Expected values-shared in SubstituteFrom")
		}
	})

	t.Run("SuccessWithKustomizationSubstitutions", func(t *testing.T) {
		// Given a kustomization with its own substitutions
		kustomizationConfigMapApplied := false
		kustomizationApplied := false
		var appliedKustomization kustomizev1.Kustomization
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if obj.GetKind() == "ConfigMap" && obj.GetName() == "values-csi" {
				kustomizationConfigMapApplied = true
			}
			if obj.GetKind() == "Kustomization" {
				kustomizationApplied = true
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &appliedKustomization); err != nil {
					t.Fatalf("Failed to convert kustomization: %v", err)
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "csi",
					Path: "csi",
					Substitutions: map[string]string{
						"volume_path":   "/custom/volumes",
						"storage_class": "fast-ssd",
					},
				},
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then its values ConfigMap is applied and referenced
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !kustomizationConfigMapApplied {
			t.Error("Expected values-csi ConfigMap to be applied")
		}
		if !kustomizationApplied {
			t.Error("Expected Kustomization to be applied")
		}
		if appliedKustomization.Spec.PostBuild == nil {
			t.Fatal("Expected PostBuild to be set when kustomization has substitutions")
		}
		if len(appliedKustomization.Spec.PostBuild.SubstituteFrom) != 1 {
			t.Fatalf("Expected 1 SubstituteFrom reference (values-csi), got %d", len(appliedKustomization.Spec.PostBuild.SubstituteFrom))
		}
		if appliedKustomization.Spec.PostBuild.SubstituteFrom[0].Name != "values-csi" {
			t.Errorf("Expected SubstituteFrom to be values-csi, got '%s'", appliedKustomization.Spec.PostBuild.SubstituteFrom[0].Name)
		}
	})

	t.Run("RegularKustomizationHasCommonMetadataLabels", func(t *testing.T) {
		// Given a blueprint with one regular kustomization
		kubernetesClient := client.NewMockKubernetesClient()

		var appliedKustomization kustomizev1.Kustomization
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &appliedKustomization); err != nil {
					t.Fatalf("Failed to convert kustomization: %v", err)
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "test-source",
					Url:  "https://github.com/example/repo.git",
					Ref:  blueprintv1alpha1.Reference{Branch: "main"},
				},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
					Path: "test/path",
				},
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "test-namespace")

		// Then its commonMetadata carries the context labels
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if appliedKustomization.Spec.CommonMetadata == nil {
			t.Fatal("Expected CommonMetadata to be set on regular kustomization")
		}

		if appliedKustomization.Spec.CommonMetadata.Labels == nil {
			t.Fatal("Expected CommonMetadata.Labels to be set on regular kustomization")
		}

		contextLabel, ok := appliedKustomization.Spec.CommonMetadata.Labels["windsorcli.dev/context"]
		if !ok {
			t.Error("Expected 'windsorcli.dev/context' label to be set")
		}
		if contextLabel != "test-context" {
			t.Errorf("Expected 'windsorcli.dev/context' label to be 'test-context', got '%s'", contextLabel)
		}

		contextIDLabel, ok := appliedKustomization.Spec.CommonMetadata.Labels["windsorcli.dev/context-id"]
		if !ok {
			t.Error("Expected 'windsorcli.dev/context-id' label to be set")
		}
		if contextIDLabel != "test-context-id" {
			t.Errorf("Expected 'windsorcli.dev/context-id' label to be 'test-context-id', got '%s'", contextIDLabel)
		}
	})

	t.Run("RegularKustomizationHasObjectMetaOwnershipLabels", func(t *testing.T) {
		// Given a manager applying a blueprint with one kustomization
		kubernetesClient := client.NewMockKubernetesClient()

		var appliedKustomization kustomizev1.Kustomization
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &appliedKustomization); err != nil {
					t.Fatalf("Failed to convert kustomization: %v", err)
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "test-kustomization", Path: "test/path"},
			},
		}

		// When the blueprint is applied
		if err := manager.ApplyBlueprint(blueprint, "test-namespace"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the Kustomization object itself carries the context ownership labels (selectable by context)
		if appliedKustomization.Labels["windsorcli.dev/context"] != "test-context" {
			t.Errorf("Expected ObjectMeta 'windsorcli.dev/context' label 'test-context', got '%s'", appliedKustomization.Labels["windsorcli.dev/context"])
		}
		if appliedKustomization.Labels["windsorcli.dev/context-id"] != "test-context-id" {
			t.Errorf("Expected ObjectMeta 'windsorcli.dev/context-id' label 'test-context-id', got '%s'", appliedKustomization.Labels["windsorcli.dev/context-id"])
		}
	})

	t.Run("PrimaryRepositoryGetsShortIntervalVendorSourceGetsLong", func(t *testing.T) {
		// Given a blueprint with a primary repository, a vendor source, and a kustomization with no source
		appliedRepos := map[string]*sourcev1.GitRepository{}
		var appliedKust *kustomizev1.Kustomization
		kc := client.NewMockKubernetesClient()
		kc.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			switch gvr.Resource {
			case "gitrepositories":
				var repo sourcev1.GitRepository
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &repo); err == nil {
					appliedRepos[repo.Name] = &repo
				}
			case "kustomizations":
				var k kustomizev1.Kustomization
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &k); err == nil {
					appliedKust = &k
				}
			}
			return obj, nil
		}
		kc.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kc)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/example/live-repo.git",
				Ref: blueprintv1alpha1.Reference{Branch: "main"},
			},
			Sources: []blueprintv1alpha1.Source{
				{Name: "test-source", Url: "https://github.com/example/vendor-repo.git", Ref: blueprintv1alpha1.Reference{Branch: "main"}},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "test-kustomization"},
			},
		}

		// When the blueprint is applied
		if err := manager.ApplyBlueprint(blueprint, "test-namespace"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the primary repository and the kustomization get the short interval and the vendor source gets the long one
		primaryRepo, ok := appliedRepos["test-blueprint"]
		if !ok {
			t.Fatal("Expected the primary repository GitRepository to be applied")
		}
		if primaryRepo.Spec.Interval.Duration != constants.DefaultFluxPrimaryRepositoryInterval {
			t.Errorf("Expected primary repository interval %v, got %v", constants.DefaultFluxPrimaryRepositoryInterval, primaryRepo.Spec.Interval.Duration)
		}

		vendorRepo, ok := appliedRepos["test-source"]
		if !ok {
			t.Fatal("Expected the vendor source GitRepository to be applied")
		}
		if vendorRepo.Spec.Interval.Duration != constants.DefaultFluxSourceInterval {
			t.Errorf("Expected vendor source interval %v, got %v", constants.DefaultFluxSourceInterval, vendorRepo.Spec.Interval.Duration)
		}

		if appliedKust == nil {
			t.Fatal("Expected Kustomization to be applied")
		}
		if appliedKust.Spec.Interval.Duration != constants.DefaultFluxPrimaryKustomizationInterval {
			t.Errorf("Expected kustomization interval %v, got %v", constants.DefaultFluxPrimaryKustomizationInterval, appliedKust.Spec.Interval.Duration)
		}
	})

	t.Run("SplitsAnOCIDigestIntoTheReference", func(t *testing.T) {
		// Given a blueprint with an OCI source pinned by digest
		var appliedRepo *sourcev1.OCIRepository
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "ocirepositories" {
				var repo sourcev1.OCIRepository
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &repo); err == nil {
					appliedRepo = &repo
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Url: "oci://ghcr.io/windsorcli/core@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"},
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "system-gitops")

		// Then the URL drops the digest and the reference holds the digest with no tag
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if appliedRepo == nil {
			t.Fatal("expected an OCIRepository to be applied")
		}
		if appliedRepo.Spec.URL != "oci://ghcr.io/windsorcli/core" {
			t.Errorf("expected URL 'oci://ghcr.io/windsorcli/core', got %s", appliedRepo.Spec.URL)
		}
		if appliedRepo.Spec.Reference == nil {
			t.Fatalf("expected a non-nil Reference")
		}
		if appliedRepo.Spec.Reference.Digest != "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234" {
			t.Errorf("expected Digest 'sha256:abcd...', got %q", appliedRepo.Spec.Reference.Digest)
		}
		if appliedRepo.Spec.Reference.Tag != "" {
			t.Errorf("expected empty Tag for a digest reference, got %q", appliedRepo.Spec.Reference.Tag)
		}
	})

	t.Run("SplitsTheLatestOCITagIntoTheReference", func(t *testing.T) {
		// Given a blueprint with an OCI source tagged latest
		var appliedRepo *sourcev1.OCIRepository
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "ocirepositories" {
				var repo sourcev1.OCIRepository
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &repo); err == nil {
					appliedRepo = &repo
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Url: "oci://ghcr.io/windsorcli/core:latest"},
			},
		}

		// When the blueprint is applied
		err := manager.ApplyBlueprint(blueprint, "system-gitops")

		// Then the URL drops the tag and the reference holds it
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if appliedRepo == nil {
			t.Fatal("expected an OCIRepository to be applied")
		}
		if appliedRepo.Spec.URL != "oci://ghcr.io/windsorcli/core" {
			t.Errorf("expected URL 'oci://ghcr.io/windsorcli/core', got %s", appliedRepo.Spec.URL)
		}
		if appliedRepo.Spec.Reference == nil || appliedRepo.Spec.Reference.Tag != "latest" {
			t.Errorf("expected Reference.Tag 'latest', got %+v", appliedRepo.Spec.Reference)
		}
	})
}

func TestBaseKubernetesManager_DeleteBlueprint(t *testing.T) {
	setup := func(t *testing.T, kubeClient client.KubernetesClient) *kubernetes.BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := kubernetes.NewKubernetesManager(kubeClient, mocks.ConfigHandler)
		manager.Timing.KustomizationWaitPollInterval = 50 * time.Millisecond
		manager.Timing.KustomizationReconcileTimeout = 100 * time.Millisecond
		manager.Timing.KustomizationReconcileSleep = 50 * time.Millisecond
		return manager
	}

	t.Run("DeletesEveryKustomization", func(t *testing.T) {
		// Given a blueprint with two kustomizations
		kubernetesClient := client.NewMockKubernetesClient()

		var deleteCalls []string
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCalls = append(deleteCalls, name)
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization-1",
				},
				{
					Name: "test-kustomization-2",
				},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then both kustomizations are deleted
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(deleteCalls) != 2 {
			t.Errorf("Expected 2 delete calls, got %d", len(deleteCalls))
		}
	})

	t.Run("DeleteTimeoutOverrideSurvivesDeleteLongerThanSpecTimeout", func(t *testing.T) {
		// Given a kustomization that outlives its 30ms spec.timeout but sets a 150ms DeleteTimeout
		clock := newFakeClock()

		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		var getCalls int
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			getCalls++
			if getCalls < 9 {
				return &unstructured.Unstructured{Object: map[string]any{
					"spec": map[string]any{"timeout": "30ms"},
				}}, nil
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 20 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 10 * time.Millisecond
		manager.Timing.KustomizationReconcileSleep = 0
		manager.Shims.TimeNow = clock.Now
		manager.Shims.TimeSleep = clock.Sleep

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:          "demo-resources",
					DeleteTimeout: &blueprintv1alpha1.DurationString{Duration: 150 * time.Millisecond},
				},
			},
		}

		// When DeleteBlueprint deletes it
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the DeleteTimeout sets the wait floor and the delete succeeds
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessSkipsDestroyFalse", func(t *testing.T) {
		// Given a blueprint where one kustomization sets destroy false
		kubernetesClient := client.NewMockKubernetesClient()

		var deleteCalls []string
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCalls = append(deleteCalls, name)
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)

		destroyFalse := false
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization-1",
					Destroy: &blueprintv1alpha1.BoolExpression{Value: &destroyFalse, IsExpr: false},
				},
				{
					Name:    "test-kustomization-2",
					Destroy: nil,
				},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then only the other kustomization is deleted
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(deleteCalls) != 1 {
			t.Errorf("Expected 1 delete call, got %d", len(deleteCalls))
		}
		if deleteCalls[0] != "test-kustomization-2" {
			t.Errorf("Expected delete call for 'test-kustomization-2', got %s", deleteCalls[0])
		}
	})

	t.Run("SuspendsEligibleKustomizationsBeforeDeleting", func(t *testing.T) {
		// Given two eligible kustomizations and one that sets destroy false
		kubernetesClient := client.NewMockKubernetesClient()

		var events []string
		var suspendPatches []string
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			if string(data) == `{"spec":{"suspend":true}}` {
				suspendPatches = append(suspendPatches, name)
				events = append(events, "suspend:"+name)
			}
			return nil, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			events = append(events, "delete:"+name)
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)

		destroyFalse := false
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "test-kustomization-1"},
				{Name: "test-kustomization-2"},
				{Name: "pinned-keep", Destroy: &blueprintv1alpha1.BoolExpression{Value: &destroyFalse, IsExpr: false}},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then only the eligible ones are suspended, all before any delete
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(suspendPatches) != 2 {
			t.Fatalf("Expected 2 suspend patches, got %d (%v)", len(suspendPatches), suspendPatches)
		}
		for _, name := range suspendPatches {
			if name == "pinned-keep" {
				t.Errorf("Expected destroy=false kustomization not to be suspended, but %q was", name)
			}
		}

		firstDelete := -1
		lastSuspend := -1
		for i, e := range events {
			switch {
			case strings.HasPrefix(e, "delete:") && firstDelete == -1:
				firstDelete = i
			case strings.HasPrefix(e, "suspend:"):
				lastSuspend = i
			}
		}
		if firstDelete == -1 {
			t.Fatalf("Expected at least one delete, got events %v", events)
		}
		if lastSuspend == -1 {
			t.Fatalf("Expected at least one suspend, got events %v", events)
		}
		if lastSuspend >= firstDelete {
			t.Fatalf("Expected all suspends before any delete, got events %v", events)
		}
	})

	t.Run("AbortsWhenSuspendFails", func(t *testing.T) {
		// Given a client that fails every suspend patch
		kubernetesClient := client.NewMockKubernetesClient()

		var deleteCalls []string
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("connection refused")
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCalls = append(deleteCalls, name)
			return nil
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "test-kustomization-1"}},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the destroy aborts before any delete
		if err == nil {
			t.Fatal("Expected error when suspend fails, got nil")
		}
		if !strings.Contains(err.Error(), "failed to suspend kustomization") {
			t.Errorf("Expected suspend-failure error, got %v", err)
		}
		if len(deleteCalls) != 0 {
			t.Errorf("Expected no delete calls when suspend fails, got %v", deleteCalls)
		}
	})

	t.Run("ToleratesNotFoundOnSuspend", func(t *testing.T) {
		// Given a client that reports not-found on every suspend patch
		kubernetesClient := client.NewMockKubernetesClient()

		var deleteCalls []string
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCalls = append(deleteCalls, name)
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "test-kustomization-1"}},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the kustomization is still deleted
		if err != nil {
			t.Fatalf("Expected NotFound on suspend to be tolerated, got %v", err)
		}
		if len(deleteCalls) != 1 {
			t.Errorf("Expected 1 delete call, got %v", deleteCalls)
		}
	})

	t.Run("ResumesEachKustomizationImmediatelyBeforeDeletingIt", func(t *testing.T) {
		// Given two eligible kustomizations and a client that records patches and deletes in order
		kubernetesClient := client.NewMockKubernetesClient()

		var events []string
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			switch string(data) {
			case `{"spec":{"suspend":true}}`:
				events = append(events, "suspend:"+name)
			case `{"spec":{"suspend":false}}`:
				events = append(events, "resume:"+name)
			}
			return nil, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			events = append(events, "delete:"+name)
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "test-kustomization-1"},
				{Name: "test-kustomization-2"},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then all suspends come first and each resume comes immediately before its delete
		firstResume, lastSuspend := -1, -1
		for i, e := range events {
			if strings.HasPrefix(e, "suspend:") {
				lastSuspend = i
			}
			if strings.HasPrefix(e, "resume:") && firstResume == -1 {
				firstResume = i
			}
		}
		if lastSuspend == -1 || firstResume == -1 {
			t.Fatalf("Expected both suspend and resume patches, got events %v", events)
		}
		if lastSuspend >= firstResume {
			t.Fatalf("Expected all suspends before any resume (freeze-all-first), got events %v", events)
		}
		for _, name := range []string{"test-kustomization-1", "test-kustomization-2"} {
			resumeIdx, deleteIdx := -1, -1
			for i, e := range events {
				switch e {
				case "resume:" + name:
					resumeIdx = i
				case "delete:" + name:
					deleteIdx = i
				}
			}
			if resumeIdx == -1 || deleteIdx == -1 {
				t.Fatalf("Expected %q to be both resumed and deleted, got events %v", name, events)
			}
			if resumeIdx != deleteIdx-1 {
				t.Fatalf("Expected resume of %q immediately before its delete, got events %v", name, events)
			}
		}
	})

	t.Run("StillResumesAndDeletesAnUnrelatedKustomizationAfterAFailure", func(t *testing.T) {
		// Given two independent kustomizations where the first one in destroy order fails to delete
		kubernetesClient := client.NewMockKubernetesClient()

		var events []string
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			switch string(data) {
			case `{"spec":{"suspend":true}}`:
				events = append(events, "suspend:"+name)
			case `{"spec":{"suspend":false}}`:
				events = append(events, "resume:"+name)
			}
			return nil, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			events = append(events, "delete:"+name)
			if name == "test-kustomization-2" {
				return fmt.Errorf("finalizer stuck")
			}
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "test-kustomization-1"},
				{Name: "test-kustomization-2"},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the failure is returned and the unrelated kustomization is still deleted
		if err == nil {
			t.Fatal("Expected error when delete fails, got nil")
		}
		if !strings.Contains(err.Error(), "failed to delete kustomization") {
			t.Errorf("Expected delete-failure error, got %v", err)
		}

		deleted := map[string]bool{}
		for _, e := range events {
			if strings.HasPrefix(e, "delete:") {
				deleted[strings.TrimPrefix(e, "delete:")] = true
			}
		}
		if !deleted["test-kustomization-1"] {
			t.Errorf("Expected the unrelated test-kustomization-1 to still be deleted, got events %v", events)
		}
	})

	t.Run("SuccessWithDestroyOnlyKustomizations", func(t *testing.T) {
		// Given a ready destroy-only kustomization and a regular one
		kubernetesClient := client.NewMockKubernetesClient()

		var applyCalls []string
		var deleteCalls []string
		deletedResources := make(map[string]bool)

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if name, ok := obj.Object["metadata"].(map[string]any)["name"].(string); ok {
				applyCalls = append(applyCalls, name)
			}
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCalls = append(deleteCalls, name)
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only-kustomization"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		manager := setup(t, kubernetesClient)

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only-kustomization",
					DestroyOnly: &destroyOnlyTrue,
				},
				{
					Name: "regular-kustomization",
				},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the destroy-only one is applied and both are deleted
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(applyCalls) != 1 {
			t.Errorf("Expected 1 apply call (destroy-only kustomization), got %d", len(applyCalls))
		}
		if len(applyCalls) >= 1 && applyCalls[0] != "destroy-only-kustomization" {
			t.Errorf("Expected apply call for 'destroy-only-kustomization', got %s", applyCalls[0])
		}

		if len(deleteCalls) != 2 {
			t.Errorf("Expected 2 delete calls (destroy-only + regular), got %d", len(deleteCalls))
		}
	})

	t.Run("DestroyOnlyDeleteTimeoutOverrideSurvivesDeleteLongerThanSpecTimeout", func(t *testing.T) {
		// Given a destroy-only kustomization that outlives its 15ms spec.timeout but sets a 150ms DeleteTimeout
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "backup-cleanup"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		var getCalls int
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			getCalls++
			if getCalls < 7 {
				return &unstructured.Unstructured{Object: map[string]any{
					"spec": map[string]any{"timeout": "15ms"},
				}}, nil
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)
		manager.Timing.KustomizationReconcileTimeout = 20 * time.Millisecond
		manager.Timing.KustomizationWaitPollInterval = 10 * time.Millisecond

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:          "backup-cleanup",
					DestroyOnly:   &destroyOnlyTrue,
					DeleteTimeout: &blueprintv1alpha1.DurationString{Duration: 150 * time.Millisecond},
				},
			},
		}

		// When DeleteBlueprint deletes it
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the DeleteTimeout sets the wait floor and the delete succeeds
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DestroyOnlyProcessedBeforeRegular", func(t *testing.T) {
		// Given a regular kustomization listed before a destroy-only one
		kubernetesClient := client.NewMockKubernetesClient()

		var operations []string
		deletedResources := make(map[string]bool)

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if name, ok := obj.Object["metadata"].(map[string]any)["name"].(string); ok {
				operations = append(operations, "apply:"+name)
			}
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			operations = append(operations, "delete:"+name)
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		manager := setup(t, kubernetesClient)

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "regular",
				},
				{
					Name:        "destroy-only",
					DestroyOnly: &destroyOnlyTrue,
				},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the destroy-only one is applied and deleted before the regular one
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		applyDestroyOnlyIdx := -1
		deleteDestroyOnlyIdx := -1
		deleteRegularIdx := -1

		for i, op := range operations {
			switch op {
			case "apply:destroy-only":
				applyDestroyOnlyIdx = i
			case "delete:destroy-only":
				deleteDestroyOnlyIdx = i
			case "delete:regular":
				deleteRegularIdx = i
			}
		}

		if applyDestroyOnlyIdx == -1 {
			t.Error("Expected destroy-only kustomization to be applied")
		}
		if deleteDestroyOnlyIdx == -1 {
			t.Error("Expected destroy-only kustomization to be deleted")
		}
		if deleteRegularIdx == -1 {
			t.Error("Expected regular kustomization to be deleted")
		}

		if applyDestroyOnlyIdx > deleteDestroyOnlyIdx {
			t.Error("Expected destroy-only to be applied before deleted")
		}
		if deleteDestroyOnlyIdx > deleteRegularIdx {
			t.Error("Expected destroy-only to be deleted before regular kustomization")
		}
	})

	t.Run("DestroyOnlyFailsWhenNotReady", func(t *testing.T) {
		// Given a destroy-only kustomization that never becomes ready
		kubernetesClient := client.NewMockKubernetesClient()

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "False"},
								},
							},
						},
					},
				},
			}, nil
		}
		manager := setup(t, kubernetesClient)

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only",
					DestroyOnly: &destroyOnlyTrue,
				},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the timeout error names the stuck kustomization
		if err == nil {
			t.Error("Expected error when destroy-only kustomization does not become ready")
		}
		if !strings.Contains(err.Error(), "did not become ready within timeout") {
			t.Errorf("Expected timeout error, got %v", err)
		}
		if !strings.Contains(err.Error(), "cleanup may not have completed: destroy-only") {
			t.Errorf("Expected timeout error to name the stuck kustomization, got: %v", err)
		}
	})

	t.Run("DestroyOnlyWithSubstitutions", func(t *testing.T) {
		// Given a destroy-only kustomization with substitutions
		kubernetesClient := client.NewMockKubernetesClient()

		var appliedConfigMaps []string
		deletedResources := make(map[string]bool)

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "configmaps" {
				appliedConfigMaps = append(appliedConfigMaps, obj.GetName())
			}
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		manager := setup(t, kubernetesClient)

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:          "destroy-only",
					DestroyOnly:   &destroyOnlyTrue,
					Substitutions: map[string]string{"key": "value"},
				},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then its values ConfigMap is applied
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		foundValuesConfigMap := false
		for _, name := range appliedConfigMaps {
			if name == "values-destroy-only" {
				foundValuesConfigMap = true
			}
		}
		if !foundValuesConfigMap {
			t.Error("Expected ConfigMap 'values-destroy-only' to be created for substitutions")
		}
	})

	t.Run("DestroyOnlyWithBlueprintConfigMaps", func(t *testing.T) {
		// Given a destroy-only kustomization and a blueprint ConfigMap
		kubernetesClient := client.NewMockKubernetesClient()

		var appliedKustomizations []map[string]any
		deletedResources := make(map[string]bool)

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				if spec, ok := obj.Object["spec"].(map[string]any); ok {
					appliedKustomizations = append(appliedKustomizations, spec)
				}
			}
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		manager := setup(t, kubernetesClient)

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata:   blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			ConfigMaps: map[string]map[string]string{"values-common": {"key": "value"}},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only",
					DestroyOnly: &destroyOnlyTrue,
				},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the destroy-only kustomization substitutes from the ConfigMap
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(appliedKustomizations) == 0 {
			t.Fatal("Expected at least one kustomization to be applied")
		}
		postBuild, ok := appliedKustomizations[0]["postBuild"].(map[string]any)
		if !ok {
			t.Fatal("Expected postBuild in kustomization spec")
		}
		substituteFrom, ok := postBuild["substituteFrom"].([]any)
		if !ok {
			t.Fatal("Expected substituteFrom in postBuild")
		}
		foundValuesCommon := false
		for _, ref := range substituteFrom {
			refMap := ref.(map[string]any)
			if refMap["name"] == "values-common" {
				foundValuesCommon = true
			}
		}
		if !foundValuesCommon {
			t.Error("Expected blueprint ConfigMap 'values-common' to be referenced in destroy-only kustomization")
		}
	})

	t.Run("DestroyOnlyBecomesReady", func(t *testing.T) {
		// Given a destroy-only kustomization that reports ready
		kubernetesClient := client.NewMockKubernetesClient()

		deletedResources := make(map[string]bool)

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		manager := setup(t, kubernetesClient)

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only",
					DestroyOnly: &destroyOnlyTrue,
				},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the destroy-only kustomization is deleted
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if !deletedResources["destroy-only"] {
			t.Error("Expected destroy-only kustomization to be deleted after becoming ready")
		}
	})

	t.Run("DestroyOnlyErrorApplyingKustomization", func(t *testing.T) {
		// Given a client that rejects the destroy-only Kustomization
		kubernetesClient := client.NewMockKubernetesClient()

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				return nil, fmt.Errorf("apply kustomization error")
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only",
					DestroyOnly: &destroyOnlyTrue,
				},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the error names the destroy-only apply
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply destroy-only kustomization") {
			t.Errorf("Expected error about applying destroy-only kustomization, got %v", err)
		}
	})

	t.Run("DestroyOnlyErrorDeletingKustomization", func(t *testing.T) {
		// Given a ready destroy-only kustomization whose delete fails
		kubernetesClient := client.NewMockKubernetesClient()

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			if name == "destroy-only" {
				return fmt.Errorf("delete kustomization error")
			}
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		manager := setup(t, kubernetesClient)

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only",
					DestroyOnly: &destroyOnlyTrue,
				},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the error names the destroy-only delete
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to delete destroy-only kustomization") {
			t.Errorf("Expected error about deleting destroy-only kustomization, got %v", err)
		}
	})

	t.Run("DestroyOnlyErrorCreatingConfigMap", func(t *testing.T) {
		// Given a client that rejects the values ConfigMap of a destroy-only kustomization
		kubernetesClient := client.NewMockKubernetesClient()

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "configmaps" && obj.GetName() == "values-destroy-only" {
				return nil, fmt.Errorf("configmap creation error")
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:          "destroy-only",
					DestroyOnly:   &destroyOnlyTrue,
					Substitutions: map[string]string{"key": "value"},
				},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the error names the ConfigMap
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create ConfigMap for destroy-only kustomization") {
			t.Errorf("Expected error about ConfigMap creation, got %v", err)
		}
	})

	t.Run("DestroyOnlyCleansUpOnApplyFailure", func(t *testing.T) {
		// Given two destroy-only kustomizations where the second apply fails
		kubernetesClient := client.NewMockKubernetesClient()

		appliedKustomizations := []string{}
		deletedKustomizations := []string{}

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				name := obj.GetName()
				appliedKustomizations = append(appliedKustomizations, name)
				if name == "destroy-only-b" {
					return nil, fmt.Errorf("apply kustomization error")
				}
			}
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			if gvr.Resource == "kustomizations" {
				deletedKustomizations = append(deletedKustomizations, name)
			}
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only-a",
					DestroyOnly: &destroyOnlyTrue,
				},
				{
					Name:        "destroy-only-b",
					DestroyOnly: &destroyOnlyTrue,
				},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the first one is deleted again
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply destroy-only kustomization") {
			t.Errorf("Expected error about applying destroy-only kustomization, got %v", err)
		}

		if len(appliedKustomizations) != 2 {
			t.Errorf("Expected 2 kustomizations to be attempted, got %d", len(appliedKustomizations))
		}
		if appliedKustomizations[0] != "destroy-only-a" {
			t.Errorf("Expected first kustomization to be destroy-only-a, got %s", appliedKustomizations[0])
		}
		if appliedKustomizations[1] != "destroy-only-b" {
			t.Errorf("Expected second kustomization to be destroy-only-b, got %s", appliedKustomizations[1])
		}

		if len(deletedKustomizations) != 1 {
			t.Errorf("Expected 1 kustomization to be cleaned up, got %d", len(deletedKustomizations))
		}
		if deletedKustomizations[0] != "destroy-only-a" {
			t.Errorf("Expected destroy-only-a to be cleaned up, got %v", deletedKustomizations)
		}
	})

	t.Run("DestroyOnlyCleansUpOnConfigMapFailure", func(t *testing.T) {
		// Given two destroy-only kustomizations where the second ConfigMap fails
		kubernetesClient := client.NewMockKubernetesClient()

		appliedKustomizations := []string{}
		deletedKustomizations := []string{}

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				name := obj.GetName()
				appliedKustomizations = append(appliedKustomizations, name)
			}
			if gvr.Resource == "configmaps" && obj.GetName() == "values-destroy-only-b" {
				return nil, fmt.Errorf("configmap creation error")
			}
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			if gvr.Resource == "kustomizations" {
				deletedKustomizations = append(deletedKustomizations, name)
			}
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager := setup(t, kubernetesClient)

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only-a",
					DestroyOnly: &destroyOnlyTrue,
				},
				{
					Name:          "destroy-only-b",
					DestroyOnly:   &destroyOnlyTrue,
					Substitutions: map[string]string{"key": "value"},
				},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the first one is deleted again
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create ConfigMap for destroy-only kustomization") {
			t.Errorf("Expected error about ConfigMap creation, got %v", err)
		}

		if len(appliedKustomizations) != 1 {
			t.Errorf("Expected 1 kustomization to be applied, got %d", len(appliedKustomizations))
		}
		if appliedKustomizations[0] != "destroy-only-a" {
			t.Errorf("Expected destroy-only-a to be applied, got %s", appliedKustomizations[0])
		}

		if len(deletedKustomizations) != 1 {
			t.Errorf("Expected 1 kustomization to be cleaned up, got %d", len(deletedKustomizations))
		}
		if deletedKustomizations[0] != "destroy-only-a" {
			t.Errorf("Expected destroy-only-a to be cleaned up, got %v", deletedKustomizations)
		}
	})

	t.Run("DestroyOnlyWithDependencyOnAnotherDestroyOnly", func(t *testing.T) {
		// Given a destroy-only kustomization that depends on another destroy-only one
		kubernetesClient := client.NewMockKubernetesClient()

		var appliedKustomizations []map[string]any
		deletedResources := make(map[string]bool)

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				spec, _ := obj.Object["spec"].(map[string]any)
				appliedKustomizations = append(appliedKustomizations, map[string]any{
					"name":      obj.GetName(),
					"dependsOn": spec["dependsOn"],
				})
			}
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only-a"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only-b"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		manager := setup(t, kubernetesClient)

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only-a",
					DestroyOnly: &destroyOnlyTrue,
				},
				{
					Name:        "destroy-only-b",
					DestroyOnly: &destroyOnlyTrue,
					DependsOn:   []string{"destroy-only-a"},
				},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the dependency is kept on the applied object
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(appliedKustomizations) != 2 {
			t.Fatalf("Expected 2 kustomizations applied, got %d", len(appliedKustomizations))
		}

		var kustB map[string]any
		for _, k := range appliedKustomizations {
			if k["name"] == "destroy-only-b" {
				kustB = k
				break
			}
		}

		if kustB == nil {
			t.Fatal("Expected destroy-only-b to be applied")
		}

		dependsOn, ok := kustB["dependsOn"].([]any)
		if !ok || len(dependsOn) != 1 {
			t.Errorf("Expected destroy-only-b to have 1 dependency, got %v", kustB["dependsOn"])
		}
	})

	t.Run("DestroyOnlyFiltersDependencyOnRegularKustomization", func(t *testing.T) {
		// Given a destroy-only kustomization that depends on a regular one
		kubernetesClient := client.NewMockKubernetesClient()

		var appliedKustomizations []map[string]any
		deletedResources := make(map[string]bool)

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				spec, _ := obj.Object["spec"].(map[string]any)
				appliedKustomizations = append(appliedKustomizations, map[string]any{
					"name":      obj.GetName(),
					"dependsOn": spec["dependsOn"],
				})
			}
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only-cleanup"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		manager := setup(t, kubernetesClient)

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "regular-kustomization",
				},
				{
					Name:        "destroy-only-cleanup",
					DestroyOnly: &destroyOnlyTrue,
					DependsOn:   []string{"regular-kustomization"},
				},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the dependency is removed from the applied object
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		var destroyOnlyKust map[string]any
		for _, k := range appliedKustomizations {
			if k["name"] == "destroy-only-cleanup" {
				destroyOnlyKust = k
				break
			}
		}

		if destroyOnlyKust == nil {
			t.Fatal("Expected destroy-only-cleanup to be applied")
		}

		dependsOn := destroyOnlyKust["dependsOn"]
		if dependsOn != nil {
			deps, ok := dependsOn.([]any)
			if ok && len(deps) > 0 {
				t.Errorf("Expected dependency on regular kustomization to be filtered out, got %v", dependsOn)
			}
		}
	})

	t.Run("ErrorDeleteKustomization", func(t *testing.T) {
		// Given a client that fails every delete
		kubernetesClient := client.NewMockKubernetesClient()

		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("delete error")
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name: "test-kustomization",
				},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the error names the kustomization delete
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to delete kustomization") {
			t.Errorf("Expected error containing 'failed to delete kustomization', got %v", err)
		}
	})

	t.Run("AbandonedInventoryUsesCurrentDestroySettingNotStaleLiveField", func(t *testing.T) {
		// Given a live kustomization with a stale MirrorPrune policy whose blueprint entry now sets destroy true
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		liveObj := &unstructured.Unstructured{Object: map[string]any{
			"spec": map[string]any{"deletionPolicy": "MirrorPrune"},
			"status": map[string]any{
				"inventory": map[string]any{
					"entries": []any{
						map[string]any{"id": "test-namespace_entry-one__ConfigMap"},
					},
				},
			},
		}}
		calls := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if name == "test-kustomization" {
				calls++
				if calls <= 5 {
					return liveObj, nil
				}
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			if name == "entry-one" {
				return blockingObject(), nil
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)

		destroyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "test-kustomization",
					Destroy: &blueprintv1alpha1.BoolExpression{Value: &destroyTrue, IsExpr: false},
				},
			},
		}

		// When DeleteBlueprint deletes it
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the error names the abandoned inventory entry
		if err == nil {
			t.Fatal("Expected an error naming the abandoned inventory, got nil")
		}
		if !strings.Contains(err.Error(), "entry-one") {
			t.Errorf("Expected error to name the still-live entry, got: %v", err)
		}
	})

	t.Run("ContinuesPastAnUnrelatedFailureButSkipsAFailedKustomizationsOwnDependency", func(t *testing.T) {
		// Given an unrelated kustomization and a failing one with its own dependency
		kubernetesClient := client.NewMockKubernetesClient()

		var deleteCalls []string
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCalls = append(deleteCalls, name)
			if name == "second-kust" {
				return fmt.Errorf("delete error")
			}
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "first-kust"},
				{Name: "second-kust", DependsOn: []string{"its-dependency"}},
				{Name: "its-dependency"},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the unrelated one is deleted and the dependency is skipped
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "second-kust") {
			t.Errorf("Expected error to name the failing kustomization 'second-kust', got %v", err)
		}

		attempted := map[string]bool{}
		for _, name := range deleteCalls {
			attempted[name] = true
		}
		if !attempted["first-kust"] {
			t.Errorf("Expected the unrelated first-kust to still be attempted, got %v", deleteCalls)
		}
		if attempted["its-dependency"] {
			t.Errorf("Expected second-kust's own dependency to be skipped, got %v", deleteCalls)
		}
	})

	t.Run("DestroyOnlyKustomizationHasCommonMetadataLabels", func(t *testing.T) {
		// Given a ready destroy-only kustomization
		kubernetesClient := client.NewMockKubernetesClient()

		var appliedKustomization kustomizev1.Kustomization
		deletedResources := make(map[string]bool)

		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "kustomizations" {
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &appliedKustomization); err != nil {
					t.Fatalf("Failed to convert kustomization: %v", err)
				}
			}
			return obj, nil
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deletedResources[name] = true
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			if deletedResources[name] {
				return nil, fmt.Errorf("the server could not find the requested resource")
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		kubernetesClient.ListResourcesFunc = func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
							"kind":       "Kustomization",
							"metadata":   map[string]any{"name": "destroy-only"},
							"status": map[string]any{
								"conditions": []any{
									map[string]any{"type": "Ready", "status": "True"},
								},
							},
						},
					},
				},
			}, nil
		}
		manager := setup(t, kubernetesClient)
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.Shims.FromUnstructured = func(obj map[string]any, target any) error {
			return runtime.DefaultUnstructuredConverter.FromUnstructured(obj, target)
		}

		destroyOnlyTrue := true
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:        "destroy-only",
					DestroyOnly: &destroyOnlyTrue,
					Path:        "test/path",
				},
			},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then it carries the context labels on the object and in commonMetadata
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if appliedKustomization.Spec.CommonMetadata == nil {
			t.Fatal("Expected CommonMetadata to be set on destroy-only kustomization")
		}

		if appliedKustomization.Spec.CommonMetadata.Labels == nil {
			t.Fatal("Expected CommonMetadata.Labels to be set on destroy-only kustomization")
		}

		contextLabel, ok := appliedKustomization.Spec.CommonMetadata.Labels["windsorcli.dev/context"]
		if !ok {
			t.Error("Expected 'windsorcli.dev/context' label to be set")
		}
		if contextLabel != "test-context" {
			t.Errorf("Expected 'windsorcli.dev/context' label to be 'test-context', got '%s'", contextLabel)
		}

		contextIDLabel, ok := appliedKustomization.Spec.CommonMetadata.Labels["windsorcli.dev/context-id"]
		if !ok {
			t.Error("Expected 'windsorcli.dev/context-id' label to be set")
		}
		if contextIDLabel != "test-context-id" {
			t.Errorf("Expected 'windsorcli.dev/context-id' label to be 'test-context-id', got '%s'", contextIDLabel)
		}

		if appliedKustomization.Labels["windsorcli.dev/context"] != "test-context" {
			t.Errorf("Expected ObjectMeta 'windsorcli.dev/context' label 'test-context', got '%s'", appliedKustomization.Labels["windsorcli.dev/context"])
		}
		if appliedKustomization.Labels["windsorcli.dev/context-id"] != "test-context-id" {
			t.Errorf("Expected ObjectMeta 'windsorcli.dev/context-id' label 'test-context-id', got '%s'", appliedKustomization.Labels["windsorcli.dev/context-id"])
		}
	})

	t.Run("AbortStopsAfterTheFirstResumeTimeout", func(t *testing.T) {
		// Given a failed suspend and a request timeout on every resume
		var resumed []string
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			if string(data) == `{"spec":{"suspend":true}}` {
				return nil, fmt.Errorf("connection refused")
			}
			resumed = append(resumed, name)
			return nil, context.DeadlineExceeded
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "crds"}, {Name: "database-install"}, {Name: "demo-resources"}},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "system-gitops")

		// Then the abort tries one resume and reports how many it left suspended
		if len(resumed) != 1 {
			t.Fatalf("expected exactly 1 un-suspend attempt, got %d: %v", len(resumed), resumed)
		}
		if resumed[0] != "crds" {
			t.Errorf("expected the first eligible kustomization attempted, got %q", resumed[0])
		}
		if err == nil || !strings.Contains(err.Error(), "stopping abort cleanup after a request timeout") {
			t.Errorf("expected an early-stop note in the joined error, got: %v", err)
		}
		if err == nil || !strings.Contains(err.Error(), "2 more kustomization(s) left un-suspended") {
			t.Errorf("expected the remaining count in the joined error, got: %v", err)
		}
	})

	t.Run("AbortContinuesPastANonTimeoutResumeFailure", func(t *testing.T) {
		// Given a failed suspend and a non-timeout rejection on every resume
		var resumed []string
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			if string(data) == `{"spec":{"suspend":true}}` {
				return nil, fmt.Errorf("connection refused")
			}
			resumed = append(resumed, name)
			return nil, fmt.Errorf("conflict: object has been modified")
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "crds"}, {Name: "database-install"}, {Name: "demo-resources"}},
		}

		// When the blueprint is deleted
		err := manager.DeleteBlueprint(context.Background(), blueprint, "system-gitops")

		// Then the abort tries to resume every eligible kustomization
		if len(resumed) != 3 {
			t.Fatalf("expected all 3 kustomizations attempted, got %d: %v", len(resumed), resumed)
		}
		if err == nil || strings.Contains(err.Error(), "stopping abort cleanup") {
			t.Errorf("expected no early-stop note for a non-timeout failure, got: %v", err)
		}
	})

	t.Run("WarnsWithPluralWhenAFailureSkipsTwoDependencies", func(t *testing.T) {
		// Given a kustomization that depends on two others and fails to delete
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			if name == "app" {
				return fmt.Errorf("delete error")
			}
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app", DependsOn: []string{"a", "b"}},
				{Name: "a"},
				{Name: "b"},
			},
		}
		ctx, rec := recordProgress(t)

		// When the blueprint is deleted
		_ = manager.DeleteBlueprint(ctx, blueprint, "test-namespace")

		// Then the warning uses the plural
		output := strings.Join(rec.warnings(), "\n")
		if !strings.Contains(output, "skipping 2 dependencies") {
			t.Errorf("expected plural wording for 2 skipped, got: %s", output)
		}
	})

	t.Run("WarnsWithSingularWhenAFailureSkipsOneDependency", func(t *testing.T) {
		// Given a kustomization that depends on one other and fails to delete
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			if name == "app" {
				return fmt.Errorf("delete error")
			}
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager := setup(t, kubernetesClient)
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app", DependsOn: []string{"a"}},
				{Name: "a"},
			},
		}
		ctx, rec := recordProgress(t)

		// When the blueprint is deleted
		_ = manager.DeleteBlueprint(ctx, blueprint, "test-namespace")

		// Then the warning stays singular
		output := strings.Join(rec.warnings(), "\n")
		if !strings.Contains(output, "skipping 1 dependency ") {
			t.Errorf("expected singular wording for 1 skipped, got: %s", output)
		}
		if strings.Contains(output, "dependencies") {
			t.Errorf("expected no plural wording for 1 skipped, got: %s", output)
		}
	})
}
