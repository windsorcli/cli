// Package kubernetes provides Kubernetes resource management functionality
// It implements server-side apply patterns for managing Kubernetes resources
// and provides a clean interface for kustomization and resource management

package kubernetes

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Public Methods
// =============================================================================

func TestMockKubernetesManager_Initialize(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}

	t.Run("FuncSet", func(t *testing.T) {
		manager := setup(t)
		manager.InitializeFunc = func() error { return fmt.Errorf("err") }
		err := manager.Initialize()
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		err := manager.Initialize()
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_ApplyKustomization(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}
	k := kustomizev1.Kustomization{}

	t.Run("FuncSet", func(t *testing.T) {
		manager := setup(t)
		manager.ApplyKustomizationFunc = func(kustomization kustomizev1.Kustomization) error { return fmt.Errorf("err") }
		err := manager.ApplyKustomization(k)
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		err := manager.ApplyKustomization(k)
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_DeleteKustomization(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}
	name, ns := "n", "ns"

	t.Run("FuncSet", func(t *testing.T) {
		manager := setup(t)
		manager.DeleteKustomizationFunc = func(n, ns string) error { return fmt.Errorf("err") }
		err := manager.DeleteKustomization(name, ns)
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		err := manager.DeleteKustomization(name, ns)
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_WaitForKustomizations(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}
	msg := "msg"
	name := "n"

	t.Run("FuncSet", func(t *testing.T) {
		manager := setup(t)
		manager.WaitForKustomizationsFunc = func(m string, n ...string) error { return fmt.Errorf("err") }
		err := manager.WaitForKustomizations(msg, name)
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		err := manager.WaitForKustomizations(msg, name)
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_GetKustomizationStatus(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}
	names := []string{"a", "b"}
	ret := map[string]bool{"a": true}

	t.Run("FuncSet", func(t *testing.T) {
		manager := setup(t)
		manager.GetKustomizationStatusFunc = func(n []string) (map[string]bool, error) { return ret, fmt.Errorf("err") }
		m, err := manager.GetKustomizationStatus(names)
		if !reflect.DeepEqual(m, ret) {
			t.Errorf("Expected %v, got %v", ret, m)
		}
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		m, err := manager.GetKustomizationStatus(names)
		if len(m) != 0 {
			t.Errorf("Expected empty map, got %v", m)
		}
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_CreateNamespace(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}
	name := "ns"

	t.Run("FuncSet", func(t *testing.T) {
		manager := setup(t)
		manager.CreateNamespaceFunc = func(n string) error { return fmt.Errorf("err") }
		err := manager.CreateNamespace(name)
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		err := manager.CreateNamespace(name)
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_DeleteNamespace(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}
	name := "ns"

	t.Run("FuncSet", func(t *testing.T) {
		manager := setup(t)
		manager.DeleteNamespaceFunc = func(n string) error { return fmt.Errorf("err") }
		err := manager.DeleteNamespace(name)
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		err := manager.DeleteNamespace(name)
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_ApplyConfigMap(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}
	name, ns := "n", "ns"
	data := map[string]string{"k": "v"}

	t.Run("FuncSet", func(t *testing.T) {
		manager := setup(t)
		manager.ApplyConfigMapFunc = func(n, ns string, d map[string]string) error { return fmt.Errorf("err") }
		err := manager.ApplyConfigMap(name, ns, data)
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		err := manager.ApplyConfigMap(name, ns, data)
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_GetHelmReleasesForKustomization(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}
	name, ns := "n", "ns"
	releases := []helmv2.HelmRelease{{}}

	t.Run("FuncSet", func(t *testing.T) {
		manager := setup(t)
		manager.GetHelmReleasesForKustomizationFunc = func(n, ns string) ([]helmv2.HelmRelease, error) { return releases, fmt.Errorf("err") }
		r, err := manager.GetHelmReleasesForKustomization(name, ns)
		if !reflect.DeepEqual(r, releases) {
			t.Errorf("Expected %v, got %v", releases, r)
		}
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		r, err := manager.GetHelmReleasesForKustomization(name, ns)
		if r != nil {
			t.Errorf("Expected nil, got %v", r)
		}
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_SuspendKustomization(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}
	name, ns := "n", "ns"

	t.Run("FuncSet", func(t *testing.T) {
		manager := setup(t)
		manager.SuspendKustomizationFunc = func(n, ns string) error { return fmt.Errorf("err") }
		err := manager.SuspendKustomization(name, ns)
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		err := manager.SuspendKustomization(name, ns)
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_SuspendHelmRelease(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}
	name, ns := "n", "ns"

	t.Run("FuncSet", func(t *testing.T) {
		manager := setup(t)
		manager.SuspendHelmReleaseFunc = func(n, ns string) error { return fmt.Errorf("err") }
		err := manager.SuspendHelmRelease(name, ns)
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		err := manager.SuspendHelmRelease(name, ns)
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_ApplyGitRepository(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}
	repo := &sourcev1.GitRepository{}

	t.Run("FuncSet", func(t *testing.T) {
		manager := setup(t)
		manager.ApplyGitRepositoryFunc = func(r *sourcev1.GitRepository) error { return fmt.Errorf("err") }
		err := manager.ApplyGitRepository(repo)
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		err := manager.ApplyGitRepository(repo)
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_CheckGitRepositoryStatus(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}

	t.Run("FuncSet", func(t *testing.T) {
		manager := setup(t)
		manager.CheckGitRepositoryStatusFunc = func() error { return fmt.Errorf("err") }
		err := manager.CheckGitRepositoryStatus()
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		err := manager.CheckGitRepositoryStatus()
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_ApplyOCIRepository(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}
	repo := &sourcev1.OCIRepository{}

	t.Run("FuncSet", func(t *testing.T) {
		manager := setup(t)
		manager.ApplyOCIRepositoryFunc = func(r *sourcev1.OCIRepository) error { return fmt.Errorf("err") }
		err := manager.ApplyOCIRepository(repo)
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		err := manager.ApplyOCIRepository(repo)
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_WaitForKubernetesHealthy(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}
	ctx := context.Background()
	endpoint := "https://kubernetes.example.com:6443"

	t.Run("FuncSet", func(t *testing.T) {
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
		err := manager.WaitForKubernetesHealthy(ctx, endpoint, nil)
		if err != errVal {
			t.Errorf("Expected err, got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		err := manager.WaitForKubernetesHealthy(ctx, endpoint, nil)
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_GetNodeReadyStatus(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}
	ctx := context.Background()
	nodeNames := []string{"node1", "node2"}

	t.Run("FuncSet", func(t *testing.T) {
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
		status, err := manager.GetNodeReadyStatus(ctx, nodeNames)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !reflect.DeepEqual(status, expectedStatus) {
			t.Errorf("Expected status %v, got %v", expectedStatus, status)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		status, err := manager.GetNodeReadyStatus(ctx, nodeNames)
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
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}
	blueprint := &blueprintv1alpha1.Blueprint{
		Metadata: blueprintv1alpha1.Metadata{
			Name: "test-blueprint",
		},
	}
	namespace := "test-namespace"

	t.Run("FuncSet", func(t *testing.T) {
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
		err := manager.ApplyBlueprint(blueprint, namespace)
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		err := manager.ApplyBlueprint(blueprint, namespace)
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockKubernetesManager_DeleteBlueprint(t *testing.T) {
	setup := func(t *testing.T) *MockKubernetesManager {
		t.Helper()
		return NewMockKubernetesManager(nil)
	}
	blueprint := &blueprintv1alpha1.Blueprint{
		Metadata: blueprintv1alpha1.Metadata{
			Name: "test-blueprint",
		},
	}
	namespace := "test-namespace"

	t.Run("FuncSet", func(t *testing.T) {
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
		err := manager.DeleteBlueprint(blueprint, namespace)
		if err == nil || err.Error() != "err" {
			t.Errorf("Expected error 'err', got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		manager := setup(t)
		err := manager.DeleteBlueprint(blueprint, namespace)
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}
