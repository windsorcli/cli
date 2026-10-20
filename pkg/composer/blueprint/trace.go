// The DefaultTraceCollector records facet contributions and config blocks during blueprint composition.
// It provides the trace for the windsor explain command. GetTrace resolves a path to its value
// and to the facets that contributed to it. It also expands the config references in each contribution.

package blueprint

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Types
// =============================================================================

// TraceContribution records a single per-key contribution from a facet during composition.
// Recorded at the per-key level (e.g. "terraform.networking.inputs.domain_name") with all
// metadata resolved at record time so no retroactive YAML reparsing is needed. ScopeRefs
// are pre-extracted from expressions during recording.
type TraceContribution struct {
	FacetPath     string
	SourceName    string
	Ordinal       int
	Strategy      string
	Line          int
	RawValue      any
	RawComponents []string
	ScopeRefs     []string
}

// ConfigBlockRecord records a config block value for scope reference resolution. ScopeRefs
// and NestedPaths are pre-extracted during recording so query-time AST parsing is not needed.
// NestedRefs maps each nested expression path to its pre-extracted scope variable references.
type ConfigBlockRecord struct {
	FacetPath   string
	Line        int
	RawValue    any
	ScopeRefs   []string
	NestedPaths []string
	NestedRefs  map[string][]string
}

// DefaultTraceCollector is the standard TraceCollector implementation backed by in-memory maps.
// It records contributions and config blocks during composition. After composition,
// Finalize stores the composed blueprint, scope, and template root. GetTrace performs lazy
// resolution on demand.
type DefaultTraceCollector struct {
	mu            sync.Mutex
	contributions map[string][]TraceContribution
	configBlocks  map[string][]ConfigBlockRecord

	blueprint    *blueprintv1alpha1.Blueprint
	scope        map[string]any
	templateRoot string
}

// =============================================================================
// Interfaces
// =============================================================================

// TraceCollector records per-key contributions and config blocks during blueprint composition,
// then provides trace queries after finalization. Implementations are set on the processor
// before composition begins and are only active when the explain command is running.
type TraceCollector interface {
	RecordContribution(composedPath string, tc TraceContribution)
	RecordConfigBlock(configPath string, record ConfigBlockRecord)
	Finalize(bp *blueprintv1alpha1.Blueprint, scope map[string]any, templateRoot string)
	GetTrace(pathStr string) (*ExplainTrace, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewTraceCollector creates a new DefaultTraceCollector with initialized storage.
func NewTraceCollector() *DefaultTraceCollector {
	return &DefaultTraceCollector{
		contributions: make(map[string][]TraceContribution),
		configBlocks:  make(map[string][]ConfigBlockRecord),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// RecordContribution stores a per-key contribution from a facet and pre-extracts scope
// references from the raw value expression. Called from the processor during composition for
// each terraform input, kustomize substitution, and kustomize components list. Thread-safe.
func (c *DefaultTraceCollector) RecordContribution(composedPath string, tc TraceContribution) {
	if tc.ScopeRefs == nil {
		tc.ScopeRefs = extractRefsFromValue(tc.RawValue)
	}
	c.mu.Lock()
	c.contributions[composedPath] = append(c.contributions[composedPath], tc)
	c.mu.Unlock()
}

// RecordConfigBlock stores a config block value and pre-extracts scope references, nested
// expression paths, and per-path refs from the raw value. No child entries are created;
// getConfigBlockWithRecord's fallback logic resolves lines via YAML AST. Thread-safe.
func (c *DefaultTraceCollector) RecordConfigBlock(configPath string, record ConfigBlockRecord) {
	if record.ScopeRefs == nil && record.NestedPaths == nil {
		switch v := record.RawValue.(type) {
		case string:
			if strings.Contains(v, "${") {
				record.ScopeRefs = extractExprASTRefs(v)
			}
		default:
			if m, ok := asMapStringAny(record.RawValue); ok && containsExpressionInValue(record.RawValue) {
				record.NestedPaths = collectNestedExprPaths(m, "")
				record.NestedRefs = make(map[string][]string)
				for _, np := range record.NestedPaths {
					childVal := navigateMapPath(m, np)
					if s, ok := childVal.(string); ok && strings.Contains(s, "${") {
						refs := extractExprASTRefs(s)
						if len(refs) > 0 {
							record.NestedRefs[np] = refs
						}
					}
				}
			}
		}
	}
	c.mu.Lock()
	c.configBlocks[configPath] = append(c.configBlocks[configPath], record)
	c.mu.Unlock()
}

// Finalize stores the composed blueprint, scope, and template root for lazy trace resolution.
// Called once after all facet processing and scope merging completes.
func (c *DefaultTraceCollector) Finalize(bp *blueprintv1alpha1.Blueprint, scope map[string]any, templateRoot string) {
	c.blueprint = bp
	c.scope = scope
	c.templateRoot = templateRoot
}

// GetTrace resolves a dotted blueprint path and returns a trace with the composed value,
// contributions sorted by ordinal, effective marking, and scope reference resolution.
func (c *DefaultTraceCollector) GetTrace(pathStr string) (*ExplainTrace, error) {
	p, err := ParseExplainPath(pathStr)
	if err != nil {
		return nil, err
	}
	bp := c.blueprint
	if bp == nil {
		return nil, fmt.Errorf("blueprint not composed")
	}

	trace := &ExplainTrace{Path: p.String()}

	switch p.Kind {
	case ExplainPathKindTerraformInput:
		comp := findTerraformComponent(bp, p.Segment)
		if comp == nil {
			return nil, fmt.Errorf("terraform component %q not found in blueprint", p.Segment)
		}
		if comp.Inputs == nil {
			return nil, fmt.Errorf("terraform component %q has no inputs", p.Segment)
		}
		v, ok := getNestedValue(comp.Inputs, p.Key)
		if !ok {
			return nil, fmt.Errorf("terraform component %q has no input %q", p.Segment, p.Key)
		}
		trace.Value = formatValue(v)
		composedPath := fmt.Sprintf("terraform.%s.inputs.%s", p.Segment, p.Key)
		trace.Contributions = c.buildContributions(composedPath)

	case ExplainPathKindKustomizeSubstitution:
		k := findKustomization(bp, p.Segment)
		if k == nil {
			return nil, fmt.Errorf("kustomization %q not found in blueprint", p.Segment)
		}
		if k.Substitutions == nil {
			trace.Value = ""
			composedPath := fmt.Sprintf("kustomize.%s.substitutions.%s", p.Segment, p.Key)
			trace.Contributions = c.buildContributions(composedPath)
			return trace, nil
		}
		val, ok := k.Substitutions[p.Key]
		if !ok {
			trace.Value = ""
			composedPath := fmt.Sprintf("kustomize.%s.substitutions.%s", p.Segment, p.Key)
			trace.Contributions = c.buildContributions(composedPath)
			return trace, nil
		}
		trace.Value = val
		composedPath := fmt.Sprintf("kustomize.%s.substitutions.%s", p.Segment, p.Key)
		trace.Contributions = c.buildContributions(composedPath)

	case ExplainPathKindKustomizeComponents:
		k := findKustomization(bp, p.Segment)
		if k == nil {
			return nil, fmt.Errorf("kustomization %q not found in blueprint", p.Segment)
		}
		composedPath := fmt.Sprintf("kustomize.%s.components", p.Segment)
		provContribs := c.toExplainContributions(composedPath)
		trace.Contributions = buildComponentContributions(k.Components, provContribs, "kustomize", p.Segment)

	case ExplainPathKindConfigMap:
		if bp.ConfigMaps == nil {
			return nil, fmt.Errorf("configMap %q not found in blueprint", p.Segment)
		}
		cm, ok := bp.ConfigMaps[p.Segment]
		if !ok {
			return nil, fmt.Errorf("configMap %q not found in blueprint", p.Segment)
		}
		val, ok := cm[p.Key]
		if !ok {
			return nil, fmt.Errorf("configMap %q has no key %q", p.Segment, p.Key)
		}
		trace.Value = val
		trace.Contributions = []ExplainContribution{{SourceName: "composition (runtime config)", Effective: true}}

	case ExplainPathKindSubstitution:
		if bp.Substitutions != nil {
			trace.Value = bp.Substitutions[p.Key]
		}
		trace.Contributions = c.buildContributions("substitutions." + p.Key)

	default:
		return nil, errors.New("unknown path kind")
	}

	c.resolveScopeRefs(trace)

	return trace, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// buildContributions converts recorded TraceContributions into ExplainContributions, sorts
// by ordinal, and marks the effective contributor.
func (c *DefaultTraceCollector) buildContributions(composedPath string) []ExplainContribution {
	contribs := c.toExplainContributions(composedPath)
	return markEffective(contribs)
}

// toExplainContributions converts recorded TraceContributions for a path into ExplainContributions
// with display paths shortened relative to the template root.
func (c *DefaultTraceCollector) toExplainContributions(composedPath string) []ExplainContribution {
	records := c.contributions[composedPath]
	if len(records) == 0 {
		return nil
	}
	contribs := make([]ExplainContribution, 0, len(records))
	for _, tc := range records {
		contribs = append(contribs, c.traceToContribution(tc))
	}
	return contribs
}

// traceToContribution converts a TraceContribution (recorded during composition) into an
// ExplainContribution. The facet path is shortened relative to the template root for display.
// Line numbers and expression values are taken directly from the record (resolved at record time).
func (c *DefaultTraceCollector) traceToContribution(tc TraceContribution) ExplainContribution {
	displayPath := tc.FacetPath
	if displayPath != "" {
		shortened := false
		if c.templateRoot != "" {
			if rel, err := filepath.Rel(c.templateRoot, displayPath); err == nil && !strings.HasPrefix(rel, "..") {
				displayPath = rel
				shortened = true
			}
		}
		if !shortened {
			displayPath = filepath.Base(displayPath)
		}
	}

	ec := ExplainContribution{
		SourceName:      tc.SourceName,
		FacetPath:       displayPath,
		AbsFacetPath:    tc.FacetPath,
		Line:            tc.Line,
		Ordinal:         tc.Ordinal,
		Strategy:        tc.Strategy,
		RawComponents:   tc.RawComponents,
		preComputedRefs: tc.ScopeRefs,
	}

	if tc.RawValue != nil {
		ec.Expression = formatValue(tc.RawValue)
		ec.HasValue = true
	}

	return ec
}

// resolveScopeRefs populates the ScopeRefs field on each effective contribution using
// pre-computed scope reference names from the recording phase. No AST parsing occurs here;
// only scope resolution and recursive config block expansion.
func (c *DefaultTraceCollector) resolveScopeRefs(trace *ExplainTrace) {
	if c.scope == nil {
		return
	}
	visited := make(map[string]bool)
	for i := range trace.Contributions {
		contrib := &trace.Contributions[i]
		if !contrib.Effective || len(contrib.preComputedRefs) == 0 {
			continue
		}
		for _, ref := range contrib.preComputedRefs {
			sr, include := c.expandScopeRef(ref, visited)
			if include {
				contrib.ScopeRefs = append(contrib.ScopeRefs, sr)
			}
		}
	}
}

// expandScopeRef builds one scope ref and recursively expands Nested using pre-computed
// ScopeRefs and NestedPaths from recorded config blocks. No AST parsing or map traversal
// occurs here. Uses visited to detect cycles and isKnownScopeRoot to filter false positives.
func (c *DefaultTraceCollector) expandScopeRef(refPath string, visited map[string]bool) (ExplainScopeRef, bool) {
	if !c.isKnownScopeRoot(refPath) {
		return ExplainScopeRef{}, false
	}
	if visited[refPath] {
		return ExplainScopeRef{Name: refPath, Status: "cycle"}, true
	}
	visited[refPath] = true
	defer delete(visited, refPath)

	evalVal, inScope := resolveScopePath(c.scope, refPath)
	blockSource, blockLine, rawVal, record, remaining := c.getConfigBlockWithRecord(refPath)

	if !inScope && rawVal == nil {
		return ExplainScopeRef{Name: refPath, Status: "not set"}, true
	}

	sr := ExplainScopeRef{Name: refPath, Source: blockSource, Line: blockLine}
	evalDeferred := containsExpressionInValue(evalVal)

	if record != nil && len(remaining) > 0 && record.NestedRefs != nil {
		nestedKey := strings.Join(remaining, ".")
		if refs, ok := record.NestedRefs[nestedKey]; ok && len(refs) > 0 {
			if evalDeferred {
				sr.Status = "deferred"
			}
			for _, nestedRef := range refs {
				nested, include := c.expandScopeRef(nestedRef, visited)
				if include {
					sr.Nested = append(sr.Nested, nested)
				}
			}
			return sr, true
		}
	}

	if record != nil && len(record.ScopeRefs) > 0 {
		if evalDeferred {
			sr.Status = "deferred"
		}
		for _, nestedRef := range record.ScopeRefs {
			nested, include := c.expandScopeRef(nestedRef, visited)
			if include {
				sr.Nested = append(sr.Nested, nested)
			}
		}
		return sr, true
	}

	if record != nil && len(record.NestedPaths) > 0 {
		if evalDeferred {
			sr.Status = "deferred"
		}
		for _, path := range record.NestedPaths {
			fullPath := refPath + "." + path
			nested, include := c.expandScopeRef(fullPath, visited)
			if include {
				sr.Nested = append(sr.Nested, nested)
			}
		}
		return sr, true
	}

	traceVal := rawVal
	if traceVal == nil {
		traceVal = evalVal
	}
	if traceVal == nil {
		return sr, true
	}

	if evalDeferred {
		sr.Status = "deferred"
	}

	return sr, true
}

// getConfigBlockWithRecord returns the facet path, line, raw config value, matching
// ConfigBlockRecord, and remaining path segments for a scope path by matching the longest
// config.* prefix in configBlocks. When the matched record is a parent of refPath, the
// remaining segments navigate into the raw value and resolve nested line numbers via YAML AST.
// The caller can use remaining with record.NestedRefs to look up pre-computed scope refs.
func (c *DefaultTraceCollector) getConfigBlockWithRecord(refPath string) (string, int, any, *ConfigBlockRecord, []string) {
	key := "config." + refPath
	var remainingSegments []string
	for {
		if entries := c.configBlocks[key]; len(entries) > 0 {
			last := entries[len(entries)-1]
			rawVal := last.RawValue
			navigated := true
			for _, seg := range remainingSegments {
				if m, ok := asMapStringAny(rawVal); ok {
					if v, exists := m[seg]; exists {
						rawVal = v
					} else {
						navigated = false
						rawVal = nil
						break
					}
				} else {
					navigated = false
					rawVal = nil
					break
				}
			}
			if !navigated && len(remainingSegments) > 0 {
				return "", 0, nil, nil, nil
			}
			line := last.Line
			if len(remainingSegments) > 0 && last.FacetPath != "" {
				if nested := resolveNestedConfigLine(last.FacetPath, key, remainingSegments); nested > 0 {
					line = nested
				}
			}
			return last.FacetPath, line, rawVal, &last, remainingSegments
		}
		i := strings.LastIndex(key, ".")
		if i <= 6 {
			return "", 0, nil, nil, nil
		}
		remainingSegments = append([]string{key[i+1:]}, remainingSegments...)
		key = key[:i]
	}
}

// isKnownScopeRoot returns true when the first segment of refPath exists as a top-level scope
// key or has config block records. Filters out false-positive matches like URL fragments.
func (c *DefaultTraceCollector) isKnownScopeRoot(refPath string) bool {
	root := refPath
	if i := strings.Index(refPath, "."); i > 0 {
		root = refPath[:i]
	}
	if _, ok := c.scope[root]; ok {
		return true
	}
	prefix := "config." + root
	for key := range c.configBlocks {
		if key == prefix || strings.HasPrefix(key, prefix+".") {
			return true
		}
	}
	return false
}

// =============================================================================
// Helpers
// =============================================================================

// extractRefsFromValue extracts scope references from a raw value at record time. For strings
// containing "${", it parses the expression AST. For maps, it traverses nested values to find
// all expression references.
func extractRefsFromValue(v any) []string {
	switch val := v.(type) {
	case string:
		if strings.Contains(val, "${") {
			return extractExprASTRefs(val)
		}
	default:
		if m, ok := asMapStringAny(v); ok {
			var refs []string
			paths := collectNestedExprPaths(m, "")
			for _, p := range paths {
				childVal := navigateMapPath(m, p)
				if s, ok := childVal.(string); ok && strings.Contains(s, "${") {
					refs = append(refs, extractExprASTRefs(s)...)
				}
			}
			return refs
		}
	}
	return nil
}

// navigateMapPath walks a map[string]any by dot-separated path segments.
func navigateMapPath(m map[string]any, path string) any {
	parts := strings.Split(path, ".")
	var cur any = m
	for _, p := range parts {
		if cm, ok := asMapStringAny(cur); ok {
			cur = cm[p]
		} else {
			return nil
		}
	}
	return cur
}

// resolveNestedConfigLine uses YAML AST navigation to find the line number of a nested key
// within a config block value. matchedKey is the config block key that was found (e.g.
// "config.talos_common.common_patch") and remaining holds the path segments beyond it
// (e.g. ["cluster", "allowSchedulingOnControlPlanes"]).
func resolveNestedConfigLine(facetPath, matchedKey string, remaining []string) int {
	afterConfig := matchedKey[len("config."):]

	var segs []any
	segs = append(segs, "config")

	dotIdx := strings.Index(afterConfig, ".")
	if dotIdx < 0 {
		segs = append(segs, namedItem(afterConfig))
	} else {
		blockName := afterConfig[:dotIdx]
		topKey := afterConfig[dotIdx+1:]
		segs = append(segs, namedItem(blockName), "value", topKey)
	}

	for i, seg := range remaining {
		if i < len(remaining)-1 {
			segs = append(segs, seg)
		} else {
			segs = append(segs, mapKeyLine(seg))
		}
	}

	return yamlNodeLine(facetPath, segs...)
}

// collectNestedExprPaths returns dotted paths into the map for which the value contains "${".
// Only descends into nested maps; slice elements are skipped (scope paths are map-key only).
func collectNestedExprPaths(m map[string]any, prefix string) []string {
	var out []string
	for k, v := range m {
		p := prefix + k
		if v == nil {
			continue
		}
		if s, ok := v.(string); ok {
			if strings.Contains(s, "${") {
				out = append(out, p)
			}
			continue
		}
		if nested, ok := asMapStringAny(v); ok {
			if containsExpressionInValue(v) {
				out = append(out, collectNestedExprPaths(nested, p+".")...)
			}
		}
	}
	return out
}

// markEffective determines which contribution produced the final composed value and sets its
// Effective flag. Contributions are sorted by ascending ordinal to ensure deterministic results
// regardless of goroutine scheduling during parallel facet processing. A replace strategy resets
// the effective index; for merge, the highest-ordinal contributor that defines the key wins.
// Returns a placeholder when contributions are empty.
func markEffective(contributions []ExplainContribution) []ExplainContribution {
	if len(contributions) == 0 {
		return []ExplainContribution{{SourceName: "composed blueprint"}}
	}
	sort.SliceStable(contributions, func(i, j int) bool {
		return contributions[i].Ordinal < contributions[j].Ordinal
	})
	effectiveIdx := -1
	for i := range contributions {
		c := &contributions[i]
		if c.Strategy == "replace" {
			effectiveIdx = i
		} else if c.HasValue {
			effectiveIdx = i
		} else if effectiveIdx == -1 {
			effectiveIdx = i
		}
	}
	if effectiveIdx >= 0 {
		contributions[effectiveIdx].Effective = true
	}
	return contributions
}
