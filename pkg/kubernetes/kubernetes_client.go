// Package kubernetes provides Kubernetes resource management functionality
// It implements server-side apply patterns for managing Kubernetes resources
// and provides a clean interface for kustomization and resource management

package kubernetes

import (
	"context"
	"fmt"
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
	CheckHealth(ctx context.Context, endpoint string) error
}

// =============================================================================
// Constructor
// =============================================================================

// DynamicKubernetesClient implements KubernetesClient using dynamic client
type DynamicKubernetesClient struct {
	client   dynamic.Interface
	endpoint string
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

// CheckHealth verifies Kubernetes API connectivity by listing nodes using the dynamic client.
// If an endpoint is specified, it overrides the default kubeconfig for this check.
// Returns an error if the client cannot be initialized or the API is unreachable.
func (c *DynamicKubernetesClient) CheckHealth(ctx context.Context, endpoint string) error {
	c.endpoint = endpoint

	if err := c.ensureClient(); err != nil {
		return fmt.Errorf("failed to initialize Kubernetes client: %w", err)
	}

	nodeGVR := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "nodes",
	}

	_, err := c.client.Resource(nodeGVR).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("failed to connect to Kubernetes API: %w", err)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// ensureClient initializes the dynamic Kubernetes client if unset. Uses endpoint, in-cluster, or kubeconfig as available.
// Returns error if client setup fails at any stage.
func (c *DynamicKubernetesClient) ensureClient() error {
	if c.client != nil {
		return nil
	}

	var config *rest.Config
	var err error

	if c.endpoint != "" {
		config = &rest.Config{
			Host: c.endpoint,
		}
	} else {
		config, err = rest.InClusterConfig()
		if err != nil {
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
	}

	cli, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}
	c.client = cli
	return nil
}
