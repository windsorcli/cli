// The processor_merge file holds the merge steps of the BlueprintProcessor.
// It provides the merge of collected entries by ordinal and by remove, replace, or merge strategy.
// It applies the collected components, kustomizations, and flux systems to the target blueprint.
// It drops entries whose when condition is false after the merge.

package blueprint

import (
	"fmt"
	"sort"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

// strategyHint is the remediation for a strategy value that is not merge, replace, or remove.
const strategyHint = "Set strategy to merge, replace, or remove."

// strategyPrecedence is the tiebreaker when component ordinals are equal: remove > replace > merge.
var strategyPrecedence = map[string]int{
	"remove":  3,
	"replace": 2,
	"merge":   1,
}

// =============================================================================
// Private Methods
// =============================================================================

// updateFluxSystemEntry merges a new FluxSystem into entries by the rules of updateKustomizationEntry.
// A higher ordinal wins through applyFluxSystemEntryByStrategy, so merge keeps merging; when either
// side is remove, new replaces the entry. Equal ordinals use remove > replace > merge. A winning remove
// stays in entries as a descriptor, and applyCollectedComponents applies it after the merges. A lower
// ordinal still merges, with the higher-ordinal entry winning scalar conflicts.
func (p *BaseBlueprintProcessor) updateFluxSystemEntry(name string, new *blueprintv1alpha1.FluxSystem, strategy string, entries map[string]*blueprintv1alpha1.FluxSystem) error {
	existing := entries[name]
	existingStrategy := existing.Strategy
	if existingStrategy == "" {
		existingStrategy = "merge"
	}

	newStrategyPrec, ok := strategyPrecedence[strategy]
	if !ok {
		return werror.New(fmt.Sprintf("invalid strategy '%s' for flux system '%s'", strategy, name), strategyHint, nil)
	}

	newOrdinal := ordinalOrZero(new.Ordinal)
	existingOrdinal := ordinalOrZero(existing.Ordinal)

	if newOrdinal > existingOrdinal {
		if existingStrategy == "remove" || strategy == "remove" {
			new.Strategy = strategy
			entries[name] = new
			return nil
		}
		return p.applyFluxSystemEntryByStrategy(name, new, strategy, existing, entries)
	}
	if newOrdinal < existingOrdinal {
		if strategy != "merge" || existingStrategy == "remove" {
			return nil
		}
		return p.applyFluxSystemEntryByStrategy(name, existing, "merge", new, entries)
	}

	existingStrategyPrec := strategyPrecedence[existingStrategy]
	if newStrategyPrec > existingStrategyPrec {
		new.Strategy = strategy
		entries[name] = new
		return nil
	}
	if newStrategyPrec < existingStrategyPrec {
		return nil
	}

	return p.applyFluxSystemEntryByStrategy(name, new, strategy, existing, entries)
}

// applyFluxSystemEntryByStrategy applies new to the slot of existing in entries by strategy. Replace
// swaps the whole entry. Remove builds a combined field-level removal descriptor with
// accumulateFluxSystemRemovals, and never deletes the whole system. Merge unions DependsOn and
// deep-merges Install and the Resources variants, with the helpers that Blueprint.UpsertFluxSystem uses.
func (p *BaseBlueprintProcessor) applyFluxSystemEntryByStrategy(name string, new *blueprintv1alpha1.FluxSystem, strategy string, existing *blueprintv1alpha1.FluxSystem, entries map[string]*blueprintv1alpha1.FluxSystem) error {
	switch strategy {
	case "replace":
		new.Strategy = strategy
		entries[name] = new
	case "remove":
		accumulated := p.accumulateFluxSystemRemovals(*existing, *new)
		accumulated.Strategy = "remove"
		accumulated.Ordinal = new.Ordinal
		entries[name] = &accumulated
	case "merge":
		merged := *existing
		merged.DependsOn = accumulateStringSlice(existing.DependsOn, new.DependsOn)
		merged.Install = blueprintv1alpha1.MergeFluxInstall(existing.Install, new.Install)
		merged.Resources = blueprintv1alpha1.MergeFluxVariants(existing.Resources, new.Resources)
		merged.Secrets = mergeSecretData(existing.Secrets, new.Secrets)
		merged.Ordinal = new.Ordinal
		merged.GlobalDependency = existing.GlobalDependency || new.GlobalDependency
		entries[name] = &merged
	}
	return nil
}

// setCrdLayer records the deduped, sorted CRD references for sourceName. Refs from the default/project
// source (empty sourceName) land on the blueprint's flat crds: list; refs from a named source land on
// that Source's crds (creating the Source entry if the loader blueprint has not declared it yet), so
// they ride with the source and install in the background when it is install:true. Either way the
// references are authored as a bare scalar list; the source attribution is derived from which source's
// facets declared them, never hand-written.
func setCrdLayer(target *blueprintv1alpha1.Blueprint, crdRefs map[string]struct{}, sourceName string) {
	if len(crdRefs) == 0 {
		return
	}
	if sourceName == "" {
		target.Crds = unionCrdRefs(target.Crds, crdRefs)
		return
	}
	for i := range target.Sources {
		if target.Sources[i].Name == sourceName {
			target.Sources[i].Crds = unionCrdRefs(target.Sources[i].Crds, crdRefs)
			return
		}
	}
	target.Sources = append(target.Sources, blueprintv1alpha1.Source{
		Name: sourceName,
		Crds: unionCrdRefs(nil, crdRefs),
	})
}

// unionCrdRefs returns existing unioned with refs, deduped and sorted. Sorting keeps the composed
// output stable across edits so diffs and the synthesized kustomization stay deterministic.
func unionCrdRefs(existing []string, refs map[string]struct{}) []string {
	seen := make(map[string]struct{}, len(existing)+len(refs))
	out := make([]string, 0, len(existing)+len(refs))
	for _, ref := range existing {
		if _, ok := seen[ref]; !ok {
			seen[ref] = struct{}{}
			out = append(out, ref)
		}
	}
	for ref := range refs {
		if _, ok := seen[ref]; !ok {
			seen[ref] = struct{}{}
			out = append(out, ref)
		}
	}
	sort.Strings(out)
	return out
}

// updateTerraformComponentEntry merges a new component into an existing entry by ordinal, strategy,
// and when. A higher ordinal wins for replace and remove. Equal ordinals use remove > replace > merge.
// A lower ordinal still merges, with the higher-ordinal entry winning scalar conflicts. An entry whose
// when is now false is dropped.
func (p *BaseBlueprintProcessor) updateTerraformComponentEntry(
	componentID string,
	new *blueprintv1alpha1.ConditionalTerraformComponent,
	strategy string,
	entries map[string]*blueprintv1alpha1.ConditionalTerraformComponent,
	scope map[string]any,
) error {
	existing := entries[componentID]
	existingStrategy := existing.Strategy
	if existingStrategy == "" {
		existingStrategy = "merge"
	}

	if existing.When == "" && new.When != "" {
		shouldInclude, err := p.shouldIncludeComponent(new.When, "", scope)
		if err == nil && !shouldInclude {
			delete(entries, componentID)
			return nil
		}
	} else if existing.When != "" {
		shouldInclude, err := p.shouldIncludeComponent(existing.When, "", scope)
		if err == nil && !shouldInclude {
			delete(entries, componentID)
			return nil
		}
		if new.When != "" {
			shouldInclude, err := p.shouldIncludeComponent(new.When, "", scope)
			if err == nil && !shouldInclude {
				delete(entries, componentID)
				return nil
			}
		}
	}

	newStrategyPrec, exists := strategyPrecedence[strategy]
	if !exists {
		return werror.New(fmt.Sprintf("invalid strategy '%s' for terraform component '%s'", strategy, componentID), strategyHint, nil)
	}

	newOrdinal := ordinalOrZero(new.Ordinal)
	existingOrdinal := ordinalOrZero(existing.Ordinal)

	if newOrdinal > existingOrdinal {
		if strategy == "replace" {
			new.Strategy = strategy
			entries[componentID] = new
			return nil
		}
		if strategy == "remove" && existingStrategy != "remove" {
			new.Strategy = strategy
			entries[componentID] = new
			return nil
		}
		return p.applyTerraformComponentEntryByStrategy(componentID, new, existing, strategy, entries)
	}

	if newOrdinal < existingOrdinal {
		if strategy != "merge" || existingStrategy == "remove" {
			return nil
		}
		return p.applyTerraformComponentEntryByStrategy(componentID, existing, new, "merge", entries)
	}
	existingStrategyPrec := strategyPrecedence[existingStrategy]
	if newStrategyPrec > existingStrategyPrec {
		new.Strategy = strategy
		entries[componentID] = new
		return nil
	}

	if newStrategyPrec < existingStrategyPrec {
		return nil
	}

	return p.applyTerraformComponentEntryByStrategy(componentID, new, existing, strategy, entries)
}

// applyTerraformComponentEntryByStrategy applies new over existing in entries by strategy. An empty strategy means merge.
// replace stores new. merge does a strategic merge of new over existing. remove accumulates the removals.
// merge and remove combine the when expressions of both entries with &&.
// An unknown strategy returns an error with a remediation hint.
func (p *BaseBlueprintProcessor) applyTerraformComponentEntryByStrategy(
	componentID string,
	new *blueprintv1alpha1.ConditionalTerraformComponent,
	existing *blueprintv1alpha1.ConditionalTerraformComponent,
	strategy string,
	entries map[string]*blueprintv1alpha1.ConditionalTerraformComponent,
) error {
	if strategy == "" {
		strategy = "merge"
	}
	combinedWhen := ""
	if existing.When != "" && new.When != "" {
		combinedWhen = fmt.Sprintf("(%s) && (%s)", existing.When, new.When)
	} else if existing.When != "" {
		combinedWhen = existing.When
	} else if new.When != "" {
		combinedWhen = new.When
	}
	switch strategy {
	case "replace":
		new.Strategy = strategy
		entries[componentID] = new
		return nil
	case "merge":
		tempBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{existing.TerraformComponent},
		}
		mergedBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{new.TerraformComponent},
		}
		if err := tempBp.StrategicMerge(mergedBp); err != nil {
			return werror.Wrap(err, "error pre-merging terraform component '%s'", componentID)
		}
		entries[componentID] = &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: tempBp.TerraformComponents[0],
			Strategy:           "merge",
			Ordinal:            new.Ordinal,
			When:               combinedWhen,
		}
		return nil
	case "remove":
		accumulated := p.accumulateTerraformRemovals(existing.TerraformComponent, new.TerraformComponent)
		entries[componentID] = &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: accumulated,
			Strategy:           "remove",
			Ordinal:            new.Ordinal,
			When:               combinedWhen,
		}
		return nil
	default:
		return werror.New(fmt.Sprintf("invalid strategy '%s' for terraform component '%s'", strategy, componentID), strategyHint, nil)
	}
}

// updateKustomizationEntry merges a new kustomization into an existing entry by ordinal, then by
// remove > replace > merge. At equal ordinal and strategy, merge pre-merges the two, remove combines
// the removals, and replace swaps the entry. A lower ordinal still merges, with the higher-ordinal
// entry winning scalar conflicts. A merge failure returns an error.
func (p *BaseBlueprintProcessor) updateKustomizationEntry(name string, new *blueprintv1alpha1.ConditionalKustomization, strategy string, entries map[string]*blueprintv1alpha1.ConditionalKustomization, scope map[string]any) error {
	existing := entries[name]
	existingStrategy := existing.Strategy
	if existingStrategy == "" {
		existingStrategy = "merge"
	}

	if existing.When == "" && new.When != "" {
		shouldInclude, err := p.shouldIncludeComponent(new.When, "", scope)
		if err == nil && !shouldInclude {
			delete(entries, name)
			return nil
		}
	} else if existing.When != "" {
		shouldInclude, err := p.shouldIncludeComponent(existing.When, "", scope)
		if err == nil && !shouldInclude {
			delete(entries, name)
			return nil
		}
		if new.When != "" {
			shouldInclude, err := p.shouldIncludeComponent(new.When, "", scope)
			if err == nil && !shouldInclude {
				delete(entries, name)
				return nil
			}
		}
	}

	newStrategyPrec, exists := strategyPrecedence[strategy]
	if !exists {
		return werror.New(fmt.Sprintf("invalid strategy '%s' for kustomization '%s'", strategy, name), strategyHint, nil)
	}

	newOrdinal := ordinalOrZero(new.Ordinal)
	existingOrdinal := ordinalOrZero(existing.Ordinal)

	if newOrdinal > existingOrdinal {
		if existingStrategy == "remove" || strategy == "remove" {
			new.Strategy = strategy
			entries[name] = new
			return nil
		}
		return p.applyKustomizationEntryByStrategy(name, new, strategy, existing, entries)
	}

	if newOrdinal < existingOrdinal {
		if strategy != "merge" || existingStrategy == "remove" {
			return nil
		}
		return p.applyKustomizationEntryByStrategy(name, existing, "merge", new, entries)
	}
	existingStrategyPrec := strategyPrecedence[existingStrategy]
	if newStrategyPrec > existingStrategyPrec {
		new.Strategy = strategy
		entries[name] = new
		return nil
	}

	if newStrategyPrec < existingStrategyPrec {
		return nil
	}

	return p.applyKustomizationEntryByStrategy(name, new, strategy, existing, entries)
}

// applyKustomizationEntryByStrategy applies new over existing in entries by strategy. An empty strategy means merge.
// replace stores new. merge does a strategic merge of new over existing. remove accumulates the removals.
// merge and remove combine the when expressions of both entries with &&.
// An unknown strategy returns an error with a remediation hint.
func (p *BaseBlueprintProcessor) applyKustomizationEntryByStrategy(name string, new *blueprintv1alpha1.ConditionalKustomization, strategy string, existing *blueprintv1alpha1.ConditionalKustomization, entries map[string]*blueprintv1alpha1.ConditionalKustomization) error {
	if strategy == "" {
		strategy = "merge"
	}
	combinedWhen := ""
	if existing.When != "" && new.When != "" {
		combinedWhen = fmt.Sprintf("(%s) && (%s)", existing.When, new.When)
	} else if existing.When != "" {
		combinedWhen = existing.When
	} else if new.When != "" {
		combinedWhen = new.When
	}
	switch strategy {
	case "replace":
		new.Strategy = strategy
		entries[name] = new
		return nil
	case "merge":
		tempBp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{existing.Kustomization},
		}
		mergedBp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{new.Kustomization},
		}
		if err := tempBp.StrategicMerge(mergedBp); err != nil {
			return werror.Wrap(err, "error pre-merging kustomization '%s'", name)
		}
		entries[name] = &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: tempBp.Kustomizations[0],
			Strategy:      "merge",
			Ordinal:       new.Ordinal,
			When:          combinedWhen,
		}
		return nil
	case "remove":
		accumulated := p.accumulateKustomizationRemovals(existing.Kustomization, new.Kustomization)
		entries[name] = &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: accumulated,
			Strategy:      "remove",
			Ordinal:       new.Ordinal,
			When:          combinedWhen,
		}
		return nil
	default:
		return werror.New(fmt.Sprintf("invalid strategy '%s' for kustomization '%s'", strategy, name), strategyHint, nil)
	}
}

// applyCollectedComponents applies all collected components and kustomizations to the target
// blueprint in the documented order: replace operations first, then merge operations, then remove
// operations last. This ensures that remove operations are applied after all merge/replace
// operations, as documented. Returns an error if any application operation fails.
func (p *BaseBlueprintProcessor) applyCollectedComponents(target *blueprintv1alpha1.Blueprint, collected collectedComponents, scope map[string]any) error {
	if err := p.dropExcludedEntries(collected, scope); err != nil {
		return err
	}

	terraformRemovals, terraformReplaces, terraformMerges, err := partitionByStrategy(collected.terraformByID, "terraform component",
		func(e *blueprintv1alpha1.ConditionalTerraformComponent) (string, blueprintv1alpha1.TerraformComponent) {
			return e.Strategy, e.TerraformComponent
		})
	if err != nil {
		return err
	}
	kustomizationRemovals, kustomizationReplaces, kustomizationMerges, err := partitionByStrategy(collected.kustomizationByName, "kustomization",
		func(e *blueprintv1alpha1.ConditionalKustomization) (string, blueprintv1alpha1.Kustomization) {
			return e.Strategy, e.Kustomization
		})
	if err != nil {
		return err
	}

	for _, replacement := range terraformReplaces {
		if err := target.ReplaceTerraformComponent(replacement); err != nil {
			return werror.Wrap(err, "error replacing terraform component '%s'", replacement.GetID())
		}
	}

	for _, merge := range terraformMerges {
		tempBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{merge},
		}
		if err := target.StrategicMerge(tempBp); err != nil {
			return werror.Wrap(err, "error merging terraform component '%s'", merge.GetID())
		}
	}

	for _, removal := range terraformRemovals {
		if err := target.RemoveTerraformComponent(removal); err != nil {
			return werror.Wrap(err, "error removing terraform component '%s'", removal.GetID())
		}
	}

	for _, replacement := range kustomizationReplaces {
		if err := target.ReplaceKustomization(replacement); err != nil {
			return werror.Wrap(err, "error replacing kustomization '%s'", replacement.Name)
		}
	}

	for _, merge := range kustomizationMerges {
		tempBp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{merge},
		}
		if err := target.StrategicMerge(tempBp); err != nil {
			return werror.Wrap(err, "error merging kustomization '%s'", merge.Name)
		}
	}

	for _, removal := range kustomizationRemovals {
		if err := target.RemoveKustomization(removal); err != nil {
			return werror.Wrap(err, "error removing kustomization '%s'", removal.Name)
		}
	}

	return applyFluxSystems(target, collected.fluxSystemByName)
}

// dropExcludedEntries re-evaluates the when: condition of each collected terraform component and
// kustomization against scope, and deletes the entries whose condition is false.
func (p *BaseBlueprintProcessor) dropExcludedEntries(collected collectedComponents, scope map[string]any) error {
	for componentID, entry := range collected.terraformByID {
		if entry.When != "" {
			shouldInclude, err := p.shouldIncludeComponent(entry.When, "", scope)
			if err != nil {
				return werror.Wrap(err, "error re-evaluating terraform component '%s' condition", componentID)
			}
			if !shouldInclude {
				delete(collected.terraformByID, componentID)
			}
		}
	}

	for name, entry := range collected.kustomizationByName {
		if entry.When != "" {
			shouldInclude, err := p.shouldIncludeComponent(entry.When, "", scope)
			if err != nil {
				return werror.Wrap(err, "error re-evaluating kustomization '%s' condition", name)
			}
			if !shouldInclude {
				delete(collected.kustomizationByName, name)
			}
		}
	}
	return nil
}

// accumulateTerraformRemovals combines the removals of two remove-strategy terraform components. It
// keeps the ID fields (Path, Name, Source), which match the component and are never removed. It unions
// the fields that RemoveTerraformComponent removes: Inputs keys and DependsOn items. A component that
// is not in the target blueprint is not changed.
func (p *BaseBlueprintProcessor) accumulateTerraformRemovals(existing, new blueprintv1alpha1.TerraformComponent) blueprintv1alpha1.TerraformComponent {
	accumulated := blueprintv1alpha1.TerraformComponent{
		Path:   existing.Path,
		Name:   existing.Name,
		Source: existing.Source,
	}

	accumulated.Inputs = accumulateMapKeys(existing.Inputs, new.Inputs)
	accumulated.DependsOn = accumulateStringSlice(existing.DependsOn, new.DependsOn)

	return accumulated
}

// accumulateKustomizationRemovals combines the removals of two remove-strategy kustomizations. It keeps
// Name, which matches the kustomization and is never removed. It unions the fields that
// RemoveKustomization removes: DependsOn, Components, Patches, and Substitutions keys. A kustomization
// that is not in the target blueprint is not changed.
func (p *BaseBlueprintProcessor) accumulateKustomizationRemovals(existing, new blueprintv1alpha1.Kustomization) blueprintv1alpha1.Kustomization {
	accumulated := blueprintv1alpha1.Kustomization{
		Name: existing.Name,
	}

	accumulated.DependsOn = accumulateStringSlice(existing.DependsOn, new.DependsOn)
	accumulated.Components = accumulateStringSlice(existing.Components, new.Components)
	accumulated.Patches = append(accumulated.Patches, existing.Patches...)
	accumulated.Patches = append(accumulated.Patches, new.Patches...)
	accumulated.Substitutions = accumulateMapKeys(existing.Substitutions, new.Substitutions)

	return accumulated
}

// accumulateFluxSystemRemovals combines two remove-strategy FluxSystem descriptors. DependsOn unions
// through accumulateStringSlice, and the Resources variants concatenate; RemoveFluxSystem drops each
// named variant, so duplicates do no harm. Install removals combine through
// accumulateKustomizationRemovals, with a nil Install treated as an empty Kustomization.
func (p *BaseBlueprintProcessor) accumulateFluxSystemRemovals(existing, new blueprintv1alpha1.FluxSystem) blueprintv1alpha1.FluxSystem {
	accumulated := blueprintv1alpha1.FluxSystem{
		Name: existing.Name,
	}

	accumulated.DependsOn = accumulateStringSlice(existing.DependsOn, new.DependsOn)

	if existing.Install != nil || new.Install != nil {
		mergedInstall := p.accumulateKustomizationRemovals(kustomizationOrZero(existing.Install), kustomizationOrZero(new.Install))
		accumulated.Install = &mergedInstall
	}

	accumulated.Resources = append(accumulated.Resources, existing.Resources...)
	accumulated.Resources = append(accumulated.Resources, new.Resources...)

	return accumulated
}

// kustomizationOrZero dereferences k, or returns a zero-value Kustomization when k is nil.
func kustomizationOrZero(k *blueprintv1alpha1.Kustomization) blueprintv1alpha1.Kustomization {
	if k == nil {
		return blueprintv1alpha1.Kustomization{}
	}
	return *k
}

// =============================================================================
// Helpers
// =============================================================================

// accumulateStringSlice merges two string slices into a deduplicated, sorted slice.
func accumulateStringSlice(existing, new []string) []string {
	if len(existing) == 0 && len(new) == 0 {
		return nil
	}
	itemMap := make(map[string]bool)
	for _, item := range existing {
		itemMap[item] = true
	}
	for _, item := range new {
		itemMap[item] = true
	}
	result := make([]string, 0, len(itemMap))
	for item := range itemMap {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

// accumulateMapKeys combines keys from two maps into a single map with zero values. Used for
// accumulating removal specifications where only the keys matter (the values are ignored).
func accumulateMapKeys[K comparable, V any](m1, m2 map[K]V) map[K]V {
	if len(m1) == 0 && len(m2) == 0 {
		return nil
	}
	result := make(map[K]V)
	for k := range m1 {
		var zero V
		result[k] = zero
	}
	for k := range m2 {
		var zero V
		result[k] = zero
	}
	return result
}

// partitionByStrategy sorts the entries by key and splits them into removals, replaces, and merges by
// strategy. An empty strategy means merge. kind names the entry type in the error for an unknown
// strategy.
func partitionByStrategy[E any, T any](entries map[string]*E, kind string, split func(*E) (string, T)) (removals, replaces, merges []T, err error) {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		strategy, item := split(entries[key])
		if strategy == "" {
			strategy = "merge"
		}

		switch strategy {
		case "remove":
			removals = append(removals, item)
		case "replace":
			replaces = append(replaces, item)
		case "merge":
			merges = append(merges, item)
		default:
			return nil, nil, nil, werror.New(fmt.Sprintf("invalid strategy '%s' for %s '%s'", strategy, kind, key), strategyHint, nil)
		}
	}
	return removals, replaces, merges, nil
}

// applyFluxSystems writes the collected flux systems to target in name order, clearing each one's
// strategy and ordinal. Systems with the remove strategy are removed after all others are written.
func applyFluxSystems(target *blueprintv1alpha1.Blueprint, fluxSystemByName map[string]*blueprintv1alpha1.FluxSystem) error {
	var fluxRemovals []blueprintv1alpha1.FluxSystem

	fluxKeys := make([]string, 0, len(fluxSystemByName))
	for key := range fluxSystemByName {
		fluxKeys = append(fluxKeys, key)
	}
	sort.Strings(fluxKeys)
	for _, key := range fluxKeys {
		sys := *fluxSystemByName[key]
		strategy := sys.Strategy
		if strategy == "" {
			strategy = "merge"
		}
		sys.Strategy = ""
		sys.Ordinal = nil

		if strategy == "remove" {
			fluxRemovals = append(fluxRemovals, sys)
			continue
		}
		if err := target.UpsertFluxSystem(sys); err != nil {
			return werror.Wrap(err, "error writing flux system '%s'", key)
		}
	}

	for _, removal := range fluxRemovals {
		if err := target.RemoveFluxSystem(removal); err != nil {
			return werror.Wrap(err, "error removing flux system '%s'", removal.Name)
		}
	}
	return nil
}
