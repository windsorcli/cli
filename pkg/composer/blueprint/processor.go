package blueprint

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"sync"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// The BaseBlueprintProcessor is a core component that processes blueprint facets and evaluates conditional logic.
// It provides functionality for evaluating 'when' conditions on facets, terraform components, and kustomizations.
// The BaseBlueprintProcessor orchestrates the collection, merging, and application of components based on ordinal
// and strategy, ensuring deterministic and predictable blueprint composition from multiple facet sources.

// =============================================================================
// Constants
// =============================================================================

// strategyPrecedence is the tiebreaker when component ordinals are equal: remove > replace > merge.
var strategyPrecedence = map[string]int{
	"remove":  3,
	"replace": 2,
	"merge":   1,
}

// =============================================================================
// Interfaces
// =============================================================================

// BlueprintProcessor evaluates when: conditions on facets, terraform components, and kustomizations.
// It determines inclusion/exclusion based on boolean condition results.
// The processor is stateless and shared across all loaders.
// ProcessFacets returns the evaluated config scope and block order for the loader so callers can merge
// scopes from multiple loaders (e.g. for user overlay and final terraform input evaluation).
type BlueprintProcessor interface {
	ProcessFacets(target *blueprintv1alpha1.Blueprint, facets []blueprintv1alpha1.Facet, sourceName ...string) (scope map[string]any, order []string, err error)
}

// =============================================================================
// Types
// =============================================================================

// BaseBlueprintProcessor provides the default implementation of the BlueprintProcessor interface.
type BaseBlueprintProcessor struct {
	runtime        *runtime.Runtime
	evaluator      evaluator.ExpressionEvaluator
	traceCollector TraceCollector
	mu             sync.Mutex
	deferredPaths  map[string]bool
	extraScope     map[string]any
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
		runtime:       rt,
		evaluator:     rt.Evaluator,
		deferredPaths: make(map[string]bool),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// SetTraceCollector sets the trace collector for recording per-key contributions and config
// blocks during composition. When non-nil, all contributions are recorded into the collector.
// Set to nil to disable trace collection.
func (p *BaseBlueprintProcessor) SetTraceCollector(tc TraceCollector) {
	p.traceCollector = tc
}

// SetExtraScope sets additional scope values that are merged over context values during facet processing.
func (p *BaseBlueprintProcessor) SetExtraScope(scope map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if scope == nil {
		p.extraScope = nil
		return
	}
	p.extraScope = make(map[string]any, len(scope))
	for k, v := range scope {
		p.extraScope[k] = v
	}
}

// GetDeferredPaths returns a copy of deferred composed paths discovered during the last ProcessFacets call.
func (p *BaseBlueprintProcessor) GetDeferredPaths() map[string]bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.deferredPaths) == 0 {
		return nil
	}
	out := make(map[string]bool, len(p.deferredPaths))
	for k, v := range p.deferredPaths {
		out[k] = v
	}
	return out
}

// ProcessFacets iterates facets, evaluating each facet's 'when' against config data. Facets with
// true (or unset) conditions contribute their terraform components, kustomizations, and config blocks to target.
// Components in facets may have 'when' for granular control. Facets are sorted by ordinal (asc),
// then by metadata.name (tiebreak). Higher ordinal means higher precedence when merging. Config blocks,
// terraform components, and kustomizations are merged by ordinal (higher wins), then by strategy precedence
// (remove > replace > merge) when ordinals match. Config block expressions are evaluated once per round
// after all same-name blocks are merged, so expressions see the final merged value for each block.
// If sourceName is set, it updates Source on components lacking it. The target blueprint is modified in place.
// Returns: evaluated config scope and block order for the loader. Runtime ConfigHandler context values are
// merged over facet-derived scope so 'when' or component expressions use the actual config.
func (p *BaseBlueprintProcessor) ProcessFacets(target *blueprintv1alpha1.Blueprint, facets []blueprintv1alpha1.Facet, sourceName ...string) (map[string]any, []string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deferredPaths = make(map[string]bool)
	if target == nil {
		return nil, nil, fmt.Errorf("target blueprint cannot be nil")
	}

	if len(facets) == 0 {
		return nil, nil, nil
	}

	var contextScope map[string]any
	if p.runtime != nil && p.runtime.ConfigHandler != nil {
		if vals, err := p.runtime.ConfigHandler.GetContextValues(); err == nil {
			contextScope = vals
		}
	}
	if p.extraScope != nil {
		contextScope = MergeScopeMaps(contextScope, p.extraScope)
	}

	sortedFacets := make([]blueprintv1alpha1.Facet, len(facets))
	copy(sortedFacets, facets)
	sort.Slice(sortedFacets, func(i, j int) bool {
		oi, oj := resolvedFacetOrdinal(sortedFacets[i]), resolvedFacetOrdinal(sortedFacets[j])
		if oi != oj {
			return oi < oj
		}
		return sortedFacets[i].Metadata.Name < sortedFacets[j].Metadata.Name
	})

	terraformByID := make(map[string]*blueprintv1alpha1.ConditionalTerraformComponent)
	kustomizationByName := make(map[string]*blueprintv1alpha1.ConditionalKustomization)
	scope := contextScope
	var globalScope map[string]any
	var cfgEntries map[string]*blueprintv1alpha1.ConfigBlock
	var configBlockOrder []string
	includedFacets := make([]blueprintv1alpha1.Facet, 0, len(sortedFacets))
	prevIncludedSet := make(map[string]bool)
	const maxFacetRounds = 10
	for range make([]struct{}, maxFacetRounds) {
		includedFacets = includedFacets[:0]
		globalScope = nil
		cfgEntries = nil
		configBlockOrder = nil
		passScope := scope
		if passScope == nil && contextScope != nil {
			passScope = maps.Clone(contextScope)
		}
		if passScope == nil {
			passScope = make(map[string]any)
		}
		for _, facet := range sortedFacets {
			shouldInclude, err := p.shouldIncludeFacet(facet, passScope)
			if err != nil {
				return nil, nil, err
			}
			if !shouldInclude {
				continue
			}
			includedFacets = append(includedFacets, facet)
			var errMerge error
			globalScope, cfgEntries, configBlockOrder, errMerge = p.mergeFacetScopeIntoGlobal(facet, globalScope, cfgEntries, configBlockOrder, passScope)
			if errMerge != nil {
				return nil, nil, fmt.Errorf("facet %s: %w", facet.Metadata.Name, errMerge)
			}
		}
		if err := p.evaluateGlobalScopeConfig(globalScope, configBlockOrder, contextScope); err != nil {
			return nil, nil, err
		}
		mergeBase := contextScope
		if mergeBase == nil {
			mergeBase = make(map[string]any)
		}
		passScope = blueprintv1alpha1.DeepMergeMaps(mergeBase, globalScope)
		scope = passScope
		currSet := make(map[string]bool, len(includedFacets))
		for _, f := range includedFacets {
			currSet[f.Metadata.Name] = true
		}
		if len(currSet) == len(prevIncludedSet) {
			eq := true
			for n := range currSet {
				if !prevIncludedSet[n] {
					eq = false
					break
				}
			}
			if eq {
				break
			}
		}
		prevIncludedSet = currSet
	}

	for _, facet := range includedFacets {
		if err := p.collectTerraformComponents(facet, sourceName, terraformByID, scope); err != nil {
			return nil, nil, err
		}
		if err := p.collectKustomizations(facet, sourceName, kustomizationByName, scope); err != nil {
			return nil, nil, err
		}
	}

	if err := p.applyCollectedComponents(target, terraformByID, kustomizationByName, scope); err != nil {
		return nil, nil, err
	}
	return globalScope, configBlockOrder, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// recordTrace records a per-key contribution to the trace collector when set.
func (p *BaseBlueprintProcessor) recordTrace(composedPath string, tc TraceContribution) {
	if p.traceCollector != nil {
		p.traceCollector.RecordContribution(composedPath, tc)
	}
}

// recordConfigTrace records a config block value to the trace collector when set.
func (p *BaseBlueprintProcessor) recordConfigTrace(configPath string, record ConfigBlockRecord) {
	if p.traceCollector != nil {
		p.traceCollector.RecordConfigBlock(configPath, record)
	}
}

// markDeferredPath records a composed path as deferred for consumer commands like show/explain.
func (p *BaseBlueprintProcessor) markDeferredPath(composedPath string) {
	if composedPath == "" {
		return
	}
	if p.deferredPaths == nil {
		p.deferredPaths = make(map[string]bool)
	}
	p.deferredPaths[composedPath] = true
}

// scopeForConfigBlock builds the evaluation scope for evaluating a single config block so that
// facet-derived config (globalScope) wins over context for all keys; the current block is set to
// currentBlockValue when non-nil (e.g. in-place evaluation or resolve pass), otherwise to
// contextScope[blockName] or omitted to avoid self-reference. This prevents context from
// overwriting other blocks' facet values when one block has a scalar value.
func (p *BaseBlueprintProcessor) scopeForConfigBlock(contextScope, globalScope map[string]any, blockName string, currentBlockValue any) map[string]any {
	if globalScope == nil {
		globalScope = make(map[string]any)
	}
	if contextScope == nil {
		contextScope = make(map[string]any)
	}
	scope := blueprintv1alpha1.DeepMergeMaps(contextScope, globalScope)
	if currentBlockValue != nil {
		scope[blockName] = currentBlockValue
	} else if contextScope != nil && contextScope[blockName] != nil {
		scope[blockName] = contextScope[blockName]
	} else {
		delete(scope, blockName)
	}
	return scope
}

// mergeFacetScopeIntoGlobal merges the facet's config block structure into the global scope
// (accumulated from prior facets) without evaluating config body expressions. Returns the
// updated global scope, per-block meta (ordinal and strategy), and config block name order.
// Config body expressions are evaluated later in evaluateGlobalScopeConfig.
// For a given name, only blocks whose when condition is true contribute; if multiple blocks
// with the same name have when true, their bodies are deep-merged in list order (later overlay).
// Merge precedence: higher ordinal wins; when ordinals match, strategy precedence remove > replace > merge.
func (p *BaseBlueprintProcessor) mergeFacetScopeIntoGlobal(facet blueprintv1alpha1.Facet, globalScope map[string]any, existing map[string]*blueprintv1alpha1.ConfigBlock, order []string, contextScope map[string]any) (map[string]any, map[string]*blueprintv1alpha1.ConfigBlock, []string, error) {
	facetOrdinal := resolvedFacetOrdinal(facet)
	incoming := make(map[string]*blueprintv1alpha1.ConfigBlock)
	byName := make(map[string][]any)
	nameOrder := make([]string, 0)
	seen := make(map[string]bool)
	for _, block := range facet.Config {
		if block.Name == "" {
			continue
		}
		if block.When != "" {
			shouldInclude, err := p.shouldIncludeComponent(block.When, facet.Path, contextScope)
			if err != nil {
				return nil, nil, order, fmt.Errorf("config block %q when: %w", block.Name, err)
			}
			if !shouldInclude {
				continue
			}
		}
		if !seen[block.Name] {
			seen[block.Name] = true
			nameOrder = append(nameOrder, block.Name)
		}
		ordinal := facetOrdinal
		if block.Ordinal != nil {
			ordinal = *block.Ordinal
		}
		incoming[block.Name] = &blueprintv1alpha1.ConfigBlock{
			Name:     block.Name,
			Strategy: block.Strategy,
			Ordinal:  &ordinal,
		}
		var rawValue any
		if block.Body != nil {
			if v, ok := block.Body["value"]; ok && v != nil {
				rawValue = v
			}
		}
		if rawValue == nil {
			rawValue = make(map[string]any)
		}
		if vm, ok := asMapStringAny(rawValue); ok {
			p.recordConfigTrace("config."+block.Name, ConfigBlockRecord{
				FacetPath: facet.Path,
				Line:      yamlNodeLine(facet.Path, "config", namedItem(block.Name)),
				RawValue:  deepCopyValue(rawValue),
			})
			for key, val := range vm {
				configPath := "config." + block.Name + "." + key
				p.recordConfigTrace(configPath, ConfigBlockRecord{
					FacetPath: facet.Path,
					Line:      yamlNodeLine(facet.Path, "config", namedItem(block.Name), "value", mapKeyLine(key)),
					RawValue:  deepCopyValue(val),
				})
			}
		} else {
			configPath := "config." + block.Name
			p.recordConfigTrace(configPath, ConfigBlockRecord{
				FacetPath: facet.Path,
				Line:      yamlNodeLine(facet.Path, "config", namedItem(block.Name)),
				RawValue:  deepCopyValue(rawValue),
			})
		}
		byName[block.Name] = append(byName[block.Name], rawValue)
	}
	for _, name := range nameOrder {
		bodies := byName[name]
		if len(bodies) == 0 {
			continue
		}
		var body any
		if len(bodies) == 1 {
			body = bodies[0]
		} else {
			allMaps := true
			for _, b := range bodies {
				if _, ok := asMapStringAny(b); !ok {
					allMaps = false
					break
				}
			}
			if allMaps {
				merged, _ := asMapStringAny(bodies[0])
				for i := 1; i < len(bodies); i++ {
					next, _ := asMapStringAny(bodies[i])
					merged = deepMergeMap(merged, next)
				}
				body = merged
			} else {
				body = bodies[len(bodies)-1]
			}
		}
		cb := incoming[name]
		cb.Body = map[string]any{"value": body}
		for i := 0; i < len(order); i++ {
			if order[i] == name {
				order = append(order[:i], order[i+1:]...)
				break
			}
		}
		order = append(order, name)
	}
	if len(incoming) == 0 {
		return globalScope, existing, order, nil
	}
	mergedScope, mergedEntries, err := p.mergeConfigBlocks(globalScope, existing, incoming)
	if err != nil {
		return nil, nil, order, err
	}
	return mergedScope, mergedEntries, order, nil
}

// mergeConfigBlocks merges incoming config blocks into the global scope using ordinal and strategy.
// Higher ordinal wins; equal ordinals use strategy precedence (remove > replace > merge);
// equal ordinal and equal strategy merges new over existing (matching terraform/kustomize behavior).
// Returns the updated scope, updated entries, or an error on invalid strategy.
func (p *BaseBlueprintProcessor) mergeConfigBlocks(scope map[string]any, existing, incoming map[string]*blueprintv1alpha1.ConfigBlock) (map[string]any, map[string]*blueprintv1alpha1.ConfigBlock, error) {
	out := make(map[string]any)
	maps.Copy(out, scope)
	if existing == nil {
		existing = make(map[string]*blueprintv1alpha1.ConfigBlock)
	}
	merged := make(map[string]*blueprintv1alpha1.ConfigBlock, len(existing))
	for k, v := range existing {
		merged[k] = v
	}
	for name, inc := range incoming {
		strategy := inc.Strategy
		if strategy == "" {
			strategy = "merge"
		}
		if strategy != "merge" && strategy != "replace" && strategy != "remove" {
			return nil, nil, fmt.Errorf("invalid strategy %q for config block %q: must be 'merge', 'replace', or 'remove'", strategy, name)
		}
		incOrdinal := 0
		if inc.Ordinal != nil {
			incOrdinal = *inc.Ordinal
		}
		if prev, hasPrev := merged[name]; hasPrev {
			prevOrdinal := 0
			if prev.Ordinal != nil {
				prevOrdinal = *prev.Ordinal
			}
			prevStrategy := prev.Strategy
			if prevStrategy == "" {
				prevStrategy = "merge"
			}
			if incOrdinal < prevOrdinal {
				continue
			}
			if incOrdinal == prevOrdinal && strategyPrecedence[strategy] < strategyPrecedence[prevStrategy] {
				continue
			}
		}
		resolved := inc.DeepCopy()
		resolved.Strategy = strategy
		merged[name] = resolved
		var rawValue any
		if inc.Body == nil {
			rawValue = nil
		} else {
			rawValue = inc.Body["value"]
		}
		switch strategy {
		case "remove":
			delete(out, name)
		case "replace":
			out[name] = rawValue
		case "merge":
			exMap, exOk := asMapStringAny(out[name])
			newMap, newOk := asMapStringAny(rawValue)
			if exOk && newOk && rawValue != nil {
				out[name] = deepMergeMap(exMap, newMap)
				continue
			}
			out[name] = rawValue
		}
	}
	return out, merged, nil
}

// evaluateGlobalScopeConfig evaluates all config block body expressions in globalScope in
// blueprint context. The evaluation scope is contextScope (values from ConfigHandler) merged
// with globalScope (facet config blocks) so expressions can reference both (e.g.
// cluster.controlplanes.schedulable from values and talos.controlplanes from config blocks).
// Same-block references are supported by re-evaluating each block until stable. Mutates
// globalScope in place. Block names are iterated in configBlockOrder.
func (p *BaseBlueprintProcessor) evaluateGlobalScopeConfig(globalScope map[string]any, configBlockOrder []string, contextScope map[string]any) error {
	if globalScope == nil {
		return nil
	}
	if contextScope == nil && p.runtime != nil && p.runtime.ConfigHandler != nil {
		if vals, err := p.runtime.ConfigHandler.GetContextValues(); err == nil {
			contextScope = vals
		}
	}
	names := configBlockOrder
	if len(names) == 0 {
		names = make([]string, 0, len(globalScope))
		for name := range globalScope {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	const maxSameBlockPasses = 5
	const maxCrossBlockRounds = 5
	for round := 0; round < maxCrossBlockRounds; round++ {
		anyBlockChanged := false
		for _, name := range names {
			body := globalScope[name]
			bodyMap, ok := body.(map[string]any)
			if !ok {
				if !containsExpressionInValue(body) {
					continue
				}
				scopeWithBlock := p.scopeForConfigBlock(contextScope, globalScope, name, nil)
				evaluated, err := p.evaluateConfigBlockValue(body, "", scopeWithBlock)
				if err != nil {
					return fmt.Errorf("config block %q: %w", name, err)
				}
				globalScope[name] = evaluated
				anyBlockChanged = true
				continue
			}
			if len(bodyMap) == 0 {
				continue
			}
			oldBody := bodyMap
			current := bodyMap
			var derivedKeys map[string]string
			for k, v := range bodyMap {
				if s, ok := v.(string); ok && evaluator.ContainsExpression(s) && expressionIsDerivedFromBlock(s, name) {
					if derivedKeys == nil {
						derivedKeys = make(map[string]string)
					}
					derivedKeys[k] = s
				}
			}
			var previousEvaluatedOnly map[string]any
			for pass := 0; pass < maxSameBlockPasses; pass++ {
				var blockVal any
				if pass > 0 {
					blockVal = current
				}
				scopeWithBlock := p.scopeForConfigBlock(contextScope, globalScope, name, blockVal)
				nonDerivedBody := make(map[string]any, len(bodyMap))
				for k, v := range bodyMap {
					if _, isDerived := derivedKeys[k]; isDerived {
						continue
					}
					nonDerivedBody[k] = v
				}
				evaluated, err := p.evaluator.EvaluateMap(nonDerivedBody, "", scopeWithBlock, false)
				if err != nil {
					return fmt.Errorf("config block %q: %w", name, err)
				}
				if normalized, ok := normalizeDeferredValue(evaluated).(map[string]any); ok {
					evaluated = normalized
				}
				if previousEvaluatedOnly != nil && reflect.DeepEqual(evaluated, previousEvaluatedOnly) && !containsExpressionInValue(evaluated) {
					for k, orig := range derivedKeys {
						evaluated[k] = orig
					}
					current = evaluated
					break
				}
				previousEvaluatedOnly = deepCopyMapStringAny(evaluated)
				for k, orig := range derivedKeys {
					evaluated[k] = orig
				}
				current = evaluated
			}
			scopeWithBlock := p.scopeForConfigBlock(contextScope, globalScope, name, current)
			const maxResolvePasses = 3
			for resolvePass := 0; resolvePass < maxResolvePasses; resolvePass++ {
				resolvedAny := false
				for k, v := range current {
					s, ok := v.(string)
					if !ok || !evaluator.ContainsExpression(s) {
						continue
					}
					if expressionIsDerivedFromBlock(s, name) {
						continue
					}
					resolved, err := p.evaluator.Evaluate(s, "", scopeWithBlock, false)
					if err != nil || resolved == nil {
						continue
					}
					if reflect.DeepEqual(resolved, v) {
						continue
					}
					current[k] = normalizeDeferredValue(resolved)
					scopeWithBlock[name] = current
					resolvedAny = true
				}
				if !resolvedAny {
					break
				}
			}
			globalScope[name] = current
			if !reflect.DeepEqual(current, oldBody) {
				anyBlockChanged = true
			}
		}
		if !anyBlockChanged {
			break
		}
	}
	return nil
}

// evaluateConfigBlockValue recursively evaluates a config block value (scalar, list, or map) so that
// expressions in scalar or list entries are evaluated. Used when a config block has value: <scalar>
// or value: [ ... ] instead of a map. Returns the evaluated value or an error.
func (p *BaseBlueprintProcessor) evaluateConfigBlockValue(v any, path string, scope map[string]any) (any, error) {
	switch x := v.(type) {
	case string:
		if !evaluator.ContainsExpression(x) {
			return x, nil
		}
		evaluated, err := p.evaluator.Evaluate(x, path, scope, false)
		if err != nil {
			return nil, err
		}
		return normalizeDeferredValue(evaluated), nil
	case map[string]any:
		evaluated, err := p.evaluator.EvaluateMap(x, path, scope, false)
		if err != nil {
			return nil, err
		}
		return normalizeDeferredValue(evaluated), nil
	case []any:
		out := make([]any, 0, len(x))
		for i, item := range x {
			evaluated, err := p.evaluateConfigBlockValue(item, path, scope)
			if err != nil {
				return nil, fmt.Errorf("config block list[%d]: %w", i, err)
			}
			out = append(out, evaluated)
		}
		return out, nil
	default:
		return v, nil
	}
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

		shouldInclude, err := p.shouldIncludeComponent(componentWhen, facet.Path, facetScope)
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
			origins := make(map[string]string, len(evaluated))
			componentID := processed.GetID()
			for k, v := range evaluated {
				origins[k] = facet.Path
				composedPath := "terraform." + componentID + ".inputs." + k
				originalVal, _ := processed.Inputs[k]
				resolvedValue, deferred := p.resolveTerraformInputDeferredState(v, originalVal, facet.Path, facetScope)
				if deferred {
					p.markDeferredPath(composedPath)
				}
				v = resolvedValue
				v = normalizeDeferredValue(v)
				if m := blueprintv1alpha1.ToMapStringAny(v); m != nil {
					normalized[k] = m
				} else if s := blueprintv1alpha1.ToSliceAny(v); s != nil {
					normalized[k] = s
				} else {
					normalized[k] = v
				}
			}
			processed.Inputs = normalized
			processed.InputOrigins = origins
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

		facetOrd := resolvedFacetOrdinal(facet)
		effectiveOrdinal := facetOrd
		if processed.Ordinal != nil {
			effectiveOrdinal = *processed.Ordinal
		}
		processed.Ordinal = &effectiveOrdinal

		strategy := processed.Strategy
		if strategy == "" {
			strategy = "merge"
		}

		componentID := processed.GetID()
		sn := ""
		if len(sourceName) > 0 {
			sn = sourceName[0]
		}
		for k, v := range tc.Inputs {
			composedPath := "terraform." + componentID + ".inputs." + k
			p.recordTrace(composedPath, TraceContribution{
				FacetPath:  facet.Path,
				SourceName: sn,
				Ordinal:    effectiveOrdinal,
				Strategy:   strategy,
				Line:       yamlNodeLine(facet.Path, "terraform", namedItem(componentID), "inputs", k),
				RawValue:   deepCopyValue(v),
			})
		}

		if _, exists := terraformByID[componentID]; !exists {
			processed.Strategy = strategy
			terraformByID[componentID] = &processed
		} else {
			if err := p.updateTerraformComponentEntry(componentID, &processed, strategy, terraformByID, facetScope); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolveTerraformInputDeferredState determines whether a terraform input should be marked deferred for display.
func (p *BaseBlueprintProcessor) resolveTerraformInputDeferredState(value any, originalValue any, facetPath string, facetScope map[string]any) (any, bool) {
	if evaluator.ContainsSecretValue(value) {
		return value, true
	}
	s, ok := originalValue.(string)
	if !ok || !evaluator.ContainsExpression(s) {
		return value, false
	}
	probe, err := p.evaluator.Evaluate(s, facetPath, facetScope, false)
	if err != nil || !evaluator.IsDeferredValue(probe) {
		return value, false
	}
	if expr, ok := evaluator.DeferredExpression(probe); ok {
		return expr, true
	}
	return value, true
}

// collectKustomizations processes all kustomizations from a facet, evaluating their conditions,
// substitutions, and source assignments. When facetScope is non-nil (evaluated facet config), it is
// merged into the expression environment so substitutions and other expressions can reference config
// block values. Kustomizations are collected into the kustomizationByName map; merge precedence
// is ordinal (higher wins), then strategy precedence (remove > replace > merge) when ordinals are equal.
// If a kustomization has an empty 'when' condition, it inherits the facet-level condition.
// Returns an error if condition evaluation or substitution processing fails.
func (p *BaseBlueprintProcessor) collectKustomizations(facet blueprintv1alpha1.Facet, sourceName []string, kustomizationByName map[string]*blueprintv1alpha1.ConditionalKustomization, facetScope map[string]any) error {
	for _, k := range facet.Kustomizations {
		componentWhen := k.When
		if componentWhen == "" && facet.When != "" {
			componentWhen = facet.When
		}

		shouldInclude, err := p.shouldIncludeComponent(componentWhen, facet.Path, facetScope)
		if err != nil {
			return fmt.Errorf("error evaluating kustomization condition: %w", err)
		}
		if !shouldInclude {
			continue
		}

		processed := k
		processed.When = componentWhen
		if processed.Substitutions != nil {
			evaluated, deferredKeys, err := p.evaluateSubstitutions(processed.Substitutions, facet.Path, facetScope)
			if err != nil {
				return fmt.Errorf("error evaluating substitutions for kustomization '%s': %w", processed.Name, err)
			}
			processed.Substitutions = evaluated
			for key, isDeferred := range deferredKeys {
				if isDeferred {
					p.markDeferredPath("kustomize." + processed.Name + ".substitutions." + key)
				}
			}
		}
		if evaluator.ContainsExpression(processed.Path) {
			evaluatedPath, err := p.evaluator.Evaluate(processed.Path, facet.Path, facetScope, false)
			if err != nil {
				return fmt.Errorf("error evaluating path for kustomization '%s': %w", processed.Name, err)
			}
			if containsDeferredInValue(evaluatedPath) {
				p.markDeferredPath("kustomize." + processed.Name + ".path")
			}
			evaluatedPath = normalizeDeferredValue(evaluatedPath)
			if pathStr, ok := evaluatedPath.(string); ok {
				processed.Path = pathStr
			} else if evaluatedPath != nil {
				processed.Path = fmt.Sprintf("%v", evaluatedPath)
			}
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

		facetOrd := resolvedFacetOrdinal(facet)
		effectiveOrdinal := facetOrd
		if processed.Ordinal != nil {
			effectiveOrdinal = *processed.Ordinal
		}
		processed.Ordinal = &effectiveOrdinal

		strategy := processed.Strategy
		if strategy == "" {
			strategy = "merge"
		}

		sn := ""
		if len(sourceName) > 0 {
			sn = sourceName[0]
		}
		for sk, sv := range k.Substitutions {
			composedPath := "kustomize." + processed.Name + ".substitutions." + sk
			p.recordTrace(composedPath, TraceContribution{
				FacetPath:  facet.Path,
				SourceName: sn,
				Ordinal:    effectiveOrdinal,
				Strategy:   strategy,
				Line:       yamlNodeLine(facet.Path, "kustomize", namedItem(processed.Name), "substitutions", sk),
				RawValue:   deepCopyValue(sv),
			})
		}
		if len(k.Components) > 0 {
			componentsPath := "kustomize." + processed.Name + ".components"
			p.recordTrace(componentsPath, TraceContribution{
				FacetPath:     facet.Path,
				SourceName:    sn,
				Ordinal:       effectiveOrdinal,
				Strategy:      strategy,
				Line:          yamlNodeLine(facet.Path, "kustomize", namedItem(processed.Name), "components"),
				RawComponents: slices.Clone(k.Components),
			})
		}

		if _, exists := kustomizationByName[processed.Name]; !exists {
			processed.Strategy = strategy
			entry := new(blueprintv1alpha1.ConditionalKustomization)
			*entry = processed
			kustomizationByName[processed.Name] = entry
		} else {
			if err := p.updateKustomizationEntry(processed.Name, &processed, strategy, kustomizationByName, facetScope); err != nil {
				return err
			}
		}
	}
	return nil
}

// shouldIncludeFacet evaluates whether a facet should be included based on its 'when' condition.
// Returns true if the facet has no condition or if the condition evaluates to true. Returns
// false if the condition evaluates to false. Returns an error if condition evaluation fails.
func (p *BaseBlueprintProcessor) shouldIncludeFacet(facet blueprintv1alpha1.Facet, scope map[string]any) (bool, error) {
	if facet.When == "" {
		return true, nil
	}
	matches, err := p.evaluateCondition(facet.When, facet.Path, scope)
	if err != nil {
		return false, fmt.Errorf("error evaluating facet '%s' condition: %w", facet.Metadata.Name, err)
	}
	return matches, nil
}

// shouldIncludeComponent evaluates whether a component or kustomization should be included based
// on its 'when' condition. Returns true if there is no condition or if the condition evaluates
// to true. Returns false if the condition evaluates to false. Returns an error if condition
// evaluation fails.
func (p *BaseBlueprintProcessor) shouldIncludeComponent(when string, facetPath string, scope map[string]any) (bool, error) {
	if when == "" {
		return true, nil
	}
	matches, err := p.evaluateCondition(when, facetPath, scope)
	if err != nil {
		return false, err
	}
	return matches, nil
}

// updateTerraformComponentEntry updates or merges a single ConditionalTerraformComponent entry in the component
// collection based on ordinal, strategy, and conditional 'when' expressions. Higher ordinal wins; when ordinals are
// equal, strategy precedence is used ('remove' > 'replace' > 'merge'), then merge behavior for equal ordinal and strategy. The function also rigorously re-evaluates 'when' conditions for
// both the new and existing entries, removing entries from the collection if any relevant condition now resolves to false.
// When the strategy is 'merge', it performs a strategic pre-merge and logically ANDs 'when' conditions. For 'remove',
// component removals are accumulated; for 'replace', the new definition overwrites the existing one. Only valid strategies are allowed; otherwise, an error is returned, as is the case for merge
// failures. This function is critical to the blueprint processor’s ability to aggregate, override, conditionally include
// or exclude, and deconflict terraform components efficiently, making it safe to combine blueprint facets or overrides
// without unintended duplication or omission. Returns an error if strategies are invalid or pre-merge fails.
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
		return fmt.Errorf("invalid strategy '%s' for terraform component '%s': must be 'remove', 'replace', or 'merge'", strategy, componentID)
	}

	newOrdinal := 0
	if new.Ordinal != nil {
		newOrdinal = *new.Ordinal
	}
	existingOrdinal := 0
	if existing.Ordinal != nil {
		existingOrdinal = *existing.Ordinal
	}

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
		return nil
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

// applyTerraformComponentEntryByStrategy updates the terraform component entry by applying new over existing
// according to strategy: replace overwrites; merge strategic-merges (existing base, new overlay); remove accumulates removals.
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
			return fmt.Errorf("error pre-merging terraform component '%s': %w", componentID, err)
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
		return fmt.Errorf("invalid strategy '%s' for terraform component '%s': must be 'remove', 'replace', or 'merge'", strategy, componentID)
	}
}

// updateKustomizationEntry updates an existing kustomization entry in the collection map based
// on ordinal and strategy. Ordinal is compared first: higher ordinal wins. If ordinals are
// equal, strategy precedence is used (remove > replace > merge). If both ordinal and strategy are
// equal, kustomizations are pre-merged (merge), removals are accumulated (remove), or new replaces
// existing (replace). Returns an error if the merge operation fails.
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
		return fmt.Errorf("invalid strategy '%s' for kustomization '%s': must be 'remove', 'replace', or 'merge'", strategy, name)
	}

	newOrdinal := 0
	if new.Ordinal != nil {
		newOrdinal = *new.Ordinal
	}
	existingOrdinal := 0
	if existing.Ordinal != nil {
		existingOrdinal = *existing.Ordinal
	}

	if newOrdinal > existingOrdinal {
		if existingStrategy == "remove" || strategy == "remove" {
			new.Strategy = strategy
			entries[name] = new
			return nil
		}
		return p.applyKustomizationEntryByStrategy(name, new, strategy, existing, entries)
	}

	if newOrdinal < existingOrdinal {
		return nil
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

// applyKustomizationEntryByStrategy updates the kustomization entry by applying new over existing
// according to strategy: replace overwrites; merge combines list fields (components, dependsOn, etc.);
// remove accumulates removals. combinedWhen is built from existing and new When expressions.
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
			return fmt.Errorf("error pre-merging kustomization '%s': %w", name, err)
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
		return fmt.Errorf("invalid strategy '%s' for kustomization '%s': must be 'remove', 'replace', or 'merge'", strategy, name)
	}
}

// applyCollectedComponents applies all collected components and kustomizations to the target
// blueprint in the documented order: replace operations first, then merge operations, then remove
// operations last. This ensures that remove operations are applied after all merge/replace
// operations, as documented. Returns an error if any application operation fails.
func (p *BaseBlueprintProcessor) applyCollectedComponents(target *blueprintv1alpha1.Blueprint, terraformByID map[string]*blueprintv1alpha1.ConditionalTerraformComponent, kustomizationByName map[string]*blueprintv1alpha1.ConditionalKustomization, scope map[string]any) error {
	for componentID, entry := range terraformByID {
		if entry.When != "" {
			shouldInclude, err := p.shouldIncludeComponent(entry.When, "", scope)
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
			shouldInclude, err := p.shouldIncludeComponent(entry.When, "", scope)
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
// if the expression is invalid or evaluates to an unexpected type. When scope is non-nil (e.g.
// context merged with facet scope), expressions can reference values like workstation.runtime.
func (p *BaseBlueprintProcessor) evaluateCondition(expr string, path string, scope map[string]any) (bool, error) {
	if !evaluator.ContainsExpression(expr) {
		expr = "${" + expr + "}"
	}
	evaluated, err := p.evaluator.Evaluate(expr, path, scope, false)
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
func (p *BaseBlueprintProcessor) evaluateSubstitutions(subs map[string]string, facetPath string, scope map[string]any) (map[string]string, map[string]bool, error) {
	result := make(map[string]string)
	deferredKeys := make(map[string]bool)
	for key, value := range subs {
		evaluated, err := p.evaluator.Evaluate(value, facetPath, scope, false)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to evaluate '%s': %w", key, err)
		}
		if evaluator.IsDeferredValue(evaluated) {
			if deferredExpr, ok := evaluator.DeferredExpression(evaluated); ok {
				result[key] = deferredExpr
				deferredKeys[key] = true
				continue
			}
			result[key] = value
			deferredKeys[key] = true
			continue
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
	return result, deferredKeys, nil
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

// MergeScopeMaps deep-merges two scope maps (e.g. from multiple loaders or scope plus context values).
// When the same block name exists in both, block bodies are deep-merged recursively (maps by key, lists/scalars replaced at that path). Returns a new map; does not mutate inputs.
func MergeScopeMaps(globalScope map[string]any, overlay map[string]any) map[string]any {
	out := make(map[string]any)
	maps.Copy(out, globalScope)
	for name, body := range overlay {
		exMap, exOk := asMapStringAny(out[name])
		newMap, newOk := asMapStringAny(body)
		if exOk && newOk && body != nil {
			out[name] = deepMergeMap(exMap, newMap)
			continue
		}
		out[name] = body
	}
	return out
}

// resolvedFacetOrdinal returns the ordinal used to order the facet. When the facet has Ordinal set, that value is used; otherwise the default is derived from the facet file path.
func resolvedFacetOrdinal(f blueprintv1alpha1.Facet) int {
	if f.Ordinal != nil {
		return *f.Ordinal
	}
	return OrdinalFromFacetPath(f.Path)
}

// deepCopyMapStringAny returns a deep copy of m so that trace records hold a snapshot of the
// original facet inputs that cannot be mutated by later evaluation or normalization. Nested
// maps and slices are copied recursively. Values that are map[any]any from YAML decode are
// converted via asMapStringAny. Returns nil if m is nil.
func deepCopyMapStringAny(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopyValue(v)
	}
	return out
}

// deepCopyValue recursively copies a value for provenance storage. map[string]any and
// map[any]any (via asMapStringAny) are deep-copied; []any is deep-copied element-wise.
// Primitives and other types are returned as-is. Used by deepCopyMapStringAny and deepCopySliceAny.
func deepCopyValue(v any) any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return deepCopyMapStringAny(m)
	}
	if m, ok := asMapStringAny(v); ok {
		return deepCopyMapStringAny(m)
	}
	if s, ok := v.([]any); ok {
		return deepCopySliceAny(s)
	}
	return v
}

// deepCopySliceAny returns a deep copy of s for provenance storage. Each element is copied via
// deepCopyValue so nested maps and slices are not shared. Returns nil if s is nil.
func deepCopySliceAny(s []any) []any {
	if s == nil {
		return nil
	}
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = deepCopyValue(v)
	}
	return out
}

// asMapStringAny returns v as a map with string keys if v is a map (e.g. map[string]any or
// map[any]any from YAML). Uses MapRange and fmt.Sprint for keys so any
// keys from YAML decode as string keys. Returns (nil, false) otherwise.
func asMapStringAny(v any) (map[string]any, bool) {
	if v == nil {
		return nil, false
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map {
		return nil, false
	}
	out := make(map[string]any, rv.Len())
	iter := rv.MapRange()
	for iter.Next() {
		k := iter.Key()
		var keyStr string
		if k.Kind() == reflect.Interface && !k.IsNil() {
			keyStr = fmt.Sprint(k.Elem().Interface())
		} else {
			keyStr = fmt.Sprint(k.Interface())
		}
		out[keyStr] = iter.Value().Interface()
	}
	return out, true
}

func containsExpressionInValue(v any) bool {
	switch x := v.(type) {
	case evaluator.DeferredValue:
		return true
	case *evaluator.DeferredValue:
		return x != nil
	case string:
		return evaluator.ContainsExpression(x)
	case map[string]any:
		for _, val := range x {
			if containsExpressionInValue(val) {
				return true
			}
		}
		return false
	case []any:
		for _, val := range x {
			if containsExpressionInValue(val) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// containsDeferredInValue reports whether v contains one or more evaluator.DeferredValue nodes.
func containsDeferredInValue(v any) bool {
	switch x := v.(type) {
	case evaluator.DeferredValue:
		return true
	case *evaluator.DeferredValue:
		return x != nil
	case map[string]any:
		for _, val := range x {
			if containsDeferredInValue(val) {
				return true
			}
		}
		return false
	case []any:
		for _, val := range x {
			if containsDeferredInValue(val) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// normalizeDeferredValue recursively replaces DeferredValue nodes with their expression text.
func normalizeDeferredValue(v any) any {
	switch x := v.(type) {
	case evaluator.DeferredValue:
		return x.Expression
	case *evaluator.DeferredValue:
		if x == nil {
			return nil
		}
		return x.Expression
	case evaluator.SecretValue:
		return evaluator.SecretValue{Value: normalizeDeferredValue(evaluator.UnwrapSecretValue(x))}
	case *evaluator.SecretValue:
		if x == nil {
			return nil
		}
		normalized := normalizeDeferredValue(evaluator.UnwrapSecretValue(x))
		return &evaluator.SecretValue{Value: normalized}
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = normalizeDeferredValue(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = normalizeDeferredValue(val)
		}
		return out
	default:
		return v
	}
}

// deepMergeMap recursively merges overlay into base. At each key: if both values are maps they are
// merged recursively; otherwise the overlay value replaces (no list concatenation). Handles
// map[any]any from YAML by normalizing to map[string]any. Returns a new map; does not mutate inputs.
func deepMergeMap(base, overlay map[string]any) map[string]any {
	result := make(map[string]any, len(base)+len(overlay))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		existingNorm, exOk := asMapStringAny(result[k])
		overlayNorm, ovOk := asMapStringAny(v)
		if exOk && ovOk {
			result[k] = deepMergeMap(existingNorm, overlayNorm)
			continue
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
