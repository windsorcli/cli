// The processor_config file holds the config block steps of the BlueprintProcessor.
// It provides the merge of facet config blocks into the global scope, by ordinal and strategy.
// It evaluates the config blocks against the context scope until the values converge.
// A block value that does not converge is an error that names the block.

package blueprint

import (
	"fmt"
	"maps"
	"math"
	"reflect"
	"sort"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Private Methods
// =============================================================================

// scopeForConfigBlock builds the scope for the evaluation of one config block. Facet config in
// globalScope wins over context for every key. The block itself is currentBlockValue overlaid on its
// context value when currentBlockValue is not nil, so schema defaults and operator values under the
// same name stay readable on every pass. Otherwise the block is its context value, or absent, so it
// does not reference itself.
func (p *BaseBlueprintProcessor) scopeForConfigBlock(contextScope, globalScope map[string]any, blockName string, currentBlockValue any) map[string]any {
	if globalScope == nil {
		globalScope = make(map[string]any)
	}
	if contextScope == nil {
		contextScope = make(map[string]any)
	}
	scope := blueprintv1alpha1.DeepMergeMaps(contextScope, globalScope)
	if currentBlockValue != nil {
		scope[blockName] = mergeBlockValueOverContext(contextScope[blockName], currentBlockValue)
	} else if contextScope != nil && contextScope[blockName] != nil {
		scope[blockName] = contextScope[blockName]
	} else {
		delete(scope, blockName)
	}
	return scope
}

// mergeFacetScopeIntoGlobal merges the config blocks of a facet into globalScope without evaluating
// their bodies, and returns the scope and the ordinal and strategy of each block. A block adds its
// value only when its when is true. Blocks with the same name deep-merge in list order. A higher
// ordinal wins, then remove > replace > merge. A block's when ignores the keys that the block writes
// (scopeBlindToBlockKeys), so a guard such as "default only when unset" tests the operator value.
func (p *BaseBlueprintProcessor) mergeFacetScopeIntoGlobal(facet blueprintv1alpha1.Facet, globalScope map[string]any, existing map[string]*blueprintv1alpha1.ConfigBlock, evalScope, contextScope map[string]any) (map[string]any, map[string]*blueprintv1alpha1.ConfigBlock, error) {
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
			guardScope := scopeBlindToBlockKeys(evalScope, contextScope, block.Name, block.Body)
			shouldInclude, err := p.shouldIncludeComponent(block.When, facet.Path, guardScope)
			if err != nil {
				return nil, nil, werror.Wrap(err, "config block %q when", block.Name)
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
	}
	if len(incoming) == 0 {
		return globalScope, existing, nil
	}
	mergedScope, mergedEntries, err := p.mergeConfigBlocks(globalScope, existing, incoming)
	if err != nil {
		return nil, nil, err
	}
	return mergedScope, mergedEntries, nil
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
		incOrdinal := ordinalOrZero(inc.Ordinal)
		if prev, hasPrev := merged[name]; hasPrev {
			prevOrdinal := ordinalOrZero(prev.Ordinal)
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

// unstableSetNames returns, sorted, the names present in exactly one of curr and prev. Used to
// name the facets a non-convergence error should point an operator at, when facet inclusion
// itself (not config values) is still changing between rounds.
func unstableSetNames(curr, prev map[string]bool) []string {
	var out []string
	for name := range curr {
		if !prev[name] {
			out = append(out, name)
		}
	}
	for name := range prev {
		if !curr[name] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// unstableConfigBlockNames returns, sorted, the names of config blocks whose presence or value
// differs between two rounds' resolved scopes. Used to name the blocks a non-convergence error
// should point an operator at.
func unstableConfigBlockNames(a, b map[string]any) []string {
	names := make(map[string]bool, len(a)+len(b))
	for name := range a {
		names[name] = true
	}
	for name := range b {
		names[name] = true
	}
	var out []string
	for name := range names {
		if !reflect.DeepEqual(a[name], b[name]) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// evaluateGlobalScopeConfig evaluates every config block body in globalScope, in place, against
// contextScope merged with globalScope. Blocks evaluate in topoSortConfigBlocks order, each up to once
// per key, so references between its own keys converge. A key derived from its own block, such as
// cidrhost(network.cidr_block, 1), resolves once after the block settles. After convergence, a key that
// still fails with an error that is not deferred fails composition, so no raw ${...} reaches config.
func (p *BaseBlueprintProcessor) evaluateGlobalScopeConfig(globalScope map[string]any, contextScope map[string]any) error {
	if globalScope == nil {
		return nil
	}
	if contextScope == nil && p.runtime != nil && p.runtime.ConfigHandler != nil {
		if vals, err := p.runtime.ConfigHandler.GetContextValues(); err == nil {
			contextScope = vals
		}
	}
	names, err := topoSortConfigBlocks(globalScope)
	if err != nil {
		return err
	}
	const minSameBlockPasses = 5
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
					return werror.Wrap(err, "config block %q", name)
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
			maxSameBlockPasses := len(bodyMap)
			if maxSameBlockPasses < minSameBlockPasses {
				maxSameBlockPasses = minSameBlockPasses
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
				evaluated := p.evaluateBlockBodyTolerant(nonDerivedBody, scopeWithBlock)
				if normalized, ok := normalizeDeferredValue(evaluated).(map[string]any); ok {
					evaluated = normalized
				}
				if previousEvaluatedOnly != nil && reflect.DeepEqual(evaluated, previousEvaluatedOnly) {
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
					resolved, err := p.evaluator.Evaluate(s, "", scopeWithBlock, false)
					if err != nil || resolved == nil || evaluator.IsDeferredValue(resolved) {
						continue
					}
					resolved = normalizeDeferredValue(resolved)
					if reflect.DeepEqual(resolved, v) {
						continue
					}
					current[k] = resolved
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

	for name, val := range globalScope {
		blockMap, ok := val.(map[string]any)
		if !ok {
			continue
		}
		validationBody := make(map[string]any, len(blockMap))
		for k, v := range blockMap {
			if s, isStr := v.(string); isStr && expressionIsDerivedFromBlock(s, name) {
				continue
			}
			validationBody[k] = v
		}
		finalScope := p.scopeForConfigBlock(contextScope, globalScope, name, blockMap)
		if _, err := p.evaluator.EvaluateMap(validationBody, "", finalScope, false); err != nil {
			return werror.Wrap(err, "config block %q", name)
		}
	}
	return nil
}

// evaluateBlockBodyTolerant evaluates each top-level key of a config block body on its own, and keeps
// the raw value of a key that fails. EvaluateMap would discard the whole body on the first failure,
// such as a key that reads a sibling that has not resolved yet. A later pass resolves the kept key.
// After the scopes converge, evaluateGlobalScopeConfig evaluates each key again and returns any error
// that is not a deferred error.
func (p *BaseBlueprintProcessor) evaluateBlockBodyTolerant(body map[string]any, scope map[string]any) map[string]any {
	evaluated := make(map[string]any, len(body))
	for k, v := range body {
		one, err := p.evaluator.EvaluateMap(map[string]any{k: v}, "", scope, false)
		if err != nil {
			evaluated[k] = v
			continue
		}
		evaluated[k] = one[k]
	}
	return evaluated
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
				return nil, werror.Wrap(err, "config block list[%d]", i)
			}
			out = append(out, evaluated)
		}
		return out, nil
	default:
		return v, nil
	}
}

// =============================================================================
// Helpers
// =============================================================================

// scopeBlindToBlockKeys returns evalScope with each key that the block writes set back to its
// context value. The block's when then tests the operator and schema values, not the block's own
// output from a prior round, and cannot turn false after the block writes a default. A block whose
// value is not a map writes its whole name. When the block writes nothing, evalScope returns as is.
func scopeBlindToBlockKeys(evalScope, contextScope map[string]any, blockName string, body map[string]any) map[string]any {
	keys, wholeName := blockWrittenKeys(body)
	if !wholeName && len(keys) == 0 {
		return evalScope
	}
	guard := maps.Clone(evalScope)
	if guard == nil {
		guard = make(map[string]any)
	}
	ctxVal, ctxHas := contextScope[blockName]
	if wholeName {
		if ctxHas {
			guard[blockName] = ctxVal
		} else {
			delete(guard, blockName)
		}
		return guard
	}
	blockMap, _ := asMapStringAny(guard[blockName])
	newBlock := maps.Clone(blockMap)
	if newBlock == nil {
		newBlock = make(map[string]any)
	}
	ctxBlock, _ := asMapStringAny(ctxVal)
	for _, k := range keys {
		if cv, ok := ctxBlock[k]; ok {
			newBlock[k] = cv
		} else {
			delete(newBlock, k)
		}
	}
	guard[blockName] = newBlock
	return guard
}

// blockWrittenKeys reports the top-level keys a config block writes under its name. A map value writes
// its own keys; a scalar or list value writes the whole name (wholeName true, no keys); a block with no
// value writes nothing.
func blockWrittenKeys(body map[string]any) (keys []string, wholeName bool) {
	if body == nil {
		return nil, false
	}
	value, ok := body["value"]
	if !ok || value == nil {
		return nil, false
	}
	valueMap, ok := asMapStringAny(value)
	if !ok {
		return nil, true
	}
	for k := range valueMap {
		keys = append(keys, k)
	}
	return keys, false
}

// operatorOverlayConfigBlocks builds a config-block entry from operatorOverlay, the explicit operator
// values without schema defaults, for each name that globalScope has. Each entry has the highest
// ordinal and the merge strategy, so the operator value wins and keys it does not set keep the facet
// value. A name that globalScope does not have is skipped, so an operator value never creates config.
func operatorOverlayConfigBlocks(globalScope map[string]any, operatorOverlay map[string]any) map[string]*blueprintv1alpha1.ConfigBlock {
	if len(globalScope) == 0 || len(operatorOverlay) == 0 {
		return nil
	}
	maxOrdinal := math.MaxInt
	var incoming map[string]*blueprintv1alpha1.ConfigBlock
	for name := range globalScope {
		value, ok := operatorOverlay[name]
		if !ok {
			continue
		}
		if incoming == nil {
			incoming = make(map[string]*blueprintv1alpha1.ConfigBlock)
		}
		incoming[name] = &blueprintv1alpha1.ConfigBlock{
			Name:     name,
			Strategy: "merge",
			Ordinal:  &maxOrdinal,
			Body:     map[string]any{"value": value},
		}
	}
	return incoming
}

// mergeBlockValueOverContext overlays a config block's in-progress value on the context value under
// the same name, so the block reads its own keys and inherits the rest. A non-map on either side has
// nothing to merge and yields the block value unchanged.
func mergeBlockValueOverContext(contextValue, blockValue any) any {
	contextMap, contextIsMap := contextValue.(map[string]any)
	blockMap, blockIsMap := blockValue.(map[string]any)
	if !contextIsMap || !blockIsMap {
		return blockValue
	}
	return blueprintv1alpha1.DeepMergeMaps(contextMap, blockMap)
}
