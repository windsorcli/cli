// Package kubernetes provides Kubernetes resource management functionality
// It implements server-side apply patterns for managing Kubernetes resources
// and provides a clean interface for kustomization and resource management

package kubernetes

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"strings"
	"sync"
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
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
)

// =============================================================================
// Interfaces
// =============================================================================

// KubernetesManager defines methods for Kubernetes resource management
type KubernetesManager interface {
	ApplyKustomization(kustomization kustomizev1.Kustomization) error
	DeleteKustomization(name, namespace string) error
	WaitForKustomizations(ctx context.Context, message string, blueprint *blueprintv1alpha1.Blueprint) error
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
	DeleteBlueprint(blueprint *blueprintv1alpha1.Blueprint, namespace string) error
	PruneBlueprint(blueprint *blueprintv1alpha1.Blueprint, namespace string) error
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
	kustomizationWaitMinErrorDuration    time.Duration
	kustomizationReconcileTimeout        time.Duration
	kustomizationReconcileSleep          time.Duration
	kustomizationDeletionPerEntryTimeout time.Duration
	kustomizationDeletionMaxExtraTimeout time.Duration
	kustomizationSpecTimeoutCeiling      time.Duration

	notReadyDescribeBudget time.Duration

	healthCheckPollInterval   time.Duration
	healthCheckSettleDuration time.Duration
	nodeReadyPollInterval     time.Duration
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
		client:                               kubernetesClient,
		configHandler:                        configHandler,
		shims:                                NewShims(),
		kustomizationWaitPollInterval:        2 * time.Second,
		kustomizationWaitMinErrorDuration:    30 * time.Second,
		kustomizationReconcileTimeout:        5 * time.Minute,
		kustomizationReconcileSleep:          2 * time.Second,
		kustomizationDeletionPerEntryTimeout: 3 * time.Second,
		kustomizationDeletionMaxExtraTimeout: 20 * time.Minute,
		kustomizationSpecTimeoutCeiling:      2 * time.Hour,
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

// DeleteKustomization removes a Kustomization using background deletion and waits for
// it to disappear. The wait floor rises to the Kustomization's own spec.timeout when
// it declares one larger than the default, capped at kustomizationSpecTimeoutCeiling.
// A slow-to-delete resource (e.g. a Crossplane-managed database) often sets a long
// install timeout; deletion deserves the same allowance. The wait also scales with
// inventory size for CRD-heavy layers. On timeout it returns an error instead of
// stripping finalizers, which would orphan inventory items and let terraform destroy
// the cluster while their cloud resources leak. The error asserts a stuck finalizer
// only when a status condition confirms one; otherwise it reports the timeout as
// unconfirmed.
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

	start := k.shims.TimeNow()
	waitFor := k.kustomizationReconcileTimeout
	var lastObj *unstructured.Unstructured
	for k.shims.TimeNow().Before(start.Add(waitFor)) {
		obj, err := k.client.GetResource(gvr, namespace, name)
		if err != nil && isNotFoundError(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("error checking kustomization deletion status: %w", err)
		}
		lastObj = obj

		if size := inventorySize(obj); size > 0 {
			waitFor = extendWaitFor(waitFor, k.kustomizationDeletionTimeout(size))
		}
		if specTO, ok := specTimeout(obj); ok {
			waitFor = extendWaitFor(waitFor, min(specTO, k.kustomizationSpecTimeoutCeiling))
		}

		k.shims.TimeSleep(k.kustomizationWaitPollInterval)
	}

	inspectCmd := fmt.Sprintf("`kubectl get kustomization %s -n %s -o yaml`", name, namespace)
	const terminatingCmd = "`kubectl get pvc,svc,ingress,certificate -A | grep Terminating`"

	reason := describeStuckKustomization(lastObj) + k.describeStuckHelmReleases(name, namespace)
	if reason == "" {
		return fmt.Errorf("timeout waiting for kustomization %s/%s to be deleted after %s; no status condition confirms a stuck finalizer, but that does not rule one out — check whether %s (status.inventory) is still shrinking before retrying; if it is not shrinking, find the stuck object with %s", namespace, name, waitFor, inspectCmd, terminatingCmd)
	}
	return fmt.Errorf("timeout waiting for kustomization %s/%s to be deleted%s; an inventory item is likely stuck on a cloud-controller finalizer — inspect with %s (status.conditions, status.inventory) and %s to find the stuck object", namespace, name, reason, inspectCmd, terminatingCmd)
}

// inventorySize counts a Kustomization's status.inventory.entries. It returns 0 for a
// nil object or a missing/malformed inventory.
func inventorySize(obj *unstructured.Unstructured) int {
	if obj == nil {
		return 0
	}
	entries, found, err := unstructured.NestedSlice(obj.Object, "status", "inventory", "entries")
	if err != nil || !found {
		return 0
	}
	return len(entries)
}

// specTimeout reads a Kustomization's own spec.timeout. Windsor writes this field from
// the blueprint's Kustomization.Timeout at apply time; see ToFluxKustomization. It
// returns false for a nil object or a missing or unparseable value.
func specTimeout(obj *unstructured.Unstructured) (time.Duration, bool) {
	if obj == nil {
		return 0, false
	}
	value, found, err := unstructured.NestedString(obj.Object, "spec", "timeout")
	if err != nil || !found {
		return 0, false
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, false
	}
	return d, true
}

// extendWaitFor raises waitFor to candidate when candidate is larger, otherwise
// returns waitFor unchanged.
func extendWaitFor(waitFor, candidate time.Duration) time.Duration {
	if candidate > waitFor {
		return candidate
	}
	return waitFor
}

// kustomizationDeletionTimeout scales DeleteKustomization's wait window by inventory
// size, capped at kustomizationDeletionMaxExtraTimeout so a corrupted or unusually
// large count cannot stall destroy indefinitely.
func (k *BaseKubernetesManager) kustomizationDeletionTimeout(entryCount int) time.Duration {
	const maxEntryCount = 100_000 // guards the Duration multiplication below against overflow
	if entryCount > maxEntryCount {
		entryCount = maxEntryCount
	}
	extra := time.Duration(entryCount) * k.kustomizationDeletionPerEntryTimeout
	if extra > k.kustomizationDeletionMaxExtraTimeout {
		extra = k.kustomizationDeletionMaxExtraTimeout
	}
	return k.kustomizationReconcileTimeout + extra
}

// describeStuckKustomization extracts the most diagnostic status condition from a
// Kustomization that failed to delete in time, formatted as " (Type=Status Reason: message)"
// for inline inclusion in the timeout error. Flux records the real failure cause
// (prune failure, healthcheck failure, dependency-not-ready) in status.conditions,
// so surfacing it here saves the operator a manual kubectl round-trip. Preference
// order is Stalled, then non-True Ready, then any other non-True condition — these
// carry the actionable reason; a bare Ready=True (rare during a stuck delete) is not
// reported. Returns an empty string when no object was captured or no condition
// carries a usable message, so the caller's sentence reads cleanly without it.
func describeStuckKustomization(obj *unstructured.Unstructured) string {
	if obj == nil {
		return ""
	}
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return ""
	}
	var ready, stalled, other map[string]any
	for _, c := range conditions {
		cond, ok := c.(map[string]any)
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		condStatus, _ := cond["status"].(string)
		switch {
		case condType == "Stalled" && condStatus == "True":
			stalled = cond
		case condType == "Ready" && condStatus != "True":
			ready = cond
		case condStatus != "True" && other == nil:
			other = cond
		}
	}
	pick := stalled
	if pick == nil {
		pick = ready
	}
	if pick == nil {
		pick = other
	}
	if pick == nil {
		return ""
	}
	condType, _ := pick["type"].(string)
	condStatus, _ := pick["status"].(string)
	reason, _ := pick["reason"].(string)
	message, _ := pick["message"].(string)
	message = strings.ReplaceAll(strings.TrimSpace(message), "\n", " ")
	if message == "" {
		return ""
	}
	if reason != "" {
		return fmt.Sprintf(" (%s=%s %s: %s)", condType, condStatus, reason, message)
	}
	return fmt.Sprintf(" (%s=%s: %s)", condType, condStatus, message)
}

// describeNotReadyKustomizations fetches the current state of each named
// kustomization and returns a ": name (condition), ..." suffix naming those not
// yet Ready, each annotated with its most diagnostic status condition. It is the
// wait-timeout counterpart to the destroy-timeout enrichment: rather than a bare
// "timeout waiting for kustomizations", the operator sees which ones are stuck
// and why (e.g. a failed reconciliation or unmet dependency) without a manual
// kubectl round-trip. A kustomization that cannot be read is listed by name
// alone. Returns an empty string when every kustomization reads back Ready, so
// the caller's sentence stays clean.
//
// Because this runs only after a wait already timed out, the API may be slow or
// unreachable. Each GetResource is individually bounded by the client's request
// timeout, but probing every kustomization serially could still compound into
// minutes; total probing is therefore capped at notReadyDescribeBudget. Once the
// budget is spent the remaining kustomizations are named without condition detail.
func (k *BaseKubernetesManager) describeNotReadyKustomizations(names []string, namespace string) string {
	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}
	start := time.Now()
	var stuck []string
	for _, name := range names {
		if time.Since(start) >= k.notReadyDescribeBudget {
			stuck = append(stuck, name)
			continue
		}
		obj, err := k.client.GetResource(gvr, namespace, name)
		if err != nil {
			stuck = append(stuck, name)
			continue
		}
		if kustomizationReady(obj) {
			continue
		}
		stuck = append(stuck, name+describeStuckKustomization(obj))
	}
	if len(stuck) == 0 {
		return ""
	}
	return ": " + strings.Join(stuck, ", ")
}

// kustomizationReady reports whether a Kustomization's status carries a
// Ready=True condition. A missing status or conditions slice reads as not ready.
func kustomizationReady(obj *unstructured.Unstructured) bool {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}
	for _, c := range conditions {
		cond, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if cond["type"] == "Ready" && cond["status"] == "True" {
			return true
		}
	}
	return false
}

// kustomizationWaitErrorBudgetFraction bounds how much of the total wait timeout a streak of
// transient GetResource errors may consume before WaitForKustomizations gives up: a sustained
// apiserver slowdown gets patience proportional to how patient the overall wait already is,
// rather than a fixed tick count that's either too tight for a big blueprint or too loose for
// a small one.
const kustomizationWaitErrorBudgetFraction = 0.25

// kustomizationCheckConcurrencyLimit caps how many GetResource calls WaitForKustomizations
// issues at once within a single tick, so a blueprint with a large number of kustomizations
// doesn't burst the apiserver with one request per kustomization on every poll interval.
const kustomizationCheckConcurrencyLimit = 10

// WaitForKustomizations waits for kustomizations to be ready, calculating the timeout from the
// longest dependency chain in the blueprint. It honors ctx: a cancelled or deadline-exceeded
// context ends the wait immediately and returns ctx.Err(), so a parent SIGTERM/Ctrl+C or command
// deadline can interrupt it cleanly.
//
// A not-found GetResource error is treated as not-ready-yet. Any other GetResource error (auth,
// RBAC, TLS, connection, apiserver timeout) is tolerated for up to
// kustomizationWaitErrorBudgetFraction of the total timeout (floored at
// kustomizationWaitMinErrorDuration), and the streak resets on any clean tick. These errors are
// deliberately not classified as permanent vs. transient — matching kustomize-controller and
// helm-controller, which reserve their no-retry path for a static error in the object's own
// spec, never a raw API error.
//
// A Kustomization's own Ready=False condition is different: a Reason in
// kustomizationTerminalReasons (BuildFailed, ArtifactFailed, ReconciliationFailed) ends the wait
// immediately, since kustomize-controller repeats the same reason on every retry. Any other
// Reason (Progressing, DependencyNotReady, or none yet) still counts as not-ready and keeps
// polling.
//
// Each pending kustomization is checked concurrently within a tick, up to
// kustomizationCheckConcurrencyLimit, so one slow or erroring check doesn't block the rest. Once
// observed Ready, a kustomization is dropped from polling, so a later reconcile flipping it back
// doesn't reset the wait. The tracked name set is deduplicated, since withCrdLayer can prepend a
// CRD kustomization whose name collides with one declared elsewhere.
func (k *BaseKubernetesManager) WaitForKustomizations(ctx context.Context, message string, blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	timeout := k.calculateTotalWaitTime(blueprint)
	kustomizationNames := make([]string, 0, len(blueprint.Kustomizations))
	seenNames := make(map[string]bool, len(blueprint.Kustomizations))
	for _, kustomization := range blueprint.Kustomizations {
		if kustomization.DestroyOnly != nil && *kustomization.DestroyOnly {
			continue
		}
		if seenNames[kustomization.Name] {
			continue
		}
		seenNames[kustomization.Name] = true
		kustomizationNames = append(kustomizationNames, kustomization.Name)
	}

	maxErrorDuration := time.Duration(float64(timeout) * kustomizationWaitErrorBudgetFraction)
	if maxErrorDuration < k.kustomizationWaitMinErrorDuration {
		maxErrorDuration = k.kustomizationWaitMinErrorDuration
	}

	tui.Start(message)

	timeoutChan := time.After(timeout)
	ticker := time.NewTicker(k.kustomizationWaitPollInterval)
	defer ticker.Stop()

	var errorStreakStart time.Time
	readyKustomizations := make(map[string]bool, len(kustomizationNames))

	for {
		select {
		case <-ctx.Done():
			tui.Pause()
			return ctx.Err()
		case <-timeoutChan:
			tui.Fail()
			return fmt.Errorf("timeout waiting for kustomizations%s", k.describeNotReadyKustomizations(kustomizationNames, k.gitopsNamespace()))
		case <-ticker.C:
			pending := make([]string, 0, len(kustomizationNames))
			seen := make(map[string]bool, len(kustomizationNames))
			for _, name := range kustomizationNames {
				if readyKustomizations[name] || seen[name] {
					continue
				}
				seen[name] = true
				pending = append(pending, name)
			}

			results := make([]kustomizationCheckResult, len(pending))
			var wg sync.WaitGroup
			sem := make(chan struct{}, kustomizationCheckConcurrencyLimit)
			for i, name := range pending {
				wg.Add(1)
				sem <- struct{}{}
				go func(i int, name string) {
					defer wg.Done()
					defer func() { <-sem }()
					ready, err := k.checkKustomizationReady(name)
					results[i] = kustomizationCheckResult{ready: ready, err: err}
				}(i, name)
			}
			wg.Wait()

			var tickErr error
			for i, name := range pending {
				result := results[i]
				if result.err != nil {
					var failed *kustomizationFailedError
					if errors.As(result.err, &failed) {
						tui.Fail()
						return fmt.Errorf("kustomization will not become ready: %w", failed)
					}
					if tickErr == nil {
						tickErr = fmt.Errorf("error checking kustomization %s: %w", name, result.err)
					}
					continue
				}
				if result.ready {
					readyKustomizations[name] = true
				}
			}
			if tickErr != nil {
				if errorStreakStart.IsZero() {
					errorStreakStart = time.Now()
				}
				if time.Since(errorStreakStart) >= maxErrorDuration {
					tui.Fail()
					return fmt.Errorf("kustomization readiness checks failing for over %s: %w", maxErrorDuration, tickErr)
				}
				continue
			}
			errorStreakStart = time.Time{}
			if len(readyKustomizations) == len(kustomizationNames) {
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

// ApplySecret creates or updates a Secret using SSA. Values are supplied as plaintext in
// stringData (write-only; the API server folds them into data) and are never logged. Mirrors
// ApplyConfigMap's server-side-apply handling, including its immutable-field guard: Kubernetes
// rejects an update that changes Secret.type, so if an existing Secret's type differs from the
// newly resolved one, ApplySecret deletes it first rather than SSA-merging a rejected change. It
// stamps the context ownership labels plus a secret-owner label naming the kustomization the
// secret belongs to; that label is set only by CLI placement (never by Flux), so PruneSecrets can
// find and reclaim CLI-placed secrets without ever touching a Flux-managed one. The Secret's type
// and stringData are resolved by secretTypeAndData: stringData already carrying
// ".dockerconfigjson", or carrying docker-username/docker-password (docker-server optional),
// produces a kubernetes.io/dockerconfigjson Secret for imagePullSecrets; anything else stays Opaque.
func (k *BaseKubernetesManager) ApplySecret(name, namespace string, stringData map[string]string, owner string) error {
	secretType, resolvedData, err := secretTypeAndData(stringData)
	if err != nil {
		return fmt.Errorf("failed to resolve secret type for %q: %w", name, err)
	}

	labels := k.ownershipLabels()
	labels[secretOwnerLabel] = owner
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"type":       secretType,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"stringData": resolvedData,
		},
	}
	obj.SetLabels(labels)

	if err := validateFields(obj); err != nil {
		return fmt.Errorf("invalid secret fields: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "secrets",
	}

	existing, err := k.client.GetResource(gvr, namespace, name)
	if err == nil && secretTypeChanged(existing, secretType) {
		if err := k.client.DeleteResource(gvr, namespace, name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete secret with changed type: %w", err)
		}
		time.Sleep(time.Second)
	}

	opts := metav1.ApplyOptions{
		FieldManager: "windsor-cli",
		Force:        false,
	}

	return k.applyWithRetry(gvr, obj, opts)
}

// PruneSecrets deletes the CLI-placed secrets for this context that the latest placement no longer
// wants, reconciling the cluster to the desired set. desired maps a namespace to the set of secret
// names just placed there; a secret whose (namespace, name) is absent is deleted. It lists only secrets
// bearing this context's id and the secret-owner marker — a label set solely by ApplySecret, never by
// Flux — so it reclaims exactly what the CLI placed (a secret dropped from a fan-out list, a secret
// removed from a system, or every CLI secret when desired is empty) and never a Flux-managed secret
// that merely inherited the context labels via CommonMetadata. It fails closed when the context id is
// unset or is not a valid label value, since without a well-formed id pruning cannot be scoped to this
// context — a malformed id would otherwise build a selector the API server rejects, failing every
// placement with an opaque error rather than a message that names the bad id.
func (k *BaseKubernetesManager) PruneSecrets(desired map[string]map[string]bool) error {
	contextID := k.configHandler.GetString("id")
	if contextID == "" {
		return fmt.Errorf("context id not set; cannot scope secret pruning to this context")
	}
	if errs := validation.IsValidLabelValue(contextID); len(errs) > 0 {
		return fmt.Errorf("context id %q is not a valid label value, cannot scope secret pruning: %s", contextID, strings.Join(errs, "; "))
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "secrets",
	}

	selector := fmt.Sprintf("windsorcli.dev/context-id=%s,%s", contextID, secretOwnerLabel)
	list, err := k.client.ListResourcesByLabel(gvr, "", selector)
	if err != nil {
		return fmt.Errorf("failed to list CLI-placed secrets: %w", err)
	}

	for i := range list.Items {
		item := list.Items[i]
		namespace := item.GetNamespace()
		if desired[namespace][item.GetName()] {
			continue
		}
		if err := k.client.DeleteResource(gvr, namespace, item.GetName(), metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete orphaned secret %q in namespace %q: %w", item.GetName(), namespace, err)
		}
	}
	return nil
}

// secretChecksumAnnotationPrefix prefixes the pod-template annotation whose value is a content digest
// of a secret the workload consumes. The suffix is the secret name, so each consumed secret gets its
// own annotation and a change to one rolls only its consumers.
const secretChecksumAnnotationPrefix = "checksum.windsorcli.dev/"

// secretChecksumAnnotationKey builds the pod-template annotation key for a secret's content digest. The
// name segment after the prefix must be at most 63 characters, but Secret names may be longer; for an
// over-long name it substitutes a deterministic, collision-resistant segment (the truncated name plus a
// short hash of the full name) so the roll still works and the key stays valid rather than failing with
// an opaque API validation error.
func secretChecksumAnnotationKey(secretName string) string {
	const maxNameSegment = 63
	segment := secretName
	if len(segment) > maxNameSegment {
		sum := sha256.Sum256([]byte(secretName))
		segment = secretName[:maxNameSegment-9] + "-" + hex.EncodeToString(sum[:])[:8]
	}
	return secretChecksumAnnotationPrefix + segment
}

// RollWorkloadsForSecret rolls the workloads in a namespace that consume the named Secret so they pick
// up new content, the way Kubernetes only ever rolls on a pod-template change. Because the CLI holds the
// resolved plaintext, it passes a precomputed content digest and stamps it as a pod-template annotation
// (checksum.windsorcli.dev/<secret>) on every Deployment, StatefulSet, and DaemonSet whose pod spec
// references the Secret via envFrom, a secretKeyRef, or a secret volume — including init containers. The
// digest is one-way and never surfaced beyond the namespace, so a reader of the annotation learns
// nothing the Secret's own RBAC did not already grant. A workload already carrying the digest is left
// untouched (idempotent, so unchanged content does not churn pods); a workload that does not reference
// the Secret is never patched. Finding no consumers is not an error — on a first apply the workload is
// created later by Flux and reads the Secret fresh — so this returns an error only on an API failure. The
// caller's context bounds the patch calls so a slow API server cannot outlast its deadline.
func (k *BaseKubernetesManager) RollWorkloadsForSecret(ctx context.Context, namespace, secretName, digest string) error {
	annotationKey := secretChecksumAnnotationKey(secretName)
	for _, resource := range []string{"deployments", "statefulsets", "daemonsets"} {
		gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: resource}
		list, err := k.client.ListResources(gvr, namespace)
		if err != nil {
			return fmt.Errorf("listing %s in namespace %q: %w", resource, namespace, err)
		}
		if list == nil {
			continue
		}
		for i := range list.Items {
			obj := &list.Items[i]
			podSpec, found, err := unstructured.NestedMap(obj.Object, "spec", "template", "spec")
			if err != nil || !found || !podSpecReferencesSecret(podSpec, secretName) {
				continue
			}
			current, _, _ := unstructured.NestedString(obj.Object, "spec", "template", "metadata", "annotations", annotationKey)
			if current == digest {
				continue
			}
			patch, err := json.Marshal(map[string]any{
				"spec": map[string]any{
					"template": map[string]any{
						"metadata": map[string]any{
							"annotations": map[string]any{annotationKey: digest},
						},
					},
				},
			})
			if err != nil {
				return fmt.Errorf("building rollout patch for %s %q: %w", resource, obj.GetName(), err)
			}
			opts := metav1.PatchOptions{FieldManager: "windsor-cli"}
			if _, err := k.client.PatchResource(ctx, gvr, namespace, obj.GetName(), types.MergePatchType, patch, opts); err != nil {
				return fmt.Errorf("patching %s %q in namespace %q: %w", resource, obj.GetName(), namespace, err)
			}
		}
	}
	return nil
}

// podSpecReferencesSecret reports whether an unstructured pod spec consumes the named Secret through any
// path that requires a restart to pick up new content: an envFrom secretRef, an env secretKeyRef, or a
// secret volume, across both containers and init containers.
func podSpecReferencesSecret(podSpec map[string]any, secretName string) bool {
	if volumes, ok := podSpec["volumes"].([]any); ok {
		for _, v := range volumes {
			vm, _ := v.(map[string]any)
			if s, ok := vm["secret"].(map[string]any); ok {
				if name, _ := s["secretName"].(string); name == secretName {
					return true
				}
			}
		}
	}
	for _, field := range []string{"containers", "initContainers"} {
		containers, ok := podSpec[field].([]any)
		if !ok {
			continue
		}
		for _, c := range containers {
			cm, _ := c.(map[string]any)
			if envFrom, ok := cm["envFrom"].([]any); ok {
				for _, e := range envFrom {
					em, _ := e.(map[string]any)
					if sr, ok := em["secretRef"].(map[string]any); ok {
						if name, _ := sr["name"].(string); name == secretName {
							return true
						}
					}
				}
			}
			if env, ok := cm["env"].([]any); ok {
				for _, e := range env {
					em, _ := e.(map[string]any)
					vf, ok := em["valueFrom"].(map[string]any)
					if !ok {
						continue
					}
					if skr, ok := vf["secretKeyRef"].(map[string]any); ok {
						if name, _ := skr["name"].(string); name == secretName {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// ApplyVersionMarker writes the applied-version marker ConfigMap to the namespace, recording which
// blueprint version the context is running. The marker is stored as JSON in a single ConfigMap so
// its encoding can evolve without churning Kustomization labels.
func (k *BaseKubernetesManager) ApplyVersionMarker(namespace string, marker VersionMarker) error {
	data, err := marker.ToConfigMapData()
	if err != nil {
		return fmt.Errorf("failed to encode version marker: %w", err)
	}
	return k.ApplyConfigMap(VersionMarkerConfigMapName, namespace, data)
}

// GetVersionMarker reads the applied-version marker ConfigMap from the namespace, reporting false
// when no marker is present — a missing ConfigMap (pre-bootstrap context) or one without marker data
// (legacy cluster). It returns an error only on a real read or decode failure, so callers can tell
// "no marker yet" (proceed as legacy) apart from "could not read the marker" (cluster unreachable).
func (k *BaseKubernetesManager) GetVersionMarker(namespace string) (VersionMarker, bool, error) {
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}
	obj, err := k.client.GetResource(gvr, namespace, VersionMarkerConfigMapName)
	if err != nil {
		if isNotFoundError(err) {
			return VersionMarker{}, false, nil
		}
		return VersionMarker{}, false, fmt.Errorf("failed to read version marker: %w", err)
	}
	data, found, err := unstructured.NestedStringMap(obj.Object, "data")
	if err != nil {
		return VersionMarker{}, false, fmt.Errorf("failed to read version marker data: %w", err)
	}
	if !found {
		return VersionMarker{}, false, nil
	}
	return ParseVersionMarker(data)
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
// has a Ready condition with Status False and a Reason in kustomizationTerminalReasons, an error is
// returned with the failure message.
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
				} else if condition.Status == "False" && kustomizationTerminalReasons[condition.Reason] {
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

// GetKustomizationReadiness returns whether each named Kustomization currently reports Ready=True, in the
// gitops namespace. Unlike GetKustomizationStatus it never fails on a Kustomization in a failed state — a
// failed one is simply reported not-ready — so a convergence driver can keep nudging it toward Ready rather
// than aborting. Names absent from the cluster report false; only an API list error propagates.
func (k *BaseKubernetesManager) GetKustomizationReadiness(names []string) (map[string]bool, error) {
	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}

	objList, err := k.client.ListResources(gvr, k.gitopsNamespace())
	if err != nil {
		return nil, fmt.Errorf("failed to list kustomizations: %w", err)
	}

	ready := make(map[string]bool, len(names))
	for _, name := range names {
		ready[name] = false
	}
	for i := range objList.Items {
		obj := &objList.Items[i]
		if _, wanted := ready[obj.GetName()]; wanted {
			ready[obj.GetName()] = kustomizationReady(obj)
		}
	}
	return ready, nil
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

// GetKustomizationInventory returns the list of resources Flux is currently
// tracking for the named Kustomization, decoded from its
// .status.inventory.entries field. This is what flux uses to drive prune
// behavior, so it is the authoritative source for "what will be deleted when
// this Kustomization is removed." Returns (nil, nil) when the Kustomization
// itself is absent (a destroy-plan caller should treat that as "not deployed"
// rather than an error). Returns an empty slice when the Kustomization exists
// but has no inventory yet (e.g., suspended, or never reconciled). API errors
// reading the Kustomization or its inventory propagate. Individual entries
// that fail to decode (malformed IDs, unexpected field shapes) are silently
// dropped — flux always emits well-formed IDs, so this branch is rare in
// practice, and resilience matters more than completeness here: failing the
// whole destroy preview because of one corrupt entry would be worse than
// rendering a slightly truncated list.
func (k *BaseKubernetesManager) GetKustomizationInventory(name, namespace string) ([]InventoryEntry, error) {
	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}
	obj, err := k.client.GetResource(gvr, namespace, name)
	if err != nil {
		if isNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	rawEntries, found, err := unstructured.NestedSlice(obj.Object, "status", "inventory", "entries")
	if err != nil {
		return nil, fmt.Errorf("error reading inventory for kustomization %q in namespace %q: %w", name, namespace, err)
	}
	if !found {
		return []InventoryEntry{}, nil
	}
	entries := make([]InventoryEntry, 0, len(rawEntries))
	for _, raw := range rawEntries {
		entryMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := entryMap["id"].(string)
		entry, ok := decodeInventoryID(id)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// decodeInventoryID parses a flux inventory ID of the form
// "<namespace>_<name>_<group>_<kind>" into an InventoryEntry. Namespace is
// empty for cluster-scoped resources; group is empty for core API objects.
// Returns (zero, false) when the ID does not have exactly four underscore-
// separated fields — flux always emits four, so anything else is a malformed
// entry we should drop rather than misrender.
func decodeInventoryID(id string) (InventoryEntry, bool) {
	parts := strings.SplitN(id, "_", 4)
	if len(parts) != 4 {
		return InventoryEntry{}, false
	}
	return InventoryEntry{
		Namespace: parts[0],
		Name:      parts[1],
		Group:     parts[2],
		Kind:      parts[3],
	}, true
}

// WaitForKubernetesHealthy waits for the Kubernetes API to become healthy within the context deadline.
// If nodeNames are provided, verifies all specified nodes reach Ready state before returning.
// A machine config apply (e.g. Talos resource reservations touching the apiServer/controllerManager/
// scheduler static pods) is accepted synchronously but reconciled asynchronously, so the very next
// health check can still observe the pre-change apiserver and return healthy moments before it's
// recreated. To catch that race, a success only counts once the API (and, if requested, node
// readiness) has held continuously for healthCheckSettleDuration; any failure during that window
// resets the clock. healthCheckSettleDuration is bounded by the context's own remaining deadline
// (minus one poll interval of margin) so a caller with a short overall timeout — e.g. the 30s
// reachability check windsor destroy runs before invoking terraform — still has a reachable window
// in which to succeed, rather than the settle requirement alone guaranteeing a timeout regardless of
// cluster health. Returns an error if the API is unreachable or any specified nodes are not Ready
// within the deadline.
func (k *BaseKubernetesManager) WaitForKubernetesHealthy(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
	if k.client == nil {
		return fmt.Errorf("kubernetes client not initialized")
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = k.shims.TimeNow().Add(5 * time.Minute)
	}

	pollInterval := k.healthCheckPollInterval
	if pollInterval == 0 {
		pollInterval = 10 * time.Second
	}

	settleDuration := k.healthCheckSettleDuration
	if settleDuration == 0 {
		settleDuration = 30 * time.Second
	}
	if remaining := deadline.Sub(k.shims.TimeNow()) - pollInterval; settleDuration > remaining {
		if remaining < 0 {
			remaining = 0
		}
		settleDuration = remaining
	}

	var lastErr error
	var settleSince time.Time
	for k.shims.TimeNow().Before(deadline) {
		select {
		case <-ctx.Done():
			return healthyTimeoutError(lastErr)
		default:
			if err := k.client.CheckHealth(ctx, endpoint); err != nil {
				lastErr = fmt.Errorf("health check for API endpoint %s failed: %w", endpoint, err)
				settleSince = time.Time{}
				select {
				case <-ctx.Done():
					return healthyTimeoutError(lastErr)
				case <-time.After(pollInterval):
					continue
				}
			}

			if len(nodeNames) > 0 {
				if err := k.waitForNodesReady(ctx, nodeNames, outputFunc); err != nil {
					lastErr = err
					settleSince = time.Time{}
					select {
					case <-ctx.Done():
						return healthyTimeoutError(lastErr)
					case <-time.After(pollInterval):
						continue
					}
				}
			}

			if settleSince.IsZero() {
				settleSince = k.shims.TimeNow()
			}
			if k.shims.TimeNow().Sub(settleSince) < settleDuration {
				select {
				case <-ctx.Done():
					return healthyTimeoutError(lastErr)
				case <-time.After(pollInterval):
					continue
				}
			}

			return nil
		}
	}

	return healthyTimeoutError(lastErr)
}

// healthyTimeoutError builds the WaitForKubernetesHealthy timeout error. When a
// health-check or node-readiness failure was seen on the final attempt it is
// appended so the operator learns why the wait gave up (a failed API health
// check, or specific nodes that never reached Ready) rather than only that it
// did. Falls back to the bare message when no underlying cause was recorded.
func healthyTimeoutError(lastErr error) error {
	if lastErr == nil {
		return fmt.Errorf("timeout waiting for Kubernetes API to be healthy")
	}
	return fmt.Errorf("timeout waiting for Kubernetes API to be healthy: %w", lastErr)
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
// blueprint installation following the intended order. Context ownership labels are stamped on
// each Kustomization's ObjectMeta (so the objects are selectable by context) and propagated to
// managed resources via CommonMetadata, using context info from the config handler.
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
		if err := k.applyBlueprintSource(source, namespace, true); err != nil {
			return fmt.Errorf("failed to apply blueprint repository: %w", err)
		}
	}

	for _, source := range blueprint.Sources {
		if blueprintv1alpha1.IsLocalTemplateSource(source) {
			continue
		}
		if err := k.applyBlueprintSource(source, namespace, false); err != nil {
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

		fluxKustomization.Labels = k.ownershipLabels()
		fluxKustomization.Spec.CommonMetadata = &kustomizev1.CommonMetadata{
			Labels: k.ownershipLabels(),
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
//     K8s DELETE → controller's finalizer holds the object in etcd
//     → controller calls cloud API to release external state
//     → finalizer lifts → object NotFound
//     → WaitForTermination satisfied
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
//
//     Every abort path (suspend failure, load balancer remediation failure, or a
//     per-Kustomization resume/delete failure) runs abortDestroy first, which
//     un-suspends the full eligible set before returning. Without this, Kustomizations
//     suspended by the up-front suspend loop but not yet reached by the destroy walk
//     would stay suspended forever — Install/ApplyBlueprint never resets spec.suspend
//     on existing objects, so no subsequent bootstrap would self-heal them.
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
	for _, kustomization := range eligible {
		if err := k.setKustomizationSuspend(kustomization.Name, namespace, true); err != nil {
			return k.abortDestroy(eligible, namespace, fmt.Errorf("destroy aborted: failed to suspend kustomization %q: %w", kustomization.Name, err))
		}
	}

	if err := k.remediateLoadBalancerOwners(eligible, namespace); err != nil {
		return k.abortDestroy(eligible, namespace, fmt.Errorf("destroy aborted: %w", err))
	}

	for _, kustomization := range orderForDestroy(eligible, "destroy") {
		tui.Start(fmt.Sprintf("Destroying kustomization %s", kustomization.Name))
		if err := k.setKustomizationSuspend(kustomization.Name, namespace, false); err != nil {
			tui.Fail()
			return k.abortDestroy(eligible, namespace, fmt.Errorf("destroy aborted: failed to resume kustomization %q before delete: %w", kustomization.Name, err))
		}
		if err := k.DeleteKustomization(kustomization.Name, namespace); err != nil {
			tui.Fail()
			return k.abortDestroy(eligible, namespace, fmt.Errorf("destroy aborted: failed to delete kustomization %q: %w (further deletions skipped to avoid cascading orphans)", kustomization.Name, err))
		}
		tui.Done()
	}

	return nil
}

// abortDestroy un-suspends every eligible Kustomization before propagating cause, so a
// DeleteBlueprint failure never leaves objects suspended by the up-front suspend loop with
// nothing left to resume them — Install/ApplyBlueprint never resets spec.suspend on existing
// objects, so a leftover suspend from an aborted destroy would otherwise persist indefinitely
// across subsequent bootstrap runs. setKustomizationSuspend is a no-op against objects that are
// already unsuspended or gone, so calling it on the full eligible set is safe regardless of how
// far the destroy walk got. Un-suspend failures are joined with cause rather than swallowed.
func (k *BaseKubernetesManager) abortDestroy(eligible []blueprintv1alpha1.Kustomization, namespace string, cause error) error {
	errs := []error{cause}
	for _, kustomization := range eligible {
		if err := k.setKustomizationSuspend(kustomization.Name, namespace, false); err != nil {
			errs = append(errs, fmt.Errorf("failed to un-suspend kustomization %q during abort cleanup: %w", kustomization.Name, err))
		}
	}
	return errors.Join(errs...)
}

// PruneBlueprint deletes Kustomizations belonging to the current context that are no longer part of
// the blueprint. It scopes strictly to this context — only objects carrying the
// windsorcli.dev/context-id label for this context are considered, so kustomizations owned by other
// contexts (or by no Windsor context) are never touched. Every non-DestroyOnly kustomization in the
// blueprint is treated as desired; the live remainder is deleted in reverse-dependency order (read
// from each object's live spec.dependsOn) so dependents tear down before their dependencies, each
// honoring its own deletionPolicy. The caller passes the same prepared blueprint Install applied
// (CRD layers included) so the synthesized crds/crds-<source> layers are recognized as desired and
// not pruned. Deletion errors are collected and joined rather than aborting on the first.
func (k *BaseKubernetesManager) PruneBlueprint(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	orphans, err := k.contextOrphanKustomizations(blueprint, namespace)
	if err != nil {
		return err
	}

	if len(orphans) == 0 {
		return nil
	}

	var errs []error
	for _, orphan := range orderForDestroy(orphans, "prune") {
		if err := k.DeleteKustomization(orphan.Name, namespace); err != nil {
			errs = append(errs, fmt.Errorf("failed to prune kustomization %q: %w", orphan.Name, err))
		}
	}
	return errors.Join(errs...)
}

// contextOrphanKustomizations lists the live Kustomizations belonging to this context (by the
// windsorcli.dev/context-id label) that the blueprint no longer declares — the set Prune deletes and
// ListPrunableKustomizations reports. It scopes strictly to this context, so kustomizations owned by
// other contexts (or by no Windsor context) are never returned. DestroyOnly entries are not desired.
// Each orphan carries its live spec.dependsOn for reverse-dependency ordering.
func (k *BaseKubernetesManager) contextOrphanKustomizations(blueprint *blueprintv1alpha1.Blueprint, namespace string) ([]blueprintv1alpha1.Kustomization, error) {
	contextID := k.configHandler.GetString("id")
	if contextID == "" {
		return nil, fmt.Errorf("context id not set; cannot scope pruning to this context")
	}

	desired := make(map[string]bool, len(blueprint.Kustomizations))
	for _, kustomization := range blueprint.Kustomizations {
		if kustomization.DestroyOnly != nil && *kustomization.DestroyOnly {
			continue
		}
		desired[kustomization.Name] = true
	}

	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}
	list, err := k.client.ListResources(gvr, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list kustomizations: %w", err)
	}

	orphans := make([]blueprintv1alpha1.Kustomization, 0)
	for i := range list.Items {
		item := list.Items[i]
		if item.GetLabels()["windsorcli.dev/context-id"] != contextID {
			continue
		}
		name := item.GetName()
		if desired[name] {
			continue
		}
		orphans = append(orphans, blueprintv1alpha1.Kustomization{
			Name:      name,
			DependsOn: dependsOnFromObject(item),
		})
	}
	return orphans, nil
}

// ListPrunableKustomizations returns the names of this context's Kustomizations that the blueprint no
// longer declares — exactly what PruneBlueprint would delete, in reverse-dependency order. It is the
// read-only input to plan's prune preview and upgrade's confirmation gate; it deletes nothing.
func (k *BaseKubernetesManager) ListPrunableKustomizations(blueprint *blueprintv1alpha1.Blueprint, namespace string) ([]string, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	orphans, err := k.contextOrphanKustomizations(blueprint, namespace)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(orphans))
	for _, orphan := range orderForDestroy(orphans, "prune") {
		names = append(names, orphan.Name)
	}
	return names, nil
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

		fluxKustomization.Labels = k.ownershipLabels()
		fluxKustomization.Spec.CommonMetadata = &kustomizev1.CommonMetadata{
			Labels: k.ownershipLabels(),
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
			errors = append(errors, fmt.Errorf("destroy-only kustomizations did not become ready within timeout - cleanup may not have completed%s", k.describeNotReadyKustomizations(kustomizationNames, k.gitopsNamespace())))
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

// describeStuckHelmReleases returns the most diagnostic non-Ready condition from a stuck
// Kustomization's HelmReleases, for DeleteKustomization's timeout error. Lookup errors are
// swallowed. Returns "" if there's nothing more specific to add.
func (k *BaseKubernetesManager) describeStuckHelmReleases(name, namespace string) string {
	helmReleases, err := k.GetHelmReleasesForKustomization(name, namespace)
	if err != nil || len(helmReleases) == 0 {
		return ""
	}

	var parts []string
	for _, hr := range helmReleases {
		var stalled, ready, other *metav1.Condition
		for i := range hr.Status.Conditions {
			cond := &hr.Status.Conditions[i]
			switch {
			case cond.Type == "Stalled" && cond.Status == "True":
				stalled = cond
			case cond.Type == "Ready" && cond.Status != "True":
				ready = cond
			case cond.Status != "True" && other == nil:
				other = cond
			}
		}
		pick := stalled
		if pick == nil {
			pick = ready
		}
		if pick == nil {
			pick = other
		}
		if pick == nil || pick.Message == "" {
			continue
		}
		message := strings.ReplaceAll(strings.TrimSpace(pick.Message), "\n", " ")
		if pick.Reason != "" {
			parts = append(parts, fmt.Sprintf("HelmRelease %s/%s (%s=%s %s: %s)", hr.Namespace, hr.Name, pick.Type, pick.Status, pick.Reason, message))
		} else {
			parts = append(parts, fmt.Sprintf("HelmRelease %s/%s (%s=%s: %s)", hr.Namespace, hr.Name, pick.Type, pick.Status, message))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "; " + strings.Join(parts, ", ")
}

// kustomizationCheckResult carries one kustomization's readiness outcome back from a
// concurrent checkKustomizationReady call to WaitForKustomizations' tick loop.
type kustomizationCheckResult struct {
	ready bool
	err   error
}

// kustomizationTerminalReasons are Ready=False Reasons that describe a permanent problem: a
// build error, an artifact fetch failure, or a failed apply. kustomize-controller repeats the
// same reason on every retry, so waiting longer never helps. This differs from a reason like
// "Progressing" or "DependencyNotReady", which does resolve with time. GetKustomizationStatus
// and checkKustomizationReady both treat these reasons as fatal instead of polling to the
// timeout.
var kustomizationTerminalReasons = map[string]bool{
	meta.BuildFailedReason:          true,
	meta.ArtifactFailedReason:       true,
	meta.ReconciliationFailedReason: true,
}

// kustomizationFailedError signals a Ready condition with a Reason in kustomizationTerminalReasons:
// a failure polling will never resolve. WaitForKustomizations fails immediately on this error.
// A raw GetResource API error still gets the error-streak tolerance instead (see WaitForKustomizations).
type kustomizationFailedError struct {
	name    string
	reason  string
	message string
}

func (e *kustomizationFailedError) Error() string {
	return fmt.Sprintf("kustomization %s failed (%s): %s", e.name, e.reason, e.message)
}

// checkKustomizationReady fetches a single Kustomization and reports whether its status
// carries a Ready=True condition. A not-found resource is reported not-ready with no error,
// since it hasn't reconciled into existence yet; any other GetResource or decode failure is
// reported not-ready with the error, for the caller's error-budget accounting. A Ready=False
// condition with a Reason in kustomizationTerminalReasons returns a *kustomizationFailedError
// instead, so WaitForKustomizations can fail immediately rather than poll a Kustomization
// that will never become Ready.
func (k *BaseKubernetesManager) checkKustomizationReady(name string) (bool, error) {
	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}
	obj, err := k.client.GetResource(gvr, k.gitopsNamespace(), name)
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, err
	}
	var kustomizationObj map[string]any
	if err := k.shims.FromUnstructured(obj.UnstructuredContent(), &kustomizationObj); err != nil {
		return false, nil
	}
	status, ok := kustomizationObj["status"].(map[string]any)
	if !ok {
		return false, nil
	}
	conditions, ok := status["conditions"].([]any)
	if !ok {
		return false, nil
	}
	for _, cond := range conditions {
		condMap, ok := cond.(map[string]any)
		if !ok {
			continue
		}
		if condMap["type"] != "Ready" {
			continue
		}
		if condMap["status"] == "True" {
			return true, nil
		}
		if condMap["status"] == "False" {
			reason, _ := condMap["reason"].(string)
			if kustomizationTerminalReasons[reason] {
				message, _ := condMap["message"].(string)
				return false, &kustomizationFailedError{name: name, reason: reason, message: message}
			}
		}
		break
	}
	return false, nil
}

// servicesGVR is the core v1 Services resource, scanned during destroy to find cloud
// LoadBalancers that must be released before their controller is torn down.
var servicesGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}

// maxOwnerWalkDepth bounds the ownerReference ascent when resolving the inventory-owned root of a
// LoadBalancer Service, guarding against cyclic or pathologically deep owner chains.
const maxOwnerWalkDepth = 8

// ownedTarget identifies an inventory-owned resource to foreground-delete during load balancer
// remediation, addressed by its resolved GVR, namespace, and name.
type ownedTarget struct {
	gvr       schema.GroupVersionResource
	namespace string
	name      string
}

// remediateLoadBalancerOwners releases cloud LoadBalancers that would otherwise be orphaned when
// their controller (a cloud-controller-manager) is torn down before the LoadBalancer Service's
// cloud finalizer runs. Flux prunes with background propagation and waits only on its own
// inventory, so a controller-generated type=LoadBalancer Service — a child of an inventory object
// such as a Gateway, never itself in the inventory — is garbage-collected asynchronously and its
// cloud-LB finalizer can outlive the controller, leaking the LB and wedging the terraform network
// delete. Called while every controller is still alive (after suspend, before the teardown walk),
// it lists live type=LoadBalancer Services, walks each one's ownerReferences to the first ancestor
// present in the eligible kustomizations' inventory, and foreground-deletes that owned root so the
// ownerReference cascade blocks on the child Service's cloud finalizer while the CCM can still
// release the LB. Services with no inventory-owned ancestor are foreign and left untouched.
func (k *BaseKubernetesManager) remediateLoadBalancerOwners(eligible []blueprintv1alpha1.Kustomization, namespace string) error {
	owned, err := k.ownedInventorySet(eligible, namespace)
	if err != nil {
		return err
	}
	if len(owned) == 0 {
		return nil
	}

	services, err := k.client.ListResources(servicesGVR, "")
	if err != nil {
		return fmt.Errorf("error listing services for load balancer remediation: %w", err)
	}
	if services == nil {
		return nil
	}

	handled := make(map[string]bool)
	for i := range services.Items {
		svc := &services.Items[i]
		if !isLoadBalancerService(svc) {
			continue
		}
		target, found, err := k.ownedRootForService(svc, owned)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		key := target.gvr.String() + "|" + target.namespace + "|" + target.name
		if handled[key] {
			continue
		}
		handled[key] = true
		if err := k.foregroundDeleteAndWaitService(target, svc); err != nil {
			return err
		}
	}
	return nil
}

// ownedInventorySet returns the set of resources managed by the eligible kustomizations, keyed by
// group/kind/namespace/name, from each kustomization's Flux inventory. This is the ground truth of
// "resources we own" that scopes load balancer remediation to our own LoadBalancers.
func (k *BaseKubernetesManager) ownedInventorySet(eligible []blueprintv1alpha1.Kustomization, namespace string) (map[string]bool, error) {
	owned := make(map[string]bool)
	for _, kustomization := range eligible {
		entries, err := k.GetKustomizationInventory(kustomization.Name, namespace)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			owned[inventoryKey(entry.Group, entry.Kind, entry.Namespace, entry.Name)] = true
		}
	}
	return owned, nil
}

// ownedRootForService walks a LoadBalancer Service's ownerReferences to the first ancestor present
// in the owned inventory set and returns it as the deletion target. A Service we applied directly
// is its own target; otherwise the ascent follows the controller ownerReference upward, fetching
// each owner to read its own references, until an owned ancestor is found or the chain ends.
// Returns found=false when no ancestor is ours (a foreign LoadBalancer we must not touch).
//
// Each hop resolves its own scope via IsNamespaced rather than reusing the Service's namespace
// unconditionally: a cluster-scoped owner (e.g. GatewayClass, ClusterRole) is addressed with an
// empty namespace, both for the inventory lookup and the GetResource call. Flux's inventory
// likewise encodes cluster-scoped entries with an empty namespace, so treating a cluster-scoped
// owner as if it lived in the Service's namespace makes both the inventory lookup and the
// GetResource call miss — the walk silently classifies an owned root as foreign and gives up.
func (k *BaseKubernetesManager) ownedRootForService(svc *unstructured.Unstructured, owned map[string]bool) (ownedTarget, bool, error) {
	namespace := svc.GetNamespace()
	if owned[inventoryKey("", "Service", namespace, svc.GetName())] {
		return ownedTarget{gvr: servicesGVR, namespace: namespace, name: svc.GetName()}, true, nil
	}

	current := svc
	for range maxOwnerWalkDepth {
		owner := controllerOwnerRef(current)
		if owner == nil {
			return ownedTarget{}, false, nil
		}
		gv, err := schema.ParseGroupVersion(owner.APIVersion)
		if err != nil {
			return ownedTarget{}, false, nil
		}
		gvk := gv.WithKind(owner.Kind)
		gvr, err := k.client.ResourceFor(gvk)
		if err != nil {
			if apimeta.IsNoMatchError(err) {
				return ownedTarget{}, false, nil
			}
			return ownedTarget{}, false, fmt.Errorf("error resolving load balancer owner %s %q: %w", owner.Kind, owner.Name, err)
		}
		namespaced, err := k.client.IsNamespaced(gvk)
		if err != nil {
			if apimeta.IsNoMatchError(err) {
				return ownedTarget{}, false, nil
			}
			return ownedTarget{}, false, fmt.Errorf("error resolving scope of load balancer owner %s %q: %w", owner.Kind, owner.Name, err)
		}
		ownerNamespace := namespace
		if !namespaced {
			ownerNamespace = ""
		}
		if owned[inventoryKey(gv.Group, owner.Kind, ownerNamespace, owner.Name)] {
			return ownedTarget{gvr: gvr, namespace: ownerNamespace, name: owner.Name}, true, nil
		}
		next, err := k.client.GetResource(gvr, ownerNamespace, owner.Name)
		if err != nil {
			if isNotFoundError(err) {
				return ownedTarget{}, false, nil
			}
			return ownedTarget{}, false, err
		}
		current = next
	}
	return ownedTarget{}, false, nil
}

// foregroundDeleteAndWaitService foreground-deletes an owned load balancer root and waits for the
// backing LoadBalancer Service to disappear, confirming the cloud-controller-manager released the
// LB. Foreground propagation holds the owner until its garbage-collected children clear, so the
// child Service's cloud finalizer runs while the CCM is still alive. A NotFound on delete is
// treated as already-gone. On timeout the LoadBalancer is likely orphaned, so it returns an error
// rather than letting destroy proceed into the terraform teardown that the orphan would wedge.
func (k *BaseKubernetesManager) foregroundDeleteAndWaitService(target ownedTarget, svc *unstructured.Unstructured) error {
	policy := metav1.DeletePropagationForeground
	err := k.client.DeleteResource(target.gvr, target.namespace, target.name, metav1.DeleteOptions{PropagationPolicy: &policy})
	if err != nil && !isNotFoundError(err) {
		return fmt.Errorf("error foreground-deleting load balancer owner %s/%s: %w", target.namespace, target.name, err)
	}

	namespace, name := svc.GetNamespace(), svc.GetName()
	timeout := time.Now().Add(k.kustomizationReconcileTimeout)
	for time.Now().Before(timeout) {
		_, err := k.client.GetResource(servicesGVR, namespace, name)
		if err != nil && isNotFoundError(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("error waiting for load balancer service %s/%s deletion: %w", namespace, name, err)
		}
		time.Sleep(k.kustomizationWaitPollInterval)
	}
	return fmt.Errorf("timeout waiting for load balancer service %s/%s to be released; its cloud finalizer has not lifted and the cloud load balancer may be orphaned — inspect with `kubectl get svc %s -n %s -o yaml` and confirm the cloud-controller-manager is still running", namespace, name, name, namespace)
}

// gitopsNamespace returns the configured gitops namespace, defaulting to DefaultGitopsNamespace.
func (k *BaseKubernetesManager) gitopsNamespace() string {
	return k.configHandler.GetString("gitops.namespace", constants.DefaultGitopsNamespace)
}

// secretOwnerLabel names the kustomization a CLI-placed Secret belongs to. It is set only by
// ApplySecret, never by Flux, so it is the reliable marker for finding secrets the CLI itself placed —
// PruneSecrets selects on it (scoped to the context) to reclaim orphans without touching Flux-managed
// secrets that happen to carry the context labels via CommonMetadata.
const secretOwnerLabel = "windsorcli.dev/secret-owner" // #nosec G101 -- label key, not a credential

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
// It routes to the appropriate repository type based on the source URL and applies it to the
// cluster. isPrimary is true for the blueprint's own repository (the top-level "repository:"
// field) and selects the short, continuously-polled default interval rather than the long
// pinned-vendor-source default; see constants.FluxSourceInterval.
func (k *BaseKubernetesManager) applyBlueprintSource(source blueprintv1alpha1.Source, namespace string, isPrimary bool) error {
	if strings.HasPrefix(source.Url, "oci://") {
		return k.applyBlueprintOCIRepository(source, namespace, isPrimary)
	}
	return k.applyBlueprintGitRepository(source, namespace, isPrimary)
}

// setKustomizationSuspend patches spec.suspend on a Kustomization. DeleteBlueprint
// suspends every eligible Kustomization up front to freeze reconciliation: an
// un-deleted Kustomization that keeps reconciling can re-create a (often
// cluster-scoped) resource that another component's Helm uninstall is mid-way
// through deleting, deadlocking it — Helm waits for the resource to disappear while
// Flux restores it, so the HelmRelease never drops its finalizers.fluxcd.io
// finalizer and the namespace hangs in Terminating until destroy times out. The
// frozen Kustomizations are then resumed (suspend=false) one at a time, each
// immediately before its own delete: the kustomize-controller's finalizer prunes
// managed inventory only when the object is NOT suspended, so deleting a still-
// suspended Kustomization strips its finalizer without garbage-collecting its
// resources, orphaning them and letting destroy race ahead. Resuming just before
// delete keeps every other Kustomization frozen while letting the one being deleted
// prune. A NotFound Kustomization is treated as success.
func (k *BaseKubernetesManager) setKustomizationSuspend(name, namespace string, suspend bool) error {
	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}
	patch := fmt.Appendf(nil, `{"spec":{"suspend":%t}}`, suspend)
	opts := metav1.PatchOptions{FieldManager: "windsor-cli"}
	if _, err := k.client.PatchResource(context.Background(), gvr, namespace, name, types.MergePatchType, patch, opts); err != nil {
		if isNotFoundError(err) {
			return nil
		}
		return err
	}
	return nil
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
// isPrimary selects the short, continuously-polled interval default for the blueprint's own
// repository rather than the long pinned-vendor-source default; see constants.FluxSourceInterval.
func (k *BaseKubernetesManager) applyBlueprintGitRepository(source blueprintv1alpha1.Source, namespace string, isPrimary bool) error {
	sourceUrl := runtimegit.NormalizeRemoteURL(source.Url)

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
				Duration: constants.FluxSourceInterval(isPrimary),
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
// isPrimary selects the short, continuously-polled interval default for the blueprint's own
// repository rather than the long pinned-vendor-source default; see constants.FluxSourceInterval.
// Checks for a "@sha256:<hex>" digest before the tag split, since the digest also has a colon.
func (k *BaseKubernetesManager) applyBlueprintOCIRepository(source blueprintv1alpha1.Source, namespace string, isPrimary bool) error {
	ociURL := source.Url
	var ref *sourcev1.OCIRepositoryRef

	if atIdx := strings.Index(ociURL, "@sha256:"); atIdx > len("oci://") {
		ociURL = ociURL[:atIdx]
		ref = &sourcev1.OCIRepositoryRef{
			Digest: source.Url[atIdx+1:],
		}
	} else if lastColon := strings.LastIndex(ociURL, ":"); lastColon > len("oci://") {
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
				Duration: constants.FluxSourceInterval(isPrimary),
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

// inventoryKey builds the group/kind/namespace/name key used to match live objects against a
// kustomization's Flux inventory. Group is empty for core API objects; namespace is empty for
// cluster-scoped resources.
func inventoryKey(group, kind, namespace, name string) string {
	return strings.Join([]string{group, kind, namespace, name}, "|")
}

// isLoadBalancerService reports whether an object is a Service of spec.type LoadBalancer.
func isLoadBalancerService(svc *unstructured.Unstructured) bool {
	svcType, found, err := unstructured.NestedString(svc.Object, "spec", "type")
	return err == nil && found && svcType == "LoadBalancer"
}

// controllerOwnerRef returns the controller ownerReference of an object, falling back to the first
// ownerReference when none is marked controller. Returns nil when the object has no owners.
func controllerOwnerRef(obj *unstructured.Unstructured) *metav1.OwnerReference {
	refs := obj.GetOwnerReferences()
	for i := range refs {
		if refs[i].Controller != nil && *refs[i].Controller {
			return &refs[i]
		}
	}
	if len(refs) > 0 {
		return &refs[0]
	}
	return nil
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

// dockerConfigSecretKey is the well-known stringData key a kubernetes.io/dockerconfigjson Secret
// carries its registry auth under.
const dockerConfigSecretKey = ".dockerconfigjson" // #nosec G101 -- well-known stringData key, not a credential

// defaultDockerConfigServer is the registry secretTypeAndData assumes when docker-username and
// docker-password are set but docker-server is not.
const defaultDockerConfigServer = "ghcr.io"

// secretTypeAndData resolves the Secret type and stringData ApplySecret should apply. stringData
// already carrying dockerConfigSecretKey passes through unchanged as kubernetes.io/dockerconfigjson.
// Otherwise, docker-username plus docker-password (docker-server optional, defaulting to
// defaultDockerConfigServer) synthesizes a dockerConfigSecretKey entry and drops the docker-* keys,
// also as kubernetes.io/dockerconfigjson. Anything else stays Opaque, unchanged. stringData is never
// mutated; a new map is returned whenever synthesis applies.
func secretTypeAndData(stringData map[string]string) (string, map[string]string, error) {
	if stringData[dockerConfigSecretKey] != "" {
		return "kubernetes.io/dockerconfigjson", stringData, nil
	}

	username := stringData["docker-username"]
	password := stringData["docker-password"]
	if username == "" || password == "" {
		return "Opaque", stringData, nil
	}

	server := stringData["docker-server"]
	if server == "" {
		server = defaultDockerConfigServer
	}

	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	dockerConfig := map[string]any{
		"auths": map[string]any{
			server: map[string]any{
				"username": username,
				"password": password,
				"auth":     auth,
			},
		},
	}
	encoded, err := json.Marshal(dockerConfig)
	if err != nil {
		return "", nil, fmt.Errorf("failed to encode docker config json: %w", err)
	}

	resolved := make(map[string]string, len(stringData))
	for k, v := range stringData {
		if k == "docker-username" || k == "docker-password" || k == "docker-server" {
			continue
		}
		resolved[k] = v
	}
	resolved[dockerConfigSecretKey] = string(encoded)

	return "kubernetes.io/dockerconfigjson", resolved, nil
}

// secretTypeChanged reports whether an existing Secret's type differs from newType. Kubernetes
// treats Secret.type as immutable after creation — the API server rejects an update that changes
// it — so ApplySecret deletes and recreates rather than SSA-merging when the resolved type (e.g.
// via docker-* key synthesis) differs from what's already there.
func secretTypeChanged(obj *unstructured.Unstructured, newType string) bool {
	existingType, _ := obj.Object["type"].(string)
	return existingType != "" && existingType != newType
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

// dependsOnFromObject extracts the dependency names from a live Kustomization's spec.dependsOn,
// used to order pruned kustomizations so dependents are deleted before their dependencies.
func dependsOnFromObject(obj unstructured.Unstructured) []string {
	raw, found, err := unstructured.NestedSlice(obj.Object, "spec", "dependsOn")
	if err != nil || !found {
		return nil
	}
	deps := make([]string, 0, len(raw))
	for _, entry := range raw {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := entryMap["name"].(string); ok && name != "" {
			deps = append(deps, name)
		}
	}
	return deps
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

// reverseTopologicalKustomizations returns ks in destroy order — each kustomization
// before its DependsOn entries. Independent nodes tie-break by reverse input order,
// so a topo-sorted input produces the same walk as a naive slice-reverse. Missing
// dependencies (a DependsOn name not in ks) are treated as no-edge, matching the
// apply-side walk. Returns an error on cycles across two or more nodes; a single-
// node input short-circuits before cycle detection, so a self-loop on a lone
// kustomization is not flagged — do not rely on this function to validate a
// single-entry slice.
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
