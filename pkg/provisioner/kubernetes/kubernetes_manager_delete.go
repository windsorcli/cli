// The kubernetes_manager_delete file holds the Kustomization delete of the BaseKubernetesManager.
// It provides the delete wait, with a timeout that grows with the inventory and the declared timeouts.
// It checks the last known inventory for live objects after the Kustomization disappears.
// It asks the controller of a live object to reconcile again before it reports an abandoned object.

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// =============================================================================
// Constants
// =============================================================================

// abandonedInventoryGraceChecks is the minimum number of polls for a live entry after its Kustomization disappears.
const abandonedInventoryGraceChecks = 3

// errUnverifiableInventory reports an entry neither confirmed live nor confirmed gone.
var errUnverifiableInventory = errors.New("inventory entry cannot be verified")

// fluxReconcileAnnotationGroups are the groups whose controllers watch fluxReconcileAnnotation.
var fluxReconcileAnnotationGroups = map[string]bool{
	"kustomize.toolkit.fluxcd.io": true,
	"helm.toolkit.fluxcd.io":      true,
}

const (
	fluxReconcileAnnotation    = "reconcile.fluxcd.io/requestedAt"
	windsorReconcileAnnotation = "windsorcli.dev/reconcile-requested-at"
)

// =============================================================================
// Types
// =============================================================================

// liveInventoryEntry is an inventory entry confirmed still live, paired with its
// resolved GVR. The GVR's Resource is the plural name kubectl accepts, not a guess
// from Kind.
type liveInventoryEntry struct {
	InventoryEntry
	gvr schema.GroupVersionResource
}

// =============================================================================
// Public Methods
// =============================================================================

// DeleteKustomization deletes a Kustomization and waits until it disappears. The wait grows to
// spec.timeout and with the inventory size. After the wait, it checks the last known inventory,
// and gives a live entry a few more polls before it reports that entry. The cluster stays up, so
// an object without a finalizer that is still live also counts as a leftover.
func (k *BaseKubernetesManager) DeleteKustomization(ctx context.Context, name, namespace string) error {
	return k.deleteKustomization(ctx, name, namespace, nil, nil, false)
}

// =============================================================================
// Private Methods
// =============================================================================

// deleteKustomization is DeleteKustomization with a known destroy expectation and a delete-wait
// override, both optional. expectWaitForTermination overrides the object's own deletionPolicy,
// and deleteTimeoutOverride replaces every spec-derived timeout floor, keeping an explicit
// DeleteTimeout a bound. It snapshots every HelmRelease's inventory once before requesting the
// delete, so a fast chart teardown still leaves a verifiable record. The wait tolerates
// kustomizationWaitMaxReadFailures read errors; destroying MUST be true only during a full teardown.
func (k *BaseKubernetesManager) deleteKustomization(ctx context.Context, name, namespace string, expectWaitForTermination *bool, deleteTimeoutOverride *time.Duration, destroying bool) error {
	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}

	helmInventory := helmReleaseInventory{}
	if preObj, err := k.client.GetResource(gvr, namespace, name); err == nil {
		k.snapshotHelmReleaseInventories(preObj, helmInventory)
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
		return werror.Wrap(err, "error deleting kustomization %s/%s", namespace, name)
	}

	start := k.shims.TimeNow()
	waitFor := k.kustomizationReconcileTimeout
	var lastObj *unstructured.Unstructured
	var lastReadErr error
	readFailures := 0
	helmTimeoutResolved := false
	for k.shims.TimeNow().Before(start.Add(waitFor)) {
		obj, err := k.client.GetResource(gvr, namespace, name)
		if err != nil && isNotFoundError(err) {
			return k.handleKustomizationDisappeared(ctx, name, namespace, lastObj, readFailures, lastReadErr, expectWaitForTermination, helmInventory, destroying)
		}
		if err != nil {
			readFailures++
			lastReadErr = err
			if readFailures > k.kustomizationWaitMaxReadFailures {
				return werror.Wrap(err, "error checking kustomization %s/%s deletion status", namespace, name)
			}
			presenter.Progress(ctx, fmt.Sprintf("retrying after a read error (%d/%d)", readFailures, k.kustomizationWaitMaxReadFailures))
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

	if waitForTermination, ok := kustomizationDeletionPolicy(lastObj, expectWaitForTermination); !ok || waitForTermination {
		if preEntries, _, _ := inventoryEntriesFromObject(lastObj); len(preEntries) > 0 {
			if live, _, checkErr := k.firstLiveInventoryEntry(preEntries, helmInventory, destroying); checkErr == nil && live != nil {
				k.triggerReconcile(live)
				presenter.Progress(ctx, fmt.Sprintf("kustomization %s/%s timed out with %s/%s still live; asked it to reconcile again", namespace, name, live.Kind, live.Name))
				recheckDeadline := k.shims.TimeNow().Add(time.Duration(abandonedInventoryGraceChecks) * k.kustomizationWaitPollInterval)
				for k.shims.TimeNow().Before(recheckDeadline) {
					k.shims.TimeSleep(k.kustomizationWaitPollInterval)
					obj, err := k.client.GetResource(gvr, namespace, name)
					if err != nil && isNotFoundError(err) {
						return k.handleKustomizationDisappeared(ctx, name, namespace, lastObj, readFailures, lastReadErr, expectWaitForTermination, helmInventory, destroying)
					}
					if err == nil {
						lastObj = obj
						k.snapshotHelmReleaseInventories(obj, helmInventory)
					}
				}
			}
		}
	}

	inspectCmd := fmt.Sprintf("`kubectl get kustomization %s -n %s -o yaml`", name, namespace)
	const terminatingCmd = "`kubectl get pvc,svc,ingress,certificate -A | grep Terminating`"

	entries, inventoryFound, dropped := inventoryEntriesFromObject(lastObj)
	live, surviving, checkErr := k.firstLiveInventoryEntry(entries, helmInventory, destroying)
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

// handleKustomizationDisappeared checks the last-known inventory of a kustomization that is gone.
// deleteKustomization calls it from its wait loop and from its recheck after a timeout.
// It triggers a reconcile on a live entry that blocks, and retries until the grace window ends.
// It returns a WindsorError with a hint when it cannot confirm that the resources are gone.
func (k *BaseKubernetesManager) handleKustomizationDisappeared(ctx context.Context, name, namespace string, lastObj *unstructured.Unstructured, readFailures int, lastReadErr error, expectWaitForTermination *bool, helmInventory helmReleaseInventory, destroying bool) error {
	if lastObj == nil && readFailures > 0 {
		return werror.New(fmt.Sprintf("kustomization %s/%s disappeared before windsor could read its inventory, so windsor cannot confirm that its resources are gone: %v", namespace, name, lastReadErr), "Find leftovers with `kubectl get pvc,svc,ingress,certificate -A | grep Terminating`, then retry.", lastReadErr)
	}
	entry, _, checkErr := k.describeAbandonedInventory(lastObj, expectWaitForTermination, helmInventory, destroying)
	graceDeadline := k.shims.TimeNow().Add(k.abandonedInventoryGraceWindow(inventorySize(lastObj)))
	for (entry != nil || checkErr != nil) && k.shims.TimeNow().Before(graceDeadline) {
		if errors.Is(checkErr, errUnverifiableInventory) {
			break
		}
		if entry != nil {
			k.triggerReconcile(entry)
			presenter.Progress(ctx, fmt.Sprintf("kustomization %s/%s disappeared with %s/%s still live; asked it to reconcile again", namespace, name, entry.Kind, entry.Name))
		}
		k.shims.TimeSleep(k.kustomizationWaitPollInterval)
		entry, _, checkErr = k.describeAbandonedInventory(lastObj, expectWaitForTermination, helmInventory, destroying)
	}
	if checkErr != nil {
		return werror.New(fmt.Sprintf("kustomization %s/%s disappeared and windsor could not confirm its resources are gone: %v", namespace, name, checkErr), "Inspect the namespace, then retry.", checkErr)
	}
	if entry == nil {
		return nil
	}
	return werror.New(fmt.Sprintf("kustomization %s/%s disappeared, but %s/%s from its inventory is still live", namespace, name, entry.Kind, entry.Name), fmt.Sprintf("Flux probably removed the finalizer early. Inspect the object with %s, then retry.", liveEntryInspectCmd(entry)), nil)
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

// triggerReconcile asks the controller of entry to reconcile it again, through a reconcile annotation.
// It never changes metadata.finalizers, and every controller MUST accept an extra reconcile.
// It ignores a patch failure, because the wait of the caller handles an entry that does not clear.
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

// firstLiveInventoryEntry returns the first entry whose object is still live, or nil when all are gone.
// An entry whose API type does not exist counts as gone. An inconclusive lookup returns an error,
// because a false "gone" could clear a finalizer or report a false clean delete. destroying MUST
// be true only just before terraform destroy: then an object without a finalizer is residue that
// the cluster removes, and it goes in the surviving list instead.
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
		return nil, nil, werror.Wrap(errUnverifiableInventory, "windsor never read what helmrelease %s/%s managed", entry.Namespace, entry.Name)
	}
	if known.partial {
		return nil, nil, werror.Wrap(errUnverifiableInventory, "helmrelease %s/%s reported an inventory windsor could not fully decode", entry.Namespace, entry.Name)
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
		return nil, nil, werror.Wrap(errUnverifiableInventory, "%d of %d inventory entries could not be decoded", dropped, dropped+len(entries))
	}
	return k.firstLiveInventoryEntry(entries, helmInventory, destroying)
}

// =============================================================================
// Helpers
// =============================================================================

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

// liveEntryInspectCmd builds the kubectl hint that names a still-live inventory entry.
func liveEntryInspectCmd(entry *liveInventoryEntry) string {
	if entry.Namespace == "" {
		return fmt.Sprintf("`kubectl get %s %s`", entry.gvr.Resource, entry.Name)
	}
	return fmt.Sprintf("`kubectl get %s %s -n %s`", entry.gvr.Resource, entry.Name, entry.Namespace)
}
