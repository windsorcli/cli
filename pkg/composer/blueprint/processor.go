package blueprint

import (
	"fmt"
	"maps"
	"sort"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// The BaseBlueprintProcessor is a core component that processes blueprint facets and evaluates conditional logic.
// It provides functionality for evaluating 'when' conditions on facets, terraform components, and kustomizations.
// The BaseBlueprintProcessor orchestrates the collection, merging, and application of components based on priority
// and strategy, ensuring deterministic and predictable blueprint composition from multiple facet sources.

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
	var globalScope map[string]any
	var configBlockOrder []string

	for _, facet := range sortedFacets {
		shouldInclude, err := p.shouldIncludeFacet(facet)
		if err != nil {
			return err
		}
		if !shouldInclude {
			continue
		}
		var errMerge error
		globalScope, configBlockOrder, errMerge = p.mergeFacetScopeIntoGlobal(facet, globalScope, configBlockOrder)
		if errMerge != nil {
			return fmt.Errorf("facet %s: %w", facet.Metadata.Name, errMerge)
		}
	}
	if err := p.evaluateGlobalScopeConfig(globalScope, configBlockOrder); err != nil {
		return err
	}

	for _, facet := range sortedFacets {
		shouldInclude, err := p.shouldIncludeFacet(facet)
		if err != nil {
			return err
		}
		if !shouldInclude {
			continue
		}
		if err := p.collectTerraformComponents(facet, sourceName, terraformByID, globalScope); err != nil {
			return err
		}
		if err := p.collectKustomizations(facet, sourceName, kustomizationByName, globalScope); err != nil {
			return err
		}
	}

	return p.applyCollectedComponents(target, terraformByID, kustomizationByName)
}

// =============================================================================
// Private Methods
// =============================================================================

// mergeFacetScopeIntoGlobal merges the facet's config block structure into the global scope
// (accumulated from prior facets) without evaluating config body expressions. Returns the
// updated global scope and the config block name order (later in list takes precedence).
// Config body expressions are evaluated later in evaluateGlobalScopeConfig.
// Same config block name merges block bodies recursively.
func (p *BaseBlueprintProcessor) mergeFacetScopeIntoGlobal(facet blueprintv1alpha1.Facet, globalScope map[string]any, order []string) (map[string]any, []string, error) {
	configMap := make(map[string]any)
	for _, block := range facet.Config {
		if block.Name == "" {
			continue
		}
		if block.When != "" {
			shouldInclude, err := p.shouldIncludeComponent(block.When, facet.Path)
			if err != nil {
				return nil, order, fmt.Errorf("config block %q when: %w", block.Name, err)
			}
			if !shouldInclude {
				continue
			}
		}
		if len(block.Body) == 0 {
			configMap[block.Name] = make(map[string]any)
		} else {
			configMap[block.Name] = block.Body
		}
		n := block.Name
		for i := 0; i < len(order); i++ {
			if order[i] == n {
				order = append(order[:i], order[i+1:]...)
				break
			}
		}
		order = append(order, n)
	}
	mergedConfig := mergeConfigMaps(globalScope, configMap)
	if len(mergedConfig) == 0 {
		return globalScope, order, nil
	}
	return mergedConfig, order, nil
}

// evaluateGlobalScopeConfig evaluates all config block body expressions in globalScope in
// blueprint context with globalScope merged into the evaluation environment so config
// blocks can reference each other (e.g. talos_common.common_patch referencing
// talos_common.patchVars, or controlplanes.patch referencing talos_common.patchVars).
// Called once after all facets have been merged. Mutates globalScope in place.
// Block names are iterated in configBlockOrder so later blocks in the list are evaluated
// after earlier ones (later takes precedence when refs are resolved).
func (p *BaseBlueprintProcessor) evaluateGlobalScopeConfig(globalScope map[string]any, configBlockOrder []string) error {
	if globalScope == nil {
		return nil
	}
	names := configBlockOrder
	if len(names) == 0 {
		names = make([]string, 0, len(globalScope))
		for name := range globalScope {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	for _, name := range names {
		body := globalScope[name]
		bodyMap, ok := body.(map[string]any)
		if !ok || len(bodyMap) == 0 {
			continue
		}
		evaluated, err := p.evaluator.EvaluateMap(bodyMap, "", globalScope, false)
		if err != nil {
			return fmt.Errorf("config block %q: %w", name, err)
		}
		globalScope[name] = evaluated
	}
	return nil
}

// collectTerraformComponents processes and collects all Terraform components from the provided facet.
// It evaluates conditions, inputs, and source assignments for each component, collects them into the
// terraformByID map, and handles merging or replacement based on strategy priorities. When facetScope
// is non-nil (evaluated facet config), it is merged into the expression environment so inputs can
// reference config block values (e.g. talos.controlplanes). If a component has an empty 'when'
// condition, it inherits the facet-level condition. Returns an error if condition evaluation or
// input processing fails.
func (p *BaseBlueprintProcessor) collectTerraformComponents(
	facet blueprintv1alpha1.Facet,
	sourceName []string,
	terraformByID map[string]*blueprintv1alpha1.ConditionalTerraformComponent,
	facetScope map[string]any,
) error {
	for _, tc := range facet.TerraformComponents {
		componentWhen := tc.When
		if componentWhen == "" && facet.When != "" {
			componentWhen = facet.When
		}

		shouldInclude, err := p.shouldIncludeComponent(componentWhen, facet.Path)
		if err != nil {
			return fmt.Errorf("error evaluating terraform component condition: %w", err)
		}
		if !shouldInclude {
			continue
		}

		processed := tc
		processed.When = componentWhen
		if processed.Inputs != nil {
			evaluated, err := p.evaluator.EvaluateMap(processed.Inputs, facet.Path, facetScope, false)
			if err != nil {
				return fmt.Errorf("error evaluating inputs for component '%s': %w", processed.GetID(), err)
			}
			normalized := make(map[string]any, len(evaluated))
			for k, v := range evaluated {
				if m := blueprintv1alpha1.ToMapStringAny(v); m != nil {
					normalized[k] = m
				} else if s := blueprintv1alpha1.ToSliceAny(v); s != nil {
					normalized[k] = s
				} else {
					normalized[k] = v
				}
			}
			processed.Inputs = normalized
		}
		if len(processed.DependsOn) > 0 {
			evaluated, err := p.evaluateStringSlice(processed.DependsOn, facet.Path, facetScope)
			if err != nil {
				return fmt.Errorf("error evaluating dependsOn for component '%s': %w", processed.GetID(), err)
			}
			processed.DependsOn = evaluated
		}
		if processed.Destroy != nil && processed.Destroy.IsExpr {
			evaluated, err := p.evaluateBooleanExpression(processed.Destroy.Expr, facet.Path, facetScope)
			if err != nil {
				return fmt.Errorf("error evaluating destroy for component '%s': %w", processed.GetID(), err)
			}
			processed.Destroy = &blueprintv1alpha1.BoolExpression{Value: evaluated, IsExpr: false}
		}
		if processed.Parallelism != nil && processed.Parallelism.IsExpr {
			evaluated, err := p.evaluateIntegerExpression(processed.Parallelism.Expr, facet.Path, facetScope)
			if err != nil {
				return fmt.Errorf("error evaluating parallelism for component '%s': %w", processed.GetID(), err)
			}
			processed.Parallelism = &blueprintv1alpha1.IntExpression{Value: evaluated, IsExpr: false}
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
// substitutions, and source assignments. When facetScope is non-nil (evaluated facet config), it is
// merged into the expression environment so substitutions and other expressions can reference config
// block values. Kustomizations are collected into the kustomizationByName map with strategy priority
// handling. Kustomizations with matching names are merged or replaced based on their strategy
// priority. If a kustomization has an empty 'when' condition, it inherits the facet-level condition.
// Returns an error if condition evaluation or substitution processing fails.
func (p *BaseBlueprintProcessor) collectKustomizations(facet blueprintv1alpha1.Facet, sourceName []string, kustomizationByName map[string]*blueprintv1alpha1.ConditionalKustomization, facetScope map[string]any) error {
	for _, k := range facet.Kustomizations {
		componentWhen := k.When
		if componentWhen == "" && facet.When != "" {
			componentWhen = facet.When
		}

		shouldInclude, err := p.shouldIncludeComponent(componentWhen, facet.Path)
		if err != nil {
			return fmt.Errorf("error evaluating kustomization condition: %w", err)
		}
		if !shouldInclude {
			continue
		}

		processed := k
		processed.When = componentWhen
		if processed.Substitutions != nil {
			evaluated, err := p.evaluateSubstitutions(processed.Substitutions, facet.Path, facetScope)
			if err != nil {
				return fmt.Errorf("error evaluating substitutions for kustomization '%s': %w", processed.Name, err)
			}
			processed.Substitutions = evaluated
		}
		if len(processed.DependsOn) > 0 {
			evaluated, err := p.evaluateStringSlice(processed.DependsOn, facet.Path, facetScope)
			if err != nil {
				return fmt.Errorf("error evaluating dependsOn for kustomization '%s': %w", processed.Name, err)
			}
			processed.DependsOn = evaluated
		}
		if len(processed.Components) > 0 {
			evaluated, err := p.evaluateStringSlice(processed.Components, facet.Path, facetScope)
			if err != nil {
				return fmt.Errorf("error evaluating components for kustomization '%s': %w", processed.Name, err)
			}
			processed.Components = evaluated
		}
		if len(processed.Cleanup) > 0 {
			evaluated, err := p.evaluateStringSlice(processed.Cleanup, facet.Path, facetScope)
			if err != nil {
				return fmt.Errorf("error evaluating cleanup for kustomization '%s': %w", processed.Name, err)
			}
			processed.Cleanup = evaluated
		}
		if processed.Destroy != nil && processed.Destroy.IsExpr {
			evaluated, err := p.evaluateBooleanExpression(processed.Destroy.Expr, facet.Path, facetScope)
			if err != nil {
				return fmt.Errorf("error evaluating destroy for kustomization '%s': %w", processed.Name, err)
			}
			processed.Destroy = &blueprintv1alpha1.BoolExpression{Value: evaluated, IsExpr: false}
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

// updateTerraformComponentEntry updates or merges a single ConditionalTerraformComponent entry in the component
// collection based on priority, strategy, and conditional 'when' expressions, ensuring consistent and predictable
// handling of multiple definitions for the same component. It provides robust conflict resolution logic by considering
// component priority (higher priority always wins), then strategy precedence ('remove' > 'replace' > 'merge'), and
// finally merge behavior for equal priority and strategy. The function also rigorously re-evaluates 'when' conditions for
// both the new and existing entries, removing entries from the collection if any relevant condition now resolves to false.
// When the strategy is 'merge', it performs a strategic pre-merge and logically ANDs 'when' conditions. For 'remove',
// component removals are accumulated, and for 'replace', the most recent definition takes precedence if priorities
// and strategies are equal. Only valid strategies are allowed; otherwise, an error is returned, as is the case for merge
// failures. This function is critical to the blueprint processor’s ability to aggregate, override, conditionally include
// or exclude, and deconflict terraform components efficiently, making it safe to combine blueprint facets or overrides
// without unintended duplication or omission. Returns an error if strategies are invalid or pre-merge fails.
func (p *BaseBlueprintProcessor) updateTerraformComponentEntry(
	componentID string,
	new *blueprintv1alpha1.ConditionalTerraformComponent,
	strategy string,
	entries map[string]*blueprintv1alpha1.ConditionalTerraformComponent,
) error {
	existing := entries[componentID]
	existingStrategy := existing.Strategy
	if existingStrategy == "" {
		existingStrategy = "merge"
	}

	if existing.When == "" && new.When != "" {
		shouldInclude, err := p.shouldIncludeComponent(new.When, "")
		if err == nil && !shouldInclude {
			delete(entries, componentID)
			return nil
		}
	} else if existing.When != "" {
		shouldInclude, err := p.shouldIncludeComponent(existing.When, "")
		if err == nil && !shouldInclude {
			delete(entries, componentID)
			return nil
		}
		if new.When != "" {
			shouldInclude, err := p.shouldIncludeComponent(new.When, "")
			if err == nil && !shouldInclude {
				delete(entries, componentID)
				return nil
			}
		}
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
		combinedWhen := ""
		if existing.When != "" && new.When != "" {
			combinedWhen = fmt.Sprintf("(%s) && (%s)", existing.When, new.When)
		} else if existing.When != "" {
			combinedWhen = existing.When
		} else if new.When != "" {
			combinedWhen = new.When
		}
		merged := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: tempBp.TerraformComponents[0],
			Strategy:           "merge",
			Priority:           newPriority,
			When:               combinedWhen,
		}
		entries[componentID] = merged
	case "remove":
		accumulated := p.accumulateTerraformRemovals(existing.TerraformComponent, new.TerraformComponent)
		combinedWhen := ""
		if existing.When != "" && new.When != "" {
			combinedWhen = fmt.Sprintf("(%s) && (%s)", existing.When, new.When)
		} else if existing.When != "" {
			combinedWhen = existing.When
		} else if new.When != "" {
			combinedWhen = new.When
		}
		entries[componentID] = &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: accumulated,
			Strategy:           "remove",
			Priority:           newPriority,
			When:               combinedWhen,
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

	if existing.When == "" && new.When != "" {
		shouldInclude, err := p.shouldIncludeComponent(new.When, "")
		if err == nil && !shouldInclude {
			delete(entries, name)
			return nil
		}
	} else if existing.When != "" {
		shouldInclude, err := p.shouldIncludeComponent(existing.When, "")
		if err == nil && !shouldInclude {
			delete(entries, name)
			return nil
		}
		if new.When != "" {
			shouldInclude, err := p.shouldIncludeComponent(new.When, "")
			if err == nil && !shouldInclude {
				delete(entries, name)
				return nil
			}
		}
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
		combinedWhen := ""
		if existing.When != "" && new.When != "" {
			combinedWhen = fmt.Sprintf("(%s) && (%s)", existing.When, new.When)
		} else if existing.When != "" {
			combinedWhen = existing.When
		} else if new.When != "" {
			combinedWhen = new.When
		}
		merged := &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: tempBp.Kustomizations[0],
			Strategy:      "merge",
			Priority:      newPriority,
			When:          combinedWhen,
		}
		entries[name] = merged
	case "remove":
		accumulated := p.accumulateKustomizationRemovals(existing.Kustomization, new.Kustomization)
		combinedWhen := ""
		if existing.When != "" && new.When != "" {
			combinedWhen = fmt.Sprintf("(%s) && (%s)", existing.When, new.When)
		} else if existing.When != "" {
			combinedWhen = existing.When
		} else if new.When != "" {
			combinedWhen = new.When
		}
		entries[name] = &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: accumulated,
			Strategy:      "remove",
			Priority:      newPriority,
			When:          combinedWhen,
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
	for componentID, entry := range terraformByID {
		if entry.When != "" {
			shouldInclude, err := p.shouldIncludeComponent(entry.When, "")
			if err != nil {
				return fmt.Errorf("error re-evaluating terraform component '%s' condition: %w", componentID, err)
			}
			if !shouldInclude {
				delete(terraformByID, componentID)
			}
		}
	}

	for name, entry := range kustomizationByName {
		if entry.When != "" {
			shouldInclude, err := p.shouldIncludeComponent(entry.When, "")
			if err != nil {
				return fmt.Errorf("error re-evaluating kustomization '%s' condition: %w", name, err)
			}
			if !shouldInclude {
				delete(kustomizationByName, name)
			}
		}
	}

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
// string "true". Returns false for nil (undefined variables) or boolean false. Returns an error
// if the expression is invalid or evaluates to an unexpected type.
func (p *BaseBlueprintProcessor) evaluateCondition(expr string, path string) (bool, error) {
	if !evaluator.ContainsExpression(expr) {
		expr = "${" + expr + "}"
	}
	evaluated, err := p.evaluator.Evaluate(expr, path, nil, false)
	if err != nil {
		return false, err
	}

	var result bool
	switch v := evaluated.(type) {
	case nil:
		result = false
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
// When scope is non-nil, it is merged into the evaluation environment (e.g. facet config so expressions
// can reference talos.controlplanes). If the result is nil (undefined path without ?? fallback), the key
// is included with an empty string value. If the result contains unresolved deferred expressions, the
// original expression is preserved for later evaluation. Returns the evaluated substitutions map or an
// error if evaluation fails for any substitution.
func (p *BaseBlueprintProcessor) evaluateSubstitutions(subs map[string]string, facetPath string, scope map[string]any) (map[string]string, error) {
	result := make(map[string]string)
	for key, value := range subs {
		evaluated, err := p.evaluator.Evaluate(value, facetPath, scope, false)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate '%s': %w", key, err)
		}
		if evaluator.ContainsExpression(evaluated) {
			result[key] = value
			continue
		}
		if evaluated == nil {
			result[key] = ""
		} else if str, ok := evaluated.(string); ok {
			result[key] = str
		} else {
			result[key] = fmt.Sprintf("%v", evaluated)
		}
	}
	return result, nil
}

// evaluateStringSlice evaluates a slice of strings, allowing expressions in each string.
// When scope is non-nil, it is merged into the evaluation environment (e.g. facet config).
// Uses evaluateDeferred=true to disallow deferred expressions (they will error). If an expression
// evaluates to an array, the array is flattened into the result. Empty strings are preserved so
// that facet-defined placeholder slots (e.g. conditional component that evaluates to "") remain in
// the result for consistent ordering and test expectations. Nil values are skipped. Returns the
// evaluated string slice, or an error if evaluation fails.
func (p *BaseBlueprintProcessor) evaluateStringSlice(slice []string, facetPath string, scope map[string]any) ([]string, error) {
	if len(slice) == 0 {
		return nil, nil
	}

	result := make([]string, 0, len(slice))
	for _, s := range slice {
		evaluated, err := p.evaluator.Evaluate(s, facetPath, scope, true)
		if err != nil {
			return nil, err
		}
		if evaluated == nil {
			continue
		}
		switch v := evaluated.(type) {
		case string:
			result = append(result, v)
		case []any:
			for _, item := range v {
				if item == nil {
					continue
				}
				var str string
				switch itemVal := item.(type) {
				case string:
					str = itemVal
				default:
					str = fmt.Sprintf("%v", itemVal)
				}
				result = append(result, str)
			}
		default:
			result = append(result, fmt.Sprintf("%v", v))
		}
	}

	return result, nil
}

// evaluateBooleanExpression evaluates a boolean expression string. When scope is non-nil, it is
// merged into the evaluation environment (e.g. facet config). Uses evaluateDeferred=true to
// disallow deferred expressions (they will error). Returns the evaluated boolean value, or an error
// if evaluation fails or the result is not a boolean.
func (p *BaseBlueprintProcessor) evaluateBooleanExpression(expr string, facetPath string, scope map[string]any) (*bool, error) {
	if expr == "" {
		return nil, nil
	}
	evaluated, err := p.evaluator.Evaluate(expr, facetPath, scope, true)
	if err != nil {
		return nil, err
	}
	var result bool
	switch v := evaluated.(type) {
	case bool:
		result = v
	case string:
		switch v {
		case "true":
			result = true
		case "false":
			result = false
		default:
			return nil, fmt.Errorf("expected boolean, got string %q", v)
		}
	default:
		return nil, fmt.Errorf("expected boolean, got %T", evaluated)
	}
	return &result, nil
}

// evaluateIntegerExpression evaluates an integer expression string. When scope is non-nil, it is
// merged into the evaluation environment (e.g. facet config). Uses evaluateDeferred=true to
// disallow deferred expressions (they will error). Returns the evaluated integer value, or an error
// if evaluation fails or the result is not an integer.
func (p *BaseBlueprintProcessor) evaluateIntegerExpression(expr string, facetPath string, scope map[string]any) (*int, error) {
	if expr == "" {
		return nil, nil
	}
	evaluated, err := p.evaluator.Evaluate(expr, facetPath, scope, true)
	if err != nil {
		return nil, err
	}
	var result int
	switch v := evaluated.(type) {
	case int:
		result = v
	case int64:
		result = int(v)
	case float64:
		result = int(v)
	case string:
		parsed, err := fmt.Sscanf(v, "%d", &result)
		if err != nil || parsed != 1 {
			return nil, fmt.Errorf("expected integer, got string %q", v)
		}
	default:
		return nil, fmt.Errorf("expected integer, got %T", evaluated)
	}
	return &result, nil
}

// =============================================================================
// Helpers
// =============================================================================

// mergeConfigMaps merges facet config blocks into the accumulated global scope.
// When the same block name exists in both, block bodies are deep-merged recursively (overlay overwrites).
// Returns a new map; does not mutate inputs.
func mergeConfigMaps(globalScope map[string]any, facetConfig map[string]any) map[string]any {
	out := make(map[string]any)
	maps.Copy(out, globalScope)
	for name, body := range facetConfig {
		existing, ok := out[name].(map[string]any)
		if ok && body != nil {
			if newBody, ok2 := body.(map[string]any); ok2 {
				out[name] = deepMergeMap(existing, newBody)
				continue
			}
		}
		out[name] = body
	}
	return out
}

// deepMergeMap recursively merges overlay into base. Values in overlay overwrite base for the same key.
// When both values are maps, they are merged recursively. Returns a new map; does not mutate inputs.
func deepMergeMap(base, overlay map[string]any) map[string]any {
	result := make(map[string]any, len(base)+len(overlay))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		existing, ok := result[k].(map[string]any)
		if ok && v != nil {
			if overlayMap, ok2 := v.(map[string]any); ok2 {
				result[k] = deepMergeMap(existing, overlayMap)
				continue
			}
		}
		result[k] = v
	}
	return result
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

// =============================================================================
// Interface Compliance
// =============================================================================

var _ BlueprintProcessor = (*BaseBlueprintProcessor)(nil)
