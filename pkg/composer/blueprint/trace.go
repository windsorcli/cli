// The ExplainResolver provides blueprint value provenance resolution for the windsor explain command.
// It resolves a dotted path against the composed blueprint and produces a trace showing
// the value and which facets contributed to it, enabling users to understand where values
// originate and how composition affected them.
package blueprint

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Constants
// =============================================================================

const (
	ExplainPathKindTerraformInput ExplainPathKind = iota
	ExplainPathKindKustomizeSubstitution
	ExplainPathKindKustomizeComponents
	ExplainPathKindConfigMap
)

// =============================================================================
// Types
// =============================================================================

// ExplainPathKind identifies the type of blueprint path being explained.
type ExplainPathKind int

// ExplainPath is a parsed explain path identifying a single value in the composed blueprint.
type ExplainPath struct {
	Kind    ExplainPathKind
	Segment string
	Key     string
}

// ExplainScopeRef describes a scope variable referenced in an expression, its resolution status,
// and the source location of the config block that defines it (if applicable). Nested holds
// refs for expressions or map keys that contain expressions, recursively until origins.
type ExplainScopeRef struct {
	Name   string
	Status string
	Source string
	Line   int
	Nested []ExplainScopeRef
}

// ExplainContribution describes one source that contributed to the value (facet file, source name, etc.).
// AbsFacetPath is the absolute filesystem path for clickable terminal references; FacetPath is
// the shortened display form. Line is the 1-based line number of the specific key (or the
// component definition if the key is not locatable). Effective is true when this contribution
// produced the final composed value; false means it was overridden by a higher-ordinal facet.
type ExplainContribution struct {
	SourceName      string
	FacetPath       string
	AbsFacetPath    string
	Line            int
	Ordinal         int
	Strategy        string
	Expression      string
	HasValue        bool
	Effective       bool
	ScopeRefs       []ExplainScopeRef
	RawComponents   []string
	preComputedRefs []string
}

// ExplainTrace holds the result of explaining a path: the value and its contributions.
type ExplainTrace struct {
	Path          string
	Value         string
	Contributions []ExplainContribution
}

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
// Contributions and config blocks are recorded during composition (Phase 1). After composition,
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

	default:
		return nil, errors.New("unknown path kind")
	}

	c.resolveScopeRefs(trace)

	return trace, nil
}

// String returns the canonical path string (e.g. terraform.cluster.inputs.domain_name).
func (p ExplainPath) String() string {
	switch p.Kind {
	case ExplainPathKindTerraformInput:
		return fmt.Sprintf("terraform.%s.inputs.%s", p.Segment, p.Key)
	case ExplainPathKindKustomizeSubstitution:
		return fmt.Sprintf("kustomize.%s.substitutions.%s", p.Segment, p.Key)
	case ExplainPathKindKustomizeComponents:
		return fmt.Sprintf("kustomize.%s.components", p.Segment)
	case ExplainPathKindConfigMap:
		return fmt.Sprintf("configMaps.%s.%s", p.Segment, p.Key)
	default:
		return ""
	}
}

// ParseExplainPath parses a path string into an ExplainPath. Supported forms:
//   - terraform.<componentID>.inputs.<key>
//   - kustomize.<name>.substitutions.<key>
//   - configMaps.<name>.<key>
//
// Returns an error if the path is malformed or empty.
func ParseExplainPath(path string) (ExplainPath, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return ExplainPath{}, errors.New("path is required")
	}
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return ExplainPath{}, fmt.Errorf("invalid path %q: expected form terraform.<id>.inputs.<key>, kustomize.<name>.substitutions.<key>, or configMaps.<name>.<key>", path)
	}
	switch parts[0] {
	case "terraform":
		if len(parts) < 4 || parts[2] != "inputs" {
			return ExplainPath{}, fmt.Errorf("invalid terraform path %q: expected terraform.<componentID>.inputs.<key>", path)
		}
		return ExplainPath{
			Kind:    ExplainPathKindTerraformInput,
			Segment: parts[1],
			Key:     strings.Join(parts[3:], "."),
		}, nil
	case "kustomize":
		if len(parts) == 3 && parts[2] == "components" {
			return ExplainPath{
				Kind:    ExplainPathKindKustomizeComponents,
				Segment: parts[1],
			}, nil
		}
		if len(parts) < 4 || parts[2] != "substitutions" {
			return ExplainPath{}, fmt.Errorf("invalid kustomize path %q: expected kustomize.<name>.substitutions.<key> or kustomize.<name>.components", path)
		}
		return ExplainPath{
			Kind:    ExplainPathKindKustomizeSubstitution,
			Segment: parts[1],
			Key:     strings.Join(parts[3:], "."),
		}, nil
	case "configMaps":
		if len(parts) < 3 {
			return ExplainPath{}, fmt.Errorf("invalid configMaps path %q: expected configMaps.<name>.<key>", path)
		}
		return ExplainPath{
			Kind:    ExplainPathKindConfigMap,
			Segment: parts[1],
			Key:     strings.Join(parts[2:], "."),
		}, nil
	default:
		return ExplainPath{}, fmt.Errorf("invalid path %q: must start with terraform., kustomize., or configMaps.", path)
	}
}

// Explain delegates to the trace collector if set, otherwise returns an error. The handler
// method exists only to satisfy the BlueprintHandler interface.
func (h *BaseBlueprintHandler) Explain(pathStr string) (*ExplainTrace, error) {
	if h.traceCollector != nil {
		return h.traceCollector.GetTrace(pathStr)
	}
	_, err := ParseExplainPath(pathStr)
	if err != nil {
		return nil, err
	}
	if h.composedBlueprint == nil {
		return nil, fmt.Errorf("blueprint not composed")
	}
	return nil, fmt.Errorf("trace collector not set")
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

	_, inScope := resolveScopePath(c.scope, refPath)
	blockSource, blockLine, rawVal, record, remaining := c.getConfigBlockWithRecord(refPath)

	if !inScope && rawVal == nil {
		return ExplainScopeRef{Name: refPath, Status: "not set"}, true
	}

	sr := ExplainScopeRef{Name: refPath, Source: blockSource, Line: blockLine}

	evalVal, _ := resolveScopePath(c.scope, refPath)
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

// =============================================================================
// Helpers
// =============================================================================

// findTerraformComponent finds a terraform component by ID in the blueprint.
func findTerraformComponent(bp *blueprintv1alpha1.Blueprint, id string) *blueprintv1alpha1.TerraformComponent {
	for i := range bp.TerraformComponents {
		if bp.TerraformComponents[i].GetID() == id {
			return &bp.TerraformComponents[i]
		}
	}
	return nil
}

// findKustomization finds a kustomization by name in the blueprint.
func findKustomization(bp *blueprintv1alpha1.Blueprint, name string) *blueprintv1alpha1.Kustomization {
	for i := range bp.Kustomizations {
		if bp.Kustomizations[i].Name == name {
			return &bp.Kustomizations[i]
		}
	}
	return nil
}

// buildComponentContributions maps each resolved component entry back to the facet that
// contributed it. For each entry in the composed components list, the provenance records are
// scanned to find a facet whose RawComponents contains a matching value. All contributions
// are marked effective since each list entry is independently contributed.
func buildComponentContributions(resolved []string, provenance []ExplainContribution, componentType, componentName string) []ExplainContribution {
	if len(resolved) == 0 {
		return nil
	}
	var result []ExplainContribution
	for _, entry := range resolved {
		var matched *ExplainContribution

		for _, c := range provenance {
			if c.RawComponents == nil {
				continue
			}
			for _, raw := range c.RawComponents {
				if raw == entry {
					line := findComponentValueLine(c.AbsFacetPath, componentType, componentName, entry)
					if line == 0 {
						line = c.Line
					}
					matched = &ExplainContribution{
						SourceName:   c.SourceName,
						FacetPath:    c.FacetPath,
						AbsFacetPath: c.AbsFacetPath,
						Line:         line,
						Ordinal:      c.Ordinal,
						Strategy:     c.Strategy,
						Expression:   entry,
						Effective:    true,
					}
					break
				}
			}
			if matched != nil {
				break
			}
		}

		if matched == nil {
			for _, c := range provenance {
				if c.RawComponents == nil {
					continue
				}
				for _, raw := range c.RawComponents {
					if strings.HasPrefix(raw, "${") && matchesExpressionEntry(raw, entry) {
						line := findComponentValueLine(c.AbsFacetPath, componentType, componentName, entry)
						if line == 0 {
							line = c.Line
						}
						matched = &ExplainContribution{
							SourceName:   c.SourceName,
							FacetPath:    c.FacetPath,
							AbsFacetPath: c.AbsFacetPath,
							Line:         line,
							Ordinal:      c.Ordinal,
							Strategy:     c.Strategy,
							Expression:   entry,
							Effective:    true,
						}
						break
					}
				}
				if matched != nil {
					break
				}
			}
		}

		if matched != nil {
			result = append(result, *matched)
		} else {
			result = append(result, ExplainContribution{
				SourceName: "composed blueprint",
				Expression: entry,
				Effective:  true,
			})
		}
	}
	return result
}

// getNestedValue traverses nested maps to resolve a dotted key path.
func getNestedValue(m map[string]any, key string) (any, bool) {
	if key == "" {
		return nil, false
	}
	parts := strings.Split(key, ".")
	current := any(m)
	for _, part := range parts {
		if part == "" {
			return nil, false
		}
		m2, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m2[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// formatValue formats a value for display, using JSON for maps and arrays with truncation.
func formatValue(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case map[string]any, []any:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return truncate(string(b), 80)
	default:
		return truncate(fmt.Sprintf("%v", val), 80)
	}
}

// truncate shortens a string to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// findComponentValueLine returns the line of a specific component list entry within the
// "components" sequence.
func findComponentValueLine(facetPath, componentType, componentName, value string) int {
	return yamlComponentEntryLine(facetPath, componentType, componentName, value)
}

// resolveScopePath traverses nested maps to resolve a dotted path like "cluster.endpoint".
func resolveScopePath(scope map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	current := any(scope)
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	if current == nil {
		return nil, false
	}
	return current, true
}

// extractExprASTRefs parses each ${...} expression in s using the expr-lang parser and walks the
// AST to find all scope variable reference paths. Returns deduplicated dotted paths in discovery order.
func extractExprASTRefs(s string) []string {
	var refs []string
	seen := make(map[string]bool)
	searchStart := 0
	for {
		idx := strings.Index(s[searchStart:], "${")
		if idx == -1 {
			break
		}
		start := searchStart + idx
		end := findExprEnd(s, start)
		if end == -1 {
			break
		}
		exprContent := s[start+2 : end]
		tree, err := parser.Parse(exprContent)
		if err != nil {
			searchStart = end + 1
			continue
		}
		walkASTForRefs(tree.Node, &refs, seen)
		searchStart = end + 1
	}
	return refs
}

// findExprEnd returns the index of the closing '}' that matches the '${' at position start.
// Tracks brace depth and skips string literals so that braces inside strings are not counted.
func findExprEnd(s string, start int) int {
	if start < 0 || start+2 > len(s) || s[start] != '$' || s[start+1] != '{' {
		return -1
	}
	depth := 1
	i := start + 2
	for i < len(s) {
		c := s[i]
		switch c {
		case '"', '\'':
			i++
			for i < len(s) {
				if s[i] == '\\' {
					i += 2
					continue
				}
				if s[i] == c {
					i++
					break
				}
				i++
			}
		case '{':
			depth++
			i++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
			i++
		default:
			i++
		}
	}
	return -1
}

// walkASTForRefs recursively walks an expr-lang AST node, collecting scope variable reference
// paths. Member chains (a.b.c) are extracted as complete dotted paths. Function call callees
// and map literal keys are skipped.
func walkASTForRefs(node ast.Node, refs *[]string, seen map[string]bool) {
	if node == nil {
		return
	}
	switch n := node.(type) {
	case *ast.MemberNode:
		path := buildMemberPath(n)
		if path != "" {
			if !seen[path] {
				seen[path] = true
				*refs = append(*refs, path)
			}
		} else {
			walkASTForRefs(n.Node, refs, seen)
			walkASTForRefs(n.Property, refs, seen)
		}
	case *ast.IdentifierNode:
		if !seen[n.Value] {
			seen[n.Value] = true
			*refs = append(*refs, n.Value)
		}
	case *ast.CallNode:
		for _, arg := range n.Arguments {
			walkASTForRefs(arg, refs, seen)
		}
	case *ast.BuiltinNode:
		for _, arg := range n.Arguments {
			walkASTForRefs(arg, refs, seen)
		}
	case *ast.ChainNode:
		walkASTForRefs(n.Node, refs, seen)
	case *ast.BinaryNode:
		walkASTForRefs(n.Left, refs, seen)
		walkASTForRefs(n.Right, refs, seen)
	case *ast.UnaryNode:
		walkASTForRefs(n.Node, refs, seen)
	case *ast.ConditionalNode:
		walkASTForRefs(n.Cond, refs, seen)
		walkASTForRefs(n.Exp1, refs, seen)
		walkASTForRefs(n.Exp2, refs, seen)
	case *ast.ArrayNode:
		for _, elem := range n.Nodes {
			walkASTForRefs(elem, refs, seen)
		}
	case *ast.MapNode:
		for _, pair := range n.Pairs {
			if p, ok := pair.(*ast.PairNode); ok {
				walkASTForRefs(p.Value, refs, seen)
			}
		}
	case *ast.PairNode:
		walkASTForRefs(n.Value, refs, seen)
	case *ast.SliceNode:
		walkASTForRefs(n.Node, refs, seen)
		walkASTForRefs(n.From, refs, seen)
		walkASTForRefs(n.To, refs, seen)
	case *ast.PredicateNode:
		walkASTForRefs(n.Node, refs, seen)
	case *ast.SequenceNode:
		for _, child := range n.Nodes {
			walkASTForRefs(child, refs, seen)
		}
	case *ast.VariableDeclaratorNode:
		walkASTForRefs(n.Value, refs, seen)
		innerSeen := make(map[string]bool)
		for k, v := range seen {
			innerSeen[k] = v
		}
		innerSeen[n.Name] = true
		walkASTForRefs(n.Expr, refs, innerSeen)
	}
}

// buildMemberPath follows a chain of MemberNode -> IdentifierNode to produce a dotted path like
// "cluster.endpoint". Returns "" if the chain contains computed property access or non-identifier root.
func buildMemberPath(n *ast.MemberNode) string {
	var parts []string
	var node ast.Node = n
	for {
		switch m := node.(type) {
		case *ast.MemberNode:
			prop, ok := m.Property.(*ast.StringNode)
			if !ok {
				return ""
			}
			parts = append([]string{prop.Value}, parts...)
			node = m.Node
		case *ast.ChainNode:
			node = m.Node
		case *ast.IdentifierNode:
			parts = append([]string{m.Value}, parts...)
			return strings.Join(parts, ".")
		default:
			return ""
		}
	}
}
