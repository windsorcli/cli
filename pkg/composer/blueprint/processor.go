package blueprint

import (
	"fmt"
	"sort"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Constants
// =============================================================================

var strategyPriorities = map[string]int{
	"remove":  3,
	"replace": 2,
	"merge":   1,
}

// =============================================================================
// Interface
// =============================================================================

// BlueprintProcessor evaluates when: conditions on facets, terraform components, and kustomizations.
// It determines inclusion/exclusion based on boolean condition results.
// The processor is stateless and shared across all loaders.
type BlueprintProcessor interface {
	ProcessFacets(target *blueprintv1alpha1.Blueprint, facets []blueprintv1alpha1.Facet, sourceName ...string) error
}

// =============================================================================
// Types
// =============================================================================

// BaseBlueprintProcessor provides the default implementation of the BlueprintProcessor interface.
type BaseBlueprintProcessor struct {
	runtime   *runtime.Runtime
	evaluator evaluator.ExpressionEvaluator
}

// =============================================================================
// Constructor
// =============================================================================

// NewBlueprintProcessor creates a new BlueprintProcessor using the runtime's expression evaluator.
// The evaluator is used to evaluate 'when' conditions on facets and components. Optional
// overrides allow replacing the evaluator for testing. The processor is stateless and can
// be shared across multiple concurrent facet processing operations. The evaluator must be
// provided either via the runtime or as an override.
func NewBlueprintProcessor(rt *runtime.Runtime) *BaseBlueprintProcessor {
	if rt == nil {
		panic("runtime is required")
	}
	if rt.Evaluator == nil {
		panic("evaluator is required on runtime")
	}

	return &BaseBlueprintProcessor{
		runtime:   rt,
		evaluator: rt.Evaluator,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// ProcessFacets iterates through a list of facets, evaluating each facet's 'when' condition
// against the provided configuration data. Facets whose conditions evaluate to true (or have no
// condition) contribute their terraform components and kustomizations to the target blueprint.
// Components within facets can also have individual 'when' conditions for fine-grained control.
// Facets are sorted by metadata.name before processing to ensure deterministic output. If sourceName
// is provided, it sets the Source field on components that don't already have one, linking them to
// their originating OCI artifact. Input expressions and substitutions are preserved as-is for later
// evaluation by their consumers. Components and kustomizations are processed according to their
// priority and strategy fields. Priority is compared first: higher priority values override lower
// priority ones. When priorities are equal, strategy priority is used: "remove" (highest) > "replace" > "merge" (default, lowest).
// When both priority and strategy are equal, components are merged, removals are accumulated, or replace wins.
// The target blueprint is modified in place.
func (p *BaseBlueprintProcessor) ProcessFacets(target *blueprintv1alpha1.Blueprint, facets []blueprintv1alpha1.Facet, sourceName ...string) error {
	if target == nil {
		return fmt.Errorf("target blueprint cannot be nil")
	}

	if len(facets) == 0 {
		return nil
	}

	sortedFacets := make([]blueprintv1alpha1.Facet, len(facets))
	copy(sortedFacets, facets)
	sort.Slice(sortedFacets, func(i, j int) bool {
		return sortedFacets[i].Metadata.Name < sortedFacets[j].Metadata.Name
	})

	terraformByID := make(map[string]*blueprintv1alpha1.ConditionalTerraformComponent)
	kustomizationByName := make(map[string]*blueprintv1alpha1.ConditionalKustomization)

	for _, facet := range sortedFacets {
		shouldInclude, err := p.shouldIncludeFacet(facet)
		if err != nil {
			return err
		}
		if !shouldInclude {
			continue
		}

		if err := p.collectTerraformComponents(facet, sourceName, terraformByID); err != nil {
			return err
		}

		if err := p.collectKustomizations(facet, sourceName, kustomizationByName); err != nil {
			return err
		}
	}

	return p.applyCollectedComponents(target, terraformByID, kustomizationByName)
}

// =============================================================================
// Private Methods
// =============================================================================

// collectTerraformComponents processes all terraform components from a facet, evaluating their
// conditions, inputs, and source assignments. Components are collected into the terraformByID map
// with strategy priority handling. Components with matching IDs are merged or replaced based on
// their strategy priority. Returns an error if condition evaluation or input processing fails.
func (p *BaseBlueprintProcessor) collectTerraformComponents(facet blueprintv1alpha1.Facet, sourceName []string, terraformByID map[string]*blueprintv1alpha1.ConditionalTerraformComponent) error {
	for _, tc := range facet.TerraformComponents {
		shouldInclude, err := p.shouldIncludeComponent(tc.When, facet.Path)
		if err != nil {
			return fmt.Errorf("error evaluating terraform component condition: %w", err)
		}
		if !shouldInclude {
			continue
		}

		processed := tc
		if processed.Inputs != nil {
			evaluated, err := p.evaluator.EvaluateMap(processed.Inputs, facet.Path, false)
			if err != nil {
				return fmt.Errorf("error evaluating inputs for component '%s': %w", processed.GetID(), err)
			}
			processed.Inputs = evaluated
		}
		if processed.Source == "" && len(sourceName) > 0 && sourceName[0] != "" && sourceName[0] != "primary" {
			processed.Source = sourceName[0]
		}

		strategy := processed.Strategy
		if strategy == "" {
			strategy = "merge"
		}

		componentID := processed.GetID()
		if _, exists := terraformByID[componentID]; !exists {
			processed.Strategy = strategy
			terraformByID[componentID] = &processed
		} else {
			if err := p.updateTerraformComponentEntry(componentID, &processed, strategy, terraformByID); err != nil {
				return err
			}
		}
	}
	return nil
}

// collectKustomizations processes all kustomizations from a facet, evaluating their conditions,
// substitutions, and source assignments. Kustomizations are collected into the kustomizationByName
// map with strategy priority handling. Kustomizations with matching names are merged or replaced
// based on their strategy priority. Returns an error if condition evaluation or substitution
// processing fails.
func (p *BaseBlueprintProcessor) collectKustomizations(facet blueprintv1alpha1.Facet, sourceName []string, kustomizationByName map[string]*blueprintv1alpha1.ConditionalKustomization) error {
	for _, k := range facet.Kustomizations {
		shouldInclude, err := p.shouldIncludeComponent(k.When, facet.Path)
		if err != nil {
			return fmt.Errorf("error evaluating kustomization condition: %w", err)
		}
		if !shouldInclude {
			continue
		}

		processed := k
		if processed.Substitutions != nil {
			evaluated, err := p.evaluateSubstitutions(processed.Substitutions, facet.Path)
			if err != nil {
				return fmt.Errorf("error evaluating substitutions for kustomization '%s': %w", processed.Name, err)
			}
			processed.Substitutions = evaluated
		}
		if processed.Source == "" && len(sourceName) > 0 && sourceName[0] != "" && sourceName[0] != "primary" {
			processed.Source = sourceName[0]
		}

		strategy := processed.Strategy
		if strategy == "" {
			strategy = "merge"
		}

		if _, exists := kustomizationByName[processed.Name]; !exists {
			processed.Strategy = strategy
			kustomizationByName[processed.Name] = &processed
		} else {
			if err := p.updateKustomizationEntry(processed.Name, &processed, strategy, kustomizationByName); err != nil {
				return err
			}
		}
	}
	return nil
}

// shouldIncludeFacet evaluates whether a facet should be included based on its 'when' condition.
// Returns true if the facet has no condition or if the condition evaluates to true. Returns
// false if the condition evaluates to false. Returns an error if condition evaluation fails.
func (p *BaseBlueprintProcessor) shouldIncludeFacet(facet blueprintv1alpha1.Facet) (bool, error) {
	if facet.When == "" {
		return true, nil
	}
	matches, err := p.evaluateCondition(facet.When, facet.Path)
	if err != nil {
		return false, fmt.Errorf("error evaluating facet '%s' condition: %w", facet.Metadata.Name, err)
	}
	return matches, nil
}

// shouldIncludeComponent evaluates whether a component or kustomization should be included based
// on its 'when' condition. Returns true if there is no condition or if the condition evaluates
// to true. Returns false if the condition evaluates to false. Returns an error if condition
// evaluation fails.
func (p *BaseBlueprintProcessor) shouldIncludeComponent(when string, facetPath string) (bool, error) {
	if when == "" {
		return true, nil
	}
	matches, err := p.evaluateCondition(when, facetPath)
	if err != nil {
		return false, err
	}
	return matches, nil
}

// updateTerraformComponentEntry updates an existing terraform component entry in the collection
// map based on priority and strategy. Priority is compared first: higher priority wins. If priorities
// are equal, strategy priority is used (remove > replace > merge). If both priority and strategy
// are equal, components are pre-merged (merge), removals are accumulated (remove), or new replaces existing
// (replace). For replace operations with equal priority and strategy, the last processed facet
// (alphabetically by name) wins. Users should set different priorities to make ordering explicit.
// Returns an error if the merge operation fails.
func (p *BaseBlueprintProcessor) updateTerraformComponentEntry(componentID string, new *blueprintv1alpha1.ConditionalTerraformComponent, strategy string, entries map[string]*blueprintv1alpha1.ConditionalTerraformComponent) error {
	existing := entries[componentID]
	existingStrategy := existing.Strategy
	if existingStrategy == "" {
		existingStrategy = "merge"
	}

	newStrategyPriority, exists := strategyPriorities[strategy]
	if !exists {
		return fmt.Errorf("invalid strategy '%s' for terraform component '%s': must be 'remove', 'replace', or 'merge'", strategy, componentID)
	}

	newPriority := new.Priority
	existingPriority := existing.Priority

	if newPriority > existingPriority {
		new.Strategy = strategy
		entries[componentID] = new
		return nil
	}

	if newPriority < existingPriority {
		return nil
	}
	existingStrategyPriority := strategyPriorities[existingStrategy]
	if newStrategyPriority > existingStrategyPriority {
		new.Strategy = strategy
		entries[componentID] = new
		return nil
	}

	if newStrategyPriority < existingStrategyPriority {
		return nil
	}

	switch strategy {
	case "merge":
		tempBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{existing.TerraformComponent},
		}
		mergedBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{new.TerraformComponent},
		}
		if err := tempBp.StrategicMerge(mergedBp); err != nil {
			return fmt.Errorf("error pre-merging terraform component '%s': %w", componentID, err)
		}
		merged := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: tempBp.TerraformComponents[0],
			Strategy:           "merge",
			Priority:           newPriority,
		}
		entries[componentID] = merged
	case "remove":
		accumulated := p.accumulateTerraformRemovals(existing.TerraformComponent, new.TerraformComponent)
		entries[componentID] = &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: accumulated,
			Strategy:           "remove",
			Priority:           newPriority,
		}
	case "replace":
		new.Strategy = strategy
		entries[componentID] = new
	default:
		return fmt.Errorf("invalid strategy '%s' for terraform component '%s': must be 'remove', 'replace', or 'merge'", strategy, componentID)
	}
	return nil
}

// updateKustomizationEntry updates an existing kustomization entry in the collection map based
// on priority and strategy. Priority is compared first: higher priority wins. If priorities are
// equal, strategy priority is used (remove > replace > merge). If both priority and strategy are
// equal, kustomizations are pre-merged (merge), removals are accumulated (remove), or new replaces
// existing (replace). For replace operations with equal priority and strategy, the last processed
// facet (alphabetically by name) wins. Users should set different priorities to make ordering explicit.
// Returns an error if the merge operation fails.
func (p *BaseBlueprintProcessor) updateKustomizationEntry(name string, new *blueprintv1alpha1.ConditionalKustomization, strategy string, entries map[string]*blueprintv1alpha1.ConditionalKustomization) error {
	existing := entries[name]
	existingStrategy := existing.Strategy
	if existingStrategy == "" {
		existingStrategy = "merge"
	}

	newStrategyPriority, exists := strategyPriorities[strategy]
	if !exists {
		return fmt.Errorf("invalid strategy '%s' for kustomization '%s': must be 'remove', 'replace', or 'merge'", strategy, name)
	}

	newPriority := new.Priority
	existingPriority := existing.Priority

	if newPriority > existingPriority {
		new.Strategy = strategy
		entries[name] = new
		return nil
	}

	if newPriority < existingPriority {
		return nil
	}
	existingStrategyPriority := strategyPriorities[existingStrategy]
	if newStrategyPriority > existingStrategyPriority {
		new.Strategy = strategy
		entries[name] = new
		return nil
	}

	if newStrategyPriority < existingStrategyPriority {
		return nil
	}

	switch strategy {
	case "merge":
		tempBp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{existing.Kustomization},
		}
		mergedBp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{new.Kustomization},
		}
		if err := tempBp.StrategicMerge(mergedBp); err != nil {
			return fmt.Errorf("error pre-merging kustomization '%s': %w", name, err)
		}
		merged := &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: tempBp.Kustomizations[0],
			Strategy:      "merge",
			Priority:      newPriority,
		}
		entries[name] = merged
	case "remove":
		accumulated := p.accumulateKustomizationRemovals(existing.Kustomization, new.Kustomization)
		entries[name] = &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: accumulated,
			Strategy:      "remove",
			Priority:      newPriority,
		}
	case "replace":
		new.Strategy = strategy
		entries[name] = new
	default:
		return fmt.Errorf("invalid strategy '%s' for kustomization '%s': must be 'remove', 'replace', or 'merge'", strategy, name)
	}
	return nil
}

// applyCollectedComponents applies all collected components and kustomizations to the target
// blueprint in the documented order: replace operations first, then merge operations, then remove
// operations last. This ensures that remove operations are applied after all merge/replace
// operations, as documented. Returns an error if any application operation fails.
func (p *BaseBlueprintProcessor) applyCollectedComponents(target *blueprintv1alpha1.Blueprint, terraformByID map[string]*blueprintv1alpha1.ConditionalTerraformComponent, kustomizationByName map[string]*blueprintv1alpha1.ConditionalKustomization) error {
	var terraformRemovals, terraformReplaces, terraformMerges []blueprintv1alpha1.TerraformComponent
	var kustomizationRemovals, kustomizationReplaces, kustomizationMerges []blueprintv1alpha1.Kustomization

	terraformKeys := make([]string, 0, len(terraformByID))
	for key := range terraformByID {
		terraformKeys = append(terraformKeys, key)
	}
	sort.Strings(terraformKeys)

	for _, key := range terraformKeys {
		entry := terraformByID[key]
		strategy := entry.Strategy
		if strategy == "" {
			strategy = "merge"
		}
		switch strategy {
		case "remove":
			terraformRemovals = append(terraformRemovals, entry.TerraformComponent)
		case "replace":
			terraformReplaces = append(terraformReplaces, entry.TerraformComponent)
		case "merge":
			terraformMerges = append(terraformMerges, entry.TerraformComponent)
		default:
			return fmt.Errorf("invalid strategy '%s' for terraform component '%s': must be 'remove', 'replace', or 'merge'", strategy, key)
		}
	}

	kustomizationKeys := make([]string, 0, len(kustomizationByName))
	for key := range kustomizationByName {
		kustomizationKeys = append(kustomizationKeys, key)
	}
	sort.Strings(kustomizationKeys)

	for _, key := range kustomizationKeys {
		entry := kustomizationByName[key]
		strategy := entry.Strategy
		if strategy == "" {
			strategy = "merge"
		}
		switch strategy {
		case "remove":
			kustomizationRemovals = append(kustomizationRemovals, entry.Kustomization)
		case "replace":
			kustomizationReplaces = append(kustomizationReplaces, entry.Kustomization)
		case "merge":
			kustomizationMerges = append(kustomizationMerges, entry.Kustomization)
		default:
			return fmt.Errorf("invalid strategy '%s' for kustomization '%s': must be 'remove', 'replace', or 'merge'", strategy, key)
		}
	}

	for _, replacement := range terraformReplaces {
		if err := target.ReplaceTerraformComponent(replacement); err != nil {
			return fmt.Errorf("error replacing terraform component '%s': %w", replacement.GetID(), err)
		}
	}

	for _, merge := range terraformMerges {
		tempBp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{merge},
		}
		if err := target.StrategicMerge(tempBp); err != nil {
			return fmt.Errorf("error merging terraform component '%s': %w", merge.GetID(), err)
		}
	}

	for _, removal := range terraformRemovals {
		if err := target.RemoveTerraformComponent(removal); err != nil {
			return fmt.Errorf("error removing terraform component '%s': %w", removal.GetID(), err)
		}
	}

	for _, replacement := range kustomizationReplaces {
		if err := target.ReplaceKustomization(replacement); err != nil {
			return fmt.Errorf("error replacing kustomization '%s': %w", replacement.Name, err)
		}
	}

	for _, merge := range kustomizationMerges {
		tempBp := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{merge},
		}
		if err := target.StrategicMerge(tempBp); err != nil {
			return fmt.Errorf("error merging kustomization '%s': %w", merge.Name, err)
		}
	}

	for _, removal := range kustomizationRemovals {
		if err := target.RemoveKustomization(removal); err != nil {
			return fmt.Errorf("error removing kustomization '%s': %w", removal.Name, err)
		}
	}

	return nil
}

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

// accumulateTerraformRemovals combines removal specifications from two terraform components when
// both have "remove" strategy. It preserves ID fields (Path, Name, Source) which are used to match
// the component but are never removed. It accumulates removal specifications only for fields that
// RemoveTerraformComponent actually removes: Inputs (map keys) and DependsOn (slice items). The
// result contains a union of all fields that should be removed from either component. If the
// component doesn't exist in the target blueprint when removals are applied, RemoveTerraformComponent
// will perform a no-op, which is the expected behavior.
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

// accumulateKustomizationRemovals combines removal specifications from two kustomizations when
// both have "remove" strategy. It preserves ID fields (Name) which are used to match the
// kustomization but are never removed. It accumulates removal specifications only for fields that
// RemoveKustomization actually removes: DependsOn, Components, Cleanup (string slices), Patches
// (BlueprintPatch slice), and Substitutions (map keys). The result contains a union of all fields
// that should be removed from either kustomization. If the kustomization doesn't exist in the target
// blueprint when removals are applied, RemoveKustomization will perform a no-op, which is the
// expected behavior.
func (p *BaseBlueprintProcessor) accumulateKustomizationRemovals(existing, new blueprintv1alpha1.Kustomization) blueprintv1alpha1.Kustomization {
	accumulated := blueprintv1alpha1.Kustomization{
		Name: existing.Name,
	}

	accumulated.DependsOn = accumulateStringSlice(existing.DependsOn, new.DependsOn)
	accumulated.Components = accumulateStringSlice(existing.Components, new.Components)
	accumulated.Cleanup = accumulateStringSlice(existing.Cleanup, new.Cleanup)
	accumulated.Patches = append(accumulated.Patches, existing.Patches...)
	accumulated.Patches = append(accumulated.Patches, new.Patches...)
	accumulated.Substitutions = accumulateMapKeys(existing.Substitutions, new.Substitutions)

	return accumulated
}

// evaluateCondition uses the expression evaluator to evaluate a 'when' condition string against
// the provided configuration data. The path parameter provides context for error messages and
// helper function resolution. Returns true if the expression evaluates to boolean true or the
// string "true", false otherwise. Returns an error if the expression is invalid or evaluates
// to an unexpected type.
func (p *BaseBlueprintProcessor) evaluateCondition(expr string, path string) (bool, error) {
	if !evaluator.ContainsExpression(expr) {
		expr = "${" + expr + "}"
	}
	evaluated, err := p.evaluator.Evaluate(expr, path, false)
	if err != nil {
		return false, err
	}

	var result bool
	switch v := evaluated.(type) {
	case bool:
		result = v
	case string:
		result = v == "true"
	default:
		return false, fmt.Errorf("condition must evaluate to boolean, got %T", evaluated)
	}
	return result, nil
}

// evaluateSubstitutions evaluates a map of string substitutions using the BaseBlueprintProcessor's expression evaluator.
// Each substitution value is evaluated; if the result contains unresolved expressions, the substitution is skipped.
// Returned values retain their string form if possible, or are stringified if not. Returns the successfully
// evaluated substitutions map or an error if evaluation fails for any substitution.
func (p *BaseBlueprintProcessor) evaluateSubstitutions(subs map[string]string, facetPath string) (map[string]string, error) {
	result := make(map[string]string)
	for key, value := range subs {
		evaluated, err := p.evaluator.Evaluate(value, facetPath, false)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate '%s': %w", key, err)
		}
		if evaluator.ContainsExpression(evaluated) {
			continue
		}
		if str, ok := evaluated.(string); ok {
			result[key] = str
		} else {
			result[key] = fmt.Sprintf("%v", evaluated)
		}
	}
	return result, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ BlueprintProcessor = (*BaseBlueprintProcessor)(nil)
