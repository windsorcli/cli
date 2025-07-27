// Package kubernetes provides Kubernetes resource management functionality
// It implements server-side apply patterns for managing Kubernetes resources
// and provides a clean interface for kustomization and resource management

package kubernetes

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// =============================================================================
// Types
// =============================================================================

// MockKubernetesClient is a mock implementation of KubernetesClient interface for testing
type MockKubernetesClient struct {
	GetResourceFunc        func(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error)
	ListResourcesFunc      func(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error)
	ApplyResourceFunc      func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error)
	DeleteResourceFunc     func(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error
	PatchResourceFunc      func(gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error)
	CheckHealthFunc        func(ctx context.Context, endpoint string) error
	GetNodeReadyStatusFunc func(ctx context.Context, nodeNames []string) (map[string]bool, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockKubernetesClient creates a new MockKubernetesClient
func NewMockKubernetesClient() *MockKubernetesClient {
	return &MockKubernetesClient{}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetResource implements KubernetesClient interface
func (m *MockKubernetesClient) GetResource(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	if m.GetResourceFunc != nil {
		return m.GetResourceFunc(gvr, namespace, name)
	}
	return nil, nil
}

// ListResources implements KubernetesClient interface
func (m *MockKubernetesClient) ListResources(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
	if m.ListResourcesFunc != nil {
		return m.ListResourcesFunc(gvr, namespace)
	}
	return &unstructured.UnstructuredList{}, nil
}

// ApplyResource implements KubernetesClient interface
func (m *MockKubernetesClient) ApplyResource(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	if m.ApplyResourceFunc != nil {
		return m.ApplyResourceFunc(gvr, obj, opts)
	}
	return obj, nil
}

// DeleteResource implements KubernetesClient interface
func (m *MockKubernetesClient) DeleteResource(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
	if m.DeleteResourceFunc != nil {
		return m.DeleteResourceFunc(gvr, namespace, name, opts)
	}
	return nil
}

// PatchResource implements KubernetesClient interface
func (m *MockKubernetesClient) PatchResource(gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
	if m.PatchResourceFunc != nil {
		return m.PatchResourceFunc(gvr, namespace, name, pt, data, opts)
	}
	return nil, nil
}

// CheckHealth implements KubernetesClient interface
func (m *MockKubernetesClient) CheckHealth(ctx context.Context, endpoint string) error {
	if m.CheckHealthFunc != nil {
		return m.CheckHealthFunc(ctx, endpoint)
	}
	return nil
}

// GetNodeReadyStatus implements KubernetesClient interface
func (m *MockKubernetesClient) GetNodeReadyStatus(ctx context.Context, nodeNames []string) (map[string]bool, error) {
	if m.GetNodeReadyStatusFunc != nil {
		return m.GetNodeReadyStatusFunc(ctx, nodeNames)
	}
	return make(map[string]bool), nil
}
