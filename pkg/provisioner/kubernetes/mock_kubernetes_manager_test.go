package kubernetes_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMockKubernetesManager_ApplyKustomization(t *testing.T) {
	setup := func(t *testing.T) *kubernetes.MockKubernetesManager {
		t.Helper()
		return kubernetes.NewMockKubernetesManager()
	}
	k := kustomizev1.Kustomization{}

	t.Run("FuncSet", func(t *testing.T) {
		// Given a mock with ApplyKustomizationFunc set
		manager := setup(t)
		manager.ApplyKustomizationFunc = func(kustomization kustomizev1.Kustomization) error { return fmt.Errorf("err") }

		// When ApplyKustomization is called
		err := manager.ApplyKustomization(k)

		// Then it returns the result of ApplyKustomizationFunc
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		// Given a mock with no ApplyKustomizationFunc set
		manager := setup(t)

		// When ApplyKustomization is called
		err := manager.ApplyKustomization(k)

		// Then it returns the default result
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_DeleteKustomization(t *testing.T) {
	setup := func(t *testing.T) *kubernetes.MockKubernetesManager {
		t.Helper()
		return kubernetes.NewMockKubernetesManager()
	}
	name, ns := "n", "ns"

	t.Run("FuncSet", func(t *testing.T) {
		// Given a mock with DeleteKustomizationFunc set
		manager := setup(t)
		manager.DeleteKustomizationFunc = func(n, ns string) error { return fmt.Errorf("err") }

		// When DeleteKustomization is called
		err := manager.DeleteKustomization(context.Background(), name, ns)

		// Then it returns the result of DeleteKustomizationFunc
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		// Given a mock with no DeleteKustomizationFunc set
		manager := setup(t)

		// When DeleteKustomization is called
		err := manager.DeleteKustomization(context.Background(), name, ns)

		// Then it returns the default result
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_WaitForKustomizations(t *testing.T) {
	setup := func(t *testing.T) *kubernetes.MockKubernetesManager {
		t.Helper()
		return kubernetes.NewMockKubernetesManager()
	}

	t.Run("FuncSet", func(t *testing.T) {
		// Given a mock with WaitForKustomizationsFunc set
		manager := setup(t)
		manager.WaitForKustomizationsFunc = func(ctx context.Context, b *blueprintv1alpha1.Blueprint, layers kubernetes.KustomizationReporter) error {
			return fmt.Errorf("err")
		}

		// When WaitForKustomizations is called
		err := manager.WaitForKustomizations(context.Background(), nil, nil)

		// Then it returns the result of WaitForKustomizationsFunc
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		// Given a mock with no WaitForKustomizationsFunc set
		manager := setup(t)

		// When WaitForKustomizations is called
		err := manager.WaitForKustomizations(context.Background(), nil, nil)

		// Then it returns the default result
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_GetKustomizationStatus(t *testing.T) {
	setup := func(t *testing.T) *kubernetes.MockKubernetesManager {
		t.Helper()
		return kubernetes.NewMockKubernetesManager()
	}
	names := []string{"a", "b"}
	ret := map[string]bool{"a": true}

	t.Run("FuncSet", func(t *testing.T) {
		// Given a mock with GetKustomizationStatusFunc set
		manager := setup(t)
		manager.GetKustomizationStatusFunc = func(n []string) (map[string]bool, error) { return ret, fmt.Errorf("err") }

		// When GetKustomizationStatus is called
		m, err := manager.GetKustomizationStatus(names)

		// Then it returns the result of GetKustomizationStatusFunc
		if !reflect.DeepEqual(m, ret) {
			t.Errorf("Expected %v, got %v", ret, m)
		}
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		// Given a mock with no GetKustomizationStatusFunc set
		manager := setup(t)

		// When GetKustomizationStatus is called
		m, err := manager.GetKustomizationStatus(names)

		// Then it returns the default result
		if len(m) != 0 {
			t.Errorf("Expected empty map, got %v", m)
		}
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_CreateNamespace(t *testing.T) {
	setup := func(t *testing.T) *kubernetes.MockKubernetesManager {
		t.Helper()
		return kubernetes.NewMockKubernetesManager()
	}
	name := "ns"

	t.Run("FuncSet", func(t *testing.T) {
		// Given a mock with CreateNamespaceFunc set
		manager := setup(t)
		manager.CreateNamespaceFunc = func(n string) error { return fmt.Errorf("err") }

		// When CreateNamespace is called
		err := manager.CreateNamespace(name)

		// Then it returns the result of CreateNamespaceFunc
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		// Given a mock with no CreateNamespaceFunc set
		manager := setup(t)

		// When CreateNamespace is called
		err := manager.CreateNamespace(name)

		// Then it returns the default result
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_DeleteNamespace(t *testing.T) {
	setup := func(t *testing.T) *kubernetes.MockKubernetesManager {
		t.Helper()
		return kubernetes.NewMockKubernetesManager()
	}
	name := "ns"

	t.Run("FuncSet", func(t *testing.T) {
		// Given a mock with DeleteNamespaceFunc set
		manager := setup(t)
		manager.DeleteNamespaceFunc = func(n string) error { return fmt.Errorf("err") }

		// When DeleteNamespace is called
		err := manager.DeleteNamespace(name)

		// Then it returns the result of DeleteNamespaceFunc
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		// Given a mock with no DeleteNamespaceFunc set
		manager := setup(t)

		// When DeleteNamespace is called
		err := manager.DeleteNamespace(name)

		// Then it returns the default result
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_ApplyConfigMap(t *testing.T) {
	setup := func(t *testing.T) *kubernetes.MockKubernetesManager {
		t.Helper()
		return kubernetes.NewMockKubernetesManager()
	}
	name, ns := "n", "ns"
	data := map[string]string{"k": "v"}

	t.Run("FuncSet", func(t *testing.T) {
		// Given a mock with ApplyConfigMapFunc set
		manager := setup(t)
		manager.ApplyConfigMapFunc = func(n, ns string, d map[string]string) error { return fmt.Errorf("err") }

		// When ApplyConfigMap is called
		err := manager.ApplyConfigMap(name, ns, data)

		// Then it returns the result of ApplyConfigMapFunc
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		// Given a mock with no ApplyConfigMapFunc set
		manager := setup(t)

		// When ApplyConfigMap is called
		err := manager.ApplyConfigMap(name, ns, data)

		// Then it returns the default result
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_GetHelmReleasesForKustomization(t *testing.T) {
	setup := func(t *testing.T) *kubernetes.MockKubernetesManager {
		t.Helper()
		return kubernetes.NewMockKubernetesManager()
	}
	name, ns := "n", "ns"
	releases := []helmv2.HelmRelease{{}}

	t.Run("FuncSet", func(t *testing.T) {
		// Given a mock with GetHelmReleasesForKustomizationFunc set
		manager := setup(t)
		manager.GetHelmReleasesForKustomizationFunc = func(n, ns string) ([]helmv2.HelmRelease, error) { return releases, fmt.Errorf("err") }

		// When GetHelmReleasesForKustomization is called
		r, err := manager.GetHelmReleasesForKustomization(name, ns)

		// Then it returns the result of GetHelmReleasesForKustomizationFunc
		if !reflect.DeepEqual(r, releases) {
			t.Errorf("Expected %v, got %v", releases, r)
		}
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		// Given a mock with no GetHelmReleasesForKustomizationFunc set
		manager := setup(t)

		// When GetHelmReleasesForKustomization is called
		r, err := manager.GetHelmReleasesForKustomization(name, ns)

		// Then it returns the default result
		if r != nil {
			t.Errorf("Expected nil, got %v", r)
		}
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_ApplyGitRepository(t *testing.T) {
	setup := func(t *testing.T) *kubernetes.MockKubernetesManager {
		t.Helper()
		return kubernetes.NewMockKubernetesManager()
	}
	repo := &sourcev1.GitRepository{}

	t.Run("FuncSet", func(t *testing.T) {
		// Given a mock with ApplyGitRepositoryFunc set
		manager := setup(t)
		manager.ApplyGitRepositoryFunc = func(r *sourcev1.GitRepository) error { return fmt.Errorf("err") }

		// When ApplyGitRepository is called
		err := manager.ApplyGitRepository(repo)

		// Then it returns the result of ApplyGitRepositoryFunc
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		// Given a mock with no ApplyGitRepositoryFunc set
		manager := setup(t)

		// When ApplyGitRepository is called
		err := manager.ApplyGitRepository(repo)

		// Then it returns the default result
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_CheckGitRepositoryStatus(t *testing.T) {
	setup := func(t *testing.T) *kubernetes.MockKubernetesManager {
		t.Helper()
		return kubernetes.NewMockKubernetesManager()
	}

	t.Run("FuncSet", func(t *testing.T) {
		// Given a mock with CheckGitRepositoryStatusFunc set
		manager := setup(t)
		manager.CheckGitRepositoryStatusFunc = func() error { return fmt.Errorf("err") }

		// When CheckGitRepositoryStatus is called
		err := manager.CheckGitRepositoryStatus()

		// Then it returns the result of CheckGitRepositoryStatusFunc
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		// Given a mock with no CheckGitRepositoryStatusFunc set
		manager := setup(t)

		// When CheckGitRepositoryStatus is called
		err := manager.CheckGitRepositoryStatus()

		// Then it returns the default result
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_ApplyOCIRepository(t *testing.T) {
	setup := func(t *testing.T) *kubernetes.MockKubernetesManager {
		t.Helper()
		return kubernetes.NewMockKubernetesManager()
	}
	repo := &sourcev1.OCIRepository{}

	t.Run("FuncSet", func(t *testing.T) {
		// Given a mock with ApplyOCIRepositoryFunc set
		manager := setup(t)
		manager.ApplyOCIRepositoryFunc = func(r *sourcev1.OCIRepository) error { return fmt.Errorf("err") }

		// When ApplyOCIRepository is called
		err := manager.ApplyOCIRepository(repo)

		// Then it returns the result of ApplyOCIRepositoryFunc
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		// Given a mock with no ApplyOCIRepositoryFunc set
		manager := setup(t)

		// When ApplyOCIRepository is called
		err := manager.ApplyOCIRepository(repo)

		// Then it returns the default result
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_WaitForKubernetesHealthy(t *testing.T) {
	setup := func(t *testing.T) *kubernetes.MockKubernetesManager {
		t.Helper()
		return kubernetes.NewMockKubernetesManager()
	}
	ctx := context.Background()
	endpoint := "https://kubernetes.example.com:6443"

	t.Run("FuncSet", func(t *testing.T) {
		// Given a mock with WaitForKubernetesHealthyFunc set
		manager := setup(t)
		errVal := fmt.Errorf("kubernetes health check failed")
		manager.WaitForKubernetesHealthyFunc = func(c context.Context, e string, outputFunc func(string), nodeNames ...string) error {
			if c != ctx {
				t.Errorf("Expected context %v, got %v", ctx, c)
			}
			if e != endpoint {
				t.Errorf("Expected endpoint %s, got %s", endpoint, e)
			}
			return errVal
		}

		// When WaitForKubernetesHealthy is called
		err := manager.WaitForKubernetesHealthy(ctx, endpoint, nil)

		// Then it returns the result of WaitForKubernetesHealthyFunc
		if err != errVal {
			t.Errorf("Expected err, got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		// Given a mock with no WaitForKubernetesHealthyFunc set
		manager := setup(t)

		// When WaitForKubernetesHealthy is called
		err := manager.WaitForKubernetesHealthy(ctx, endpoint, nil)

		// Then it returns the default result
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_GetNodeReadyStatus(t *testing.T) {
	setup := func(t *testing.T) *kubernetes.MockKubernetesManager {
		t.Helper()
		return kubernetes.NewMockKubernetesManager()
	}
	ctx := context.Background()
	nodeNames := []string{"node1", "node2"}

	t.Run("FuncSet", func(t *testing.T) {
		// Given a mock with GetNodeReadyStatusFunc set
		manager := setup(t)
		expectedStatus := map[string]bool{
			"node1": true,
			"node2": false,
		}
		manager.GetNodeReadyStatusFunc = func(c context.Context, names []string) (map[string]bool, error) {
			if c != ctx {
				t.Errorf("Expected context %v, got %v", ctx, c)
			}
			if !reflect.DeepEqual(names, nodeNames) {
				t.Errorf("Expected nodeNames %v, got %v", nodeNames, names)
			}
			return expectedStatus, nil
		}

		// When GetNodeReadyStatus is called
		status, err := manager.GetNodeReadyStatus(ctx, nodeNames)

		// Then it returns the result of GetNodeReadyStatusFunc
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !reflect.DeepEqual(status, expectedStatus) {
			t.Errorf("Expected status %v, got %v", expectedStatus, status)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		// Given a mock with no GetNodeReadyStatusFunc set
		manager := setup(t)

		// When GetNodeReadyStatus is called
		status, err := manager.GetNodeReadyStatus(ctx, nodeNames)

		// Then it returns the default result
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if status == nil {
			t.Error("Expected empty map, got nil")
		}
		if len(status) != 0 {
			t.Errorf("Expected empty map, got %v", status)
		}
	})
}

func TestMockKubernetesManager_ApplyBlueprint(t *testing.T) {
	setup := func(t *testing.T) *kubernetes.MockKubernetesManager {
		t.Helper()
		return kubernetes.NewMockKubernetesManager()
	}
	blueprint := &blueprintv1alpha1.Blueprint{
		Metadata: blueprintv1alpha1.Metadata{
			Name: "test-blueprint",
		},
	}
	namespace := "test-namespace"

	t.Run("FuncSet", func(t *testing.T) {
		// Given a mock with ApplyBlueprintFunc set
		manager := setup(t)
		manager.ApplyBlueprintFunc = func(b *blueprintv1alpha1.Blueprint, ns string) error {
			if b != blueprint {
				t.Errorf("Expected blueprint %v, got %v", blueprint, b)
			}
			if ns != namespace {
				t.Errorf("Expected namespace %s, got %s", namespace, ns)
			}
			return fmt.Errorf("err")
		}

		// When ApplyBlueprint is called
		err := manager.ApplyBlueprint(blueprint, namespace)

		// Then it returns the result of ApplyBlueprintFunc
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		// Given a mock with no ApplyBlueprintFunc set
		manager := setup(t)

		// When ApplyBlueprint is called
		err := manager.ApplyBlueprint(blueprint, namespace)

		// Then it returns the default result
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_DeleteBlueprint(t *testing.T) {
	setup := func(t *testing.T) *kubernetes.MockKubernetesManager {
		t.Helper()
		return kubernetes.NewMockKubernetesManager()
	}
	blueprint := &blueprintv1alpha1.Blueprint{
		Metadata: blueprintv1alpha1.Metadata{
			Name: "test-blueprint",
		},
	}
	namespace := "test-namespace"

	t.Run("FuncSet", func(t *testing.T) {
		// Given a mock with DeleteBlueprintFunc set
		manager := setup(t)
		manager.DeleteBlueprintFunc = func(b *blueprintv1alpha1.Blueprint, ns string) error {
			if b != blueprint {
				t.Errorf("Expected blueprint %v, got %v", blueprint, b)
			}
			if ns != namespace {
				t.Errorf("Expected namespace %s, got %s", namespace, ns)
			}
			return fmt.Errorf("err")
		}

		// When DeleteBlueprint is called
		err := manager.DeleteBlueprint(context.Background(), blueprint, namespace)

		// Then it returns the result of DeleteBlueprintFunc
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		// Given a mock with no DeleteBlueprintFunc set
		manager := setup(t)

		// When DeleteBlueprint is called
		err := manager.DeleteBlueprint(context.Background(), blueprint, namespace)

		// Then it returns the default result
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}
