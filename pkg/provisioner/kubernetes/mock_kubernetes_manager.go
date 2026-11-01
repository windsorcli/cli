// Package kubernetes provides Kubernetes resource management functionality
// It implements server-side apply patterns for managing Kubernetes resources
// and provides a clean interface for kustomization and resource management

package kubernetes

import (
	"context"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Types
// =============================================================================

// MockKubernetesManager is a mock implementation of KubernetesManager interface for testing
type MockKubernetesManager struct {
	DeleteKustomizationFunc             func(name, namespace string) error
	WaitForKustomizationsFunc           func(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, layers KustomizationReporter) error
	GetKustomizationReadinessFunc       func(names []string) (map[string]bool, error)
	ApplySecretFunc                     func(name, namespace, secretType string, stringData map[string]string, owner string) error
	PruneSecretsFunc                    func(desired map[string]map[string]bool) error
	RollWorkloadsForSecretFunc          func(ctx context.Context, namespace, secretName, digest string) error
	ApplyVersionMarkerFunc              func(namespace string, marker VersionMarker) error
	GetVersionMarkerFunc                func(namespace string) (VersionMarker, bool, error)
	GetHelmReleasesForKustomizationFunc func(name, namespace string) ([]helmv2.HelmRelease, error)
	KustomizationExistsFunc             func(name, namespace string) (bool, error)
	NamespaceExistsFunc                 func(name string) (bool, error)
	GetKustomizationInventoryFunc       func(name, namespace string) ([]InventoryEntry, error)
	WaitForKubernetesHealthyFunc        func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error
	GetNodeReadyStatusFunc              func(ctx context.Context, nodeNames []string) (map[string]bool, error)
	ApplyBlueprintFunc                  func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error
	DeleteBlueprintFunc                 func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error
	PruneBlueprintFunc                  func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error
	ListPrunableKustomizationsFunc      func(blueprint *blueprintv1alpha1.Blueprint, namespace string) ([]string, error)
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

// DeleteKustomization implements KubernetesManager interface
func (m *MockKubernetesManager) DeleteKustomization(_ context.Context, name, namespace string) error {
	if m.DeleteKustomizationFunc != nil {
		return m.DeleteKustomizationFunc(name, namespace)
	}
	return nil
}

// WaitForKustomizations implements KubernetesManager interface
func (m *MockKubernetesManager) WaitForKustomizations(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, layers KustomizationReporter) error {
	if m.WaitForKustomizationsFunc != nil {
		return m.WaitForKustomizationsFunc(ctx, blueprint, layers)
	}
	return nil
}

// GetKustomizationReadiness implements KubernetesManager interface. The default reports every requested
// kustomization Ready so a convergence pass returns immediately in tests that do not exercise it; tests
// that drive convergence set GetKustomizationReadinessFunc.
func (m *MockKubernetesManager) GetKustomizationReadiness(names []string) (map[string]bool, error) {
	if m.GetKustomizationReadinessFunc != nil {
		return m.GetKustomizationReadinessFunc(names)
	}
	ready := make(map[string]bool, len(names))
	for _, n := range names {
		ready[n] = true
	}
	return ready, nil
}

// ApplySecret implements KubernetesManager interface
func (m *MockKubernetesManager) ApplySecret(name, namespace, secretType string, stringData map[string]string, owner string) error {
	if m.ApplySecretFunc != nil {
		return m.ApplySecretFunc(name, namespace, secretType, stringData, owner)
	}
	return nil
}

// PruneSecrets implements KubernetesManager interface
func (m *MockKubernetesManager) PruneSecrets(desired map[string]map[string]bool) error {
	if m.PruneSecretsFunc != nil {
		return m.PruneSecretsFunc(desired)
	}
	return nil
}

// RollWorkloadsForSecret implements KubernetesManager interface
func (m *MockKubernetesManager) RollWorkloadsForSecret(ctx context.Context, namespace, secretName, digest string) error {
	if m.RollWorkloadsForSecretFunc != nil {
		return m.RollWorkloadsForSecretFunc(ctx, namespace, secretName, digest)
	}
	return nil
}

// ApplyVersionMarker implements KubernetesManager interface
func (m *MockKubernetesManager) ApplyVersionMarker(namespace string, marker VersionMarker) error {
	if m.ApplyVersionMarkerFunc != nil {
		return m.ApplyVersionMarkerFunc(namespace, marker)
	}
	return nil
}

// GetVersionMarker implements KubernetesManager interface
func (m *MockKubernetesManager) GetVersionMarker(namespace string) (VersionMarker, bool, error) {
	if m.GetVersionMarkerFunc != nil {
		return m.GetVersionMarkerFunc(namespace)
	}
	return VersionMarker{}, false, nil
}

// GetHelmReleasesForKustomization implements KubernetesManager interface
func (m *MockKubernetesManager) GetHelmReleasesForKustomization(name, namespace string) ([]helmv2.HelmRelease, error) {
	if m.GetHelmReleasesForKustomizationFunc != nil {
		return m.GetHelmReleasesForKustomizationFunc(name, namespace)
	}
	return nil, nil
}

// WaitForKustomizationDeletionProcessed waits for the specified kustomization deletion to be processed.

// KustomizationExists implements KubernetesManager interface
func (m *MockKubernetesManager) KustomizationExists(name, namespace string) (bool, error) {
	if m.KustomizationExistsFunc != nil {
		return m.KustomizationExistsFunc(name, namespace)
	}
	return false, nil
}

// NamespaceExists implements KubernetesManager interface
func (m *MockKubernetesManager) NamespaceExists(name string) (bool, error) {
	if m.NamespaceExistsFunc != nil {
		return m.NamespaceExistsFunc(name)
	}
	return false, nil
}

// GetKustomizationInventory implements KubernetesManager interface
func (m *MockKubernetesManager) GetKustomizationInventory(name, namespace string) ([]InventoryEntry, error) {
	if m.GetKustomizationInventoryFunc != nil {
		return m.GetKustomizationInventoryFunc(name, namespace)
	}
	return nil, nil
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
func (m *MockKubernetesManager) DeleteBlueprint(_ context.Context, blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
	if m.DeleteBlueprintFunc != nil {
		return m.DeleteBlueprintFunc(blueprint, namespace)
	}
	return nil
}

// PruneBlueprint implements KubernetesManager interface
func (m *MockKubernetesManager) PruneBlueprint(_ context.Context, blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
	if m.PruneBlueprintFunc != nil {
		return m.PruneBlueprintFunc(blueprint, namespace)
	}
	return nil
}

// ListPrunableKustomizations implements KubernetesManager interface
func (m *MockKubernetesManager) ListPrunableKustomizations(blueprint *blueprintv1alpha1.Blueprint, namespace string) ([]string, error) {
	if m.ListPrunableKustomizationsFunc != nil {
		return m.ListPrunableKustomizationsFunc(blueprint, namespace)
	}
	return nil, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockKubernetesManager implements KubernetesManager interface
var _ KubernetesManager = (*MockKubernetesManager)(nil)
