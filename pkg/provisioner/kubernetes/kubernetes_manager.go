// Package kubernetes provides Kubernetes resource management functionality
// It implements server-side apply patterns for managing Kubernetes resources
// and provides a clean interface for kustomization and resource management

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"strings"
	"time"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	meta "github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"github.com/windsorcli/cli/pkg/runtime/config"
	runtimegit "github.com/windsorcli/cli/pkg/runtime/git"
	"github.com/windsorcli/cli/pkg/tui"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	ApplyGitRepository(repo *sourcev1.GitRepository) error
	ApplyOCIRepository(repo *sourcev1.OCIRepository) error
	CheckGitRepositoryStatus() error
	GetKustomizationStatus(names []string) (map[string]bool, error)
	KustomizationExists(name, namespace string) (bool, error)
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
//
// On timeout (Flux's WaitForTermination has not lifted its finalizer within
// kustomizationReconcileTimeout), the method returns an error rather than stripping
// finalizers. The timeout almost always means an inventory item is stuck on a
// cloud-controller finalizer (CSI external-attacher, aws-load-balancer-controller's
// service.k8s.aws/resources, cert-manager) — stripping the Kustomization's own
// finalizer at that point reaps the Kustomization from etcd but leaves the inventory
// items orphaned in their namespace, where the controller that should lift their
// finalizers may already be torn down. The orphan then blocks namespace termination
// and (worse) lets terraform proceed to destroy the cluster while cloud resources
// (LBs, EBS volumes, DNS records) leak. Returning an error here surfaces the stuck
// state so the operator can re-deploy the controller, let it finish cleanup, and
// re-run destroy — vs. silently masking the failure.
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

	return fmt.Errorf("timeout waiting for kustomization %s/%s to be deleted; an inventory item is likely stuck on a cloud-controller finalizer — inspect with `kubectl get kustomization %s -n %s -o yaml` (status.conditions, status.inventory) and `kubectl get pvc,svc,ingress,certificate -A | grep Terminating` to find the stuck object", namespace, name, name, namespace)
}

// WaitForKustomizations waits for kustomizations to be ready, calculating the timeout
// from the longest dependency chain in the blueprint. Outputs a debug message describing
// the total wait timeout being used before beginning polling.
func (k *BaseKubernetesManager) WaitForKustomizations(message string, blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	timeout := k.calculateTotalWaitTime(blueprint)
	kustomizationNames := make([]string, 0, len(blueprint.Kustomizations))
	for _, kustomization := range blueprint.Kustomizations {
		if kustomization.DestroyOnly != nil && *kustomization.DestroyOnly {
			continue
		}
		kustomizationNames = append(kustomizationNames, kustomization.Name)
	}

	tui.Start(message)

	timeoutChan := time.After(timeout)
	ticker := time.NewTicker(k.kustomizationWaitPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutChan:
			tui.Fail()
			return fmt.Errorf("timeout waiting for kustomizations")
		case <-ticker.C:
			allReady := true
			for _, name := range kustomizationNames {
				gvr := schema.GroupVersionResource{
					Group:    "kustomize.toolkit.fluxcd.io",
					Version:  "v1",
					Resource: "kustomizations",
				}
				obj, err := k.client.GetResource(gvr, k.gitopsNamespace(), name)
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
				tui.Done()
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

	gitObjList, err := k.client.ListResources(gitGvr, k.gitopsNamespace())
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

	ociObjList, err := k.client.ListResources(ociGvr, k.gitopsNamespace())
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

	objList, err := k.client.ListResources(gvr, k.gitopsNamespace())
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

// KustomizationExists returns true if a Kustomization resource with the given name exists in the given namespace.
// Returns false (not an error) when the resource is simply absent; propagates other API errors.
func (k *BaseKubernetesManager) KustomizationExists(name, namespace string) (bool, error) {
	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}
	_, err := k.client.GetResource(gvr, namespace, name)
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
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

	mode := k.gitopsMode()

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
		if blueprintv1alpha1.IsLocalTemplateSource(source) {
			continue
		}
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
		fluxKustomization := kustomization.ToFluxKustomization(namespace, defaultSourceName, blueprint.Sources, mode, blueprint.ConfigMaps)

		fluxKustomization.Spec.CommonMetadata = &kustomizev1.CommonMetadata{
			Labels: map[string]string{
				"windsorcli.dev/context":    k.configHandler.GetContext(),
				"windsorcli.dev/context-id": k.configHandler.GetString("id"),
			},
		}

		if err := k.ApplyKustomization(fluxKustomization); err != nil {
			return fmt.Errorf("failed to apply kustomization %s: %w", kustomization.Name, err)
		}
	}

	return nil
}

// DeleteBlueprint tears the blueprint down in two phases:
//
//  1. Destroy-only kustomizations: apply, wait ready, delete in reverse-topological
//     order. These are blueprint entries that exist only at destroy time for bespoke
//     teardown work (e.g. backups, snapshots, last-mile state exports). Any errors
//     from this phase are joined and returned immediately — the destroy walk does
//     not start until the destroy hooks succeed.
//
//  2. Regular kustomizations in reverse-topological order. Each Kustomization carries
//     spec.deletionPolicy=WaitForTermination (set at apply time by ToFluxKustomization),
//     so DELETE blocks until every managed resource is fully gone from etcd. The chain
//     for cloud resources is:
//
//         K8s DELETE → controller's finalizer holds the object in etcd
//         → controller calls cloud API to release external state
//         → finalizer lifts → object NotFound
//         → WaitForTermination satisfied
//
//     This is what makes CSI volumes, LB Services, Ingresses, and cert-manager
//     Certificates clean up cloud-side without the orchestrator ever calling a
//     cloud API. external-dns is the one outlier — it has no finalizer; the DNS
//     record is removed on its next reconcile after the K8s object disappears.
//
//     Phase 2 aborts on the first per-Kustomization failure (typically an inventory
//     item stuck on a cloud-controller finalizer). Continuing the walk would tear
//     down upstream controllers (lb-base, dns, pki-base) still needed to lift those
//     finalizers, turning a recoverable stuck-Kustomization into a cascade of
//     orphaned cloud resources. Re-running destroy after the operator restores the
//     controller picks up where it left off — already-deleted Kustomizations
//     short-circuit to NotFound on retry.
func (k *BaseKubernetesManager) DeleteBlueprint(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
	defaultSourceName := blueprint.Metadata.Name

	destroyOnly := []blueprintv1alpha1.Kustomization{}
	for _, kustomization := range blueprint.Kustomizations {
		if kustomization.DestroyOnly == nil || !*kustomization.DestroyOnly {
			continue
		}
		destroy := kustomization.Destroy.ToBool()
		if destroy != nil && !*destroy {
			continue
		}
		destroyOnly = append(destroyOnly, kustomization)
	}
	if len(destroyOnly) > 0 {
		if errs := k.processDestroyOnlyKustomizations(destroyOnly, blueprint, namespace, defaultSourceName); len(errs) > 0 {
			return fmt.Errorf("destroy-only hooks failed: %w", errors.Join(errs...))
		}
	}

	eligible := make([]blueprintv1alpha1.Kustomization, 0, len(blueprint.Kustomizations))
	for _, kustomization := range blueprint.Kustomizations {
		if kustomization.DestroyOnly != nil && *kustomization.DestroyOnly {
			continue
		}
		destroy := kustomization.Destroy.ToBool()
		if destroy != nil && !*destroy {
			continue
		}
		eligible = append(eligible, kustomization)
	}
	for _, kustomization := range orderForDestroy(eligible, "destroy") {
		tui.Start(fmt.Sprintf("Destroying kustomization %s", kustomization.Name))
		if err := k.DeleteKustomization(kustomization.Name, namespace); err != nil {
			tui.Fail()
			return fmt.Errorf("destroy aborted: failed to delete kustomization %q: %w (further deletions skipped to avoid cascading orphans)", kustomization.Name, err)
		}
		tui.Done()
	}

	return nil
}

// processDestroyOnlyKustomizations applies all destroy-only kustomizations, waits for all to become ready, then deletes all.
// This approach ensures dependencies remain available while Flux reconciles dependent kustomizations.
// Returns a slice of errors encountered during the process, which may be empty if no errors occurred.
func (k *BaseKubernetesManager) processDestroyOnlyKustomizations(kustomizations []blueprintv1alpha1.Kustomization, blueprint *blueprintv1alpha1.Blueprint, namespace, defaultSourceName string) []error {
	mode := k.gitopsMode()
	var errors []error

	destroyOnlyNames := make(map[string]bool)
	for _, kust := range kustomizations {
		destroyOnlyNames[kust.Name] = true
	}

	appliedKustomizations := []blueprintv1alpha1.Kustomization{}

	for _, kustomization := range kustomizations {
		if len(kustomization.Substitutions) > 0 {
			configMapName := fmt.Sprintf("values-%s", kustomization.Name)
			if err := k.ApplyConfigMap(configMapName, namespace, kustomization.Substitutions); err != nil {
				errors = append(errors, fmt.Errorf("failed to create ConfigMap for destroy-only kustomization %s: %w", kustomization.Name, err))
				for i := len(appliedKustomizations) - 1; i >= 0; i-- {
					appliedKust := appliedKustomizations[i]
					if deleteErr := k.DeleteKustomization(appliedKust.Name, namespace); deleteErr != nil {
						errors = append(errors, fmt.Errorf("failed to delete failed destroy-only kustomization %s: %w", appliedKust.Name, deleteErr))
					}
				}
				return errors
			}
		}

		fluxKustomization := kustomization.ToFluxKustomization(namespace, defaultSourceName, blueprint.Sources, mode, blueprint.ConfigMaps)

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

		tui.Start(fmt.Sprintf("Applying destroy-only kustomization %s", kustomization.Name))

		if err := k.ApplyKustomization(fluxKustomization); err != nil {
			tui.Fail()
			errors = append(errors, fmt.Errorf("failed to apply destroy-only kustomization %s: %w", kustomization.Name, err))
			for i := len(appliedKustomizations) - 1; i >= 0; i-- {
				appliedKust := appliedKustomizations[i]
				if deleteErr := k.DeleteKustomization(appliedKust.Name, namespace); deleteErr != nil {
					errors = append(errors, fmt.Errorf("failed to delete failed destroy-only kustomization %s: %w", appliedKust.Name, deleteErr))
				}
			}
			return errors
		}
		tui.Done()
		appliedKustomizations = append(appliedKustomizations, kustomization)
	}

	kustomizationNames := make([]string, len(kustomizations))
	for i, kust := range kustomizations {
		kustomizationNames[i] = kust.Name
	}

	tui.Start(fmt.Sprintf("Waiting for %d destroy-only kustomization(s) to become ready", len(kustomizations)))

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

	if !allReady {
		tui.Fail()
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
	tui.Done()

	for _, kustomization := range orderForDestroy(kustomizations, "destroy-only") {
		tui.Start(fmt.Sprintf("Destroying destroy-only kustomization %s", kustomization.Name))

		if err := k.DeleteKustomization(kustomization.Name, namespace); err != nil {
			tui.Fail()
			errors = append(errors, fmt.Errorf("failed to delete destroy-only kustomization %s: %w", kustomization.Name, err))
		} else {
			tui.Done()
		}
	}

	return errors
}

// =============================================================================
// Private Methods
// =============================================================================

// gitopsNamespace returns the configured gitops namespace, defaulting to DefaultGitopsNamespace.
func (k *BaseKubernetesManager) gitopsNamespace() string {
	return k.configHandler.GetString("gitops.namespace", constants.DefaultGitopsNamespace)
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

// gitopsMode returns the configured gitops mode, defaulting to pull. Centralising
// the "gitops.mode" config read here keeps the several call sites below in sync:
// Kustomization intervals (ApplyBlueprint, deleteKustomizationWithCleanup,
// processDestroyOnlyKustomizations) and Source intervals (applyBlueprintGit/OCI
// Repository) must all read the same value; having one accessor makes that a
// single point of change if the config key ever moves or gains validation.
func (k *BaseKubernetesManager) gitopsMode() constants.GitopsMode {
	return constants.ParseGitopsMode(k.configHandler.GetString("gitops.mode", ""))
}

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
	sourceUrl := runtimegit.NormalizeRemoteURL(source.Url)
	mode := k.gitopsMode()

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
				Duration: constants.FluxSourceInterval(mode),
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
	mode := k.gitopsMode()
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
				Duration: constants.FluxSourceInterval(mode),
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

// orderForDestroy returns the input slice ordered for destroy: reverse-topological
// when reverseTopologicalKustomizations succeeds, falling back to reverse-array
// order on cycle-detection error (with a stderr warning). Cycles are normally
// rejected by blueprint validation; the fallback is defensive so a malformed
// blueprint can still be torn down. The label is interpolated into the warning
// message to distinguish destroy contexts (e.g. "destroy" vs "destroy-only").
func orderForDestroy(ks []blueprintv1alpha1.Kustomization, label string) []blueprintv1alpha1.Kustomization {
	ordered, err := reverseTopologicalKustomizations(ks)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not compute reverse-topological %s order (%v); falling back to reverse-array order\n", label, err)
		ordered = make([]blueprintv1alpha1.Kustomization, len(ks))
		for i, kustomization := range ks {
			ordered[len(ks)-1-i] = kustomization
		}
	}
	return ordered
}

// reverseTopologicalKustomizations returns the input slice reordered so that
// each kustomization appears before any kustomization it depends on — destroy
// order. Apply order places dependencies first and dependents last; destroy
// order is the reverse, so consumers prune before their providers do. By the
// time a dependency's kustomization is deleted, every kustomization that
// referenced its outputs is already gone, which lets controllers shut down
// without orphaning anyone.
//
// Determinism: independent kustomizations (no dependency relation) appear in
// destroy-output in the reverse of their input-slice order. A blueprint
// already authored or sorted in topological order therefore produces the
// same destroy walk as a naive slice-reverse, making this helper a strict
// superset of the slice-reverse loops it replaces.
//
// Missing dependencies — a name in DependsOn that does not match any
// kustomization in the input — are silently treated as no-edge. The destroy
// walk does not block on something it does not own; this matches the
// semantics of the apply-side dependency walk.
//
// Returns an error if a dependency cycle is detected. Cycles are normally
// rejected at blueprint validation; the check here is defensive so a destroy
// walk over a malformed blueprint cannot recurse infinitely.
func reverseTopologicalKustomizations(ks []blueprintv1alpha1.Kustomization) ([]blueprintv1alpha1.Kustomization, error) {
	if len(ks) == 0 {
		return []blueprintv1alpha1.Kustomization{}, nil
	}
	if len(ks) == 1 {
		out := make([]blueprintv1alpha1.Kustomization, 1)
		out[0] = ks[0]
		return out, nil
	}

	nameToIndex := make(map[string]int, len(ks))
	for i := range ks {
		nameToIndex[ks[i].Name] = i
	}

	forward := make([]int, 0, len(ks))
	visited := make(map[int]bool, len(ks))
	visiting := make(map[int]bool, len(ks))

	var visit func(idx int) error
	visit = func(idx int) error {
		if visiting[idx] {
			return fmt.Errorf("dependency cycle detected involving kustomization %q", ks[idx].Name)
		}
		if visited[idx] {
			return nil
		}
		visiting[idx] = true
		for _, dep := range ks[idx].DependsOn {
			depIdx, ok := nameToIndex[dep]
			if !ok {
				continue
			}
			if err := visit(depIdx); err != nil {
				return err
			}
		}
		visiting[idx] = false
		visited[idx] = true
		forward = append(forward, idx)
		return nil
	}

	for i := range ks {
		if !visited[i] {
			if err := visit(i); err != nil {
				return nil, err
			}
		}
	}

	out := make([]blueprintv1alpha1.Kustomization, len(ks))
	for i, idx := range forward {
		out[len(forward)-1-i] = ks[idx]
	}
	return out, nil
}
