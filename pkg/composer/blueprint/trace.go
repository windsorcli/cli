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
	"strings"

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
	SourceName    string
	FacetPath     string
	AbsFacetPath  string
	Line          int
	Ordinal       int
	Strategy      string
	Expression    string
	Effective     bool
	ScopeRefs     []ExplainScopeRef
	RawComponents []string
}

// ExplainTrace holds the result of explaining a path: the value and its contributions.
type ExplainTrace struct {
	Path          string
	Value         string
	Contributions []ExplainContribution
}

// SourceLocation identifies a specific value location within a facet YAML document, used as
// the recording key for trace data. DocumentPath is the composed path key (e.g.
// "terraform.networking.inputs.domain_name" or "config.cluster.endpoint") and FacetPath is
// the absolute filesystem path to the facet file that defines it.
type SourceLocation struct {
	FacetPath    string
	DocumentPath string
}

// DefaultTraceCollector is the standard TraceCollector implementation backed by in-memory maps.
type DefaultTraceCollector struct {
	scopeRefs   map[string][]string
	nestedPaths map[string][]string
}

// =============================================================================
// Interfaces
// =============================================================================

// TraceCollector records and retrieves expression trace data during blueprint composition.
// Implementations are set on the processor before composition begins and are only active
// when the explain command is running. RecordValue accepts a raw value and performs
// extraction internally; the processor only passes location and value.
type TraceCollector interface {
	Record(loc SourceLocation, scopeRefs []string, nestedPaths []string)
	RecordValue(loc SourceLocation, val any)
	GetScopeRefs(documentPath string) []string
	GetNestedPaths(documentPath string) []string
}

// =============================================================================
// Constructor
// =============================================================================

// NewTraceCollector creates a new DefaultTraceCollector with initialized storage.
func NewTraceCollector() *DefaultTraceCollector {
	return &DefaultTraceCollector{
		scopeRefs:   make(map[string][]string),
		nestedPaths: make(map[string][]string),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Record stores scope references and nested expression paths for the given source location.
func (c *DefaultTraceCollector) Record(loc SourceLocation, scopeRefs []string, nestedPaths []string) {
	if len(scopeRefs) > 0 {
		c.scopeRefs[loc.DocumentPath] = scopeRefs
	}
	if len(nestedPaths) > 0 {
		c.nestedPaths[loc.DocumentPath] = nestedPaths
	}
}

// GetScopeRefs returns the scope variable references recorded for a document path.
func (c *DefaultTraceCollector) GetScopeRefs(documentPath string) []string {
	return c.scopeRefs[documentPath]
}

// GetNestedPaths returns the nested expression paths recorded for a document path.
func (c *DefaultTraceCollector) GetNestedPaths(documentPath string) []string {
	return c.nestedPaths[documentPath]
}

// RecordValue extracts scope refs and nested paths from val and records them. The processor
// calls this only with location and raw value; all expression/trace logic lives here.
func (c *DefaultTraceCollector) RecordValue(loc SourceLocation, val any) {
	if val == nil {
		return
	}
	switch v := val.(type) {
	case string:
		if strings.Contains(v, "${") {
			c.Record(loc, extractExprASTRefs(v), nil)
		}
	default:
		if m, ok := asMapStringAny(val); ok && containsExpressionInValue(val) {
			nestedPaths := collectNestedExprPaths(m, "")
			if len(nestedPaths) > 0 {
				c.Record(loc, nil, nestedPaths)
				for _, np := range nestedPaths {
					childVal := navigateMapPath(m, np)
					if s, ok := childVal.(string); ok && strings.Contains(s, "${") {
						c.Record(SourceLocation{FacetPath: loc.FacetPath, DocumentPath: loc.DocumentPath + "." + np}, extractExprASTRefs(s), nil)
					}
				}
			}
		}
	}
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

// Explain resolves a dotted blueprint path against the composed blueprint and returns a trace
// containing the resolved value, provenance contributions, and scope reference resolution.
// Provenance is read from records accumulated during composition (not retroactively scanned).
// Scope references in expressions are resolved against the composed scope to determine their
// status (resolved, not set, or deferred) and config block source locations are looked up from
// the accumulated config block provenance.
func (h *BaseBlueprintHandler) Explain(pathStr string) (*ExplainTrace, error) {
	p, err := ParseExplainPath(pathStr)
	if err != nil {
		return nil, err
	}
	bp := h.composedBlueprint
	if bp == nil {
		return nil, fmt.Errorf("blueprint not composed")
	}

	provMap := h.getProcessorProvenance()

	var provenanceKey, keySection string
	switch p.Kind {
	case ExplainPathKindTerraformInput:
		provenanceKey = "terraform." + p.Segment
		keySection = "inputs"
	case ExplainPathKindKustomizeSubstitution:
		provenanceKey = "kustomize." + p.Segment
		keySection = "substitutions"
	case ExplainPathKindKustomizeComponents:
		provenanceKey = "kustomize." + p.Segment
		keySection = "components"
	}

	var contributions []ExplainContribution
	if provenanceKey != "" {
		for _, pe := range provMap[provenanceKey] {
			contributions = append(contributions, h.provenanceToContribution(pe, keySection, p.Key, p.Segment))
		}
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
		trace.Contributions = markEffective(contributions)

	case ExplainPathKindKustomizeSubstitution:
		k := findKustomization(bp, p.Segment)
		if k == nil {
			return nil, fmt.Errorf("kustomization %q not found in blueprint", p.Segment)
		}
		if k.Substitutions == nil {
			trace.Value = ""
			trace.Contributions = markEffective(contributions)
			return trace, nil
		}
		val, ok := k.Substitutions[p.Key]
		if !ok {
			trace.Value = ""
			trace.Contributions = markEffective(contributions)
			return trace, nil
		}
		trace.Value = val
		trace.Contributions = markEffective(contributions)

	case ExplainPathKindKustomizeComponents:
		k := findKustomization(bp, p.Segment)
		if k == nil {
			return nil, fmt.Errorf("kustomization %q not found in blueprint", p.Segment)
		}
		trace.Contributions = buildComponentContributions(k.Components, contributions, "kustomize", p.Segment)

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

	h.resolveScopeRefs(trace, provMap)

	return trace, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// navigateMapPath traverses a map using a dotted path to reach a nested value.
func navigateMapPath(m map[string]any, dottedPath string) any {
	parts := strings.Split(dottedPath, ".")
	var current any = m
	for _, part := range parts {
		cm, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = cm[part]
	}
	return current
}

// provenanceToContribution converts a ProvenanceEntry (recorded during composition) into an
// ExplainContribution. The line number for the specific key within the component is resolved
// via AST on demand. The facet path is shortened relative to the template root for display.
func (h *BaseBlueprintHandler) provenanceToContribution(pe ProvenanceEntry, keySection, key, componentID string) ExplainContribution {
	displayPath := pe.FacetPath
	if displayPath != "" {
		shortened := false
		if h.runtime != nil && h.runtime.TemplateRoot != "" {
			if rel, err := filepath.Rel(h.runtime.TemplateRoot, displayPath); err == nil && !strings.HasPrefix(rel, "..") {
				displayPath = rel
				shortened = true
			}
		}
		if !shortened {
			displayPath = filepath.Base(displayPath)
		}
	}

	line := pe.Line
	if keySection != "" && key != "" && keySection != "components" {
		componentType := "terraform"
		if keySection == "substitutions" {
			componentType = "kustomize"
		}
		if keyLine := yamlNodeLine(pe.FacetPath, componentType, namedItem(componentID), keySection, key); keyLine > 0 {
			line = keyLine
		}
	}

	c := ExplainContribution{
		SourceName:    pe.SourceName,
		FacetPath:     displayPath,
		AbsFacetPath:  pe.FacetPath,
		Line:          line,
		Ordinal:       pe.Ordinal,
		Strategy:      pe.Strategy,
		RawComponents: pe.RawComponents,
	}

	if pe.RawInputs != nil && keySection == "inputs" {
		if rawVal, ok := pe.RawInputs[key]; ok {
			c.Expression = formatValue(rawVal)
		}
	} else if pe.RawSubs != nil && keySection == "substitutions" {
		if rawVal, ok := pe.RawSubs[key]; ok {
			c.Expression = rawVal
		}
	}

	return c
}

// getProcessorProvenance retrieves the accumulated provenance map from the processor. Returns an
// empty map if the processor is not a BaseBlueprintProcessor or has no provenance.
func (h *BaseBlueprintHandler) getProcessorProvenance() map[string][]ProvenanceEntry {
	if bp, ok := h.processor.(*BaseBlueprintProcessor); ok {
		return bp.GetAllProvenance()
	}
	return make(map[string][]ProvenanceEntry)
}

// resolveScopeRefs populates the ScopeRefs field on each effective contribution that has an
// expression containing scope references. When a TraceCollector is available, pre-recorded
// scope refs are used; otherwise refs are extracted on-demand via AST parsing. Refs are
// resolved against the composed scope and recursively expanded until origins (literals or
// not set). A visited set prevents infinite recursion on circular config references.
func (h *BaseBlueprintHandler) resolveScopeRefs(trace *ExplainTrace, provMap map[string][]ProvenanceEntry) {
	scope := h.composedScope
	if scope == nil {
		return
	}
	visited := make(map[string]bool)
	for i := range trace.Contributions {
		c := &trace.Contributions[i]
		if !c.Effective || c.Expression == "" || !strings.Contains(c.Expression, "${") {
			continue
		}
		var refs []string
		if h.traceCollector != nil {
			refs = h.traceCollector.GetScopeRefs(trace.Path)
		}
		if len(refs) == 0 {
			refs = extractExprASTRefs(c.Expression)
		}
		for _, ref := range refs {
			sr, include := expandScopeRef(scope, provMap, ref, visited, h.traceCollector)
			if include {
				c.ScopeRefs = append(c.ScopeRefs, sr)
			}
		}
	}
}

// expandScopeRef builds one scope ref and recursively expands Nested by tracing RAW config
// values from provenance rather than evaluated scope values. This allows following the
// definition chain through expressions that were already evaluated. When a TraceCollector
// is provided (non-nil), pre-recorded scope refs and nested paths are used for expansion;
// otherwise falls back to on-demand AST extraction. When no raw value is available (e.g.
// context values), falls back to the evaluated scope. Uses visited to detect cycles and
// isKnownScopeRoot to filter false-positive matches.
func expandScopeRef(scope map[string]any, provMap map[string][]ProvenanceEntry, refPath string, visited map[string]bool, collector TraceCollector) (ExplainScopeRef, bool) {
	if !isKnownScopeRoot(scope, provMap, refPath) {
		return ExplainScopeRef{}, false
	}
	if visited[refPath] {
		return ExplainScopeRef{Name: refPath, Status: "cycle"}, true
	}
	visited[refPath] = true
	defer delete(visited, refPath)

	_, inScope := resolveScopePath(scope, refPath)
	blockSource, blockLine, rawVal := getProvenanceWithRawValue(provMap, refPath)

	if !inScope && rawVal == nil {
		return ExplainScopeRef{Name: refPath, Status: "not set"}, true
	}

	sr := ExplainScopeRef{Name: refPath, Source: blockSource, Line: blockLine}

	evalVal, _ := resolveScopePath(scope, refPath)
	evalDeferred := containsExpressionInValue(evalVal)

	traceVal := rawVal
	if traceVal == nil {
		traceVal = evalVal
	}
	if traceVal == nil {
		return sr, true
	}

	configDocPath := "config." + refPath
	if s, ok := traceVal.(string); ok && strings.Contains(s, "${") {
		if evalDeferred {
			sr.Status = "deferred"
		}
		var allRefs []string
		if collector != nil {
			allRefs = collector.GetScopeRefs(configDocPath)
		}
		if len(allRefs) == 0 {
			allRefs = extractExprASTRefs(s)
		}
		for _, nestedRef := range allRefs {
			nested, include := expandScopeRef(scope, provMap, nestedRef, visited, collector)
			if include {
				sr.Nested = append(sr.Nested, nested)
			}
		}
		return sr, true
	}

	if m, ok := asMapStringAny(traceVal); ok && containsExpressionInValue(traceVal) {
		if evalDeferred {
			sr.Status = "deferred"
		}
		if strings.Contains(refPath, ".") {
			var nestedExprPaths []string
			if collector != nil {
				nestedExprPaths = collector.GetNestedPaths(configDocPath)
			}
			if len(nestedExprPaths) == 0 {
				nestedExprPaths = collectNestedExprPaths(m, "")
			}
			for _, path := range nestedExprPaths {
				fullPath := refPath + "." + path
				nested, include := expandScopeRef(scope, provMap, fullPath, visited, collector)
				if include {
					sr.Nested = append(sr.Nested, nested)
				}
			}
		}
		return sr, true
	}

	return sr, true
}

// getProvenanceWithRawValue returns the facet path, line, and raw config value for a scope
// path by matching the longest config.* prefix in provMap. When the matched provenance is a
// parent of refPath, the remaining path segments are used to navigate into the raw value
// (drilling into nested maps) so that deeply-nested keys get their specific raw value.
// The line number is re-resolved via YAML AST when remaining segments exist so that nested
// keys report their own line rather than the parent block's line.
func getProvenanceWithRawValue(provMap map[string][]ProvenanceEntry, refPath string) (string, int, any) {
	key := "config." + refPath
	var remainingSegments []string
	for {
		if entries := provMap[key]; len(entries) > 0 {
			last := entries[len(entries)-1]
			rawVal := last.RawConfigValue
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
				return "", 0, nil
			}
			line := last.Line
			if len(remainingSegments) > 0 && last.FacetPath != "" {
				if nested := resolveNestedConfigLine(last.FacetPath, key, remainingSegments); nested > 0 {
					line = nested
				}
			}
			return last.FacetPath, line, rawVal
		}
		i := strings.LastIndex(key, ".")
		if i <= 6 {
			return "", 0, nil
		}
		remainingSegments = append([]string{key[i+1:]}, remainingSegments...)
		key = key[:i]
	}
}

// resolveNestedConfigLine uses YAML AST navigation to find the line number of a nested key
// within a config block value. matchedKey is the provenance key that was found (e.g.
// "config.talos_common.common_patch") and remaining holds the path segments beyond it
// (e.g. ["cluster", "allowSchedulingOnControlPlanes"]). The YAML path is reconstructed
// from the provenance key structure and the remaining segments are appended.
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

// isKnownScopeRoot returns true when the first segment of refPath exists as a top-level scope
// key or has config provenance. This filters out false-positive regex matches like URL
// fragments (raw.githubusercontent.com) or filenames (install.yaml).
func isKnownScopeRoot(scope map[string]any, provMap map[string][]ProvenanceEntry, refPath string) bool {
	root := refPath
	if i := strings.Index(refPath, "."); i > 0 {
		root = refPath[:i]
	}
	if _, ok := scope[root]; ok {
		return true
	}
	prefix := "config." + root
	for key := range provMap {
		if key == prefix || strings.HasPrefix(key, prefix+".") {
			return true
		}
	}
	return false
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

// =============================================================================
// Helpers
// =============================================================================

// extractExprASTRefs parses each ${...} expression in s using the expr-lang parser and walks the
// AST to find all scope variable reference paths. Unlike regex extraction, this correctly
// distinguishes variable references from function calls, string literals, map keys, and other
// non-reference identifiers. Returns deduplicated dotted paths (e.g. ["cluster.endpoint",
// "common_patch"]) in discovery order.
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
// Tracks brace depth and skips string literals (single and double quoted) so that braces inside
// strings are not counted.
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
// and map literal keys are skipped. Let-bound variable names are tracked to avoid reporting
// local bindings as scope references.
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

// buildMemberPath follows a chain of MemberNode → IdentifierNode to produce a dotted path like
// "cluster.endpoint". Returns "" if the chain contains computed property access (non-string
// property) or the root is not an IdentifierNode.
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

// markEffective determines which contribution produced the final composed value and sets its
// Effective flag. Follows Windsor composition semantics: contributions are assumed sorted by
// ascending ordinal. A replace strategy resets the effective index; for merge, the highest-ordinal
// contributor that defines the key wins. Returns a placeholder when contributions are empty.
func markEffective(contributions []ExplainContribution) []ExplainContribution {
	if len(contributions) == 0 {
		return []ExplainContribution{{SourceName: "composed blueprint"}}
	}
	effectiveIdx := -1
	for i := range contributions {
		c := &contributions[i]
		if c.Strategy == "replace" {
			effectiveIdx = i
		} else if c.Expression != "" {
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
// "components" sequence. When multiple blocks share the same name (merge), scans all blocks
// and returns the line from the one that contains this value so the line is the list entry,
// not the top-level block (e.g. - name: observability).
func findComponentValueLine(facetPath, componentType, componentName, value string) int {
	return yamlComponentEntryLine(facetPath, componentType, componentName, value)
}
