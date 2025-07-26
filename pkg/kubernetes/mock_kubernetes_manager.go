// Package kubernetes provides Kubernetes resource management functionality
// It implements server-side apply patterns for managing Kubernetes resources
// and provides a clean interface for kustomization and resource management

package kubernetes

import (
	"context"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Types
// =============================================================================

// MockKubernetesManager is a mock implementation of KubernetesManager interface for testing
type MockKubernetesManager struct {
	InitializeFunc                      func() error
	ApplyKustomizationFunc              func(kustomization kustomizev1.Kustomization) error
	DeleteKustomizationFunc             func(name, namespace string) error
	WaitForKustomizationsFunc           func(message string, names ...string) error
	GetKustomizationStatusFunc          func(names []string) (map[string]bool, error)
	CreateNamespaceFunc                 func(name string) error
	DeleteNamespaceFunc                 func(name string) error
	ApplyConfigMapFunc                  func(name, namespace string, data map[string]string) error
	GetHelmReleasesForKustomizationFunc func(name, namespace string) ([]helmv2.HelmRelease, error)
	SuspendKustomizationFunc            func(name, namespace string) error
	SuspendHelmReleaseFunc              func(name, namespace string) error
	ApplyGitRepositoryFunc              func(repo *sourcev1.GitRepository) error
	ApplyOCIRepositoryFunc              func(repo *sourcev1.OCIRepository) error
	WaitForKustomizationsDeletedFunc    func(message string, names ...string) error
	CheckGitRepositoryStatusFunc        func() error
	WaitForKubernetesHealthyFunc        func(ctx context.Context, endpoint string, nodeNames ...string) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockKubernetesManager creates a new instance of MockKubernetesManager
func NewMockKubernetesManager(injector di.Injector) *MockKubernetesManager {
	return &MockKubernetesManager{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize implements KubernetesManager interface
func (m *MockKubernetesManager) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

// ApplyKustomization implements KubernetesManager interface
func (m *MockKubernetesManager) ApplyKustomization(kustomization kustomizev1.Kustomization) error {
	if m.ApplyKustomizationFunc != nil {
		return m.ApplyKustomizationFunc(kustomization)
	}
	return nil
}

// DeleteKustomization implements KubernetesManager interface
func (m *MockKubernetesManager) DeleteKustomization(name, namespace string) error {
	if m.DeleteKustomizationFunc != nil {
		return m.DeleteKustomizationFunc(name, namespace)
	}
	return nil
}

// WaitForKustomizations implements KubernetesManager interface
func (m *MockKubernetesManager) WaitForKustomizations(message string, names ...string) error {
	if m.WaitForKustomizationsFunc != nil {
		return m.WaitForKustomizationsFunc(message, names...)
	}
	return nil
}

// GetKustomizationStatus implements KubernetesManager interface
func (m *MockKubernetesManager) GetKustomizationStatus(names []string) (map[string]bool, error) {
	if m.GetKustomizationStatusFunc != nil {
		return m.GetKustomizationStatusFunc(names)
	}
	return make(map[string]bool), nil
}

// CreateNamespace implements KubernetesManager interface
func (m *MockKubernetesManager) CreateNamespace(name string) error {
	if m.CreateNamespaceFunc != nil {
		return m.CreateNamespaceFunc(name)
	}
	return nil
}

// DeleteNamespace implements KubernetesManager interface
func (m *MockKubernetesManager) DeleteNamespace(name string) error {
	if m.DeleteNamespaceFunc != nil {
		return m.DeleteNamespaceFunc(name)
	}
	return nil
}

// ApplyConfigMap implements KubernetesManager interface
func (m *MockKubernetesManager) ApplyConfigMap(name, namespace string, data map[string]string) error {
	if m.ApplyConfigMapFunc != nil {
		return m.ApplyConfigMapFunc(name, namespace, data)
	}
	return nil
}

// GetHelmReleasesForKustomization implements KubernetesManager interface
func (m *MockKubernetesManager) GetHelmReleasesForKustomization(name, namespace string) ([]helmv2.HelmRelease, error) {
	if m.GetHelmReleasesForKustomizationFunc != nil {
		return m.GetHelmReleasesForKustomizationFunc(name, namespace)
	}
	return nil, nil
}

// SuspendKustomization implements KubernetesManager interface
func (m *MockKubernetesManager) SuspendKustomization(name, namespace string) error {
	if m.SuspendKustomizationFunc != nil {
		return m.SuspendKustomizationFunc(name, namespace)
	}
	return nil
}

// SuspendHelmRelease implements KubernetesManager interface
func (m *MockKubernetesManager) SuspendHelmRelease(name, namespace string) error {
	if m.SuspendHelmReleaseFunc != nil {
		return m.SuspendHelmReleaseFunc(name, namespace)
	}
	return nil
}

// ApplyGitRepository implements KubernetesManager interface
func (m *MockKubernetesManager) ApplyGitRepository(repo *sourcev1.GitRepository) error {
	if m.ApplyGitRepositoryFunc != nil {
		return m.ApplyGitRepositoryFunc(repo)
	}
	return nil
}

// ApplyOCIRepository implements KubernetesManager interface
func (m *MockKubernetesManager) ApplyOCIRepository(repo *sourcev1.OCIRepository) error {
	if m.ApplyOCIRepositoryFunc != nil {
		return m.ApplyOCIRepositoryFunc(repo)
	}
	return nil
}

// WaitForKustomizationsDeleted waits for the specified kustomizations to be deleted.
func (m *MockKubernetesManager) WaitForKustomizationsDeleted(message string, names ...string) error {
	if m.WaitForKustomizationsDeletedFunc != nil {
		return m.WaitForKustomizationsDeletedFunc(message, names...)
	}
	return nil
}

// CheckGitRepositoryStatus checks the status of all GitRepository resources
func (m *MockKubernetesManager) CheckGitRepositoryStatus() error {
	if m.CheckGitRepositoryStatusFunc != nil {
		return m.CheckGitRepositoryStatusFunc()
	}
	return nil
}

// WaitForKubernetesHealthy waits for the Kubernetes API endpoint to be healthy with polling and timeout
func (m *MockKubernetesManager) WaitForKubernetesHealthy(ctx context.Context, endpoint string, nodeNames ...string) error {
	if m.WaitForKubernetesHealthyFunc != nil {
		return m.WaitForKubernetesHealthyFunc(ctx, endpoint, nodeNames...)
	}
	return nil
}
