// Package kubernetes provides Kubernetes resource management functionality
// It implements server-side apply patterns for managing Kubernetes resources
// and provides a clean interface for kustomization and resource management

package kubernetes

import (
	"context"
	"fmt"
	"maps"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// =============================================================================
// Interfaces
// =============================================================================

// KubernetesManager defines methods for Kubernetes resource management
type KubernetesManager interface {
	Initialize() error
	ApplyKustomization(kustomization kustomizev1.Kustomization) error
	DeleteKustomization(name, namespace string) error
	WaitForKustomizations(message string, names ...string) error
	CreateNamespace(name string) error
	DeleteNamespace(name string) error
	ApplyConfigMap(name, namespace string, data map[string]string) error
	GetHelmReleasesForKustomization(name, namespace string) ([]helmv2.HelmRelease, error)
	SuspendKustomization(name, namespace string) error
	SuspendHelmRelease(name, namespace string) error
	ApplyGitRepository(repo *sourcev1.GitRepository) error
	ApplyOCIRepository(repo *sourcev1.OCIRepository) error
	CheckGitRepositoryStatus() error
	GetKustomizationStatus(names []string) (map[string]bool, error)
	WaitForKubernetesHealthy(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error
	GetNodeReadyStatus(ctx context.Context, nodeNames []string) (map[string]bool, error)
}

// =============================================================================
// Constructor
// =============================================================================

// BaseKubernetesManager implements KubernetesManager interface
type BaseKubernetesManager struct {
	injector di.Injector
	shims    *Shims
	client   KubernetesClient

	kustomizationWaitPollInterval time.Duration
	kustomizationReconcileTimeout time.Duration
	kustomizationReconcileSleep   time.Duration
}

// NewKubernetesManager creates a new instance of BaseKubernetesManager
func NewKubernetesManager(injector di.Injector) *BaseKubernetesManager {
	return &BaseKubernetesManager{
		injector:                      injector,
		shims:                         NewShims(),
		kustomizationWaitPollInterval: 2 * time.Second,
		kustomizationReconcileTimeout: 5 * time.Minute,
		kustomizationReconcileSleep:   2 * time.Second,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the BaseKubernetesManager by resolving dependencies
func (k *BaseKubernetesManager) Initialize() error {
	client, ok := k.injector.Resolve("kubernetesClient").(KubernetesClient)
	if !ok {
		return fmt.Errorf("error resolving kubernetesClient")
	}
	k.client = client
	return nil
}

// ApplyKustomization creates or updates a Kustomization resource using SSA
func (k *BaseKubernetesManager) ApplyKustomization(kustomization kustomizev1.Kustomization) error {
	obj := &unstructured.Unstructured{}
	unstructuredMap, err := k.shims.ToUnstructured(&kustomization)
	if err != nil {
		return fmt.Errorf("failed to convert kustomization to unstructured: %w", err)
	}
	obj.Object = unstructuredMap

	if err := validateFields(obj); err != nil {
		return fmt.Errorf("invalid kustomization fields: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}

	opts := metav1.ApplyOptions{
		FieldManager: "windsor-cli",
		Force:        false,
	}

	return k.applyWithRetry(gvr, obj, opts)
}

// DeleteKustomization removes a Kustomization resource using background deletion.
// Background deletion allows the kustomization to enter "Terminating" state while its
// children are deleted in the background. The method waits for the deletion to complete.
func (k *BaseKubernetesManager) DeleteKustomization(name, namespace string) error {
	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}

	propagationPolicy := metav1.DeletePropagationBackground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}

	err := k.client.DeleteResource(gvr, namespace, name, deleteOptions)
	if err != nil && isNotFoundError(err) {
		return nil
	}
	if err != nil {
		return err
	}

	timeout := time.Now().Add(k.kustomizationReconcileTimeout)
	for time.Now().Before(timeout) {
		_, err := k.client.GetResource(gvr, namespace, name)
		if err != nil && isNotFoundError(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("error checking kustomization deletion status: %w", err)
		}
		time.Sleep(k.kustomizationWaitPollInterval)
	}

	return fmt.Errorf("timeout waiting for kustomization %s to be deleted", name)
}

// WaitForKustomizations waits for kustomizations to be ready
func (k *BaseKubernetesManager) WaitForKustomizations(message string, names ...string) error {
	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " " + message
	spin.Start()
	defer spin.Stop()

	timeout := time.After(k.kustomizationReconcileTimeout)
	ticker := time.NewTicker(k.kustomizationWaitPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("timeout waiting for kustomizations")
		case <-ticker.C:
			allReady := true
			for _, name := range names {
				gvr := schema.GroupVersionResource{
					Group:    "kustomize.toolkit.fluxcd.io",
					Version:  "v1",
					Resource: "kustomizations",
				}
				obj, err := k.client.GetResource(gvr, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE, name)
				if err != nil {
					allReady = false
					break
				}
				var kustomizationObj map[string]any
				if err := k.shims.FromUnstructured(obj.UnstructuredContent(), &kustomizationObj); err != nil {
					allReady = false
					break
				}
				status, ok := kustomizationObj["status"].(map[string]any)
				if !ok {
					allReady = false
					break
				}
				conditions, ok := status["conditions"].([]any)
				if !ok {
					allReady = false
					break
				}
				ready := false
				for _, cond := range conditions {
					condMap, ok := cond.(map[string]any)
					if !ok {
						continue
					}
					if condMap["type"] == "Ready" && condMap["status"] == "True" {
						ready = true
						break
					}
				}
				if !ready {
					allReady = false
					break
				}
			}
			if allReady {
				spin.Stop()
				fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m%s - Done\n", spin.Suffix)
				return nil
			}
		}
	}
}

// CreateNamespace creates a new namespace
func (k *BaseKubernetesManager) CreateNamespace(name string) error {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name": name,
				"labels": map[string]any{
					"app.kubernetes.io/managed-by": "windsor-cli",
				},
			},
		},
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	}

	opts := metav1.ApplyOptions{
		FieldManager: "windsor-cli",
		Force:        false,
	}

	return k.applyWithRetry(gvr, obj, opts)
}

// DeleteNamespace deletes the specified namespace using foreground deletion.
// Foreground deletion ensures all resources in the namespace are removed before the namespace is deleted.
// This method waits for the deletion to complete before returning. Returns nil if the namespace is deleted successfully,
// or an error if deletion fails or times out.
func (k *BaseKubernetesManager) DeleteNamespace(name string) error {
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	}

	return k.client.DeleteResource(gvr, "", name, metav1.DeleteOptions{})
}

// ApplyConfigMap creates or updates a ConfigMap using SSA
func (k *BaseKubernetesManager) ApplyConfigMap(name, namespace string, data map[string]string) error {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"data": data,
		},
	}

	if err := validateFields(obj); err != nil {
		return fmt.Errorf("invalid configmap fields: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	existing, err := k.client.GetResource(gvr, namespace, name)
	if err == nil && isImmutableConfigMap(existing) {
		if err := k.client.DeleteResource(gvr, namespace, name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete immutable configmap: %w", err)
		}
		time.Sleep(time.Second)
	}

	opts := metav1.ApplyOptions{
		FieldManager: "windsor-cli",
		Force:        false,
	}

	return k.applyWithRetry(gvr, obj, opts)
}

// GetHelmReleasesForKustomization gets HelmReleases associated with a Kustomization
func (k *BaseKubernetesManager) GetHelmReleasesForKustomization(name, namespace string) ([]helmv2.HelmRelease, error) {
	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}

	obj, err := k.client.GetResource(gvr, namespace, name)
	if err != nil {
		if isNotFoundError(err) {
			return []helmv2.HelmRelease{}, nil
		}
		return nil, fmt.Errorf("failed to get kustomization: %w", err)
	}

	var kustomization kustomizev1.Kustomization
	if err := k.shims.FromUnstructured(obj.UnstructuredContent(), &kustomization); err != nil {
		return nil, fmt.Errorf("failed to convert kustomization: %w", err)
	}

	var helmReleases []helmv2.HelmRelease
	if kustomization.Status.Inventory == nil {
		return helmReleases, nil
	}

	for _, entry := range kustomization.Status.Inventory.Entries {
		parts := strings.Split(entry.ID, "_")
		if len(parts) >= 4 && parts[2] == "helm.toolkit.fluxcd.io" && parts[3] == "HelmRelease" {
			helmRelease, err := k.getHelmRelease(parts[1], parts[0])
			if err != nil {
				return nil, err
			}
			helmReleases = append(helmReleases, *helmRelease)
		}
	}

	return helmReleases, nil
}

// SuspendKustomization applies a JSON merge patch to set spec.suspend=true on the specified Kustomization.
func (k *BaseKubernetesManager) SuspendKustomization(name, namespace string) error {
	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}

	patch := []byte(`{"spec":{"suspend":true}}`)
	_, err := k.client.PatchResource(gvr, namespace, name, types.MergePatchType, patch, metav1.PatchOptions{
		FieldManager: "windsor-cli",
	})

	return err
}

// SuspendHelmRelease applies a JSON merge patch to set spec.suspend=true on the specified HelmRelease.
func (k *BaseKubernetesManager) SuspendHelmRelease(name, namespace string) error {
	gvr := schema.GroupVersionResource{
		Group:    "helm.toolkit.fluxcd.io",
		Version:  "v2",
		Resource: "helmreleases",
	}

	patch := []byte(`{"spec":{"suspend":true}}`)
	_, err := k.client.PatchResource(gvr, namespace, name, types.MergePatchType, patch, metav1.PatchOptions{
		FieldManager: "windsor-cli",
	})

	return err
}

// ApplyGitRepository creates or updates a GitRepository resource using SSA
func (k *BaseKubernetesManager) ApplyGitRepository(repo *sourcev1.GitRepository) error {
	obj := &unstructured.Unstructured{}
	unstructuredMap, err := k.shims.ToUnstructured(repo)
	if err != nil {
		return fmt.Errorf("failed to convert gitrepository to unstructured: %w", err)
	}
	obj.Object = unstructuredMap

	if err := validateFields(obj); err != nil {
		return fmt.Errorf("invalid gitrepository fields: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "source.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "gitrepositories",
	}

	opts := metav1.ApplyOptions{
		FieldManager: "windsor-cli",
		Force:        false,
	}

	return k.applyWithRetry(gvr, obj, opts)
}

// ApplyOCIRepository creates or updates an OCIRepository resource using SSA
func (k *BaseKubernetesManager) ApplyOCIRepository(repo *sourcev1.OCIRepository) error {
	obj := &unstructured.Unstructured{}
	unstructuredMap, err := k.shims.ToUnstructured(repo)
	if err != nil {
		return fmt.Errorf("failed to convert ocirepository to unstructured: %w", err)
	}
	obj.Object = unstructuredMap

	if err := validateFields(obj); err != nil {
		return fmt.Errorf("invalid ocirepository fields: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "source.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "ocirepositories",
	}

	opts := metav1.ApplyOptions{
		FieldManager: "windsor-cli",
		Force:        false,
	}

	return k.applyWithRetry(gvr, obj, opts)
}

// CheckGitRepositoryStatus checks the status of all GitRepository and OCIRepository resources
func (k *BaseKubernetesManager) CheckGitRepositoryStatus() error {
	gitGvr := schema.GroupVersionResource{
		Group:    "source.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "gitrepositories",
	}

	gitObjList, err := k.client.ListResources(gitGvr, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE)
	if err != nil {
		return fmt.Errorf("failed to list git repositories: %w", err)
	}

	for _, obj := range gitObjList.Items {
		var gitRepo sourcev1.GitRepository
		if err := k.shims.FromUnstructured(obj.UnstructuredContent(), &gitRepo); err != nil {
			return fmt.Errorf("failed to convert git repository %s: %w", gitRepo.Name, err)
		}

		for _, condition := range gitRepo.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == "False" {
				return fmt.Errorf("%s: %s", gitRepo.Name, condition.Message)
			}
		}
	}

	ociGvr := schema.GroupVersionResource{
		Group:    "source.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "ocirepositories",
	}

	ociObjList, err := k.client.ListResources(ociGvr, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE)
	if err != nil {
		return fmt.Errorf("failed to list oci repositories: %w", err)
	}

	for _, obj := range ociObjList.Items {
		var ociRepo sourcev1.OCIRepository
		if err := k.shims.FromUnstructured(obj.UnstructuredContent(), &ociRepo); err != nil {
			return fmt.Errorf("failed to convert oci repository %s: %w", ociRepo.Name, err)
		}

		for _, condition := range ociRepo.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == "False" {
				return fmt.Errorf("%s: %s", ociRepo.Name, condition.Message)
			}
		}
	}

	return nil
}

// GetKustomizationStatus returns a map indicating readiness for each specified kustomization in the default
// Flux system namespace. If a kustomization is not found, its status is set to false. If any kustomization
// has a Ready condition with Status False and Reason "ReconciliationFailed", an error is returned with the
// failure message.
func (k *BaseKubernetesManager) GetKustomizationStatus(names []string) (map[string]bool, error) {
	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}

	objList, err := k.client.ListResources(gvr, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE)
	if err != nil {
		return nil, fmt.Errorf("failed to list kustomizations: %w", err)
	}

	status := make(map[string]bool)
	found := make(map[string]bool)

	for _, obj := range objList.Items {
		var kustomizeObj kustomizev1.Kustomization
		if err := k.shims.FromUnstructured(obj.UnstructuredContent(), &kustomizeObj); err != nil {
			return nil, fmt.Errorf("failed to convert kustomization %s: %w", kustomizeObj.Name, err)
		}

		found[kustomizeObj.Name] = true
		ready := false
		for _, condition := range kustomizeObj.Status.Conditions {
			if condition.Type == "Ready" {
				if condition.Status == "True" {
					ready = true
				} else if condition.Status == "False" && condition.Reason == "ReconciliationFailed" {
					return nil, fmt.Errorf("kustomization %s failed: %s", kustomizeObj.Name, condition.Message)
				}
				break
			}
		}
		status[kustomizeObj.Name] = ready
	}

	for _, name := range names {
		if !found[name] {
			status[name] = false
			continue
		}
	}

	return status, nil
}

// WaitForKubernetesHealthy waits for the Kubernetes API to become healthy within the context deadline.
// If nodeNames are provided, verifies all specified nodes reach Ready state before returning.
// Returns an error if the API is unreachable or any specified nodes are not Ready within the deadline.
func (k *BaseKubernetesManager) WaitForKubernetesHealthy(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
	if k.client == nil {
		return fmt.Errorf("kubernetes client not initialized")
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Minute)
	}

	pollInterval := 10 * time.Second

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for Kubernetes API to be healthy")
		default:
			if err := k.client.CheckHealth(ctx, endpoint); err != nil {
				time.Sleep(pollInterval)
				continue
			}

			if len(nodeNames) > 0 {
				if err := k.waitForNodesReady(ctx, nodeNames, outputFunc); err != nil {
					time.Sleep(pollInterval)
					continue
				}
			}

			return nil
		}
	}

	return fmt.Errorf("timeout waiting for Kubernetes API to be healthy")
}

// waitForNodesReady blocks until all specified nodes exist and are in Ready state or the context deadline is reached.
// It periodically queries node status, invokes outputFunc on status changes, and returns an error if any nodes are missing or not Ready within the deadline.
// If the context is cancelled, returns an error immediately.
func (k *BaseKubernetesManager) waitForNodesReady(ctx context.Context, nodeNames []string, outputFunc func(string)) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Minute)
	}

	pollInterval := 5 * time.Second
	lastStatus := make(map[string]string)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for nodes to be ready")
		default:
			readyStatus, err := k.client.GetNodeReadyStatus(ctx, nodeNames)
			if err != nil {
				time.Sleep(pollInterval)
				continue
			}

			var missingNodes []string
			var notReadyNodes []string
			var readyNodes []string

			for _, nodeName := range nodeNames {
				if ready, exists := readyStatus[nodeName]; !exists {
					missingNodes = append(missingNodes, nodeName)
				} else if !ready {
					notReadyNodes = append(notReadyNodes, nodeName)
				} else {
					readyNodes = append(readyNodes, nodeName)
				}
			}

			if outputFunc != nil {
				for _, nodeName := range nodeNames {
					var currentStatus string
					if ready, exists := readyStatus[nodeName]; !exists {
						currentStatus = "NOT FOUND"
					} else if ready {
						currentStatus = "READY"
					} else {
						currentStatus = "NOT READY"
					}

					if lastStatus[nodeName] != currentStatus {
						outputFunc(fmt.Sprintf("Node %s: %s", nodeName, currentStatus))
						lastStatus[nodeName] = currentStatus
					}
				}
			}

			if len(missingNodes) == 0 && len(notReadyNodes) == 0 {
				return nil
			}

			time.Sleep(pollInterval)
		}
	}

	// Final check to get the current status for error reporting
	readyStatus, err := k.client.GetNodeReadyStatus(ctx, nodeNames)
	if err != nil {
		return fmt.Errorf("timeout waiting for nodes to be ready: failed to get final status: %w", err)
	}

	var missingNodes []string
	var notReadyNodes []string

	for _, nodeName := range nodeNames {
		if ready, exists := readyStatus[nodeName]; !exists {
			missingNodes = append(missingNodes, nodeName)
		} else if !ready {
			notReadyNodes = append(notReadyNodes, nodeName)
		}
	}

	if len(missingNodes) > 0 {
		return fmt.Errorf("timeout waiting for nodes to appear: %s", strings.Join(missingNodes, ", "))
	}

	if len(notReadyNodes) > 0 {
		return fmt.Errorf("timeout waiting for nodes to be ready: %s", strings.Join(notReadyNodes, ", "))
	}

	return fmt.Errorf("timeout waiting for nodes to be ready")
}

// GetNodeReadyStatus returns a map of node names to their Ready condition status.
// Returns a map of node names to Ready status (true if Ready, false if NotReady), or an error if listing fails.
func (k *BaseKubernetesManager) GetNodeReadyStatus(ctx context.Context, nodeNames []string) (map[string]bool, error) {
	if k.client == nil {
		return nil, fmt.Errorf("kubernetes client not initialized")
	}
	return k.client.GetNodeReadyStatus(ctx, nodeNames)
}

// applyWithRetry applies a resource using SSA with minimal logic
func (k *BaseKubernetesManager) applyWithRetry(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) error {
	existing, err := k.client.GetResource(gvr, obj.GetNamespace(), obj.GetName())
	if err == nil {
		applyConfig, err := k.shims.ToUnstructured(existing)
		if err != nil {
			return fmt.Errorf("failed to convert existing object to unstructured: %w", err)
		}

		maps.Copy(applyConfig, obj.Object)

		mergedObj := &unstructured.Unstructured{Object: applyConfig}
		mergedObj.SetResourceVersion(existing.GetResourceVersion())

		opts.Force = true

		_, err = k.client.ApplyResource(gvr, mergedObj, opts)
		return err
	}

	_, err = k.client.ApplyResource(gvr, obj, opts)
	return err
}

// getHelmRelease gets a HelmRelease by name and namespace
func (k *BaseKubernetesManager) getHelmRelease(name, namespace string) (*helmv2.HelmRelease, error) {
	gvr := schema.GroupVersionResource{
		Group:    "helm.toolkit.fluxcd.io",
		Version:  "v2",
		Resource: "helmreleases",
	}

	obj, err := k.client.GetResource(gvr, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get helm release: %w", err)
	}

	var helmRelease helmv2.HelmRelease
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &helmRelease); err != nil {
		return nil, fmt.Errorf("failed to convert helm release: %w", err)
	}

	return &helmRelease, nil
}

// =============================================================================
// Helpers
// =============================================================================

// validateFields validates required fields and types
func validateFields(obj *unstructured.Unstructured) error {
	if obj == nil {
		return fmt.Errorf("object cannot be nil")
	}

	metadata, ok := obj.Object["metadata"].(map[string]any)
	if !ok {
		return fmt.Errorf("metadata is required")
	}

	if _, ok := metadata["name"]; !ok {
		return fmt.Errorf("metadata.name is required")
	}
	if name, ok := metadata["name"].(string); ok && strings.TrimSpace(name) == "" {
		return fmt.Errorf("metadata.name cannot be empty")
	}

	if obj.GetKind() == "ConfigMap" {
		if _, hasData := obj.Object["data"]; !hasData {
			return fmt.Errorf("data is required for ConfigMap")
		}
		data, _ := obj.Object["data"]
		if data == nil {
			return fmt.Errorf("data cannot be nil for ConfigMap")
		}
		if m, ok := data.(map[string]string); ok && len(m) == 0 {
			return fmt.Errorf("data cannot be empty for ConfigMap")
		}
		if m, ok := data.(map[string]any); ok && len(m) == 0 {
			return fmt.Errorf("data cannot be empty for ConfigMap")
		}
		return nil
	}

	if _, ok := obj.Object["spec"]; !ok {
		return fmt.Errorf("spec is required")
	}

	return nil
}

// isImmutableConfigMap checks if a ConfigMap is immutable
func isImmutableConfigMap(obj *unstructured.Unstructured) bool {
	if obj.GetKind() != "ConfigMap" {
		return false
	}

	spec, ok := obj.Object["spec"].(map[string]any)
	if !ok {
		return false
	}

	immutable, ok := spec["immutable"].(bool)
	return ok && immutable
}

// isNotFoundError checks if an error is a Kubernetes resource not found error
// This is used during cleanup to ignore errors when resources don't exist
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())
	// Check for resource not found errors, but not namespace not found errors
	return (strings.Contains(errMsg, "resource not found") ||
		strings.Contains(errMsg, "could not find the requested resource") ||
		strings.Contains(errMsg, "the server could not find the requested resource") ||
		strings.Contains(errMsg, "\" not found")) &&
		!strings.Contains(errMsg, "namespace not found")
}
