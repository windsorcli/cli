package kubernetes

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

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("PreservesGitSchemeForSourceURL", func(t *testing.T) {
		manager := setup(t)
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
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Sources: []blueprintv1alpha1.Source{
				{Name: "test-source", Url: "git://github.com/example/repo.git", Ref: blueprintv1alpha1.Reference{Branch: "main"}},
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Sources: []blueprintv1alpha1.Source{
				{Name: "test-source", Url: "1invalid://github.com/example/repo.git", Ref: blueprintv1alpha1.Reference{Branch: "main"}},
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
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
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

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

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error when blueprint has only local template source, got %v", err)
		}
	})

	t.Run("SuccessSkipsDestroyOnlyKustomizations", func(t *testing.T) {
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create namespace") {
			t.Errorf("Expected error about namespace creation, got %v", err)
		}
	})

	t.Run("SuccessWithRepository", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

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

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithRepositoryAndSecretName", func(t *testing.T) {
		manager := setup(t)
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
		manager.client = kubernetesClient
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
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

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
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

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
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

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Repository: blueprintv1alpha1.Repository{
				Url: "https://github.com/example/repo.git",
			},
		}

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply blueprint repository") {
			t.Errorf("Expected error about blueprint repository, got %v", err)
		}
	})

	t.Run("ErrorOnApplySource", func(t *testing.T) {
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply source") {
			t.Errorf("Expected error about applying source, got %v", err)
		}
	})

	t.Run("ErrorOnApplyConfigMap", func(t *testing.T) {
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create ConfigMap") {
			t.Errorf("Expected error about ConfigMap, got %v", err)
		}
	})

	t.Run("ErrorOnApplyKustomization", func(t *testing.T) {
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply kustomization") {
			t.Errorf("Expected error about kustomization, got %v", err)
		}
	})

	t.Run("SuccessWithOCISource", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

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

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithBlueprintConfigMaps", func(t *testing.T) {
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.ApplyBlueprint(blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient

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
		// Given a blueprint with its own primary repository, a named vendor source, and a
		// kustomization that sets no explicit source (falls back to the primary repository)
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		manager.shims = mocks.Shims
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return runtime.DefaultUnstructuredConverter.FromUnstructured(obj, target)
		}

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
		manager.client = kc

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

		// Then the primary repository (named after the blueprint) gets the short,
		// continuously-polled interval
		primaryRepo, ok := appliedRepos["test-blueprint"]
		if !ok {
			t.Fatal("Expected the primary repository GitRepository to be applied")
		}
		if primaryRepo.Spec.Interval.Duration != constants.DefaultFluxPrimaryRepositoryInterval {
			t.Errorf("Expected primary repository interval %v, got %v", constants.DefaultFluxPrimaryRepositoryInterval, primaryRepo.Spec.Interval.Duration)
		}

		// And the named vendor source gets the long, backstop-only interval
		vendorRepo, ok := appliedRepos["test-source"]
		if !ok {
			t.Fatal("Expected the vendor source GitRepository to be applied")
		}
		if vendorRepo.Spec.Interval.Duration != constants.DefaultFluxSourceInterval {
			t.Errorf("Expected vendor source interval %v, got %v", constants.DefaultFluxSourceInterval, vendorRepo.Spec.Interval.Duration)
		}

		// And the kustomization, which set no explicit source, resolves to the primary
		// repository and gets its short interval too
		if appliedKust == nil {
			t.Fatal("Expected Kustomization to be applied")
		}
		if appliedKust.Spec.Interval.Duration != constants.DefaultFluxPrimaryKustomizationInterval {
			t.Errorf("Expected kustomization interval %v, got %v", constants.DefaultFluxPrimaryKustomizationInterval, appliedKust.Spec.Interval.Duration)
		}
	})
}

func TestBaseKubernetesManager_DeleteBlueprint(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		manager.kustomizationWaitPollInterval = 50 * time.Millisecond
		manager.kustomizationReconcileTimeout = 100 * time.Millisecond
		manager.kustomizationReconcileSleep = 50 * time.Millisecond
		return manager
	}

	t.Run("Success", func(t *testing.T) {
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(deleteCalls) != 2 {
			t.Errorf("Expected 2 delete calls, got %d", len(deleteCalls))
		}
	})

	t.Run("DeleteTimeoutOverrideSurvivesDeleteLongerThanSpecTimeout", func(t *testing.T) {
		// Given a Kustomization whose install spec.timeout (30ms) is much
		// smaller than how long it actually takes to delete, but which
		// declares its own DeleteTimeout (150ms) for the destroy path
		manager := setup(t)
		manager.kustomizationReconcileTimeout = 20 * time.Millisecond
		manager.kustomizationWaitPollInterval = 10 * time.Millisecond
		manager.kustomizationReconcileSleep = 0
		clock := newFakeClock()
		manager.shims.TimeNow = clock.Now
		manager.shims.TimeSleep = clock.Sleep

		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return nil
		}
		var getCalls int
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			getCalls++
			// Two calls precede DeleteKustomization's own wait loop (inventory
			// lookup for load-balancer remediation, then the post-resume
			// reconcile check), then the loop itself polls live for 6 more
			// iterations (60ms of its own wait) before disappearing — past the
			// 30ms spec.timeout-derived window a pre-fix DeleteBlueprint would
			// have aborted at, but well inside the 150ms DeleteTimeout override.
			if getCalls < 9 {
				return &unstructured.Unstructured{Object: map[string]any{
					"spec": map[string]any{"timeout": "30ms"},
				}}, nil
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

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

		// Then it does not abort at the spec.timeout-derived window, since
		// DeleteTimeout replaces that heuristic as the wait floor
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessSkipsDestroyFalse", func(t *testing.T) {
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var deleteCalls []string
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCalls = append(deleteCalls, name)
			return nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient

		destroyFalse := false
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "test-kustomization-1"},
				{Name: "test-kustomization-2"},
				{Name: "pinned-keep", Destroy: &blueprintv1alpha1.BoolExpression{Value: &destroyFalse, IsExpr: false}},
			},
		}

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
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
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var deleteCalls []string
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("connection refused")
		}
		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			deleteCalls = append(deleteCalls, name)
			return nil
		}
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "test-kustomization-1"}},
		}

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "test-kustomization-1"}},
		}

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
		if err != nil {
			t.Fatalf("Expected NotFound on suspend to be tolerated, got %v", err)
		}
		if len(deleteCalls) != 1 {
			t.Errorf("Expected 1 delete call, got %v", deleteCalls)
		}
	})

	t.Run("ResumesEachKustomizationImmediatelyBeforeDeletingIt", func(t *testing.T) {
		// Given a blueprint with two eligible kustomizations and a client that
		// records the ordered sequence of suspend/resume patches and deletes
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		// Then every eligible kustomization is suspended before any delete starts
		// (the freeze that prevents a still-active kustomization from re-creating
		// resources mid-teardown), and each is resumed immediately before its own
		// delete so the Flux finalizer prunes its inventory rather than orphaning it
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
		// Given two eligible kustomizations with no DependsOn edge between them,
		// ordered so that deleting the first fails
		manager := setup(t)
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
		manager.client = kubernetesClient

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "test-kustomization-1"},
				{Name: "test-kustomization-2"},
			},
		}

		// When test-kustomization-2 fails to delete (no DependsOn, so the destroy
		// walk visits the input in reverse: test-kustomization-2 then
		// test-kustomization-1)
		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")

		// Then the failure is surfaced, and the unrelated test-kustomization-1 is
		// still resumed and deleted rather than left behind
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
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
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
		// Given a destroy-only Kustomization — the backup/managed-resource teardown
		// case DeleteBlueprint's own doc comment cites — whose install spec.timeout
		// (15ms) is much smaller than how long it actually takes to delete, but
		// which declares its own DeleteTimeout (150ms)
		manager := setup(t)
		manager.kustomizationReconcileTimeout = 20 * time.Millisecond
		manager.kustomizationWaitPollInterval = 10 * time.Millisecond

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
			// Stays live for 6 polls (60ms) — past the 15ms/20ms spec.timeout-
			// and reconcile-derived window a pre-fix destroy-only delete would
			// have aborted at — then disappears well inside the 150ms override.
			if getCalls < 7 {
				return &unstructured.Unstructured{Object: map[string]any{
					"spec": map[string]any{"timeout": "15ms"},
				}}, nil
			}
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

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

		// Then it does not abort at the spec.timeout-derived window, since
		// DeleteTimeout replaces that heuristic for destroy-only kustomizations too
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DestroyOnlyProcessedBeforeRegular", func(t *testing.T) {
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error when destroy-only kustomization does not become ready")
		}
		if !strings.Contains(err.Error(), "did not become ready within timeout") {
			t.Errorf("Expected timeout error, got %v", err)
		}
		// The timeout names which destroy-only kustomization is still not Ready
		// rather than leaving the operator with a bare "cleanup may not have completed".
		if !strings.Contains(err.Error(), "cleanup may not have completed: destroy-only") {
			t.Errorf("Expected timeout error to name the stuck kustomization, got: %v", err)
		}
	})

	t.Run("DestroyOnlyWithSubstitutions", func(t *testing.T) {
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if !deletedResources["destroy-only"] {
			t.Error("Expected destroy-only kustomization to be deleted after becoming ready")
		}
	})

	t.Run("DestroyOnlyErrorApplyingKustomization", func(t *testing.T) {
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to apply destroy-only kustomization") {
			t.Errorf("Expected error about applying destroy-only kustomization, got %v", err)
		}
	})

	t.Run("DestroyOnlyErrorDeletingKustomization", func(t *testing.T) {
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to delete destroy-only kustomization") {
			t.Errorf("Expected error about deleting destroy-only kustomization, got %v", err)
		}
	})

	t.Run("DestroyOnlyErrorCreatingConfigMap", func(t *testing.T) {
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create ConfigMap for destroy-only kustomization") {
			t.Errorf("Expected error about ConfigMap creation, got %v", err)
		}
	})

	t.Run("DestroyOnlyCleansUpOnApplyFailure", func(t *testing.T) {
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient

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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
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
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		kubernetesClient.DeleteResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
			return fmt.Errorf("delete error")
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to delete kustomization") {
			t.Errorf("Expected error containing 'failed to delete kustomization', got %v", err)
		}
	})

	t.Run("AbandonedInventoryUsesCurrentDestroySettingNotStaleLiveField", func(t *testing.T) {
		// Given a kustomization last applied with destroy:false (live object still
		// carries deletionPolicy=MirrorPrune), whose blueprint entry now sets
		// destroy:true, and this destroy run never re-applies it first
		manager := setup(t)
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
				// Call 1 is remediateLoadBalancerOwners' pre-destroy inventory snapshot,
				// calls 2-3 are waitForResumeReconcile's checks after resume, call 4 is
				// deleteKustomization's own pre-delete snapshot, and the delete wait
				// loop's own first check is the fifth.
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
		manager.client = kubernetesClient

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

		// Then it still reports the abandoned inventory: the current blueprint's
		// destroy:true is trusted over the live object's stale MirrorPrune field
		if err == nil {
			t.Fatal("Expected an error naming the abandoned inventory, got nil")
		}
		if !strings.Contains(err.Error(), "entry-one") {
			t.Errorf("Expected error to name the still-live entry, got: %v", err)
		}
	})

	t.Run("ContinuesPastAnUnrelatedFailureButSkipsAFailedKustomizationsOwnDependency", func(t *testing.T) {
		// Pins the corrected contract: DeleteBlueprint does not abort the whole run
		// on a single failure. Two independent kustomizations (no DependsOn between
		// them) do not cascade-abort each other — a regression that reintroduced
		// abort-everything would fail this test by issuing only one DeleteResource
		// call. A kustomization's own dependency is still protected: it is never
		// attempted, since it may be the controller the failure needs to resolve.
		manager := setup(t)
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
		manager.client = kubernetesClient

		// second-kust depends on its-dependency; first-kust is unrelated to both.
		// Reverse-topo destroys a dependent before its dependency, so the walk
		// attempts second-kust before its-dependency.
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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
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
		manager := setup(t)
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
		manager.client = kubernetesClient
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
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

		err := manager.DeleteBlueprint(context.Background(), blueprint, "test-namespace")
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
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestBaseKubernetesManager_applyBlueprintOCIRepository(t *testing.T) {
	// setup returns a manager whose ToUnstructured shim intercepts the *sourcev1.OCIRepository
	// applyBlueprintOCIRepository builds, capturing it into *captured so the test can inspect
	// the Spec fields the digest/tag parsing logic actually produced.
	setup := func(t *testing.T) (manager *BaseKubernetesManager, captured **sourcev1.OCIRepository) {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager = NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			return obj, nil
		}
		manager.client = kubernetesClient

		var repo *sourcev1.OCIRepository
		captured = &repo
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			r, ok := obj.(*sourcev1.OCIRepository)
			if !ok {
				return nil, fmt.Errorf("expected *sourcev1.OCIRepository, got %T", obj)
			}
			*captured = r
			return map[string]any{
				"apiVersion": "source.toolkit.fluxcd.io/v1",
				"kind":       "OCIRepository",
				"metadata": map[string]any{
					"name":      r.Name,
					"namespace": r.Namespace,
				},
				"spec": map[string]any{
					"url": r.Spec.URL,
				},
			}, nil
		}
		return manager, captured
	}

	t.Run("DigestReference", func(t *testing.T) {
		manager, captured := setup(t)

		source := blueprintv1alpha1.Source{
			Name: "core",
			Url:  "oci://ghcr.io/windsorcli/core@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
		}

		err := manager.applyBlueprintSource(source, "system-gitops", true)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		repo := *captured

		if repo.Spec.URL != "oci://ghcr.io/windsorcli/core" {
			t.Errorf("expected URL 'oci://ghcr.io/windsorcli/core', got %s", repo.Spec.URL)
		}
		if repo.Spec.Reference == nil {
			t.Fatalf("expected a non-nil Reference")
		}
		if repo.Spec.Reference.Digest != "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234" {
			t.Errorf("expected Digest 'sha256:abcd...', got %q", repo.Spec.Reference.Digest)
		}
		if repo.Spec.Reference.Tag != "" {
			t.Errorf("expected empty Tag for a digest reference, got %q", repo.Spec.Reference.Tag)
		}
	})

	t.Run("TagReference", func(t *testing.T) {
		manager, captured := setup(t)

		source := blueprintv1alpha1.Source{
			Name: "core",
			Url:  "oci://ghcr.io/windsorcli/core:latest",
		}

		err := manager.applyBlueprintSource(source, "system-gitops", true)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		repo := *captured

		if repo.Spec.URL != "oci://ghcr.io/windsorcli/core" {
			t.Errorf("expected URL 'oci://ghcr.io/windsorcli/core', got %s", repo.Spec.URL)
		}
		if repo.Spec.Reference == nil || repo.Spec.Reference.Tag != "latest" {
			t.Errorf("expected Reference.Tag 'latest', got %+v", repo.Spec.Reference)
		}
	})
}

func TestBaseKubernetesManager_waitForResumeReconcile(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		manager.kustomizationWaitPollInterval = 10 * time.Millisecond
		manager.kustomizationReconcileSleep = 30 * time.Millisecond
		clock := newFakeClock()
		manager.shims.TimeNow = clock.Now
		manager.shims.TimeSleep = clock.Sleep
		return manager
	}

	staleObj := func() *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"generation": int64(2)},
			"status":   map[string]any{"observedGeneration": int64(1)},
		}}
	}

	t.Run("ReturnsAsSoonAsGenerationSettles", func(t *testing.T) {
		// Given a Kustomization whose observedGeneration catches up on the second read
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			calls++
			if calls >= 2 {
				return &unstructured.Unstructured{Object: map[string]any{
					"metadata": map[string]any{"generation": int64(2)},
					"status":   map[string]any{"observedGeneration": int64(2)},
				}}, nil
			}
			return staleObj(), nil
		}
		manager.client = kubernetesClient

		// When waiting for the resume to settle
		manager.waitForResumeReconcile("test-kustomization", "test-namespace")

		// Then it stops polling right after the generations match
		if calls != 2 {
			t.Errorf("Expected exactly 2 GetResource calls before settling, got %d", calls)
		}
	})

	t.Run("GivesUpAfterTheBudgetWithoutSettling", func(t *testing.T) {
		// Given a Kustomization whose observedGeneration never catches up
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			return staleObj(), nil
		}
		manager.client = kubernetesClient

		// When waiting for the resume to settle, it must return once the fake clock
		// passes kustomizationReconcileSleep rather than loop forever
		manager.waitForResumeReconcile("test-kustomization", "test-namespace")
	})

	t.Run("ReturnsImmediatelyOnAReadError", func(t *testing.T) {
		// Given the Kustomization can't be read at all
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()
		calls := 0
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
			calls++
			return nil, fmt.Errorf("the server could not find the requested resource")
		}
		manager.client = kubernetesClient

		// When waiting for the resume to settle
		manager.waitForResumeReconcile("test-kustomization", "test-namespace")

		// Then it gives up on the first error rather than retrying
		if calls != 1 {
			t.Errorf("Expected exactly 1 GetResource call before giving up on error, got %d", calls)
		}
	})
}

func TestBaseKubernetesManager_abortDestroy(t *testing.T) {
	eligible := []blueprintv1alpha1.Kustomization{{Name: "crds"}, {Name: "database-install"}, {Name: "demo-resources"}}

	t.Run("StopsAfterFirstRequestTimeoutRatherThanRepeatingIt", func(t *testing.T) {
		mocks := setupKubernetesMocks(t)
		kubernetesClient := client.NewMockKubernetesClient()
		var attempted []string
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			attempted = append(attempted, name)
			return nil, context.DeadlineExceeded
		}
		manager := NewKubernetesManager(kubernetesClient, mocks.ConfigHandler)

		// Given a request timeout on the first un-suspend attempt
		// When abortDestroy runs
		err := manager.abortDestroy(eligible, "system-gitops", fmt.Errorf("original failure"))

		// Then it stops instead of repeating the timeout on every remaining kustomization
		if len(attempted) != 1 {
			t.Fatalf("expected exactly 1 un-suspend attempt, got %d: %v", len(attempted), attempted)
		}
		if attempted[0] != "crds" {
			t.Errorf("expected the first eligible kustomization attempted, got %q", attempted[0])
		}
		if err == nil || !strings.Contains(err.Error(), "stopping abort cleanup after a request timeout") {
			t.Errorf("expected an early-stop note in the joined error, got: %v", err)
		}
		if err == nil || !strings.Contains(err.Error(), "2 more kustomization(s) left un-suspended") {
			t.Errorf("expected the remaining count in the joined error, got: %v", err)
		}
	})

	t.Run("ContinuesPastANonTimeoutFailure", func(t *testing.T) {
		mocks := setupKubernetesMocks(t)
		kubernetesClient := client.NewMockKubernetesClient()
		var attempted []string
		kubernetesClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			attempted = append(attempted, name)
			return nil, fmt.Errorf("conflict: object has been modified")
		}
		manager := NewKubernetesManager(kubernetesClient, mocks.ConfigHandler)

		// Given a non-timeout rejection on every un-suspend attempt
		// When abortDestroy runs
		err := manager.abortDestroy(eligible, "system-gitops", fmt.Errorf("original failure"))

		// Then it still attempts every eligible kustomization
		if len(attempted) != len(eligible) {
			t.Fatalf("expected all %d kustomizations attempted, got %d: %v", len(eligible), len(attempted), attempted)
		}
		if err == nil || strings.Contains(err.Error(), "stopping abort cleanup") {
			t.Errorf("expected no early-stop note for a non-timeout failure, got: %v", err)
		}
	})
}

func TestBaseKubernetesManager_blockDependencyClosure(t *testing.T) {
	mocks := setupKubernetesMocks(t)
	manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)

	t.Run("PluralizesDependenciesForMoreThanOneSkipped", func(t *testing.T) {
		ks := []blueprintv1alpha1.Kustomization{
			{Name: "app", DependsOn: []string{"a", "b"}},
			{Name: "a"},
			{Name: "b"},
		}

		// Given a failed kustomization whose closure skips two others
		// When blockDependencyClosure runs
		ctx, rec := recordProgress(t)
		manager.blockDependencyClosure(ctx, ks, "app", map[string]bool{})
		output := strings.Join(rec.warnings(), "\n")

		// Then the warning uses the plural
		if !strings.Contains(output, "skipping 2 dependencies") {
			t.Errorf("expected plural wording for 2 skipped, got: %s", output)
		}
	})

	t.Run("KeepsDependencySingularForExactlyOneSkipped", func(t *testing.T) {
		ks := []blueprintv1alpha1.Kustomization{
			{Name: "app", DependsOn: []string{"a"}},
			{Name: "a"},
		}

		// Given a failed kustomization whose closure skips exactly one other
		// When blockDependencyClosure runs
		ctx, rec := recordProgress(t)
		manager.blockDependencyClosure(ctx, ks, "app", map[string]bool{})
		output := strings.Join(rec.warnings(), "\n")

		// Then the warning stays singular
		if !strings.Contains(output, "skipping 1 dependency ") {
			t.Errorf("expected singular wording for 1 skipped, got: %s", output)
		}
		if strings.Contains(output, "dependencies") {
			t.Errorf("expected no plural wording for 1 skipped, got: %s", output)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestReconcileGenerationSettled(t *testing.T) {
	t.Run("FalseForNilObject", func(t *testing.T) {
		if reconcileGenerationSettled(nil) {
			t.Error("Expected false for a nil object")
		}
	})

	t.Run("FalseWhenObservedGenerationMissing", func(t *testing.T) {
		obj := &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"generation": int64(2)},
		}}
		if reconcileGenerationSettled(obj) {
			t.Error("Expected false when status.observedGeneration is missing")
		}
	})

	t.Run("FalseWhenObservedGenerationLagsBehind", func(t *testing.T) {
		obj := &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"generation": int64(3)},
			"status":   map[string]any{"observedGeneration": int64(2)},
		}}
		if reconcileGenerationSettled(obj) {
			t.Error("Expected false when observedGeneration lags generation")
		}
	})

	t.Run("TrueWhenObservedGenerationCaughtUp", func(t *testing.T) {
		obj := &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"generation": int64(3)},
			"status":   map[string]any{"observedGeneration": int64(3)},
		}}
		if !reconcileGenerationSettled(obj) {
			t.Error("Expected true when observedGeneration matches generation")
		}
	})
}

func TestReverseTopologicalKustomizations(t *testing.T) {
	t.Run("ReturnsEmptyForEmptyInput", func(t *testing.T) {
		// Given an empty input slice
		input := []blueprintv1alpha1.Kustomization{}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then no error is returned and the output is also empty
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("expected empty output, got %d entries", len(out))
		}
	})

	t.Run("ReturnsSingleNodeUnchanged", func(t *testing.T) {
		// Given a single kustomization with no dependencies
		input := []blueprintv1alpha1.Kustomization{{Name: "only"}}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then the single node is returned unchanged
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 1 || out[0].Name != "only" {
			t.Errorf("expected [only], got %v", kustomizationNames(out))
		}
	})

	t.Run("ReversesIndependentNodesByInputOrder", func(t *testing.T) {
		// Given three independent kustomizations with no dependency relation
		input := []blueprintv1alpha1.Kustomization{{Name: "a"}, {Name: "b"}, {Name: "c"}}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then the output is the input reversed, matching naive slice-reverse
		// — preserves backward compatibility for blueprints already authored
		// in topological order.
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		want := []string{"c", "b", "a"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("PutsDependentBeforeDependencyWhenInputAlreadyTopological", func(t *testing.T) {
		// Given input in apply order: dependency b first, dependent a second
		input := []blueprintv1alpha1.Kustomization{
			{Name: "b"},
			{Name: "a", DependsOn: []string{"b"}},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then a (the dependent) destroys first, then b
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		want := []string{"a", "b"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("PutsDependentBeforeDependencyWhenInputOutOfOrder", func(t *testing.T) {
		// Given input in non-topological order: dependent a first, dependency b second
		input := []blueprintv1alpha1.Kustomization{
			{Name: "a", DependsOn: []string{"b"}},
			{Name: "b"},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then a still destroys before b, regardless of input ordering
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		want := []string{"a", "b"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("OrdersDiamondDestroyDependentsFirst", func(t *testing.T) {
		// Given a diamond DAG:
		//   d ← b ← a
		//   d ← c ← a
		// Apply order is d → b,c → a; destroy order is a → b,c → d.
		input := []blueprintv1alpha1.Kustomization{
			{Name: "d"},
			{Name: "b", DependsOn: []string{"d"}},
			{Name: "c", DependsOn: []string{"d"}},
			{Name: "a", DependsOn: []string{"b", "c"}},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then a destroys first, then b and c (in input order's reverse), then d
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		// Independent siblings (b, c) appear in reverse input order: c then b.
		want := []string{"a", "c", "b", "d"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("HandlesDisconnectedComponents", func(t *testing.T) {
		// Given two disconnected dependency graphs:
		//   y ← x   (chain 1)
		//   q ← p   (chain 2)
		input := []blueprintv1alpha1.Kustomization{
			{Name: "y"},
			{Name: "x", DependsOn: []string{"y"}},
			{Name: "q"},
			{Name: "p", DependsOn: []string{"q"}},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then each chain's dependent precedes its dependency, and the chains
		// appear in reverse of input order.
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		// Forward DFS visits y, then x (already y), then q, then p (already q)
		// → forward order [y, x, q, p]; reverse → [p, q, x, y].
		want := []string{"p", "q", "x", "y"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("IgnoresMissingDependencyName", func(t *testing.T) {
		// Given a kustomization that depends on a name not present in the slice
		// — e.g. a destroy-only kustomization referencing a regular one that
		// was filtered out before this call.
		input := []blueprintv1alpha1.Kustomization{
			{Name: "a", DependsOn: []string{"missing"}},
			{Name: "b"},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then the missing dep is silently ignored and the slice is reversed
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := kustomizationNames(out)
		want := []string{"b", "a"}
		if !equalStringSlice(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("DetectsTwoNodeCycle", func(t *testing.T) {
		// Given a two-node cycle: a depends on b, b depends on a
		input := []blueprintv1alpha1.Kustomization{
			{Name: "a", DependsOn: []string{"b"}},
			{Name: "b", DependsOn: []string{"a"}},
		}

		// When computing reverse-topological order
		_, err := reverseTopologicalKustomizations(input)

		// Then an error is returned mentioning a kustomization in the cycle
		if err == nil {
			t.Fatalf("expected cycle error, got nil")
		}
		if !strings.Contains(err.Error(), "cycle") {
			t.Errorf("expected error mentioning cycle, got %q", err.Error())
		}
	})

	t.Run("DetectsSelfLoopOnSingleNodeAsNoOp", func(t *testing.T) {
		// Given a single kustomization that depends on itself, the single-
		// node fast path returns it unchanged. A self-loop on a single node
		// does not change destroy correctness — there is nothing else to
		// order against — so the short-circuit is intentional.
		input := []blueprintv1alpha1.Kustomization{
			{Name: "a", DependsOn: []string{"a"}},
		}

		// When computing reverse-topological order
		out, err := reverseTopologicalKustomizations(input)

		// Then the node is returned without a cycle error
		if err != nil {
			t.Fatalf("unexpected error on single self-loop: %v", err)
		}
		if len(out) != 1 || out[0].Name != "a" {
			t.Errorf("expected [a], got %v", kustomizationNames(out))
		}
	})

	t.Run("DetectsLargerCycle", func(t *testing.T) {
		// Given a three-node cycle: a → b → c → a
		input := []blueprintv1alpha1.Kustomization{
			{Name: "a", DependsOn: []string{"b"}},
			{Name: "b", DependsOn: []string{"c"}},
			{Name: "c", DependsOn: []string{"a"}},
		}

		// When computing reverse-topological order
		_, err := reverseTopologicalKustomizations(input)

		// Then an error is returned
		if err == nil {
			t.Fatalf("expected cycle error, got nil")
		}
		if !strings.Contains(err.Error(), "cycle") {
			t.Errorf("expected error mentioning cycle, got %q", err.Error())
		}
	})

	t.Run("DoesNotMutateInput", func(t *testing.T) {
		// Given an input slice
		input := []blueprintv1alpha1.Kustomization{
			{Name: "b"},
			{Name: "a", DependsOn: []string{"b"}},
		}
		inputNames := kustomizationNames(input)

		// When computing reverse-topological order
		_, err := reverseTopologicalKustomizations(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Then the input slice is unchanged in order and identity
		gotNames := kustomizationNames(input)
		if !equalStringSlice(gotNames, inputNames) {
			t.Errorf("input mutated: was %v, now %v", inputNames, gotNames)
		}
	})
}

func TestDependencyClosure(t *testing.T) {
	t.Run("IncludesStartWithNoDependencies", func(t *testing.T) {
		ks := []blueprintv1alpha1.Kustomization{{Name: "solo"}}

		// Given a kustomization with no DependsOn
		// When dependencyClosure runs
		closure := dependencyClosure(ks, "solo")

		// Then the closure is just itself
		if len(closure) != 1 || !closure["solo"] {
			t.Errorf("expected closure {solo}, got %v", closure)
		}
	})

	t.Run("IncludesADirectDependency", func(t *testing.T) {
		ks := []blueprintv1alpha1.Kustomization{
			{Name: "app", DependsOn: []string{"controller"}},
			{Name: "controller"},
		}

		// Given app depends on controller
		// When dependencyClosure runs from app
		closure := dependencyClosure(ks, "app")

		// Then both are in the closure
		if !closure["app"] || !closure["controller"] {
			t.Errorf("expected closure {app, controller}, got %v", closure)
		}
	})

	t.Run("FollowsATransitiveChain", func(t *testing.T) {
		ks := []blueprintv1alpha1.Kustomization{
			{Name: "app", DependsOn: []string{"controller"}},
			{Name: "controller", DependsOn: []string{"crds"}},
			{Name: "crds"},
		}

		// Given app depends on controller, which depends on crds
		// When dependencyClosure runs from app
		closure := dependencyClosure(ks, "app")

		// Then the closure reaches all three
		if !closure["app"] || !closure["controller"] || !closure["crds"] {
			t.Errorf("expected closure {app, controller, crds}, got %v", closure)
		}
	})

	t.Run("TreatsAMissingDependsOnNameAsNoEdge", func(t *testing.T) {
		ks := []blueprintv1alpha1.Kustomization{
			{Name: "app", DependsOn: []string{"not-in-blueprint"}},
		}

		// Given app depends on a name absent from the blueprint
		// When dependencyClosure runs from app
		closure := dependencyClosure(ks, "app")

		// Then the closure is just app; the missing name is not followed
		if len(closure) != 1 || !closure["app"] {
			t.Errorf("expected closure {app}, got %v", closure)
		}
	})

	t.Run("DoesNotIncludeAnUnrelatedKustomization", func(t *testing.T) {
		ks := []blueprintv1alpha1.Kustomization{
			{Name: "app", DependsOn: []string{"controller"}},
			{Name: "controller"},
			{Name: "unrelated"},
		}

		// Given a third kustomization with no path from app
		// When dependencyClosure runs from app
		closure := dependencyClosure(ks, "app")

		// Then it is not in the closure
		if closure["unrelated"] {
			t.Errorf("expected unrelated excluded from closure, got %v", closure)
		}
	})
}
