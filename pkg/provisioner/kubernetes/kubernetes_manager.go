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
	"slices"
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
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
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

// abandonedInventoryGraceChecks is the minimum number of extra polls DeleteKustomization
// spends re-checking a still-live inventory entry after the Kustomization disappears,
// regardless of inventory size. It gives a resource still finishing its own normal
// termination a chance to clear before the delete is reported as abandoned rather than
// clean; see abandonedInventoryGraceWindow for the size-scaled window built on top of it.
const abandonedInventoryGraceChecks = 3

// DeleteKustomization deletes a Kustomization and waits for it to disappear. The wait
// floor rises to spec.timeout when set, and scales with inventory size (see
// kustomizationSpecTimeoutCeiling). On timeout or a clean disappearance, it checks the
// last-known inventory (see firstLiveInventoryEntry) before trusting the result. A
// still-live entry gets a few retries first, to rule out normal in-flight termination.
// This runs outside a destroy. The cluster stays up, so a finalizer-free leftover still
// blocks, the same as any other live one.
func (k *BaseKubernetesManager) DeleteKustomization(name, namespace string) error {
	return k.deleteKustomization(name, namespace, nil, nil, false)
}

// deleteKustomization is DeleteKustomization with a known destroy expectation and a delete-wait
// override, both optional. expectWaitForTermination overrides the live object's own, possibly
// stale, deletionPolicy; see kustomizationDeletionPolicy. deleteTimeoutOverride replaces every
// spec-derived floor, spec.timeout and helmReleaseUninstallTimeout alike, keeping an explicit
// DeleteTimeout a bound. The wait absorbs kustomizationWaitMaxReadFailures consecutive read
// errors, but fails if the object disappears before any read succeeded. destroying MUST be true
// only during a full cluster teardown. See firstLiveInventoryEntry.
func (k *BaseKubernetesManager) deleteKustomization(name, namespace string, expectWaitForTermination *bool, deleteTimeoutOverride *time.Duration, destroying bool) error {
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
		return fmt.Errorf("error deleting kustomization %s/%s: %w", namespace, name, err)
	}

	start := k.shims.TimeNow()
	waitFor := k.kustomizationReconcileTimeout
	var lastObj *unstructured.Unstructured
	var lastReadErr error
	readFailures := 0
	helmTimeoutResolved := false
	helmInventory := helmReleaseInventory{}
	for k.shims.TimeNow().Before(start.Add(waitFor)) {
		obj, err := k.client.GetResource(gvr, namespace, name)
		if err != nil && isNotFoundError(err) {
			if lastObj == nil && readFailures > 0 {
				return fmt.Errorf("kustomization %s/%s disappeared before windsor could read its inventory: %w. Windsor cannot confirm the resources it managed are gone. Check for leftovers with `kubectl get pvc,svc,ingress,certificate -A | grep Terminating` before retrying", namespace, name, lastReadErr)
			}
			entry, surviving, checkErr := k.describeAbandonedInventory(lastObj, expectWaitForTermination, helmInventory, destroying)
			graceDeadline := k.shims.TimeNow().Add(k.abandonedInventoryGraceWindow(inventorySize(lastObj)))
			for (entry != nil || checkErr != nil) && k.shims.TimeNow().Before(graceDeadline) {
				if errors.Is(checkErr, errUnverifiableInventory) {
					break
				}
				if entry != nil {
					k.triggerReconcile(entry)
				}
				k.shims.TimeSleep(k.kustomizationWaitPollInterval)
				entry, surviving, checkErr = k.describeAbandonedInventory(lastObj, expectWaitForTermination, helmInventory, destroying)
			}
			reportSurvivingResources(namespace, name, surviving)
			if checkErr != nil {
				return fmt.Errorf("kustomization %s/%s disappeared and windsor could not confirm its resources are gone: %w. Inspect the namespace before retrying", namespace, name, checkErr)
			}
			if entry == nil {
				return nil
			}
			return fmt.Errorf("kustomization %s/%s disappeared. %s/%s from its inventory is still live. Flux likely gave up waiting and removed the finalizer early. Inspect it with %s before retrying", namespace, name, entry.Kind, entry.Name, liveEntryInspectCmd(entry))
		}
		if err != nil {
			readFailures++
			lastReadErr = err
			if readFailures > k.kustomizationWaitMaxReadFailures {
				return fmt.Errorf("error checking kustomization %s/%s deletion status: %w", namespace, name, err)
			}
			k.shims.TimeSleep(k.kustomizationWaitPollInterval)
			continue
		}
		readFailures = 0
		lastObj = obj
		k.snapshotHelmReleaseInventories(obj, helmInventory)

		if size := inventorySize(obj); size > 0 {
			waitFor = extendWaitFor(waitFor, k.kustomizationDeletionTimeout(size))
		}
		if deleteTimeoutOverride != nil {
			waitFor = extendWaitFor(waitFor, min(*deleteTimeoutOverride, k.kustomizationDeleteTimeoutCeiling))
		} else {
			if specTO, ok := specTimeout(obj); ok {
				waitFor = extendWaitFor(waitFor, min(specTO, k.kustomizationSpecTimeoutCeiling))
			}
			if !helmTimeoutResolved {
				if declared, ok := k.helmReleaseUninstallTimeout(obj); ok {
					waitFor = extendWaitFor(waitFor, min(declared, k.kustomizationSpecTimeoutCeiling))
					helmTimeoutResolved = true
				}
			}
		}

		k.shims.TimeSleep(k.kustomizationWaitPollInterval)
	}

	inspectCmd := fmt.Sprintf("`kubectl get kustomization %s -n %s -o yaml`", name, namespace)
	const terminatingCmd = "`kubectl get pvc,svc,ingress,certificate -A | grep Terminating`"

	entries, inventoryFound, dropped := inventoryEntriesFromObject(lastObj)
	live, surviving, checkErr := k.firstLiveInventoryEntry(entries, helmInventory, destroying)
	reportSurvivingResources(namespace, name, surviving)
	if inventoryFound && checkErr == nil && dropped == 0 && live == nil {
		if len(surviving) == 0 {
			return fmt.Errorf("kustomization %s/%s is fully drained. Every inventory item is confirmed gone, but its own finalizer is stuck. This is Flux bookkeeping, not leaked infrastructure. Clear it with `kubectl patch kustomization %s -n %s --type=merge -p '{\"metadata\":{\"finalizers\":null}}'`", namespace, name, name, namespace)
		}
		return fmt.Errorf("kustomization %s/%s left %d resource(s) behind. None of it blocks the cluster, but its own finalizer is stuck. This is Flux bookkeeping, not leaked infrastructure. Clear it with `kubectl patch kustomization %s -n %s --type=merge -p '{\"metadata\":{\"finalizers\":null}}'`", namespace, name, len(surviving), name, namespace)
	}
	reason := describeStuckKustomization(lastObj) + k.describeStuckHelmReleases(name, namespace) + describeInventoryVerdict(live, checkErr)
	if waitForTermination, ok := kustomizationDeletionPolicy(lastObj, expectWaitForTermination); ok && !waitForTermination {
		if reason == "" {
			return fmt.Errorf("windsor timed out after %s waiting for kustomization %s/%s to delete. It uses MirrorPrune, which deletes resources without waiting for confirmation, so this timeout does not mean an inventory item is stuck. Check for a suspended reconcile or an RBAC failure. Inspect with %s and %s", waitFor, namespace, name, inspectCmd, terminatingCmd)
		}
		return fmt.Errorf("windsor timed out after %s waiting for kustomization %s/%s to delete%s. It uses MirrorPrune, which deletes resources without waiting for confirmation, so this timeout does not mean an inventory item is stuck. Inspect with %s (status.conditions) and %s", waitFor, namespace, name, reason, inspectCmd, terminatingCmd)
	}
	if reason == "" {
		return fmt.Errorf("windsor timed out after %s waiting for kustomization %s/%s to delete. No status condition confirms a stuck finalizer. Check its inventory with %s:\n  - if it is still shrinking, wait and retry\n  - if it is not shrinking, find the stuck object with %s", waitFor, namespace, name, inspectCmd, terminatingCmd)
	}
	return fmt.Errorf("windsor timed out after %s waiting for kustomization %s/%s to delete%s. An inventory item is likely stuck on a cloud-controller finalizer. Inspect with %s (status.conditions, status.inventory) and %s to find the stuck object", waitFor, namespace, name, reason, inspectCmd, terminatingCmd)
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

// kustomizationDeletionPolicy resolves a Kustomization's effective deletionPolicy: expect
// when the caller already knows it, otherwise obj's own spec.deletionPolicy. ok is false
// when neither source resolves a value.
func kustomizationDeletionPolicy(obj *unstructured.Unstructured, expect *bool) (waitForTermination, ok bool) {
	if expect != nil {
		return *expect, true
	}
	if obj == nil {
		return false, false
	}
	value, found, err := unstructured.NestedString(obj.Object, "spec", "deletionPolicy")
	if err != nil || !found {
		return false, false
	}
	switch value {
	case "WaitForTermination":
		return true, true
	case "MirrorPrune":
		return false, true
	default:
		return false, false
	}
}

// kustomizationDeleteTimeout returns kustomization's DeleteTimeout as deleteKustomization's
// override parameter, or nil when unset.
func kustomizationDeleteTimeout(kustomization blueprintv1alpha1.Kustomization) *time.Duration {
	if kustomization.DeleteTimeout == nil {
		return nil
	}
	d := kustomization.DeleteTimeout.Duration
	return &d
}

// extendWaitFor raises waitFor to candidate when candidate is larger, otherwise
// returns waitFor unchanged.
func extendWaitFor(waitFor, candidate time.Duration) time.Duration {
	if candidate > waitFor {
		return candidate
	}
	return waitFor
}

// scaledExtraTimeout multiplies entryCount by perEntry, capped at maxExtra so a corrupted
// or unusually large count cannot stall destroy indefinitely. entryCount is clamped before
// the multiplication to guard against Duration overflow.
func scaledExtraTimeout(entryCount int, perEntry, maxExtra time.Duration) time.Duration {
	const maxEntryCount = 100_000
	if entryCount > maxEntryCount {
		entryCount = maxEntryCount
	}
	extra := time.Duration(entryCount) * perEntry
	if extra > maxExtra {
		extra = maxExtra
	}
	return extra
}

// kustomizationDeletionTimeout scales DeleteKustomization's wait window by inventory
// size, capped at kustomizationDeletionMaxExtraTimeout so a corrupted or unusually
// large count cannot stall destroy indefinitely.
func (k *BaseKubernetesManager) kustomizationDeletionTimeout(entryCount int) time.Duration {
	extra := scaledExtraTimeout(entryCount, k.kustomizationDeletionPerEntryTimeout, k.kustomizationDeletionMaxExtraTimeout)
	return k.kustomizationReconcileTimeout + extra
}

// abandonedInventoryGraceWindow scales DeleteKustomization's re-check window for a
// still-live inventory entry by inventory size, capped at kustomizationAbandonedGraceMaxExtra
// so a corrupted or unusually large count cannot stall destroy indefinitely.
// abandonedInventoryGraceChecks sets the floor.
func (k *BaseKubernetesManager) abandonedInventoryGraceWindow(entryCount int) time.Duration {
	base := time.Duration(abandonedInventoryGraceChecks) * k.kustomizationWaitPollInterval
	extra := scaledExtraTimeout(entryCount, k.kustomizationDeletionPerEntryTimeout, k.kustomizationAbandonedGraceMaxExtra)
	return base + extra
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
	start := time.Now()
	var stuck []string
	for _, name := range names {
		if time.Since(start) >= k.notReadyDescribeBudget {
			stuck = append(stuck, name)
			continue
		}
		obj, err := k.client.GetResource(kustomizationsGVR, namespace, name)
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
// Ready=True condition. A nil object or a missing status or conditions slice reads as not ready.
func kustomizationReady(obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
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
// transient ListResources errors may consume before WaitForKustomizations gives up. A sustained
// apiserver slowdown gets patience proportional to how patient the overall wait already is,
// rather than a fixed tick count that's either too tight for a big blueprint or too loose for
// a small one.
const kustomizationWaitErrorBudgetFraction = 0.25

// kustomizationsGVR is the Flux Kustomization resource. WaitForKustomizations lists it once per
// tick rather than issuing one GetResource per pending name.
var kustomizationsGVR = schema.GroupVersionResource{
	Group:    "kustomize.toolkit.fluxcd.io",
	Version:  "v1",
	Resource: "kustomizations",
}

// WaitForKustomizations waits for kustomizations to be ready, using a timeout derived from the
// blueprint's longest dependency chain. It honors ctx cancellation and returns ctx.Err().
//
// A list-call error and a ReconciliationFailed condition share one error-budget tolerance before
// they fail the wait. BuildFailed and ArtifactFailed fail immediately, since kustomize-controller
// never recovers from them on its own.
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
	failureStreakStart := make(map[string]time.Time, len(kustomizationNames))

	for {
		select {
		case <-ctx.Done():
			tui.Pause()
			return ctx.Err()
		case <-timeoutChan:
			tui.Fail()
			return fmt.Errorf("timeout waiting for kustomizations%s", k.describeNotReadyKustomizations(kustomizationNames, k.gitopsNamespace()))
		case <-ticker.C:
			objList, err := k.client.ListResources(kustomizationsGVR, k.gitopsNamespace())
			if err != nil && isNotFoundError(err) {
				errorStreakStart = time.Time{}
				continue
			}
			if err != nil {
				if errorStreakStart.IsZero() {
					errorStreakStart = time.Now()
				}
				if time.Since(errorStreakStart) >= maxErrorDuration {
					tui.Fail()
					return fmt.Errorf("kustomization readiness checks failing for over %s: %w", maxErrorDuration, err)
				}
				continue
			}
			if objList == nil {
				objList = &unstructured.UnstructuredList{}
			}

			byName := make(map[string]*unstructured.Unstructured, len(objList.Items))
			for i := range objList.Items {
				byName[objList.Items[i].GetName()] = &objList.Items[i]
			}

			for _, name := range kustomizationNames {
				if readyKustomizations[name] {
					continue
				}
				obj, exists := byName[name]
				if !exists {
					continue
				}
				ready, pending, failed := kustomizationConditionStatus(obj)
				if failed != nil {
					tui.Fail()
					return fmt.Errorf("kustomization will not become ready: %w", failed)
				}
				if pending != nil {
					start, tracking := failureStreakStart[name]
					if !tracking {
						start = time.Now()
						failureStreakStart[name] = start
					}
					if time.Since(start) >= maxErrorDuration {
						tui.Fail()
						return fmt.Errorf("kustomization failing for over %s: %w", maxErrorDuration, pending)
					}
					continue
				}
				delete(failureStreakStart, name)
				if ready {
					readyKustomizations[name] = true
				}
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

// ApplyConfigMap creates or updates a ConfigMap using SSA. It refuses to write a value that still
// contains an unresolved expression, such as an unresolved terraform_output().
func (k *BaseKubernetesManager) ApplyConfigMap(name, namespace string, data map[string]string) error {
	for key, value := range data {
		if evaluator.ContainsExpression(value) {
			return fmt.Errorf("configmap %s: value for %q still contains an unresolved expression; run `windsor apply` (or `windsor upgrade`) first to resolve it", name, key)
		}
	}

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
		decoded, ok := decodeInventoryID(entry.ID)
		if !ok || decoded.Group != "helm.toolkit.fluxcd.io" || decoded.Kind != "HelmRelease" {
			continue
		}
		helmRelease, err := k.getHelmRelease(decoded.Name, decoded.Namespace)
		if err != nil {
			return nil, err
		}
		helmReleases = append(helmReleases, *helmRelease)
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
// has a Ready condition with Status False and a Reason in kustomizationFailureReasons, an error is
// returned with the failure message.
func (k *BaseKubernetesManager) GetKustomizationStatus(names []string) (map[string]bool, error) {
	objList, err := k.client.ListResources(kustomizationsGVR, k.gitopsNamespace())
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
				} else if condition.Status == "False" {
					if _, failed := kustomizationFailureReasons[condition.Reason]; failed {
						return nil, fmt.Errorf("kustomization %s failed: %s", kustomizeObj.Name, condition.Message)
					}
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
	objList, err := k.client.ListResources(kustomizationsGVR, k.gitopsNamespace())
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
	_, err := k.client.GetResource(kustomizationsGVR, namespace, name)
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
	entries, _ := decodeInventoryEntries(rawEntries)
	return entries, nil
}

// inventoryEntriesFromObject decodes status.inventory.entries directly from an
// already-fetched object, avoiding another API round trip. found reports whether the
// object has ever reported an inventory at all — false for a Kustomization that has
// not reconciled that far, which is not the same as a reconciled, now-empty one.
// Callers must not treat "not found" as "confirmed empty." dropped counts entries that could
// not be decoded, which makes the returned list incomplete for the same reason.
func inventoryEntriesFromObject(obj *unstructured.Unstructured) (entries []InventoryEntry, found bool, dropped int) {
	if obj == nil {
		return nil, false, 0
	}
	rawEntries, found, err := unstructured.NestedSlice(obj.Object, "status", "inventory", "entries")
	if err != nil || !found {
		return nil, false, 0
	}
	entries, dropped = decodeInventoryEntries(rawEntries)
	return entries, true, dropped
}

// decodeInventoryEntries decodes a raw status.inventory.entries slice. An entry that fails to
// decode is dropped rather than failing the whole read. dropped counts them. A caller must not
// read a short list as a confirmed-empty one.
func decodeInventoryEntries(rawEntries []any) (entries []InventoryEntry, dropped int) {
	entries = make([]InventoryEntry, 0, len(rawEntries))
	for _, raw := range rawEntries {
		entryMap, ok := raw.(map[string]any)
		if !ok {
			dropped++
			continue
		}
		id, _ := entryMap["id"].(string)
		entry, ok := decodeInventoryID(id)
		if !ok {
			dropped++
			continue
		}
		entries = append(entries, entry)
	}
	return entries, dropped
}

// chartInventory is what windsor learned about one HelmRelease. partial marks an inventory that
// only decoded in part, which cannot be verified as though it were whole.
type chartInventory struct {
	entries []InventoryEntry
	partial bool
}

// helmReleaseInventory holds what each HelmRelease in a Kustomization's inventory reported
// managing, keyed by "namespace/name". A HelmRelease is usually gone by the time its parent
// Kustomization disappears, so the entries are captured while it is still readable. A key absent
// from the map was never observed live, which is not the same as observed managing nothing.
type helmReleaseInventory map[string]*chartInventory

// helmReleaseKey identifies a HelmRelease within a helmReleaseInventory.
func helmReleaseKey(namespace, name string) string {
	return namespace + "/" + name
}

// isHelmReleaseEntry reports whether entry names a flux HelmRelease.
func isHelmReleaseEntry(entry InventoryEntry) bool {
	return entry.Group == "helm.toolkit.fluxcd.io" && entry.Kind == "HelmRelease"
}

// record notes that a HelmRelease was observed live, and adds whatever inventory it reported to
// what is already known. It accumulates a union rather than replacing: helm-controller shrinks
// status.inventory as an uninstall proceeds, so the last reading can be empty while the resources
// it listed moments earlier are still live.
func (h helmReleaseInventory) record(namespace, name string, entries []InventoryEntry, partial bool) {
	key := helmReleaseKey(namespace, name)
	known := h[key]
	if known == nil {
		known = &chartInventory{}
		h[key] = known
	}
	known.partial = known.partial || partial

	seen := make(map[InventoryEntry]struct{}, len(known.entries))
	for _, existing := range known.entries {
		seen[existing] = struct{}{}
	}
	for _, entry := range entries {
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		known.entries = append(known.entries, entry)
	}
}

// snapshotHelmReleaseInventories records what every HelmRelease in obj's inventory reports
// managing, while it can still be read. A read that fails records nothing, NotFound included.
// Windsor cannot tell a chart that finished cleanly from one helm-controller abandoned. An
// unobserved HelmRelease is reported as unverifiable, never as gone. An inventory that only
// partly decodes is marked partial.
func (k *BaseKubernetesManager) snapshotHelmReleaseInventories(obj *unstructured.Unstructured, into helmReleaseInventory) {
	entries, found, _ := inventoryEntriesFromObject(obj)
	if !found {
		return
	}
	for _, entry := range entries {
		if !isHelmReleaseEntry(entry) {
			continue
		}
		helmRelease, err := k.getHelmRelease(entry.Name, entry.Namespace)
		if err != nil {
			continue
		}
		child, childFound, dropped := inventoryEntriesFromObject(&unstructured.Unstructured{Object: map[string]any{
			"status": map[string]any{"inventory": helmReleaseInventoryMap(helmRelease)},
		}})
		into.record(entry.Namespace, entry.Name, child, childFound && dropped > 0)
	}
}

// helmReleaseInventoryMap renders a HelmRelease's status.inventory back into the untyped shape
// inventoryEntriesFromObject reads, so both layers decode through one path.
func helmReleaseInventoryMap(helmRelease *helmv2.HelmRelease) map[string]any {
	if helmRelease == nil || helmRelease.Status.Inventory == nil {
		return nil
	}
	raw := make([]any, 0, len(helmRelease.Status.Inventory.Entries))
	for _, ref := range helmRelease.Status.Inventory.Entries {
		raw = append(raw, map[string]any{"id": ref.ID, "v": ref.Version})
	}
	return map[string]any{"entries": raw}
}

// decodeInventoryID parses a flux inventory ID of the form
// "<namespace>_<name>_<group>_<kind>" into an InventoryEntry. Namespace is empty for
// cluster-scoped resources, group for core API objects. Flux writes a colon in an RBAC name as
// a double underscore. So the fields are read from the right, and the name is restored, which
// matches how cli-utils parses the same ID. A malformed ID is dropped, not misrendered.
func decodeInventoryID(id string) (InventoryEntry, bool) {
	parts := strings.Split(id, "_")
	if len(parts) < 4 {
		return InventoryEntry{}, false
	}
	name := strings.ReplaceAll(strings.Join(parts[1:len(parts)-2], "_"), "__", ":")
	kind := parts[len(parts)-1]
	if name == "" || kind == "" {
		return InventoryEntry{}, false
	}
	return InventoryEntry{
		Namespace: parts[0],
		Name:      name,
		Group:     parts[len(parts)-2],
		Kind:      kind,
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

// DeleteBlueprint tears the blueprint down in two phases: destroy-only kustomizations first
// (applied, waited ready, then deleted, for bespoke teardown work like backups), then regular
// kustomizations in reverse-topological order. Each regular delete blocks on
// spec.deletionPolicy=WaitForTermination, so cloud resources release before the object
// disappears. Its delete-wait floor uses the Kustomization's own DeleteTimeout when set.
// Otherwise it falls back to deleteKustomization's spec.timeout-derived heuristic, since
// install and delete latency for the same resource can differ substantially (a managed
// database, for example). Phase 2 aborts on the first per-Kustomization failure rather than
// risk orphaning cloud resources a later Kustomization still needs; a retry picks up where it
// left off. Every abort path runs abortDestroy first to un-suspend the full eligible set,
// since Install/ApplyBlueprint never resets spec.suspend on existing objects.
// waitForResumeReconcile runs between each resume and its delete, giving the resume's own
// reconcile a chance to settle first.
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
		k.waitForResumeReconcile(kustomization.Name, namespace)
		destroy := kustomization.Destroy.ToBool()
		expectWaitForTermination := destroy == nil || *destroy
		if err := k.deleteKustomization(kustomization.Name, namespace, &expectWaitForTermination, kustomizationDeleteTimeout(kustomization), true); err != nil {
			tui.Fail()
			return k.abortDestroy(eligible, namespace, fmt.Errorf("destroy aborted: failed to delete kustomization: %w. Windsor skipped the remaining kustomizations to avoid orphaning them", err))
		}
		tui.Done()
	}

	return nil
}

// abortDestroy un-suspends every eligible Kustomization before propagating cause, so a
// DeleteBlueprint failure never leaves objects suspended with nothing left to resume them.
// setKustomizationSuspend is a no-op against objects already unsuspended or gone. It stops early
// on a request timeout rather than repeating a call the unreachable cluster will fail again.
func (k *BaseKubernetesManager) abortDestroy(eligible []blueprintv1alpha1.Kustomization, namespace string, cause error) error {
	errs := []error{cause}
	for idx, kustomization := range eligible {
		err := k.setKustomizationSuspend(kustomization.Name, namespace, false)
		if err == nil {
			continue
		}
		errs = append(errs, fmt.Errorf("failed to un-suspend kustomization %q during abort cleanup: %w", kustomization.Name, err))
		if errors.Is(err, context.DeadlineExceeded) {
			if remaining := len(eligible) - idx - 1; remaining > 0 {
				errs = append(errs, fmt.Errorf("stopping abort cleanup after a request timeout: %d more kustomization(s) left un-suspended, since the cluster is not answering", remaining))
			}
			break
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
					if deleteErr := k.deleteKustomization(appliedKust.Name, namespace, nil, kustomizationDeleteTimeout(appliedKust), true); deleteErr != nil {
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
				if deleteErr := k.deleteKustomization(appliedKust.Name, namespace, nil, kustomizationDeleteTimeout(appliedKust), true); deleteErr != nil {
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
			if deleteErr := k.deleteKustomization(kustomization.Name, namespace, nil, kustomizationDeleteTimeout(kustomization), true); deleteErr != nil {
				errors = append(errors, fmt.Errorf("failed to delete failed destroy-only kustomization %s: %w", kustomization.Name, deleteErr))
			}
		}
		return errors
	}
	tui.Done()

	for _, kustomization := range orderForDestroy(kustomizations, "destroy-only") {
		tui.Start(fmt.Sprintf("Destroying destroy-only kustomization %s", kustomization.Name))

		if err := k.deleteKustomization(kustomization.Name, namespace, nil, kustomizationDeleteTimeout(kustomization), true); err != nil {
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

// helmReleaseUninstallTimeout returns the longest uninstall budget the HelmRelease entries in
// obj's inventory declare, resolved the way flux does: spec.uninstall.timeout, else the
// HelmRelease's own spec.timeout. A HelmRelease that declares neither, or that cannot be read,
// contributes nothing, so the caller never shortens a wait on a lookup failure.
func (k *BaseKubernetesManager) helmReleaseUninstallTimeout(obj *unstructured.Unstructured) (time.Duration, bool) {
	entries, found, _ := inventoryEntriesFromObject(obj)
	if !found {
		return 0, false
	}

	var longest time.Duration
	for _, entry := range entries {
		if !isHelmReleaseEntry(entry) {
			continue
		}
		helmRelease, err := k.getHelmRelease(entry.Name, entry.Namespace)
		if err != nil {
			continue
		}
		var fallback metav1.Duration
		if helmRelease.Spec.Timeout != nil {
			fallback = *helmRelease.Spec.Timeout
		}
		if declared := helmRelease.GetUninstall().GetTimeout(fallback).Duration; declared > longest {
			longest = declared
		}
	}
	return longest, longest > 0
}

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

// kustomizationFailureKind classifies a Ready=False Reason: hard reasons never recover, pending
// reasons are ones kustomize-controller retries on its own.
type kustomizationFailureKind int

const (
	kustomizationFailureHard kustomizationFailureKind = iota
	kustomizationFailurePending
)

// kustomizationFailureReasons maps each Ready=False Reason that counts as a failure to its kind.
// A Reason absent here, such as Progressing or DependencyNotReady, resolves with time instead.
var kustomizationFailureReasons = map[string]kustomizationFailureKind{
	meta.BuildFailedReason:          kustomizationFailureHard,
	meta.ArtifactFailedReason:       kustomizationFailureHard,
	meta.ReconciliationFailedReason: kustomizationFailurePending,
}

// kustomizationFailedError signals a Ready condition with a Reason in kustomizationFailureReasons.
type kustomizationFailedError struct {
	name    string
	reason  string
	message string
}

func (e *kustomizationFailedError) Error() string {
	return fmt.Sprintf("kustomization %s failed (%s): %s", e.name, e.reason, e.message)
}

// kustomizationConditionStatus reports a Kustomization's Ready condition: ready true on
// Ready=True, failed set on a hard-failure Reason, pending set on a retryable one.
func kustomizationConditionStatus(obj *unstructured.Unstructured) (ready bool, pending *kustomizationFailedError, failed *kustomizationFailedError) {
	if obj == nil {
		return false, nil, nil
	}
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false, nil, nil
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
			return true, nil, nil
		}
		if condMap["status"] == "False" {
			reason, _ := condMap["reason"].(string)
			if kind, isFailure := kustomizationFailureReasons[reason]; isFailure {
				message, _ := condMap["message"].(string)
				failedErr := &kustomizationFailedError{name: obj.GetName(), reason: reason, message: message}
				if kind == kustomizationFailureHard {
					return false, nil, failedErr
				}
				return false, failedErr, nil
			}
		}
		break
	}
	return false, nil, nil
}

// servicesGVR is the core v1 Services resource, scanned during destroy to find cloud
// LoadBalancers that must be released before their controller is torn down.
var servicesGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}

// maxOwnerWalkDepth bounds the ownerReference ascent when resolving the inventory-owned root of a
// LoadBalancer Service, guarding against cyclic or pathologically deep owner chains.
const maxOwnerWalkDepth = 8

// gatewayAPIGroup and gatewayClassResource identify a Gateway API GatewayClass, so
// deleteBlockingGateways knows when an owned root needs its blocking Gateways cleared.
const (
	gatewayAPIGroup      = "gateway.networking.k8s.io"
	gatewayClassResource = "gatewayclasses"
)

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
	owned, entries, err := k.ownedInventorySet(eligible, namespace)
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
		if err := k.deleteBlockingGateways(target.gvr, target.name, entries); err != nil {
			return err
		}
		if err := k.foregroundDeleteAndWaitService(target, svc); err != nil {
			return err
		}
	}
	return nil
}

// ownedInventorySet returns the set of resources managed by the eligible kustomizations, keyed by
// group/kind/namespace/name, alongside the raw entries, from each kustomization's Flux inventory.
// The set is the ground truth of "resources we own" that scopes load balancer remediation to our
// own LoadBalancers; the raw entries let callers filter by kind, e.g. deleteBlockingGateways.
func (k *BaseKubernetesManager) ownedInventorySet(eligible []blueprintv1alpha1.Kustomization, namespace string) (map[string]bool, []InventoryEntry, error) {
	owned := make(map[string]bool)
	var all []InventoryEntry
	for _, kustomization := range eligible {
		entries, err := k.GetKustomizationInventory(kustomization.Name, namespace)
		if err != nil {
			return nil, nil, err
		}
		for _, entry := range entries {
			owned[inventoryKey(entry.Group, entry.Kind, entry.Namespace, entry.Name)] = true
			all = append(all, entry)
		}
	}
	return owned, all, nil
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
		gvr, ownerNamespace, ok, err := k.resolveScopedGVR(gvk, namespace)
		if err != nil {
			return ownedTarget{}, false, fmt.Errorf("error resolving load balancer owner %s %q: %w", owner.Kind, owner.Name, err)
		}
		if !ok {
			return ownedTarget{}, false, nil
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

// deleteBlockingGateways foreground-deletes every eligible-inventory Gateway naming the given
// GatewayClass before its own foreground-delete-and-wait begins. The class's gateway-exists-
// finalizer only lifts once no Gateway anywhere still names it, and without this a Gateway
// belonging to a not-yet-reached Kustomization — one orderForDestroy would delete moments later
// anyway — can time out remediation for no reason. Only inventory Gateways are touched; a
// foreign one is left alone, matching remediation's existing scope. A no-op for any other kind
// of owned root.
func (k *BaseKubernetesManager) deleteBlockingGateways(classGVR schema.GroupVersionResource, className string, entries []InventoryEntry) error {
	if classGVR.Group != gatewayAPIGroup || classGVR.Resource != gatewayClassResource {
		return nil
	}
	gatewayGVR, err := k.client.ResourceFor(schema.GroupVersionKind{Group: classGVR.Group, Version: classGVR.Version, Kind: "Gateway"})
	if err != nil {
		if apimeta.IsNoMatchError(err) {
			return nil
		}
		return fmt.Errorf("error resolving gateway resource while clearing gatewayclass %s: %w", className, err)
	}
	for _, entry := range entries {
		if entry.Kind != "Gateway" || entry.Group != gatewayAPIGroup {
			continue
		}
		gw, err := k.client.GetResource(gatewayGVR, entry.Namespace, entry.Name)
		if err != nil {
			if isNotFoundError(err) {
				continue
			}
			return fmt.Errorf("error reading gateway %s/%s while clearing gatewayclass %s: %w", entry.Namespace, entry.Name, className, err)
		}
		gatewayClassName, _, _ := unstructured.NestedString(gw.Object, "spec", "gatewayClassName")
		if gatewayClassName != className {
			continue
		}
		if err := k.client.DeleteResource(gatewayGVR, entry.Namespace, entry.Name, metav1.DeleteOptions{}); err != nil && !isNotFoundError(err) {
			return fmt.Errorf("error deleting gateway %s/%s to release gatewayclass %s: %w", entry.Namespace, entry.Name, className, err)
		}
	}
	return nil
}

// foregroundDeleteAndWaitService foreground-deletes an owned load balancer root. It waits for the
// Service to confirm the cloud-controller-manager released the LB. It also waits for the root
// itself to confirm deletion finished, since some finalizers do not use ownerReferences. The
// Gateway API's gateway-exists-finalizer on a GatewayClass is one example. That finalizer clears
// only when no Gateway still names the GatewayClass. When the target is the Service itself, the two
// waits collapse into one. A NotFound on delete counts as already gone. The wait uses
// loadBalancerTeardownTimeout, not kustomizationReconcileTimeout: cloud-provider LB deprovisioning
// is a longer, unrelated wait. On timeout the error names every object still present, since
// proceeding could wedge the terraform teardown.
func (k *BaseKubernetesManager) foregroundDeleteAndWaitService(target ownedTarget, svc *unstructured.Unstructured) error {
	policy := metav1.DeletePropagationForeground
	err := k.client.DeleteResource(target.gvr, target.namespace, target.name, metav1.DeleteOptions{PropagationPolicy: &policy})
	if err != nil && !isNotFoundError(err) {
		return fmt.Errorf("error foreground-deleting load balancer owner %s/%s: %w", target.namespace, target.name, err)
	}

	svcNamespace, svcName := svc.GetNamespace(), svc.GetName()
	rootIsService := target.gvr == servicesGVR && target.namespace == svcNamespace && target.name == svcName
	serviceGone := false
	rootGone := rootIsService

	timeout := time.Now().Add(k.loadBalancerTeardownTimeout)
	for time.Now().Before(timeout) {
		if !serviceGone {
			gone, err := k.resourceGone(servicesGVR, svcNamespace, svcName)
			if err != nil {
				return fmt.Errorf("error waiting for load balancer service %s/%s deletion: %w", svcNamespace, svcName, err)
			}
			serviceGone = gone
		}
		if !rootGone {
			gone, err := k.resourceGone(target.gvr, target.namespace, target.name)
			if err != nil {
				return fmt.Errorf("error waiting for load balancer owner %s/%s deletion: %w", target.namespace, target.name, err)
			}
			rootGone = gone
		}
		if serviceGone && rootGone {
			return nil
		}
		time.Sleep(k.kustomizationWaitPollInterval)
	}

	return foregroundDeleteTimeoutError(svcNamespace, svcName, serviceGone, target, rootGone)
}

// resourceGone reports whether a resource is gone (NotFound), or a non-NotFound error if the check
// itself failed. It backs the Service and root polls in foregroundDeleteAndWaitService.
func (k *BaseKubernetesManager) resourceGone(gvr schema.GroupVersionResource, namespace, name string) (bool, error) {
	if _, err := k.client.GetResource(gvr, namespace, name); err != nil {
		if isNotFoundError(err) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// foregroundDeleteTimeoutError builds the timeout error for foregroundDeleteAndWaitService, naming
// every object still present — the Service, the root, or both — so an operator is never pointed at
// the wrong one.
func foregroundDeleteTimeoutError(svcNamespace, svcName string, serviceGone bool, target ownedTarget, rootGone bool) error {
	var stuck []string
	var hints []string
	if !serviceGone {
		stuck = append(stuck, fmt.Sprintf("load balancer service %s/%s", svcNamespace, svcName))
		hints = append(hints, fmt.Sprintf("`kubectl get svc %s -n %s -o yaml`", svcName, svcNamespace))
	}
	if !rootGone {
		stuck = append(stuck, fmt.Sprintf("load balancer owner %s %s", target.gvr.Resource, target.name))
		hints = append(hints, fmt.Sprintf("`kubectl get %s %s%s -o yaml`", target.gvr.Resource, target.name, namespaceFlag(target.namespace)))
	}
	return fmt.Errorf("timeout waiting for load balancer teardown. %s still present. A finalizer has not lifted. Inspect with %s", strings.Join(stuck, " and "), strings.Join(hints, " and "))
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
// suspends every eligible Kustomization up front, since an un-deleted one that keeps
// reconciling can re-create a resource another component's Helm uninstall is mid-way
// through deleting, deadlocking it. It then resumes each one right before deleting it,
// since the finalizer only prunes inventory when not suspended — deleting while still
// suspended would orphan resources. A NotFound Kustomization is treated as success.
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

// waitForResumeReconcile polls up to kustomizationReconcileSleep for a Kustomization's
// status.observedGeneration to catch up to metadata.generation after a resume. A resume
// triggers a reconcile. Deleting before that reconcile settles can race
// kustomize-controller and strand the object mid-delete. This check is best effort. It
// returns on any read error or on timeout. Each sleep between polls is capped to the
// time left in the budget, so it never adds a full kustomizationWaitPollInterval on top
// of an already-expired deadline. deleteKustomization's own wait loop is the real safety
// net.
func (k *BaseKubernetesManager) waitForResumeReconcile(name, namespace string) {
	deadline := k.shims.TimeNow().Add(k.kustomizationReconcileSleep)
	for {
		obj, err := k.client.GetResource(kustomizationsGVR, namespace, name)
		if err != nil {
			return
		}
		if reconcileGenerationSettled(obj) {
			return
		}
		remaining := deadline.Sub(k.shims.TimeNow())
		if remaining <= 0 {
			return
		}
		k.shims.TimeSleep(min(k.kustomizationWaitPollInterval, remaining))
	}
}

// reconcileGenerationSettled reports whether a Kustomization's status.observedGeneration
// has caught up to metadata.generation. A match means the most recent spec change has
// been through a full reconcile.
func reconcileGenerationSettled(obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	observed, found, err := unstructured.NestedInt64(obj.Object, "status", "observedGeneration")
	if err != nil || !found {
		return false
	}
	return observed >= obj.GetGeneration()
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

// errUnverifiableInventory reports an entry neither confirmed live nor confirmed gone.
var errUnverifiableInventory = errors.New("inventory entry cannot be verified")

// blocksClusterTeardown reports whether a live object must be waited on before terraform destroys
// the cluster. A finalizer means a controller must still act, and a deletionTimestamp means
// something is already preventing the object from going. Anything else dies with the cluster.
// A finalizer is the only native way to require work before deletion, so a controller that owns
// state outside the cluster must use one; flux sets finalizers.fluxcd.io on anything it handles.
func blocksClusterTeardown(obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	if finalizers, found, err := unstructured.NestedStringSlice(obj.Object, "metadata", "finalizers"); err == nil && found && len(finalizers) > 0 {
		return true
	}
	deletionTimestamp, found, err := unstructured.NestedString(obj.Object, "metadata", "deletionTimestamp")
	return err == nil && found && deletionTimestamp != ""
}

// fluxReconcileAnnotationGroups are the groups whose controllers watch
// fluxReconcileAnnotation by contract. Any other group falls back to
// windsorReconcileAnnotation, which reaches a controller only if it filters nothing.
var fluxReconcileAnnotationGroups = map[string]bool{
	"kustomize.toolkit.fluxcd.io": true,
	"helm.toolkit.fluxcd.io":      true,
}

const (
	fluxReconcileAnnotation    = "reconcile.fluxcd.io/requestedAt"
	windsorReconcileAnnotation = "windsorcli.dev/reconcile-requested-at"
)

// triggerReconcile asks entry's own controller to look again. It asserts nothing
// about the object's state and never touches metadata.finalizers. Every controller
// MUST already tolerate an unsolicited reconcile, so this is always safe. A patch
// failure is not reported; the caller's own wait handles an entry that never
// clears regardless. See ADR 0009 Decision 6.
func (k *BaseKubernetesManager) triggerReconcile(entry *liveInventoryEntry) {
	key := windsorReconcileAnnotation
	if fluxReconcileAnnotationGroups[entry.Group] {
		key = fluxReconcileAnnotation
	}
	value := k.shims.TimeNow().UTC().Format(time.RFC3339Nano)
	patch := fmt.Appendf(nil, `{"metadata":{"annotations":{%q:%q}}}`, key, value)
	opts := metav1.PatchOptions{FieldManager: "windsor-cli"}
	_, _ = k.client.PatchResource(context.Background(), entry.gvr, entry.Namespace, entry.Name, types.MergePatchType, patch, opts)
}

// liveInventoryEntry is an inventory entry confirmed still live, paired with its
// resolved GVR. The GVR's Resource is the plural name kubectl accepts, not a guess
// from Kind.
type liveInventoryEntry struct {
	InventoryEntry
	gvr schema.GroupVersionResource
}

// firstLiveInventoryEntry returns the first inventory entry whose own live object is
// still present, or nil if every entry is confirmed absent. An entry whose API type no
// longer exists counts as gone. It returns an error, not a false negative, on an
// inconclusive lookup. A wrong "gone" reading could clear a finalizer or report a
// false clean delete.
//
// destroying MUST be true only right before terraform destroy. A finalizer-free object then
// counts as residue, since the cluster removes it anyway. Outside a destroy, such as
// PruneBlueprint's cleanup, the cluster keeps running. There a finalizer-free object still
// blocks, or it would never get cleaned up.
func (k *BaseKubernetesManager) firstLiveInventoryEntry(entries []InventoryEntry, helmInventory helmReleaseInventory, destroying bool) (*liveInventoryEntry, []InventoryEntry, error) {
	var surviving []InventoryEntry
	for _, entry := range entries {
		gvk := schema.GroupVersionKind{Group: entry.Group, Kind: entry.Kind}
		gvr, namespace, ok, err := k.resolveScopedGVR(gvk, entry.Namespace)
		if err != nil {
			return nil, surviving, err
		}
		if !ok {
			continue
		}
		obj, err := k.client.GetResource(gvr, namespace, entry.Name)
		if err != nil {
			if !isNotFoundError(err) {
				return nil, surviving, err
			}
			if !isHelmReleaseEntry(entry) {
				continue
			}
			blocking, childSurviving, err := k.firstLiveChartResource(entry, helmInventory, destroying)
			surviving = append(surviving, childSurviving...)
			if err != nil {
				return nil, surviving, err
			}
			if blocking != nil {
				return blocking, surviving, nil
			}
			continue
		}
		if destroying && !blocksClusterTeardown(obj) {
			surviving = append(surviving, entry)
			continue
		}
		return &liveInventoryEntry{InventoryEntry: entry, gvr: gvr}, surviving, nil
	}
	return nil, surviving, nil
}

// firstLiveChartResource verifies what a vanished HelmRelease managed. Its disappearance proves
// only that helm-controller cleared its own finalizer. It can do that after abandoning a stuck
// uninstall. So each resource the HelmRelease reported managing is checked. A HelmRelease never
// observed, or one whose inventory only partly decoded, is unverifiable. A nested HelmRelease is
// checked for liveness but not descended into. destroying gates blocksClusterTeardown: see
// firstLiveInventoryEntry.
func (k *BaseKubernetesManager) firstLiveChartResource(entry InventoryEntry, helmInventory helmReleaseInventory, destroying bool) (*liveInventoryEntry, []InventoryEntry, error) {
	known := helmInventory[helmReleaseKey(entry.Namespace, entry.Name)]
	if known == nil {
		return nil, nil, fmt.Errorf("windsor never read what helmrelease %s/%s managed: %w", entry.Namespace, entry.Name, errUnverifiableInventory)
	}
	if known.partial {
		return nil, nil, fmt.Errorf("helmrelease %s/%s reported an inventory windsor could not fully decode: %w", entry.Namespace, entry.Name, errUnverifiableInventory)
	}

	var surviving []InventoryEntry
	for _, child := range known.entries {
		gvk := schema.GroupVersionKind{Group: child.Group, Kind: child.Kind}
		gvr, namespace, ok, err := k.resolveScopedGVR(gvk, child.Namespace)
		if err != nil {
			return nil, surviving, err
		}
		if !ok {
			continue
		}
		obj, err := k.client.GetResource(gvr, namespace, child.Name)
		if err != nil {
			if isNotFoundError(err) {
				continue
			}
			return nil, surviving, err
		}
		if destroying && !blocksClusterTeardown(obj) {
			surviving = append(surviving, child)
			continue
		}
		return &liveInventoryEntry{InventoryEntry: child, gvr: gvr}, surviving, nil
	}
	return nil, surviving, nil
}

// describeAbandonedInventory checks lastObj's last-known inventory for an entry still live, after
// the Kustomization itself disappeared. It only applies to a WaitForTermination kustomization;
// see kustomizationDeletionPolicy. A MirrorPrune one is expected to leave live entries behind.
// It returns nil when there is no inventory to check. It returns an error when the answer is
// inconclusive. An inventory windsor cannot read is not one it can call empty. destroying is
// forwarded to firstLiveInventoryEntry unchanged.
func (k *BaseKubernetesManager) describeAbandonedInventory(lastObj *unstructured.Unstructured, expectWaitForTermination *bool, helmInventory helmReleaseInventory, destroying bool) (*liveInventoryEntry, []InventoryEntry, error) {
	waitForTermination, ok := kustomizationDeletionPolicy(lastObj, expectWaitForTermination)
	if !ok || !waitForTermination {
		return nil, nil, nil
	}
	entries, found, dropped := inventoryEntriesFromObject(lastObj)
	if !found {
		return nil, nil, nil
	}
	if dropped > 0 {
		return nil, nil, fmt.Errorf("%d of %d inventory entries could not be decoded: %w", dropped, dropped+len(entries), errUnverifiableInventory)
	}
	return k.firstLiveInventoryEntry(entries, helmInventory, destroying)
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

// describeInventoryVerdict renders what the inventory check found, for a timeout error. It names
// a still-live entry, or reports that the check could not conclude. Both add to whatever the
// Kustomization and its HelmReleases already said, since neither replaces the other.
func describeInventoryVerdict(live *liveInventoryEntry, checkErr error) string {
	switch {
	case live != nil:
		return fmt.Sprintf(". %s/%s from its inventory is still live; inspect it with %s", live.Kind, live.Name, liveEntryInspectCmd(live))
	case checkErr != nil:
		return fmt.Sprintf(". Windsor could not confirm its resources are gone: %v", checkErr)
	default:
		return ""
	}
}

// reportSurvivingResources warns about objects a delete left behind that hold nothing back. They
// die with the cluster, so they do not stop a destroy, but they are evidence a chart did not
// clean up after itself.
func reportSurvivingResources(namespace, name string, surviving []InventoryEntry) {
	if len(surviving) == 0 {
		return
	}
	named := make([]string, 0, len(surviving))
	for _, entry := range surviving {
		named = append(named, entry.Kind+"/"+entry.Name)
	}
	slices.Sort(named)
	fmt.Fprintf(os.Stderr, "warning: kustomization %s/%s left %d resource(s) behind, which will go with the cluster: %s\n", namespace, name, len(named), strings.Join(named, ", "))
}

// liveEntryInspectCmd builds the kubectl hint that names a still-live inventory entry.
func liveEntryInspectCmd(entry *liveInventoryEntry) string {
	if entry.Namespace == "" {
		return fmt.Sprintf("`kubectl get %s %s`", entry.gvr.Resource, entry.Name)
	}
	return fmt.Sprintf("`kubectl get %s %s -n %s`", entry.gvr.Resource, entry.Name, entry.Namespace)
}

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

// namespaceFlag formats a kubectl -n flag for a diagnostic hint. It returns an empty string for a
// cluster-scoped resource, so a GatewayClass or other cluster-scoped root never gets -n "".
func namespaceFlag(namespace string) string {
	if namespace == "" {
		return ""
	}
	return fmt.Sprintf(" -n %s", namespace)
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
