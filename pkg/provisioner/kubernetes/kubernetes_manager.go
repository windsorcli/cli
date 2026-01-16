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
	meta "github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"github.com/windsorcli/cli/pkg/runtime/config"
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
	ApplyKustomization(kustomization kustomizev1.Kustomization) error
	DeleteKustomization(name, namespace string) error
	WaitForKustomizations(message string, blueprint *blueprintv1alpha1.Blueprint) error
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
	ApplyBlueprint(blueprint *blueprintv1alpha1.Blueprint, namespace string) error
	DeleteBlueprint(blueprint *blueprintv1alpha1.Blueprint, namespace string) error
}

// =============================================================================
// Constructor
// =============================================================================

// BaseKubernetesManager implements KubernetesManager interface
type BaseKubernetesManager struct {
	shims         *Shims
	client        client.KubernetesClient
	configHandler config.ConfigHandler

	kustomizationWaitPollInterval time.Duration
	kustomizationReconcileTimeout time.Duration
	kustomizationReconcileSleep   time.Duration

	healthCheckPollInterval time.Duration
	nodeReadyPollInterval   time.Duration
}

// NewKubernetesManager creates a new instance of BaseKubernetesManager.
// The configHandler is used to retrieve context name and context ID for CommonMetadata labels.
func NewKubernetesManager(kubernetesClient client.KubernetesClient, configHandler config.ConfigHandler) *BaseKubernetesManager {
	if kubernetesClient == nil {
		panic("kubernetes client is required")
	}
	if configHandler == nil {
		panic("config handler is required")
	}

	manager := &BaseKubernetesManager{
		client:                        kubernetesClient,
		configHandler:                 configHandler,
		shims:                         NewShims(),
		kustomizationWaitPollInterval: 2 * time.Second,
		kustomizationReconcileTimeout: 5 * time.Minute,
		kustomizationReconcileSleep:   2 * time.Second,
		healthCheckPollInterval:       10 * time.Second,
		nodeReadyPollInterval:         5 * time.Second,
	}

	return manager
}

// =============================================================================
// Public Methods
// =============================================================================

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

// WaitForKustomizations waits for kustomizations to be ready, calculating the timeout
// from the longest dependency chain in the blueprint. Outputs a debug message describing
// the total wait timeout being used before beginning polling.
func (k *BaseKubernetesManager) WaitForKustomizations(message string, blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	timeout := k.calculateTotalWaitTime(blueprint)
	kustomizationNames := make([]string, len(blueprint.Kustomizations))
	for i, kustomization := range blueprint.Kustomizations {
		kustomizationNames[i] = kustomization.Name
	}

	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " " + message
	spin.Start()
	defer spin.Stop()

	timeoutChan := time.After(timeout)
	ticker := time.NewTicker(k.kustomizationWaitPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutChan:
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("timeout waiting for kustomizations")
		case <-ticker.C:
			allReady := true
			for _, name := range kustomizationNames {
				gvr := schema.GroupVersionResource{
					Group:    "kustomize.toolkit.fluxcd.io",
					Version:  "v1",
					Resource: "kustomizations",
				}
				obj, err := k.client.GetResource(gvr, constants.DefaultFluxSystemNamespace, name)
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

	gitObjList, err := k.client.ListResources(gitGvr, constants.DefaultFluxSystemNamespace)
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

	ociObjList, err := k.client.ListResources(ociGvr, constants.DefaultFluxSystemNamespace)
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

	objList, err := k.client.ListResources(gvr, constants.DefaultFluxSystemNamespace)
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
				} else if condition.Status == "False" && (condition.Reason == "ReconciliationFailed" || condition.Reason == "ArtifactFailed") {
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

	pollInterval := k.healthCheckPollInterval
	if pollInterval == 0 {
		pollInterval = 10 * time.Second
	}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for Kubernetes API to be healthy")
		default:
			if err := k.client.CheckHealth(ctx, endpoint); err != nil {
				select {
				case <-ctx.Done():
					return fmt.Errorf("timeout waiting for Kubernetes API to be healthy")
				case <-time.After(pollInterval):
					continue
				}
			}

			if len(nodeNames) > 0 {
				if err := k.waitForNodesReady(ctx, nodeNames, outputFunc); err != nil {
					select {
					case <-ctx.Done():
						return fmt.Errorf("timeout waiting for Kubernetes API to be healthy")
					case <-time.After(pollInterval):
						continue
					}
				}
			}

			return nil
		}
	}

	return fmt.Errorf("timeout waiting for Kubernetes API to be healthy")
}

// GetNodeReadyStatus returns a map of node names to their Ready condition status.
// Returns a map of node names to Ready status (true if Ready, false if NotReady), or an error if listing fails.
func (k *BaseKubernetesManager) GetNodeReadyStatus(ctx context.Context, nodeNames []string) (map[string]bool, error) {
	if k.client == nil {
		return nil, fmt.Errorf("kubernetes client not initialized")
	}
	return k.client.GetNodeReadyStatus(ctx, nodeNames)
}

// ApplyBlueprint applies the entire blueprint to the cluster in the proper sequence.
// It creates the target namespace, applies all blueprint source repositories (Git and OCI),
// applies all individual sources, applies any standalone ConfigMaps, and finally applies
// all kustomizations and their associated ConfigMaps. This orchestrates a complete
// blueprint installation following the intended order. CommonMetadata labels are added to all
// kustomizations for resource provenance tracking using context info from the config handler.
// Returns an error if any step fails.
func (k *BaseKubernetesManager) ApplyBlueprint(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
	if err := k.CreateNamespace(namespace); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	if blueprint.Repository.Url != "" {
		var secretName string
		if blueprint.Repository.SecretName != nil {
			secretName = *blueprint.Repository.SecretName
		}
		source := blueprintv1alpha1.Source{
			Name:       blueprint.Metadata.Name,
			Url:        blueprint.Repository.Url,
			Ref:        blueprint.Repository.Ref,
			SecretName: secretName,
		}
		if err := k.applyBlueprintSource(source, namespace); err != nil {
			return fmt.Errorf("failed to apply blueprint repository: %w", err)
		}
	}

	for _, source := range blueprint.Sources {
		if err := k.applyBlueprintSource(source, namespace); err != nil {
			return fmt.Errorf("failed to apply source %s: %w", source.Name, err)
		}
	}

	defaultSourceName := blueprint.Metadata.Name

	if blueprint.ConfigMaps != nil {
		for configMapName, data := range blueprint.ConfigMaps {
			if err := k.ApplyConfigMap(configMapName, namespace, data); err != nil {
				return fmt.Errorf("failed to create ConfigMap %s: %w", configMapName, err)
			}
		}
	}

	for _, kustomization := range blueprint.Kustomizations {
		if kustomization.DestroyOnly != nil && *kustomization.DestroyOnly {
			continue
		}
		if len(kustomization.Substitutions) > 0 {
			configMapName := fmt.Sprintf("values-%s", kustomization.Name)
			if err := k.ApplyConfigMap(configMapName, namespace, kustomization.Substitutions); err != nil {
				return fmt.Errorf("failed to create ConfigMap for kustomization %s: %w", kustomization.Name, err)
			}
		}
		fluxKustomization := kustomization.ToFluxKustomization(namespace, defaultSourceName, blueprint.Sources)

		fluxKustomization.Spec.CommonMetadata = &kustomizev1.CommonMetadata{
			Labels: map[string]string{
				"windsorcli.dev/context":    k.configHandler.GetContext(),
				"windsorcli.dev/context-id": k.configHandler.GetString("id"),
			},
		}

		if len(blueprint.ConfigMaps) > 0 {
			if fluxKustomization.Spec.PostBuild == nil {
				fluxKustomization.Spec.PostBuild = &kustomizev1.PostBuild{
					SubstituteFrom: make([]kustomizev1.SubstituteReference, 0),
				}
			}
			for configMapName := range blueprint.ConfigMaps {
				fluxKustomization.Spec.PostBuild.SubstituteFrom = append(fluxKustomization.Spec.PostBuild.SubstituteFrom, kustomizev1.SubstituteReference{
					Kind:     "ConfigMap",
					Name:     configMapName,
					Optional: false,
				})
			}
		}

		if err := k.ApplyKustomization(fluxKustomization); err != nil {
			return fmt.Errorf("failed to apply kustomization %s: %w", kustomization.Name, err)
		}
	}

	return nil
}

// DeleteBlueprint deletes all kustomizations defined in the given blueprint in the specified namespace.
// Destroy-only kustomizations are applied first (in dependency order), waited for, then deleted.
// Regular kustomizations are then deleted in reverse dependency order. For each regular kustomization
// with cleanup steps defined, cleanup kustomizations are applied and waited for before deletion.
// Errors are accumulated and reported at the end; the function continues attempts even after encountering failures.
func (k *BaseKubernetesManager) DeleteBlueprint(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
	defaultSourceName := blueprint.Metadata.Name
	var errors []error

	if k.hasCleanupOperations(blueprint) {
		if err := k.deployCleanupSemaphore(); err != nil {
			errors = append(errors, fmt.Errorf("failed to deploy cleanup semaphore: %w", err))
		}
	}

	destroyOnlyKustomizations := []blueprintv1alpha1.Kustomization{}
	for _, kustomization := range blueprint.Kustomizations {
		if kustomization.DestroyOnly == nil || !*kustomization.DestroyOnly {
			continue
		}
		destroy := kustomization.Destroy.ToBool()
		if destroy != nil && !*destroy {
			continue
		}
		destroyOnlyKustomizations = append(destroyOnlyKustomizations, kustomization)
	}

	if len(destroyOnlyKustomizations) > 0 {
		if errs := k.processDestroyOnlyKustomizations(destroyOnlyKustomizations, blueprint, namespace, defaultSourceName); len(errs) > 0 {
			errors = append(errors, errs...)
			return fmt.Errorf("cleanup failed with %d error(s): %v", len(errors), errors[0])
		}
	}

	for i := len(blueprint.Kustomizations) - 1; i >= 0; i-- {
		kustomization := blueprint.Kustomizations[i]
		if kustomization.DestroyOnly != nil && *kustomization.DestroyOnly {
			continue
		}
		destroy := kustomization.Destroy.ToBool()
		if destroy != nil && !*destroy {
			continue
		}
		if errs := k.deleteKustomizationWithCleanup(kustomization, blueprint, namespace, defaultSourceName); len(errs) > 0 {
			errors = append(errors, errs...)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("deletion completed with %d error(s): %v", len(errors), errors[0])
	}

	return nil
}

// deleteKustomizationWithCleanup handles the deletion of a single kustomization, including cleanup steps if defined.
// Returns a slice of errors encountered during deletion, which may be empty if no errors occurred.
func (k *BaseKubernetesManager) deleteKustomizationWithCleanup(kustomization blueprintv1alpha1.Kustomization, blueprint *blueprintv1alpha1.Blueprint, namespace, defaultSourceName string) []error {
	var errors []error

	if len(kustomization.Cleanup) > 0 {
		sourceName := kustomization.Source
		if sourceName == "" {
			sourceName = defaultSourceName
		}

		sourceKind := "GitRepository"
		for _, source := range blueprint.Sources {
			if source.Name == sourceName && strings.HasPrefix(source.Url, "oci://") {
				sourceKind = "OCIRepository"
				break
			}
		}

		basePath := kustomization.Path
		if basePath == "" {
			basePath = "kustomize"
		} else {
			basePath = strings.ReplaceAll(basePath, "\\", "/")
			if basePath != "kustomize" && !strings.HasPrefix(basePath, "kustomize/") {
				basePath = "kustomize/" + basePath
			}
		}

		cleanupKustomizationName := fmt.Sprintf("%s-cleanup", kustomization.Name)
		cleanupPath := basePath + "/cleanup"

		timeout := metav1.Duration{Duration: 30 * time.Minute}
		if kustomization.Timeout != nil && kustomization.Timeout.Duration != 0 {
			timeout = metav1.Duration{Duration: kustomization.Timeout.Duration}
		}

		interval := metav1.Duration{Duration: constants.DefaultFluxKustomizationInterval}
		if kustomization.Interval != nil && kustomization.Interval.Duration != 0 {
			interval = metav1.Duration{Duration: kustomization.Interval.Duration}
		}

		retryInterval := metav1.Duration{Duration: constants.DefaultFluxKustomizationRetryInterval}
		if kustomization.RetryInterval != nil && kustomization.RetryInterval.Duration != 0 {
			retryInterval = metav1.Duration{Duration: kustomization.RetryInterval.Duration}
		}

		wait := true
		force := true

		cleanupKustomization := kustomizev1.Kustomization{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Kustomization",
				APIVersion: "kustomize.toolkit.fluxcd.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      cleanupKustomizationName,
				Namespace: namespace,
			},
			Spec: kustomizev1.KustomizationSpec{
				SourceRef: kustomizev1.CrossNamespaceSourceReference{
					Kind: sourceKind,
					Name: sourceName,
				},
				Path:          cleanupPath,
				Interval:      interval,
				RetryInterval: &retryInterval,
				Timeout:       &timeout,
				Wait:          wait,
				Force:         force,
				Prune:         true,
				Components:    kustomization.Cleanup,
			},
		}

		if len(kustomization.Substitutions) > 0 || len(blueprint.ConfigMaps) > 0 {
			cleanupKustomization.Spec.PostBuild = &kustomizev1.PostBuild{
				SubstituteFrom: make([]kustomizev1.SubstituteReference, 0),
			}
			if len(kustomization.Substitutions) > 0 {
				cleanupKustomization.Spec.PostBuild.SubstituteFrom = append(cleanupKustomization.Spec.PostBuild.SubstituteFrom, kustomizev1.SubstituteReference{
					Kind:     "ConfigMap",
					Name:     fmt.Sprintf("values-%s", kustomization.Name),
					Optional: false,
				})
			}
			for configMapName := range blueprint.ConfigMaps {
				cleanupKustomization.Spec.PostBuild.SubstituteFrom = append(cleanupKustomization.Spec.PostBuild.SubstituteFrom, kustomizev1.SubstituteReference{
					Kind:     "ConfigMap",
					Name:     configMapName,
					Optional: false,
				})
			}
		}

		cleanupSpin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
		cleanupSpin.Suffix = fmt.Sprintf(" 🧹 Applying cleanup kustomization for %s", kustomization.Name)
		cleanupSpin.Start()

		if err := k.ApplyKustomization(cleanupKustomization); err != nil {
			cleanupSpin.Stop()
			errors = append(errors, fmt.Errorf("failed to apply cleanup kustomization %s: %w", cleanupKustomizationName, err))
		} else {
			waitTimeout := time.After(k.kustomizationReconcileTimeout)
			ticker := time.NewTicker(k.kustomizationWaitPollInterval)
			cleanupReady := false
			cleanupStatusCheckFailed := false

		cleanupLoop:
			for !cleanupReady {
				select {
				case <-waitTimeout:
					break cleanupLoop
				case <-ticker.C:
					status, err := k.GetKustomizationStatus([]string{cleanupKustomizationName})
					if err != nil {
						cleanupSpin.Stop()
						errors = append(errors, fmt.Errorf("cleanup kustomization %s failed: %w", cleanupKustomizationName, err))
						cleanupStatusCheckFailed = true
						break cleanupLoop
					}
					if status[cleanupKustomizationName] {
						cleanupReady = true
					}
				}
			}
			ticker.Stop()
			cleanupSpin.Stop()

			if !cleanupReady {
				fmt.Fprintf(os.Stderr, "\033[31m✗ 🧹 Applying cleanup kustomization for %s - Failed\033[0m\n", kustomization.Name)
				if !cleanupStatusCheckFailed {
					errors = append(errors, fmt.Errorf("cleanup kustomization %s did not become ready within timeout - cleanup may not have completed", cleanupKustomizationName))
				}
				if deleteErr := k.DeleteKustomization(cleanupKustomizationName, namespace); deleteErr != nil {
					errors = append(errors, fmt.Errorf("failed to delete failed cleanup kustomization %s: %w", cleanupKustomizationName, deleteErr))
				}
				return errors
			}
			fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m 🧹 Applying cleanup kustomization for %s - \033[32mDone\033[0m\n", kustomization.Name)

			cleanupDeleteSpin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
			cleanupDeleteSpin.Suffix = fmt.Sprintf(" 🗑️  Deleting cleanup kustomization %s", cleanupKustomizationName)
			cleanupDeleteSpin.Start()

			if err := k.DeleteKustomization(cleanupKustomizationName, namespace); err != nil {
				cleanupDeleteSpin.Stop()
				errors = append(errors, fmt.Errorf("failed to delete cleanup kustomization %s: %w", cleanupKustomizationName, err))
			} else {
				cleanupDeleteSpin.Stop()
				fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m 🗑️  Deleting cleanup kustomization %s - \033[32mDone\033[0m\n", cleanupKustomizationName)
			}
		}
	}

	deleteSpin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	deleteSpin.Suffix = fmt.Sprintf(" 🗑️  Deleting kustomization %s", kustomization.Name)
	deleteSpin.Start()

	if err := k.DeleteKustomization(kustomization.Name, namespace); err != nil {
		deleteSpin.Stop()
		errors = append(errors, fmt.Errorf("failed to delete kustomization %s: %w", kustomization.Name, err))
		fmt.Fprintf(os.Stderr, "Warning: failed to delete kustomization %s: %v\n", kustomization.Name, err)
	} else {
		deleteSpin.Stop()
		fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m 🗑️  Deleting kustomization %s - \033[32mDone\033[0m\n", kustomization.Name)
	}

	return errors
}

// processDestroyOnlyKustomizations applies all destroy-only kustomizations, waits for all to become ready, then deletes all.
// This approach ensures dependencies remain available while Flux reconciles dependent kustomizations.
// Returns a slice of errors encountered during the process, which may be empty if no errors occurred.
func (k *BaseKubernetesManager) processDestroyOnlyKustomizations(kustomizations []blueprintv1alpha1.Kustomization, blueprint *blueprintv1alpha1.Blueprint, namespace, defaultSourceName string) []error {
	var errors []error

	destroyOnlyNames := make(map[string]bool)
	for _, kust := range kustomizations {
		destroyOnlyNames[kust.Name] = true
	}

	for _, kustomization := range kustomizations {
		if len(kustomization.Substitutions) > 0 {
			configMapName := fmt.Sprintf("values-%s", kustomization.Name)
			if err := k.ApplyConfigMap(configMapName, namespace, kustomization.Substitutions); err != nil {
				errors = append(errors, fmt.Errorf("failed to create ConfigMap for destroy-only kustomization %s: %w", kustomization.Name, err))
				return errors
			}
		}

		fluxKustomization := kustomization.ToFluxKustomization(namespace, defaultSourceName, blueprint.Sources)

		fluxKustomization.Spec.CommonMetadata = &kustomizev1.CommonMetadata{
			Labels: map[string]string{
				"windsorcli.dev/context":    k.configHandler.GetContext(),
				"windsorcli.dev/context-id": k.configHandler.GetString("id"),
			},
		}

		filteredDependsOn := make([]kustomizev1.DependencyReference, 0)
		for _, dep := range fluxKustomization.Spec.DependsOn {
			if destroyOnlyNames[dep.Name] {
				filteredDependsOn = append(filteredDependsOn, dep)
			}
		}
		fluxKustomization.Spec.DependsOn = filteredDependsOn

		if len(blueprint.ConfigMaps) > 0 {
			if fluxKustomization.Spec.PostBuild == nil {
				fluxKustomization.Spec.PostBuild = &kustomizev1.PostBuild{
					SubstituteFrom: make([]kustomizev1.SubstituteReference, 0),
				}
			}
			for configMapName := range blueprint.ConfigMaps {
				fluxKustomization.Spec.PostBuild.SubstituteFrom = append(fluxKustomization.Spec.PostBuild.SubstituteFrom, kustomizev1.SubstituteReference{
					Kind:     "ConfigMap",
					Name:     configMapName,
					Optional: false,
				})
			}
		}

		applySpin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
		applySpin.Suffix = fmt.Sprintf(" 🔧 Applying destroy-only kustomization %s", kustomization.Name)
		applySpin.Start()

		if err := k.ApplyKustomization(fluxKustomization); err != nil {
			applySpin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ 🔧 Applying destroy-only kustomization %s - Failed\033[0m\n", kustomization.Name)
			errors = append(errors, fmt.Errorf("failed to apply destroy-only kustomization %s: %w", kustomization.Name, err))
			return errors
		}
		applySpin.Stop()
		fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m 🔧 Applied destroy-only kustomization %s\n", kustomization.Name)
	}

	kustomizationNames := make([]string, len(kustomizations))
	for i, kust := range kustomizations {
		kustomizationNames[i] = kust.Name
	}

	waitSpin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	waitSpin.Suffix = fmt.Sprintf(" ⏳ Waiting for %d destroy-only kustomization(s) to become ready", len(kustomizations))
	waitSpin.Start()

	waitTimeout := time.After(k.kustomizationReconcileTimeout)
	ticker := time.NewTicker(k.kustomizationWaitPollInterval)
	allReady := false
	statusCheckFailed := false

waitLoop:
	for !allReady {
		select {
		case <-waitTimeout:
			break waitLoop
		case <-ticker.C:
			status, err := k.GetKustomizationStatus(kustomizationNames)
			if err != nil {
				errors = append(errors, fmt.Errorf("destroy-only kustomizations failed: %w", err))
				statusCheckFailed = true
				break waitLoop
			}
			allReady = true
			for _, name := range kustomizationNames {
				if !status[name] {
					allReady = false
					break
				}
			}
		}
	}
	ticker.Stop()
	waitSpin.Stop()

	if !allReady {
		fmt.Fprintf(os.Stderr, "\033[31m✗ ⏳ Waiting for destroy-only kustomizations - Failed\033[0m\n")
		if !statusCheckFailed {
			errors = append(errors, fmt.Errorf("destroy-only kustomizations did not become ready within timeout - cleanup may not have completed"))
		}
		for i := len(kustomizations) - 1; i >= 0; i-- {
			kustomization := kustomizations[i]
			if deleteErr := k.DeleteKustomization(kustomization.Name, namespace); deleteErr != nil {
				errors = append(errors, fmt.Errorf("failed to delete failed destroy-only kustomization %s: %w", kustomization.Name, deleteErr))
			}
		}
		return errors
	}
	fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m ⏳ All destroy-only kustomizations ready\n")

	for i := len(kustomizations) - 1; i >= 0; i-- {
		kustomization := kustomizations[i]
		deleteSpin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
		deleteSpin.Suffix = fmt.Sprintf(" 🗑️  Deleting destroy-only kustomization %s", kustomization.Name)
		deleteSpin.Start()

		if err := k.DeleteKustomization(kustomization.Name, namespace); err != nil {
			deleteSpin.Stop()
			errors = append(errors, fmt.Errorf("failed to delete destroy-only kustomization %s: %w", kustomization.Name, err))
		} else {
			deleteSpin.Stop()
			fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m 🗑️  Deleting destroy-only kustomization %s - \033[32mDone\033[0m\n", kustomization.Name)
		}
	}

	return errors
}

// =============================================================================
// Private Methods
// =============================================================================

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

// hasCleanupOperations checks if any kustomization in the blueprint has destroy-only or cleanup operations.
func (k *BaseKubernetesManager) hasCleanupOperations(blueprint *blueprintv1alpha1.Blueprint) bool {
	for _, kustomization := range blueprint.Kustomizations {
		if kustomization.DestroyOnly != nil && *kustomization.DestroyOnly {
			destroy := kustomization.Destroy.ToBool()
			if destroy == nil || *destroy {
				return true
			}
		}
		if len(kustomization.Cleanup) > 0 {
			destroy := kustomization.Destroy.ToBool()
			if destroy == nil || *destroy {
				return true
			}
		}
	}
	return false
}

// deployCleanupSemaphore creates the cleanup namespace and authorization semaphore ConfigMap.
func (k *BaseKubernetesManager) deployCleanupSemaphore() error {
	if err := k.CreateNamespace(constants.DefaultCleanupNamespace); err != nil {
		return fmt.Errorf("failed to create cleanup namespace: %w", err)
	}

	semaphoreData := map[string]string{
		"authorized": "true",
		"timestamp":  time.Now().Format(time.RFC3339),
	}
	if err := k.ApplyConfigMap(constants.DefaultCleanupSemaphoreName, constants.DefaultCleanupNamespace, semaphoreData); err != nil {
		return fmt.Errorf("failed to create cleanup semaphore: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m 🔓 Cleanup authorization deployed\n")
	return nil
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

// applyBlueprintSource applies a blueprint Source as a GitRepository or OCIRepository resource.
// It routes to the appropriate repository type based on the source URL and applies it to the cluster.
func (k *BaseKubernetesManager) applyBlueprintSource(source blueprintv1alpha1.Source, namespace string) error {
	if strings.HasPrefix(source.Url, "oci://") {
		return k.applyBlueprintOCIRepository(source, namespace)
	}
	return k.applyBlueprintGitRepository(source, namespace)
}

// =============================================================================
// Private Methods
// =============================================================================

// waitForNodesReady blocks until all specified nodes exist and are in Ready state or the context deadline is reached.
// It periodically queries node status, invokes outputFunc on status changes, and returns an error if any nodes are missing or not Ready within the deadline.
// If the context is cancelled, returns an error immediately.
func (k *BaseKubernetesManager) waitForNodesReady(ctx context.Context, nodeNames []string, outputFunc func(string)) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Minute)
	}

	pollInterval := k.nodeReadyPollInterval
	if pollInterval == 0 {
		pollInterval = 5 * time.Second
	}
	lastStatus := make(map[string]string)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for nodes to be ready")
		default:
			readyStatus, err := k.client.GetNodeReadyStatus(ctx, nodeNames)
			if err != nil {
				select {
				case <-ctx.Done():
					return fmt.Errorf("context cancelled while waiting for nodes to be ready")
				case <-time.After(pollInterval):
					continue
				}
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

			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled while waiting for nodes to be ready")
			case <-time.After(pollInterval):
				continue
			}
		}
	}

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

// applyBlueprintGitRepository converts and applies a blueprint Source as a GitRepository.
func (k *BaseKubernetesManager) applyBlueprintGitRepository(source blueprintv1alpha1.Source, namespace string) error {
	sourceUrl := source.Url
	if strings.HasPrefix(sourceUrl, "git@") && strings.Contains(sourceUrl, ":") {
		sourceUrl = "ssh://" + strings.Replace(sourceUrl, ":", "/", 1)
	} else if !strings.HasPrefix(sourceUrl, "http://") && !strings.HasPrefix(sourceUrl, "https://") && !strings.HasPrefix(sourceUrl, "ssh://") {
		sourceUrl = "https://" + sourceUrl
	}

	gitRepo := &sourcev1.GitRepository{
		TypeMeta: metav1.TypeMeta{
			Kind:       "GitRepository",
			APIVersion: "source.toolkit.fluxcd.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      source.Name,
			Namespace: namespace,
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: sourceUrl,
			Interval: metav1.Duration{
				Duration: constants.DefaultFluxSourceInterval,
			},
			Timeout: &metav1.Duration{
				Duration: constants.DefaultFluxSourceTimeout,
			},
			Reference: &sourcev1.GitRepositoryRef{
				Branch: source.Ref.Branch,
				Tag:    source.Ref.Tag,
				SemVer: source.Ref.SemVer,
				Commit: source.Ref.Commit,
			},
		},
	}

	if source.SecretName != "" {
		gitRepo.Spec.SecretRef = &meta.LocalObjectReference{
			Name: source.SecretName,
		}
	}

	return k.ApplyGitRepository(gitRepo)
}

// applyBlueprintOCIRepository converts and applies a blueprint Source as an OCIRepository.
func (k *BaseKubernetesManager) applyBlueprintOCIRepository(source blueprintv1alpha1.Source, namespace string) error {
	ociURL := source.Url
	var ref *sourcev1.OCIRepositoryRef

	if lastColon := strings.LastIndex(ociURL, ":"); lastColon > len("oci://") {
		if tagPart := ociURL[lastColon+1:]; tagPart != "" && !strings.Contains(tagPart, "/") {
			ociURL = ociURL[:lastColon]
			ref = &sourcev1.OCIRepositoryRef{
				Tag: tagPart,
			}
		}
	}

	if ref == nil && (source.Ref.Tag != "" || source.Ref.SemVer != "" || source.Ref.Commit != "") {
		ref = &sourcev1.OCIRepositoryRef{
			Tag:    source.Ref.Tag,
			SemVer: source.Ref.SemVer,
			Digest: source.Ref.Commit,
		}
	}

	if ref == nil {
		ref = &sourcev1.OCIRepositoryRef{
			Tag: "latest",
		}
	}

	ociRepo := &sourcev1.OCIRepository{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OCIRepository",
			APIVersion: "source.toolkit.fluxcd.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      source.Name,
			Namespace: namespace,
		},
		Spec: sourcev1.OCIRepositorySpec{
			URL: ociURL,
			Interval: metav1.Duration{
				Duration: constants.DefaultFluxSourceInterval,
			},
			Timeout: &metav1.Duration{
				Duration: constants.DefaultFluxSourceTimeout,
			},
			Reference: ref,
		},
	}

	if source.SecretName != "" {
		ociRepo.Spec.SecretRef = &meta.LocalObjectReference{
			Name: source.SecretName,
		}
	}

	return k.ApplyOCIRepository(ociRepo)
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

// calculateTotalWaitTime calculates the total timeout for the longest dependency chain
// by summing the timeouts of all kustomizations along the path. It traverses the dependency graph
// to find the path with the maximum cumulative timeout. Returns the calculated timeout or the default
// if no kustomizations exist. Cycles are not detected and may cause stack overflow.
func (k *BaseKubernetesManager) calculateTotalWaitTime(blueprint *blueprintv1alpha1.Blueprint) time.Duration {
	if len(blueprint.Kustomizations) == 0 {
		return constants.DefaultKustomizationWaitTotalTimeout
	}

	nameToIndex := make(map[string]int)
	for i, kustomization := range blueprint.Kustomizations {
		nameToIndex[kustomization.Name] = i
	}

	var calculateChainTimeout func(componentIndex int, visited map[int]bool) time.Duration
	calculateChainTimeout = func(componentIndex int, visited map[int]bool) time.Duration {
		if visited[componentIndex] {
			return 0
		}
		visited[componentIndex] = true
		defer delete(visited, componentIndex)

		kustomization := blueprint.Kustomizations[componentIndex]

		currentTimeout := constants.DefaultFluxKustomizationTimeout
		if kustomization.Timeout != nil && kustomization.Timeout.Duration != 0 {
			currentTimeout = kustomization.Timeout.Duration
		}

		if len(kustomization.DependsOn) == 0 {
			return currentTimeout
		}

		maxDependencyTimeout := time.Duration(0)
		for _, depName := range kustomization.DependsOn {
			if depIndex, exists := nameToIndex[depName]; exists {
				depTimeout := calculateChainTimeout(depIndex, visited)
				if depTimeout > maxDependencyTimeout {
					maxDependencyTimeout = depTimeout
				}
			}
		}

		return currentTimeout + maxDependencyTimeout
	}

	maxTimeout := time.Duration(0)
	for i := range blueprint.Kustomizations {
		timeout := calculateChainTimeout(i, make(map[int]bool))
		if timeout > maxTimeout {
			maxTimeout = timeout
		}
	}

	if maxTimeout == 0 {
		return constants.DefaultKustomizationWaitTotalTimeout
	}

	return maxTimeout
}

// isNotFoundError checks if an error is a Kubernetes resource not found error
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())
	return (strings.Contains(errMsg, "resource not found") ||
		strings.Contains(errMsg, "could not find the requested resource") ||
		strings.Contains(errMsg, "the server could not find the requested resource") ||
		strings.Contains(errMsg, "\" not found")) &&
		!strings.Contains(errMsg, "namespace not found")
}
