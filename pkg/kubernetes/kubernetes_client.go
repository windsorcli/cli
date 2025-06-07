// Package kubernetes provides Kubernetes resource management functionality
// It implements server-side apply patterns for managing Kubernetes resources
// and provides a clean interface for kustomization and resource management

package kubernetes

import (
	"context"

	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// =============================================================================
// Interfaces
// =============================================================================

// KubernetesClient defines methods for low-level Kubernetes operations
type KubernetesClient interface {
	// Resource operations
	GetResource(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error)
	ListResources(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error)
	ApplyResource(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error)
	DeleteResource(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error
	PatchResource(gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error)
}

// =============================================================================
// Constructor
// =============================================================================

// DynamicKubernetesClient implements KubernetesClient using dynamic client
type DynamicKubernetesClient struct {
	client dynamic.Interface
}

// NewDynamicKubernetesClient creates a new DynamicKubernetesClient
func NewDynamicKubernetesClient() *DynamicKubernetesClient {
	return &DynamicKubernetesClient{}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetResource gets a resource by name and namespace
func (c *DynamicKubernetesClient) GetResource(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	if err := c.ensureClient(); err != nil {
		return nil, err
	}
	return c.client.Resource(gvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

// ListResources lists resources in a namespace
func (c *DynamicKubernetesClient) ListResources(gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
	if err := c.ensureClient(); err != nil {
		return nil, err
	}
	return c.client.Resource(gvr).Namespace(namespace).List(context.Background(), metav1.ListOptions{})
}

// ApplyResource applies a resource using server-side apply
func (c *DynamicKubernetesClient) ApplyResource(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	if err := c.ensureClient(); err != nil {
		return nil, err
	}
	return c.client.Resource(gvr).Namespace(obj.GetNamespace()).Apply(context.Background(), obj.GetName(), obj, opts)
}

// DeleteResource deletes a resource
func (c *DynamicKubernetesClient) DeleteResource(gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
	if err := c.ensureClient(); err != nil {
		return err
	}
	return c.client.Resource(gvr).Namespace(namespace).Delete(context.Background(), name, opts)
}

// PatchResource patches a resource
func (c *DynamicKubernetesClient) PatchResource(gvr schema.GroupVersionResource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
	if err := c.ensureClient(); err != nil {
		return nil, err
	}
	return c.client.Resource(gvr).Namespace(namespace).Patch(context.Background(), name, pt, data, opts)
}

// =============================================================================
// Private Methods
// =============================================================================

// ensureClient lazily initializes the dynamic client if not already set.
// This is a Windsor CLI exception: see comment above.
func (c *DynamicKubernetesClient) ensureClient() error {
	if c.client != nil {
		return nil
	}
	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to kubeconfig from KUBECONFIG or default location
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			kubeconfig = home + "/.kube/config"
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return err
		}
	}
	cli, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}
	c.client = cli
	return nil
}
