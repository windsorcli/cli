// The kubernetes_manager_inventory file holds the Flux inventory reads of the BaseKubernetesManager.
// It provides the decode of status.inventory entries and of Flux inventory IDs.
// It records the inventory of each HelmRelease while the HelmRelease is still readable.
// It drops and counts an entry that does not decode, so a caller can tell a partial list from an empty one.

package kubernetes

import (
	"strings"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/windsorcli/cli/internal/werror"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// =============================================================================
// Types
// =============================================================================

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

// =============================================================================
// Public Methods
// =============================================================================

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
		return nil, werror.Wrap(err, "error reading inventory for kustomization %q in namespace %q", name, namespace)
	}
	if !found {
		return []InventoryEntry{}, nil
	}
	entries, _ := decodeInventoryEntries(rawEntries)
	return entries, nil
}

// =============================================================================
// Private Methods
// =============================================================================

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

// =============================================================================
// Helpers
// =============================================================================

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

// helmReleaseKey identifies a HelmRelease within a helmReleaseInventory.
func helmReleaseKey(namespace, name string) string {
	return namespace + "/" + name
}

// isHelmReleaseEntry reports whether entry names a flux HelmRelease.
func isHelmReleaseEntry(entry InventoryEntry) bool {
	return entry.Group == "helm.toolkit.fluxcd.io" && entry.Kind == "HelmRelease"
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

// inventoryKey builds the group/kind/namespace/name key used to match live objects against a
// kustomization's Flux inventory. Group is empty for core API objects; namespace is empty for
// cluster-scoped resources.
func inventoryKey(group, kind, namespace, name string) string {
	return strings.Join([]string{group, kind, namespace, name}, "|")
}
