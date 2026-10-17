package blueprint

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// The BaseBlueprintProcessor is a core component that processes blueprint facets and evaluates conditional logic.
// It provides functionality for evaluating 'when' conditions on facets, terraform components, and kustomizations.
// The BaseBlueprintProcessor orchestrates the collection, merging, and application of components based on ordinal
// and strategy, ensuring deterministic and predictable blueprint composition from multiple facet sources.

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
	excludedFacets []ExcludedFacet
}

// ExcludedFacet records a facet dropped from composition by a false when: condition, along with the
// component and kustomization names it would have contributed. Dependency validation joins a dangling
// dependency name against these so it can name the excluded facet and the condition that excluded it,
// rather than only reporting the unresolvable edge.
type ExcludedFacet struct {
	Name     string   // facet metadata name
	When     string   // the when: expression that evaluated false
	Provides []string // terraform component IDs and kustomization names (including flux tiers) it declares
}

// collectedComponents holds the intermediate accumulation maps built during ProcessFacets before
// they are written to the target blueprint by applyCollectedComponents.
type collectedComponents struct {
	terraformByID       map[string]*blueprintv1alpha1.ConditionalTerraformComponent
	kustomizationByName map[string]*blueprintv1alpha1.ConditionalKustomization
	fluxSystemByName    map[string]*blueprintv1alpha1.FluxSystem
}

// SourceFacetSet is one source's contribution to global composition: its name, its dependency depth
// (deeper/referencing sources override shallower ones), the blueprint to emit its components into, and
// its facets.
type SourceFacetSet struct {
	Name   string
	Depth  int
	Target *blueprintv1alpha1.Blueprint
	Facets []blueprintv1alpha1.Facet
}

// =============================================================================
// Interfaces
// =============================================================================

// BlueprintProcessor evaluates when: conditions on facets, terraform components, and kustomizations.
// It determines inclusion/exclusion based on boolean condition results.
// The processor is stateless and shared across all loaders.
// ProcessFacets returns the evaluated config scope for the loader so callers can merge
// scopes from multiple loaders (e.g. for user overlay and final terraform input evaluation).
type BlueprintProcessor interface {
	ProcessFacets(target *blueprintv1alpha1.Blueprint, facets []blueprintv1alpha1.Facet, sourceName ...string) (scope map[string]any, err error)
	ProcessGlobally(sources []SourceFacetSet) (map[string]any, error)
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

// ResetExcludedFacets clears the accumulated excluded-facet record. Callers invoke it once before a
// composition's per-source ProcessFacets pass, so the set reflects only that composition. ProcessFacets
// appends across sources rather than resetting, letting a multi-source compose accumulate one set.
func (p *BaseBlueprintProcessor) ResetExcludedFacets() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.excludedFacets = nil
}

// GetExcludedFacets returns a copy of the facets excluded by a false when: across every ProcessFacets
// call since the last reset. Read after the per-source passes complete so the set is stable.
func (p *BaseBlueprintProcessor) GetExcludedFacets() []ExcludedFacet {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.excludedFacets) == 0 {
		return nil
	}
	out := make([]ExcludedFacet, len(p.excludedFacets))
	copy(out, p.excludedFacets)
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
// An active facet contributes its config blocks (not its components) even while its requires are unmet, so
// facets with mutually dependent config and requires resolve across rounds; a facet still unsatisfied when the
// loop settles fails composition, so a permanently-blocked facet never leaks config into a successful blueprint.
// An operator's own explicit value wins over a facet's computed value for the same key. This
// matches the guarantee ProcessGlobally provides.
func (p *BaseBlueprintProcessor) ProcessFacets(target *blueprintv1alpha1.Blueprint, facets []blueprintv1alpha1.Facet, sourceName ...string) (map[string]any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deferredPaths = make(map[string]bool)
	if target == nil {
		return nil, fmt.Errorf("target blueprint cannot be nil")
	}

	if len(facets) == 0 {
		return nil, nil
	}

	contextScope := p.buildContextScope()

	sortedFacets := make([]blueprintv1alpha1.Facet, len(facets))
	copy(sortedFacets, facets)
	sort.Slice(sortedFacets, func(i, j int) bool {
		oi, oj := resolvedFacetOrdinal(sortedFacets[i]), resolvedFacetOrdinal(sortedFacets[j])
		if oi != oj {
			return oi < oj
		}
		return sortedFacets[i].Metadata.Name < sortedFacets[j].Metadata.Name
	})

	var operatorOverlay map[string]any
	if p.runtime != nil && p.runtime.ConfigHandler != nil {
		operatorOverlay = p.runtime.ConfigHandler.GetSetValues()
	}
	globalScope, scope, includedFacets, err := p.resolveConfigAndInclusion(sortedFacets, contextScope, operatorOverlay)
	if err != nil {
		return nil, err
	}

	if err := p.emitFacetComponents(target, includedFacets, scope, sourceName...); err != nil {
		return nil, err
	}
	return globalScope, nil
}

// ProcessGlobally resolves config and facet inclusion once across every source's facets, then emits
// each source's included components into its own target blueprint. Facets are ranked by a
// depth-adjusted ordinal so a deeper source's config wins over a shallower one's for the same key,
// while derivations resolve against the final merged scope and an upstream facet's when: sees a
// downstream source's config. Returns the resolved global scope. An operator's explicit raw value
// always wins over a facet's computed value for the same key — see resolveConfigAndInclusion.
func (p *BaseBlueprintProcessor) ProcessGlobally(sources []SourceFacetSet) (map[string]any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deferredPaths = make(map[string]bool)

	contextScope := p.buildContextScope()

	const sourceOrdinalStride = 1_000_000
	var combined []blueprintv1alpha1.Facet
	for _, src := range sources {
		for _, f := range src.Facets {
			fc := f
			fc.Source = src.Name
			eff := resolvedFacetOrdinal(f) + src.Depth*sourceOrdinalStride
			fc.Ordinal = &eff
			combined = append(combined, fc)
		}
	}
	if len(combined) == 0 {
		return nil, nil
	}
	sort.Slice(combined, func(i, j int) bool {
		oi, oj := resolvedFacetOrdinal(combined[i]), resolvedFacetOrdinal(combined[j])
		if oi != oj {
			return oi < oj
		}
		return combined[i].Metadata.Name < combined[j].Metadata.Name
	})

	var operatorOverlay map[string]any
	if p.runtime != nil && p.runtime.ConfigHandler != nil {
		operatorOverlay = p.runtime.ConfigHandler.GetSetValues()
	}

	globalScope, scope, included, err := p.resolveConfigAndInclusion(combined, contextScope, operatorOverlay)
	if err != nil {
		return nil, err
	}

	includedBySource := make(map[string][]blueprintv1alpha1.Facet)
	for _, f := range included {
		includedBySource[f.Source] = append(includedBySource[f.Source], f)
	}
	for _, src := range sources {
		facets := includedBySource[src.Name]
		if len(facets) == 0 {
			continue
		}
		if err := p.emitFacetComponents(src.Target, facets, scope, src.Name); err != nil {
			return nil, err
		}
	}

	return globalScope, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// buildContextScope returns the operator/schema context values merged with any injected extra scope.
func (p *BaseBlueprintProcessor) buildContextScope() map[string]any {
	var contextScope map[string]any
	if p.runtime != nil && p.runtime.ConfigHandler != nil {
		if vals, err := p.runtime.ConfigHandler.GetContextValues(); err == nil {
			contextScope = vals
		}
	}
	if p.extraScope != nil {
		contextScope = MergeScopeMaps(contextScope, p.extraScope)
	}
	return contextScope
}

// resolveConfigAndInclusion runs the config-merge and facet-inclusion fixpoint over sortedFacets
// against contextScope. It returns the resolved config scope (globalScope), the full evaluation scope
// (contextScope deep-merged with globalScope), and the included facets. Excluded facets are recorded,
// and a requirements error is returned if any included facet's requires stay unmet. It emits no
// components — split from ProcessFacets so cross-source composition can resolve config and inclusion
// once, globally, before emitting each source's components. A round stops the loop only when the
// included-facet set and the resolved config scope both match the prior round, since a config block's
// `when:` can depend on a block only just included this round. It errors if the scope never settles
// within maxFacetRounds, instead of silently returning whichever round ran last.
//
// A config: block always computes its own value. It cannot defer to an operator's raw
// override on its own. Each round, after facets merge their blocks, operatorOverlay joins as one
// more contributor. It carries the highest ordinal. mergeConfigBlocks' existing precedence rule
// then makes it win. This runs before block bodies evaluate. So a block referencing another
// block's key sees the operator's corrected value too.
func (p *BaseBlueprintProcessor) resolveConfigAndInclusion(sortedFacets []blueprintv1alpha1.Facet, contextScope map[string]any, operatorOverlay map[string]any) (map[string]any, map[string]any, []blueprintv1alpha1.Facet, error) {
	scope := contextScope
	var globalScope map[string]any
	var prevGlobalScope map[string]any
	var cfgEntries map[string]*blueprintv1alpha1.ConfigBlock
	includedFacets := make([]blueprintv1alpha1.Facet, 0, len(sortedFacets))
	prevIncludedSet := make(map[string]bool)
	pendingRequirements := make(map[string]facetRequirementMisses)
	const maxFacetRounds = 10
	converged := false
	var unstableNames []string
	for range make([]struct{}, maxFacetRounds) {
		prevGlobalScope = globalScope
		includedFacets = includedFacets[:0]
		globalScope = nil
		cfgEntries = nil
		pendingRequirements = make(map[string]facetRequirementMisses)
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
				return nil, nil, nil, err
			}
			if !shouldInclude {
				continue
			}
			misses, err := p.evaluateRequirements(facet, passScope, contextScope)
			if err != nil {
				return nil, nil, nil, err
			}
			var errMerge error
			globalScope, cfgEntries, errMerge = p.mergeFacetScopeIntoGlobal(facet, globalScope, cfgEntries, passScope, contextScope)
			if errMerge != nil {
				return nil, nil, nil, werror.Wrap(errMerge, "facet %s", facet.Metadata.Name)
			}
			if len(misses) > 0 {
				pendingRequirements[facet.Metadata.Name] = facetRequirementMisses{Misses: misses}
				continue
			}
			includedFacets = append(includedFacets, facet)
		}
		if incoming := operatorOverlayConfigBlocks(globalScope, operatorOverlay); len(incoming) > 0 {
			var err error
			globalScope, cfgEntries, err = p.mergeConfigBlocks(globalScope, cfgEntries, incoming)
			if err != nil {
				return nil, nil, nil, err
			}
		}
		if err := p.evaluateGlobalScopeConfig(globalScope, contextScope); err != nil {
			return nil, nil, nil, err
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
		facetsStable := len(currSet) == len(prevIncludedSet)
		if facetsStable {
			for n := range currSet {
				if !prevIncludedSet[n] {
					facetsStable = false
					break
				}
			}
		}
		if facetsStable && reflect.DeepEqual(globalScope, prevGlobalScope) {
			converged = true
			break
		}
		if !facetsStable {
			unstableNames = unstableSetNames(currSet, prevIncludedSet)
		} else {
			unstableNames = nil
		}
		prevIncludedSet = currSet
	}

	if !converged {
		named := unstableNames
		if len(named) == 0 {
			named = unstableConfigBlockNames(globalScope, prevGlobalScope)
		}
		return nil, nil, nil, fmt.Errorf(
			"config did not converge after %d rounds. Check for a 'when:' or 'value:' cycle involving: %s",
			maxFacetRounds, strings.Join(named, ", "),
		)
	}

	if len(pendingRequirements) > 0 {
		return nil, nil, nil, formatRequirementsError(pendingRequirements)
	}

	includedSet := make(map[string]bool, len(includedFacets))
	for _, f := range includedFacets {
		includedSet[f.Metadata.Name] = true
	}
	for _, facet := range sortedFacets {
		if includedSet[facet.Metadata.Name] {
			continue
		}
		p.excludedFacets = append(p.excludedFacets, ExcludedFacet{
			Name:     facet.Metadata.Name,
			When:     facet.When,
			Provides: facetProvides(facet),
		})
	}

	return globalScope, scope, includedFacets, nil
}

// emitFacetComponents materializes the included facets' components — terraform components,
// kustomizations, flux systems, CRDs, blueprint-level substitutions, and messages — into target,
// resolving expressions against the already-resolved scope. sourceName stamps Source on components
// that lack one. This is the emission half of ProcessFacets, split out so cross-source composition can
// resolve config and inclusion globally first and then emit each source's components separately.
func (p *BaseBlueprintProcessor) emitFacetComponents(target *blueprintv1alpha1.Blueprint, includedFacets []blueprintv1alpha1.Facet, scope map[string]any, sourceName ...string) error {
	collected := collectedComponents{
		terraformByID:       make(map[string]*blueprintv1alpha1.ConditionalTerraformComponent),
		kustomizationByName: make(map[string]*blueprintv1alpha1.ConditionalKustomization),
		fluxSystemByName:    make(map[string]*blueprintv1alpha1.FluxSystem),
	}
	crdRefs := make(map[string]struct{})

	for _, facet := range includedFacets {
		if len(facet.Crds) > 0 {
			evaluated, err := p.evaluateStringSlice(facet.Crds, facet.Path, scope)
			if err != nil {
				return werror.Wrap(err, "error evaluating crds for facet '%s'", facet.Metadata.Name)
			}
			facet.Crds = slices.DeleteFunc(evaluated, func(s string) bool { return s == "" })
			for _, ref := range facet.Crds {
				if err := validateCrdRef(ref); err != nil {
					return werror.Wrap(err, "facet %q", facet.Metadata.Name)
				}
			}
		}
		if err := p.collectTerraformComponents(facet, sourceName, collected.terraformByID, scope); err != nil {
			return err
		}
		if err := p.collectKustomizations(facet, sourceName, collected.kustomizationByName, scope); err != nil {
			return err
		}
		if err := p.collectFluxSystems(facet, sourceName, collected.fluxSystemByName, scope); err != nil {
			return err
		}
		for _, ref := range facet.Crds {
			crdRefs[ref] = struct{}{}
		}
		if facet.Backend != "" {
			target.Backend = facet.Backend
		}

		if len(facet.Substitutions) > 0 {
			evaluated, deferredKeys, err := p.evaluateSubstitutions(facet.Substitutions, facet.Path, scope)
			if err != nil {
				return werror.Wrap(err, "error evaluating substitutions for facet '%s'", facet.Metadata.Name)
			}
			if target.Substitutions == nil {
				target.Substitutions = make(map[string]string)
			}
			maps.Copy(target.Substitutions, evaluated)
			facetOrd := resolvedFacetOrdinal(facet)
			sn := ""
			if len(sourceName) > 0 {
				sn = sourceName[0]
			}
			for key, rawVal := range facet.Substitutions {
				p.recordTrace("substitutions."+key, TraceContribution{
					FacetPath:  facet.Path,
					SourceName: sn,
					Ordinal:    facetOrd,
					Strategy:   "merge",
					Line:       yamlNodeLine(facet.Path, "substitutions", key),
					RawValue:   deepCopyValue(rawVal),
				})
			}
			for key, isDeferred := range deferredKeys {
				if isDeferred {
					p.markDeferredPath("substitutions." + key)
				}
			}
		}

		// Carry raw message templates through in included-facet order (ordinal then name). They are not
		// evaluated here: GenerateResolved renders when/text against the resolved scope, where run values
		// like terraform_output are available, and dedups the rendered results.
		for _, msg := range facet.Messages {
			if msg.Text != "" {
				target.Messages = append(target.Messages, msg)
			}
		}
	}

	setCrdLayer(target, crdRefs, resolveSourceName(sourceName))

	return p.applyCollectedComponents(target, collected, scope)
}

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

// =============================================================================
// Helpers
// =============================================================================

// facetProvides returns the names a facet would contribute to composition: each terraform component's
// ID, each kustomization's name, and each flux system's compiled tier names ("<name>-install" /
// "<name>-resources"). Dependency validation matches a dangling dependency against these to attribute it
// to the excluded facet that would have provided it.
func facetProvides(facet blueprintv1alpha1.Facet) []string {
	var provides []string
	for i := range facet.TerraformComponents {
		provides = append(provides, facet.TerraformComponents[i].GetID())
	}
	for _, k := range facet.Kustomizations {
		provides = append(provides, k.Name)
	}
	for _, sys := range facet.FluxSystems {
		provides = append(provides, sys.TierNames()...)
	}
	return provides
}

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

// ordinalOrZero dereferences ordinal, or returns 0 when it is nil. Shared by every ordinal
// precedence comparison across terraform/kustomization/flux-system entry merging and config-block
// merging.
func ordinalOrZero(ordinal *int) int {
	if ordinal == nil {
		return 0
	}
	return *ordinal
}

// resolveOrdinal returns ordinal's value when set, or facet's own default ordinal otherwise.
// Shared by each collect* function's per-entry ordinal resolution.
func resolveOrdinal(facet blueprintv1alpha1.Facet, ordinal *int) int {
	if ordinal != nil {
		return *ordinal
	}
	return resolvedFacetOrdinal(facet)
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

// =============================================================================
// Interface Compliance
// =============================================================================

var _ BlueprintProcessor = (*BaseBlueprintProcessor)(nil)
