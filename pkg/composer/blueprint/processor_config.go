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

// scopeForConfigBlock builds the evaluation scope for evaluating a single config block so that
// facet-derived config (globalScope) wins over context for all keys; the current block is set to
// currentBlockValue when non-nil (e.g. in-place evaluation or resolve pass), otherwise to
// contextScope[blockName] or omitted to avoid self-reference. This prevents context from
// overwriting other blocks' facet values when one block has a scalar value. currentBlockValue is
// overlaid on the context value rather than replacing it, so keys the block does not define itself —
// schema defaults and operator-set values under the same name — stay readable from inside the block
// on every stabilization pass, not just the first.
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

// mergeFacetScopeIntoGlobal merges the facet's config blocks into globalScope (accumulated from
// prior facets), without evaluating body expressions. Returns the updated global scope and
// per-block meta (ordinal, strategy). evaluateGlobalScopeConfig evaluates bodies later, in the
// dependency order topoSortConfigBlocks computes from inter-block references. A block contributes
// only when its `when:` is true; blocks sharing a name deep-merge in list order (later overlays
// earlier). Higher ordinal wins; equal ordinals fall back to strategy precedence (remove > replace
// > merge). A block's `when:` evaluates against evalScope made blind to the keys that same block
// writes, falling those keys back to contextScope (the round-0 operator/schema values), so a
// "default only when unset" guard tests operator intent, not the block's own prior-round output,
// and cannot cancel itself across rounds.
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

// evaluateGlobalScopeConfig evaluates every config block body in globalScope against contextScope
// (ConfigHandler values) merged with globalScope (facet config blocks), so expressions can
// reference both. Same-block sibling references converge by re-evaluating a block repeatedly, up
// to its own key count. A chain of N dependent keys can take up to N passes, since each pass
// propagates values only one dependency level further. Cross-block order comes from
// topoSortConfigBlocks (alphabetical tiebreak for independents). A key derived from its own block
// (e.g. a call like cidrhost(network.cidr_block, 1) inside the network block) skips this same-block
// pass and resolves once, later, against the settled block, keeping its raw expression when the
// result is still deferred. Mutates globalScope in place.
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

	// After the block scopes have converged, surface a genuine (non-deferred) evaluation error that the
	// tolerant per-key pass kept raw so transient same-block sibling references could resolve across
	// passes. A value still failing here against the final scope is a real mistake (a typo or bad
	// reference), not a sibling awaiting resolution — without this it would reach rendered config as an
	// unexpanded ${...} rather than failing composition. EvaluateMap tolerates a DeferredError at every
	// nesting level (a helper like terraform_output resolves later), so only genuine errors are returned;
	// top-level derived keys (${string(block.key)}) are kept raw for JIT and so are excluded.
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

// evaluateBlockBodyTolerant evaluates each top-level key of a config block body independently against
// scope, keeping a key that fails to evaluate at its raw value rather than aborting the whole body.
// EvaluateMap fails the entire map atomically on the first key error and returns nothing, which would
// discard siblings that did resolve; a key referencing a sibling not yet present in scope (e.g.
// `${identity_effective.issuer + '/auth'}` before issuer has resolved) would then abort the block on
// the first stabilization pass, before the multi-pass loop and resolve pass can feed the resolved
// sibling back in. Keeping the raw value lets a later pass resolve it once its dependency is available.
// This tolerance is deliberately scoped to the stabilization passes: a key that is still failing after
// the scopes converge is NOT silently dropped — evaluateGlobalScopeConfig's post-convergence validation
// re-evaluates it against the final scope and surfaces any genuine (non-deferred) error as a composition
// failure. A DeferredError is already handled inside EvaluateMap (the value is kept raw), so it is not an
// error here.
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

// scopeBlindToBlockKeys returns evalScope with the keys a config block writes under its own name
// replaced by their round-0 context value, instead of what composition has since accumulated, so
// the block's `when:` guard is blind to the block's own contribution from a prior round. A guard
// like `(network.cidr_block ?? '') == ''` therefore tests whether the operator or schema set
// cidr_block, not whether this block already defaulted it, and cannot flip false after the block
// writes it. Keys the block does not write — other blocks' keys, sibling facets' keys — stay
// untouched; a block whose value is not a map writes its whole name, so the whole name falls back
// to context. Returns evalScope unchanged when the block writes nothing, so the common path
// allocates nothing.
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

// operatorOverlayConfigBlocks builds synthetic config-block entries from operatorOverlay.
// operatorOverlay holds the operator's own explicit values (ConfigHandler.GetSetValues), never
// schema defaults. One entry is built per name globalScope already has. Each entry carries the
// highest possible ordinal. mergeConfigBlocks' own precedence rule then makes it win. That is the
// same rule that settles every facet-vs-facet conflict. Each entry uses "merge" strategy. An
// untouched key still surfaces from the facet's own value. A name absent from globalScope is
// skipped. No facet claims it this round, so it is context, not config. An operator value alone
// must never manufacture a new config: scope entry.
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
