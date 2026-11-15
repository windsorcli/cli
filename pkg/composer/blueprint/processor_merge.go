// The processor_merge file holds the merge steps of the BlueprintProcessor.
// It provides the merge of collected entries by ordinal and by replace or merge strategy.
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

// strategyPrecedence is the tiebreaker when the ordinals of two entries are equal: remove > replace >
// merge. Config blocks use all three. Component entries use replace > merge, because collect stores
// the component removals apart.
var strategyPrecedence = map[string]int{
	"remove":  3,
	"replace": 2,
	"merge":   1,
}

// =============================================================================
// Private Methods
// =============================================================================

// updateFluxSystemEntry merges a new merge or replace FluxSystem into entries by the rules of
// updateKustomizationEntry. A higher ordinal wins through applyFluxSystemEntryByStrategy, so merge
// keeps merging. Equal ordinals use replace > merge. A lower ordinal still merges, with the
// higher-ordinal entry winning scalar conflicts. A remove system goes to addFluxSystemRemoval instead.
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
		return p.applyFluxSystemEntryByStrategy(name, new, strategy, existing, entries)
	}
	if newOrdinal < existingOrdinal {
		if strategy != "merge" {
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
// swaps the whole entry. Merge unions DependsOn and deep-merges Install and the Resources variants,
// with the helpers that Blueprint.UpsertFluxSystem uses.
func (p *BaseBlueprintProcessor) applyFluxSystemEntryByStrategy(name string, new *blueprintv1alpha1.FluxSystem, strategy string, existing *blueprintv1alpha1.FluxSystem, entries map[string]*blueprintv1alpha1.FluxSystem) error {
	switch strategy {
	case "replace":
		new.Strategy = strategy
		entries[name] = new
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

// updateTerraformComponentEntry merges a new merge or replace component into an existing entry by
// ordinal, strategy, and when. A higher ordinal wins for replace. Equal ordinals use replace > merge.
// A lower ordinal still merges, with the higher-ordinal entry winning scalar conflicts. An entry whose
// when is now false is dropped. A remove component goes to addTerraformRemoval instead.
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
		return p.applyTerraformComponentEntryByStrategy(componentID, new, existing, strategy, entries)
	}

	if newOrdinal < existingOrdinal {
		if strategy != "merge" {
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
// replace stores new. merge does a strategic merge of new over existing, and combines the when
// expressions of both entries with &&. Any other strategy returns an error with a remediation hint.
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
	default:
		return werror.New(fmt.Sprintf("invalid strategy '%s' for terraform component '%s'", strategy, componentID), strategyHint, nil)
	}
}

// updateKustomizationEntry merges a new merge or replace kustomization into an existing entry by
// ordinal, then by replace > merge. At equal ordinal and strategy, merge pre-merges the two, and
// replace swaps the entry. A lower ordinal still merges, with the higher-ordinal entry winning scalar
// conflicts. A merge failure returns an error. A remove entry goes to addKustomizationRemoval instead.
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
		return p.applyKustomizationEntryByStrategy(name, new, strategy, existing, entries)
	}

	if newOrdinal < existingOrdinal {
		if strategy != "merge" {
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
// replace stores new. merge does a strategic merge of new over existing, and combines the when
// expressions of both entries with &&. Any other strategy returns an error with a remediation hint.
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
	default:
		return werror.New(fmt.Sprintf("invalid strategy '%s' for kustomization '%s'", strategy, name), strategyHint, nil)
	}
}

// applyCollectedComponents applies the collected entries to target in this order:
//  1. The removals strip their fields from the entries that target already holds.
//  2. The replace entries swap the matching entries, and the merge entries merge.
//  3. The flux systems are written.
//
// A higher-ordinal merge can add a removed field back, because collect strips only lower entries.
func (p *BaseBlueprintProcessor) applyCollectedComponents(target *blueprintv1alpha1.Blueprint, collected collectedComponents, scope map[string]any) error {
	if err := p.dropExcludedEntries(collected, scope); err != nil {
		return err
	}

	terraformReplaces, terraformMerges, err := partitionByStrategy(collected.terraformByID, "terraform component",
		func(e *blueprintv1alpha1.ConditionalTerraformComponent) (string, blueprintv1alpha1.TerraformComponent) {
			return e.Strategy, e.TerraformComponent
		})
	if err != nil {
		return err
	}
	kustomizationReplaces, kustomizationMerges, err := partitionByStrategy(collected.kustomizationByName, "kustomization",
		func(e *blueprintv1alpha1.ConditionalKustomization) (string, blueprintv1alpha1.Kustomization) {
			return e.Strategy, e.Kustomization
		})
	if err != nil {
		return err
	}

	if err := applyRemovals(target, collected.removals()); err != nil {
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

// sortedKeys returns the keys of m in ascending order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// traceEntryKey returns the name of one merged entry in the trace collector. The key holds the
// source, because each source merges its facets on its own.
func traceEntryKey(source, kind, name string) string {
	return source + "/" + kind + "/" + name
}

// settleEntryTrace tells the trace collector which contributions survive one merge of an entry.
// previous is the entry before the merge, incoming is the entry that the facet added, and result
// is the entry after the merge. A merge that stores incoming drops the earlier contributions. A
// merge that keeps previous drops the new contributions. No result drops both.
func settleEntryTrace[E any](p *BaseBlueprintProcessor, entryKey string, previous, incoming, result *E) {
	if p.traceCollector == nil {
		return
	}
	keepExisting, keepNew := true, true
	switch {
	case result == nil:
		keepExisting, keepNew = false, false
	case result == incoming:
		keepExisting = false
	case result == previous:
		keepNew = false
	}
	p.traceCollector.SettleEntry(entryKey, keepExisting, keepNew)
}

// partitionByStrategy sorts the entries by key and splits them into replaces and merges by strategy.
// An empty strategy means merge. Any other strategy returns an error, because the collect step stores
// removals apart from the entries. kind names the entry type in the error.
func partitionByStrategy[E any, T any](entries map[string]*E, kind string, split func(*E) (string, T)) (replaces, merges []T, err error) {
	for _, key := range sortedKeys(entries) {
		strategy, item := split(entries[key])
		switch strategy {
		case "replace":
			replaces = append(replaces, item)
		case "", "merge":
			merges = append(merges, item)
		default:
			return nil, nil, werror.New(fmt.Sprintf("invalid strategy '%s' for %s '%s'", strategy, kind, key), strategyHint, nil)
		}
	}
	return replaces, merges, nil
}

// applyFluxSystems writes the collected flux systems to target in name order, clearing each one's
// strategy and ordinal.
func applyFluxSystems(target *blueprintv1alpha1.Blueprint, fluxSystemByName map[string]*blueprintv1alpha1.FluxSystem) error {
	for _, key := range sortedKeys(fluxSystemByName) {
		sys := *fluxSystemByName[key]
		sys.Strategy = ""
		sys.Ordinal = nil
		if err := target.UpsertFluxSystem(sys); err != nil {
			return werror.Wrap(err, "error writing flux system '%s'", key)
		}
	}
	return nil
}
