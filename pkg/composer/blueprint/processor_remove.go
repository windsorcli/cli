// The processor_remove file holds the remove steps of the BlueprintProcessor.
// A remove-strategy entry deletes only the fields that it names. It never deletes a whole entry.
// The facet ordinal decides between a removal and a merge or replace entry of the same source.
// The removals of a source also apply to the sources that the processor handles before it.

package blueprint

import (
	"math"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

// minOrdinal is the lowest ordinal, so removalItems with it returns every removal.
const minOrdinal = math.MinInt

// =============================================================================
// Types
// =============================================================================

// removal is one remove-strategy entry and the effective ordinal of its facet.
type removal[T any] struct {
	item    T
	ordinal int
}

// =============================================================================
// Private Methods
// =============================================================================

// addTerraformRemoval records a remove-strategy terraform component in collected. It strips the
// named fields from the collected entry with the same ID when that entry has an equal or lower
// ordinal. The trace keeps the contributions of the entry and of the removal.
func (p *BaseBlueprintProcessor) addTerraformRemoval(collected collectedComponents, componentID string, entry blueprintv1alpha1.ConditionalTerraformComponent, entryKey string) error {
	r := removal[blueprintv1alpha1.TerraformComponent]{item: entry.TerraformComponent, ordinal: ordinalOrZero(entry.Ordinal)}
	if existing := collected.terraformByID[componentID]; existing != nil && removalApplies(ordinalOrZero(existing.Ordinal), r.ordinal) {
		stripped, err := stripTerraformComponent(existing.TerraformComponent, []blueprintv1alpha1.TerraformComponent{r.item})
		if err != nil {
			return werror.Wrap(err, "error removing fields from terraform component '%s'", componentID)
		}
		existing.TerraformComponent = stripped
	}
	collected.terraformRemovals[componentID] = append(collected.terraformRemovals[componentID], r)
	p.keepEntryTrace(entryKey)
	return nil
}

// addKustomizationRemoval records a remove-strategy kustomization in collected. It strips the named
// fields from the collected entry with the same name when that entry has an equal or lower ordinal.
// The trace keeps the contributions of the entry and of the removal.
func (p *BaseBlueprintProcessor) addKustomizationRemoval(collected collectedComponents, entry blueprintv1alpha1.ConditionalKustomization, entryKey string) error {
	r := removal[blueprintv1alpha1.Kustomization]{item: entry.Kustomization, ordinal: ordinalOrZero(entry.Ordinal)}
	if existing := collected.kustomizationByName[entry.Name]; existing != nil && removalApplies(ordinalOrZero(existing.Ordinal), r.ordinal) {
		stripped, err := stripKustomization(existing.Kustomization, []blueprintv1alpha1.Kustomization{r.item})
		if err != nil {
			return werror.Wrap(err, "error removing fields from kustomization '%s'", entry.Name)
		}
		existing.Kustomization = stripped
	}
	collected.kustomizationRemovals[entry.Name] = append(collected.kustomizationRemovals[entry.Name], r)
	p.keepEntryTrace(entryKey)
	return nil
}

// addFluxSystemRemoval records a remove-strategy flux system in collected. It strips the named fields
// from the collected system with the same name when that system has an equal or lower ordinal. The
// trace keeps the contributions of the system and of the removal.
func (p *BaseBlueprintProcessor) addFluxSystemRemoval(collected collectedComponents, system blueprintv1alpha1.FluxSystem, entryKey string) {
	system.Strategy = ""
	r := removal[blueprintv1alpha1.FluxSystem]{item: system, ordinal: ordinalOrZero(system.Ordinal)}
	r.item.Ordinal = nil
	if existing := collected.fluxSystemByName[system.Name]; existing != nil && removalApplies(ordinalOrZero(existing.Ordinal), r.ordinal) {
		*existing = stripFluxSystem(*existing, []blueprintv1alpha1.FluxSystem{r.item})
	}
	collected.fluxSystemRemovals[system.Name] = append(collected.fluxSystemRemovals[system.Name], r)
	p.keepEntryTrace(entryKey)
}

// keepEntryTrace tells the trace collector to keep all contributions of the entry. A removal changes
// fields of an entry but does not replace the entry.
func (p *BaseBlueprintProcessor) keepEntryTrace(entryKey string) {
	if p.traceCollector != nil {
		p.traceCollector.SettleEntry(entryKey, true, true)
	}
}

// removals returns all removals in collected, grouped by kind and in name order.
func (c collectedComponents) removals() sourceRemovals {
	var out sourceRemovals
	for _, id := range sortedKeys(c.terraformRemovals) {
		out.terraformComponents = append(out.terraformComponents, removalItems(c.terraformRemovals[id], minOrdinal)...)
	}
	for _, name := range sortedKeys(c.kustomizationRemovals) {
		out.kustomizations = append(out.kustomizations, removalItems(c.kustomizationRemovals[name], minOrdinal)...)
	}
	for _, name := range sortedKeys(c.fluxSystemRemovals) {
		out.fluxSystems = append(out.fluxSystems, removalItems(c.fluxSystemRemovals[name], minOrdinal)...)
	}
	return out
}

// =============================================================================
// Helpers
// =============================================================================

// removalApplies reports whether a removal with removalOrdinal strips the fields of an entry with
// entryOrdinal. A removal applies to an entry with an equal or lower ordinal.
func removalApplies(entryOrdinal, removalOrdinal int) bool {
	return entryOrdinal <= removalOrdinal
}

// removalItems returns the items of the removals that apply to an entry with the given ordinal.
func removalItems[T any](removals []removal[T], ordinal int) []T {
	var items []T
	for _, r := range removals {
		if removalApplies(ordinal, r.ordinal) {
			items = append(items, r.item)
		}
	}
	return items
}

// stripTerraformComponent returns a copy of component without the fields that the removals name.
func stripTerraformComponent(component blueprintv1alpha1.TerraformComponent, removals []blueprintv1alpha1.TerraformComponent) (blueprintv1alpha1.TerraformComponent, error) {
	if len(removals) == 0 {
		return component, nil
	}
	tmp := &blueprintv1alpha1.Blueprint{TerraformComponents: []blueprintv1alpha1.TerraformComponent{*component.DeepCopy()}}
	for _, r := range removals {
		r.Name, r.Path = component.Name, component.Path
		if err := tmp.RemoveTerraformComponent(r); err != nil {
			return component, err
		}
	}
	return tmp.TerraformComponents[0], nil
}

// stripKustomization returns a copy of k without the fields that the removals name.
func stripKustomization(k blueprintv1alpha1.Kustomization, removals []blueprintv1alpha1.Kustomization) (blueprintv1alpha1.Kustomization, error) {
	if len(removals) == 0 {
		return k, nil
	}
	tmp := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{*k.DeepCopy()}}
	for _, r := range removals {
		if err := tmp.RemoveKustomization(r); err != nil {
			return k, err
		}
	}
	return tmp.Kustomizations[0], nil
}

// stripFluxSystem returns a copy of sys without the fields that the removals name. It keeps the
// strategy and ordinal of sys.
func stripFluxSystem(sys blueprintv1alpha1.FluxSystem, removals []blueprintv1alpha1.FluxSystem) blueprintv1alpha1.FluxSystem {
	if len(removals) == 0 {
		return sys
	}
	tmp := &blueprintv1alpha1.Blueprint{FluxSystems: []blueprintv1alpha1.FluxSystem{*sys.DeepCopy()}}
	for _, r := range removals {
		_ = tmp.RemoveFluxSystem(r)
	}
	return tmp.FluxSystems[0]
}

// applyRemovals applies the terraform, kustomization, and flux system removals to target. A removal
// has no effect when target has no entry with the same ID or name.
func applyRemovals(target *blueprintv1alpha1.Blueprint, removals sourceRemovals) error {
	for _, r := range removals.terraformComponents {
		if err := target.RemoveTerraformComponent(r); err != nil {
			return werror.Wrap(err, "error removing terraform component '%s'", r.GetID())
		}
	}
	for _, r := range removals.kustomizations {
		if err := target.RemoveKustomization(r); err != nil {
			return werror.Wrap(err, "error removing kustomization '%s'", r.Name)
		}
	}
	for _, r := range removals.fluxSystems {
		if err := target.RemoveFluxSystem(r); err != nil {
			return werror.Wrap(err, "error removing flux system '%s'", r.Name)
		}
	}
	return nil
}

// applyRemovalsToShallowerSources applies the removals of each source to the target of each source
// with a lower depth. A source with a higher depth overrides a source with a lower depth. Thus a
// facet can remove the fields that an earlier source contributes.
func applyRemovalsToShallowerSources(sources []SourceFacetSet, removalsBySource map[string]sourceRemovals) error {
	for _, src := range sources {
		removals, ok := removalsBySource[src.Name]
		if !ok {
			continue
		}
		for _, other := range sources {
			if other.Depth >= src.Depth || other.Target == nil {
				continue
			}
			if err := applyRemovals(other.Target, removals); err != nil {
				return werror.Wrap(err, "error applying removals to source '%s'", other.Name)
			}
		}
	}
	return nil
}
