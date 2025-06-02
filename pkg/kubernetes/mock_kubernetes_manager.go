package kubernetes

import (
	"github.com/windsorcli/cli/pkg/di"
)

// MockKubernetesManager is a mock implementation of KubernetesManager interface for testing
type MockKubernetesManager struct {
	InitializeFunc                      func() error
	ApplyKustomizationFunc              func(kustomization Kustomization) error
	DeleteKustomizationFunc             func(name, namespace string) error
	WaitForKustomizationsFunc           func(message string, names ...string) error
	GetKustomizationStatusFunc          func(names []string) (map[string]bool, error)
	CreateNamespaceFunc                 func(name string) error
	DeleteNamespaceFunc                 func(name string) error
	ApplyConfigMapFunc                  func(name, namespace string, data map[string]string) error
	GetHelmReleasesForKustomizationFunc func(name, namespace string) ([]HelmRelease, error)
	SuspendKustomizationFunc            func(name, namespace string) error
	SuspendHelmReleaseFunc              func(name, namespace string) error
}

// NewMockKubernetesManager creates a new instance of MockKubernetesManager
func NewMockKubernetesManager(injector di.Injector) *MockKubernetesManager {
	return &MockKubernetesManager{}
}

// Initialize implements KubernetesManager interface
func (m *MockKubernetesManager) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

// ApplyKustomization implements KubernetesManager interface
func (m *MockKubernetesManager) ApplyKustomization(kustomization Kustomization) error {
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
func (m *MockKubernetesManager) GetHelmReleasesForKustomization(name, namespace string) ([]HelmRelease, error) {
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
