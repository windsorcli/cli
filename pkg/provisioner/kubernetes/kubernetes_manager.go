// The KubernetesManager is the Windsor interface to Flux and Kubernetes resources in a cluster.
// It provides apply, wait, delete, and prune of blueprints, Kustomizations, and their sources.
// It applies resources with server-side apply and labels them with the context that owns them.
// The other kubernetes_manager files hold its methods, grouped by concern.

package kubernetes

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"github.com/windsorcli/cli/pkg/runtime/config"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// =============================================================================
// Interfaces
// =============================================================================

// KustomizationReporter receives the per-Kustomization steps of a readiness wait.
type KustomizationReporter interface {
	Start(name string)
	Progress(name, text string)
	Done(name string)
	Fail(name string, err error)
}

// KubernetesManager defines methods for Kubernetes resource management
type KubernetesManager interface {
	ApplyKustomization(kustomization kustomizev1.Kustomization) error
	DeleteKustomization(ctx context.Context, name, namespace string) error
	WaitForKustomizations(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, layers KustomizationReporter) error
	CreateNamespace(name string) error
	DeleteNamespace(name string) error
	ApplyConfigMap(name, namespace string, data map[string]string) error
	ApplySecret(name, namespace string, stringData map[string]string, owner string) error
	PruneSecrets(desired map[string]map[string]bool) error
	RollWorkloadsForSecret(ctx context.Context, namespace, secretName, digest string) error
	GetHelmReleasesForKustomization(name, namespace string) ([]helmv2.HelmRelease, error)
	ApplyGitRepository(repo *sourcev1.GitRepository) error
	ApplyOCIRepository(repo *sourcev1.OCIRepository) error
	CheckGitRepositoryStatus() error
	GetKustomizationStatus(names []string) (map[string]bool, error)
	GetKustomizationReadiness(names []string) (map[string]bool, error)
	KustomizationExists(name, namespace string) (bool, error)
	NamespaceExists(name string) (bool, error)
	GetKustomizationInventory(name, namespace string) ([]InventoryEntry, error)
	WaitForKubernetesHealthy(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error
	GetNodeReadyStatus(ctx context.Context, nodeNames []string) (map[string]bool, error)
	ApplyBlueprint(blueprint *blueprintv1alpha1.Blueprint, namespace string) error
	DeleteBlueprint(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, namespace string) error
	PruneBlueprint(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, namespace string) error
	ListPrunableKustomizations(blueprint *blueprintv1alpha1.Blueprint, namespace string) ([]string, error)
	ApplyVersionMarker(namespace string, marker VersionMarker) error
	GetVersionMarker(namespace string) (VersionMarker, bool, error)
}

// InventoryEntry identifies one resource Flux is tracking for a Kustomization,
// decoded from a single .status.inventory.entries[] record. The encoded form
// flux writes is "<namespace>_<name>_<group>_<kind>"; namespace is empty for
// cluster-scoped resources, group is empty for core API objects ("v1"). These
// entries are exactly what flux deletes when a Kustomization is removed, so
// they are the truthful source for "what will go away on destroy."
type InventoryEntry struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
}

// =============================================================================
// Constructor
// =============================================================================

// BaseKubernetesManager implements KubernetesManager interface
type BaseKubernetesManager struct {
	shims         *Shims
	client        client.KubernetesClient
	configHandler config.ConfigHandler

	kustomizationWaitPollInterval        time.Duration
	kustomizationWaitMaxReadFailures     int
	kustomizationWaitMinErrorDuration    time.Duration
	kustomizationReconcileTimeout        time.Duration
	kustomizationReconcileSleep          time.Duration
	kustomizationDeletionPerEntryTimeout time.Duration
	kustomizationDeletionMaxExtraTimeout time.Duration
	kustomizationSpecTimeoutCeiling      time.Duration
	kustomizationDeleteTimeoutCeiling    time.Duration
	kustomizationAbandonedGraceMaxExtra  time.Duration

	loadBalancerTeardownTimeout time.Duration

	notReadyDescribeBudget time.Duration

	healthCheckPollInterval   time.Duration
	healthCheckSettleDuration time.Duration
	nodeReadyPollInterval     time.Duration
}

// NewKubernetesManager returns a BaseKubernetesManager with the default timeouts. It panics on a nil argument.
// The configHandler supplies the context name and id for the ownership labels.
func NewKubernetesManager(kubernetesClient client.KubernetesClient, configHandler config.ConfigHandler) *BaseKubernetesManager {
	if kubernetesClient == nil {
		panic("kubernetes client is required")
	}
	if configHandler == nil {
		panic("config handler is required")
	}

	manager := &BaseKubernetesManager{
		client:                               kubernetesClient,
		configHandler:                        configHandler,
		shims:                                NewShims(),
		kustomizationWaitPollInterval:        constants.DefaultKustomizationWaitPollInterval,
		kustomizationWaitMaxReadFailures:     constants.DefaultKustomizationWaitMaxFailures,
		kustomizationWaitMinErrorDuration:    30 * time.Second,
		kustomizationReconcileTimeout:        5 * time.Minute,
		kustomizationReconcileSleep:          2 * time.Second,
		kustomizationDeletionPerEntryTimeout: 3 * time.Second,
		kustomizationDeletionMaxExtraTimeout: 20 * time.Minute,
		kustomizationSpecTimeoutCeiling:      2 * time.Hour,
		kustomizationDeleteTimeoutCeiling:    6 * time.Hour,
		kustomizationAbandonedGraceMaxExtra:  3 * time.Minute,
		loadBalancerTeardownTimeout:          constants.DefaultLoadBalancerTeardownTimeout,
		notReadyDescribeBudget:               10 * time.Second,
		healthCheckPollInterval:              10 * time.Second,
		healthCheckSettleDuration:            30 * time.Second,
		nodeReadyPollInterval:                5 * time.Second,
	}

	return manager
}

// =============================================================================
// Public Methods
// =============================================================================

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

// NamespaceExists reports whether the named namespace exists in the cluster. Namespaces are cluster-scoped,
// so the lookup passes an empty namespace to GetResource. A NotFound is reported as (false, nil); any other
// API error propagates.
func (k *BaseKubernetesManager) NamespaceExists(name string) (bool, error) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	_, err := k.client.GetResource(gvr, "", name)
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// gitopsNamespace returns the configured gitops namespace, defaulting to DefaultGitopsNamespace.
func (k *BaseKubernetesManager) gitopsNamespace() string {
	return k.configHandler.GetString("gitops.namespace", constants.DefaultGitopsNamespace)
}

// ownershipLabels returns the Windsor context labels stamped on each Kustomization object (so the
// objects are selectable by context) and propagated to its managed resources via CommonMetadata.
func (k *BaseKubernetesManager) ownershipLabels() map[string]string {
	return map[string]string{
		"windsorcli.dev/context":    k.configHandler.GetContext(),
		"windsorcli.dev/context-id": k.configHandler.GetString("id"),
	}
}

// applyWithRetry applies a resource using SSA with minimal logic
func (k *BaseKubernetesManager) applyWithRetry(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) error {
	existing, err := k.client.GetResource(gvr, obj.GetNamespace(), obj.GetName())
	if err == nil {
		applyConfig, err := k.shims.ToUnstructured(existing)
		if err != nil {
			return werror.Wrap(err, "failed to convert existing object to unstructured")
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

// resolveScopedGVR resolves gvk to its GroupVersionResource and correct namespace scope
// via discovery. namespace is the caller's default scope, cleared when the resolved kind
// is cluster-scoped. ok is false when the API type no longer exists.
func (k *BaseKubernetesManager) resolveScopedGVR(gvk schema.GroupVersionKind, namespace string) (gvr schema.GroupVersionResource, scopedNamespace string, ok bool, err error) {
	gvr, err = k.client.ResourceFor(gvk)
	if err != nil {
		if apimeta.IsNoMatchError(err) {
			return schema.GroupVersionResource{}, "", false, nil
		}
		return schema.GroupVersionResource{}, "", false, err
	}
	namespaced, err := k.client.IsNamespaced(gvk)
	if err != nil {
		if apimeta.IsNoMatchError(err) {
			return schema.GroupVersionResource{}, "", false, nil
		}
		return schema.GroupVersionResource{}, "", false, err
	}
	if !namespaced {
		namespace = ""
	}
	return gvr, namespace, true, nil
}

// gitopsMode returns the configured gitops mode, defaulting to pull. Centralising
// the "gitops.mode" config read here keeps the several call sites below in sync:
// Kustomization intervals (ApplyBlueprint, deleteKustomizationWithCleanup,
// processDestroyOnlyKustomizations) and Source intervals (applyBlueprintGit/OCI
// Repository) must all read the same value; having one accessor makes that a
// single point of change if the config key ever moves or gains validation.
func (k *BaseKubernetesManager) gitopsMode() constants.GitopsMode {
	return constants.ParseGitopsMode(k.configHandler.GetString("gitops.mode", ""))
}

// =============================================================================
// Helpers
// =============================================================================

// namespaceFlag formats a kubectl -n flag for a diagnostic hint. It returns an empty string for a
// cluster-scoped resource, so a GatewayClass or other cluster-scoped root never gets -n "".
func namespaceFlag(namespace string) string {
	if namespace == "" {
		return ""
	}
	return fmt.Sprintf(" -n %s", namespace)
}

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

	if obj.GetKind() == "Secret" {
		return nil
	}

	if _, ok := obj.Object["spec"]; !ok {
		return fmt.Errorf("spec is required")
	}

	return nil
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
