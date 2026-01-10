// Package kubernetes provides Kubernetes resource management functionality
// It implements server-side apply patterns for managing Kubernetes resources
// and provides a clean interface for kustomization and resource management

package kubernetes

import (
	"context"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Types
// =============================================================================

// MockKubernetesManager is a mock implementation of KubernetesManager interface for testing
type MockKubernetesManager struct {
	ApplyKustomizationFunc              func(kustomization kustomizev1.Kustomization) error
	DeleteKustomizationFunc             func(name, namespace string) error
	WaitForKustomizationsFunc           func(message string, blueprint *blueprintv1alpha1.Blueprint) error
	GetKustomizationStatusFunc          func(names []string) (map[string]bool, error)
	CreateNamespaceFunc                 func(name string) error
	DeleteNamespaceFunc                 func(name string) error
	ApplyConfigMapFunc                  func(name, namespace string, data map[string]string) error
	GetHelmReleasesForKustomizationFunc func(name, namespace string) ([]helmv2.HelmRelease, error)
	SuspendKustomizationFunc            func(name, namespace string) error
	SuspendHelmReleaseFunc              func(name, namespace string) error
	ApplyGitRepositoryFunc              func(repo *sourcev1.GitRepository) error
	ApplyOCIRepositoryFunc              func(repo *sourcev1.OCIRepository) error
	CheckGitRepositoryStatusFunc        func() error
	WaitForKubernetesHealthyFunc        func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error
	GetNodeReadyStatusFunc              func(ctx context.Context, nodeNames []string) (map[string]bool, error)
	ApplyBlueprintFunc                  func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error
	DeleteBlueprintFunc                 func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockKubernetesManager creates a new instance of MockKubernetesManager
func NewMockKubernetesManager() *MockKubernetesManager {
	return &MockKubernetesManager{}
}

// =============================================================================
// Public Methods
// =============================================================================

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
func (m *MockKubernetesManager) WaitForKustomizations(message string, blueprint *blueprintv1alpha1.Blueprint) error {
	if m.WaitForKustomizationsFunc != nil {
		return m.WaitForKustomizationsFunc(message, blueprint)
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

// WaitForKustomizationDeletionProcessed waits for the specified kustomization deletion to be processed.

// CheckGitRepositoryStatus checks the status of all GitRepository resources
func (m *MockKubernetesManager) CheckGitRepositoryStatus() error {
	if m.CheckGitRepositoryStatusFunc != nil {
		return m.CheckGitRepositoryStatusFunc()
	}
	return nil
}

// WaitForKubernetesHealthy waits for the Kubernetes API endpoint to be healthy with polling and timeout
func (m *MockKubernetesManager) WaitForKubernetesHealthy(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
	if m.WaitForKubernetesHealthyFunc != nil {
		return m.WaitForKubernetesHealthyFunc(ctx, endpoint, outputFunc, nodeNames...)
	}
	return nil
}

// GetNodeReadyStatus returns a map of node names to their Ready condition status.
func (m *MockKubernetesManager) GetNodeReadyStatus(ctx context.Context, nodeNames []string) (map[string]bool, error) {
	if m.GetNodeReadyStatusFunc != nil {
		return m.GetNodeReadyStatusFunc(ctx, nodeNames)
	}
	return make(map[string]bool), nil
}

// ApplyBlueprint implements KubernetesManager interface
func (m *MockKubernetesManager) ApplyBlueprint(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
	if m.ApplyBlueprintFunc != nil {
		return m.ApplyBlueprintFunc(blueprint, namespace)
	}
	return nil
}

// DeleteBlueprint implements KubernetesManager interface
func (m *MockKubernetesManager) DeleteBlueprint(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
	if m.DeleteBlueprintFunc != nil {
		return m.DeleteBlueprintFunc(blueprint, namespace)
	}
	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockKubernetesManager implements KubernetesManager interface
var _ KubernetesManager = (*MockKubernetesManager)(nil)
