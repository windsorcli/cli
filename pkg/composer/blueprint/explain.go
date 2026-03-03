// The ExplainResolver provides blueprint value provenance resolution for the windsor explain command.
// It resolves a dotted path against the composed blueprint and produces a trace showing
// the value and which facets contributed to it, enabling users to understand where values
// originate and how composition affected them.
package blueprint

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Constants
// =============================================================================

// ExplainPathKind identifies the type of blueprint path being explained.
type ExplainPathKind int

const (
	ExplainPathKindTerraformInput ExplainPathKind = iota
	ExplainPathKindKustomizeSubstitution
	ExplainPathKindKustomizeComponents
	ExplainPathKindConfigMap
)

// =============================================================================
// Types
// =============================================================================

// ComponentProvenance records a single facet's contribution to a component in the composed blueprint.
// ComponentLine is the 1-based line number where the component is defined in the facet file.
type ComponentProvenance struct {
	SourceName    string
	FacetName     string
	FacetPath     string
	Ordinal       int
	Strategy      string
	RawInputs     map[string]any
	RawSubs       map[string]string
	RawComponents []string
	ComponentLine int
}

// ExplainPath is a parsed explain path identifying a single value in the composed blueprint.
type ExplainPath struct {
	Kind    ExplainPathKind
	Segment string
	Key     string
}

// ExplainScopeRef describes a scope variable referenced in an expression, its resolution status,
// and the source location of the config block that defines it (if applicable).
type ExplainScopeRef struct {
	Name   string
	Status string
	Source string
	Line   int
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

// =============================================================================
// Public Methods
// =============================================================================

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
// Provenance records are derived from active facets, with path shortening relative to the
// template root and line numbers resolved to the specific key within each facet file. Scope
// references in expressions are resolved against the composed scope to determine their status
// (resolved, not set, or deferred) and config block source locations.
func (h *BaseBlueprintHandler) Explain(pathStr string) (*ExplainTrace, error) {
	p, err := ParseExplainPath(pathStr)
	if err != nil {
		return nil, err
	}
	bp := h.composedBlueprint
	if bp == nil {
		return nil, fmt.Errorf("blueprint not composed")
	}

	var componentType, keySection string
	switch p.Kind {
	case ExplainPathKindTerraformInput:
		componentType = "terraform"
		keySection = "inputs"
	case ExplainPathKindKustomizeSubstitution:
		componentType = "kustomize"
		keySection = "substitutions"
	case ExplainPathKindKustomizeComponents:
		componentType = "kustomize"
		keySection = "components"
	}

	var contributions []ExplainContribution
	if componentType != "" {
		for _, cp := range h.getProvenance(componentType, p.Segment) {
			contributions = append(contributions, h.toExplainContribution(cp, keySection, p.Key))
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

	h.resolveScopeRefs(trace)

	return trace, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// toExplainContribution converts a ComponentProvenance into an ExplainContribution, shortening
// the facet path to a user-friendly display form and resolving the line number for the specific
// key within the facet file. For local facets under the template root, the path is shown relative
// to that root. For OCI-sourced facets outside the template root, only the filename is shown.
func (h *BaseBlueprintHandler) toExplainContribution(cp ComponentProvenance, keySection, key string) ExplainContribution {
	displayPath := cp.FacetPath
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

	line := cp.ComponentLine
	if line > 0 && keySection != "" && key != "" {
		if keyLine := findKeyLine(cp.FacetPath, line, keySection, key); keyLine > 0 {
			line = keyLine
		}
	}

	c := ExplainContribution{
		SourceName:    cp.SourceName,
		FacetPath:     displayPath,
		AbsFacetPath:  cp.FacetPath,
		Line:          line,
		Ordinal:       cp.Ordinal,
		Strategy:      cp.Strategy,
		RawComponents: cp.RawComponents,
	}

	if cp.RawInputs != nil && keySection == "inputs" {
		if rawVal, ok := cp.RawInputs[key]; ok {
			c.Expression = formatValue(rawVal)
		}
	} else if cp.RawSubs != nil && keySection == "substitutions" {
		if rawVal, ok := cp.RawSubs[key]; ok {
			c.Expression = rawVal
		}
	}

	return c
}

// resolveScopeRefs populates the ScopeRefs field on each effective contribution by extracting
// dotted identifier references from the expression, resolving them against the composed scope,
// and looking up config block source locations. Only references that are not set, deferred, or
// traceable to a config block source are included.
func (h *BaseBlueprintHandler) resolveScopeRefs(trace *ExplainTrace) {
	scope := h.composedScope
	if scope == nil {
		return
	}
	for i := range trace.Contributions {
		c := &trace.Contributions[i]
		if !c.Effective || c.Expression == "" || !strings.Contains(c.Expression, "${") {
			continue
		}
		refs := extractScopeRefs(c.Expression)
		for _, ref := range refs {
			val, ok := resolveScopePath(scope, ref)
			if !ok {
				c.ScopeRefs = append(c.ScopeRefs, ExplainScopeRef{Name: ref, Status: "not set"})
				continue
			}
			s := formatScopeValue(val)
			deferred := strings.Contains(s, "${")
			blockSource, blockLine := h.getConfigBlockSource(strings.SplitN(ref, ".", 2)[0])
			hasSource := blockSource != "" && blockLine > 0
			if !deferred && !hasSource {
				continue
			}
			sr := ExplainScopeRef{Name: ref, Source: blockSource, Line: blockLine}
			if deferred {
				sr.Status = "deferred"
			}
			c.ScopeRefs = append(c.ScopeRefs, sr)
		}
	}
}

// getProvenance scans all loaded facets across all source loaders and returns only the facets
// that actually contributed to the given component in the current context. Facets and components
// whose 'when' conditions evaluate to false are excluded. componentType is "terraform" or
// "kustomize"; componentID is the component's GetID() or kustomization Name.
func (h *BaseBlueprintHandler) getProvenance(componentType, componentID string) []ComponentProvenance {
	scope := h.composedScope
	if scope == nil {
		scope = h.getConfigValues()
	}
	if scope == nil {
		scope = make(map[string]any)
	}

	var results []ComponentProvenance
	for sourceName, loader := range h.sourceBlueprintLoaders {
		for _, facet := range loader.GetFacets() {
			if !h.evaluateWhen(facet.When, facet.Path, scope) {
				continue
			}

			ordinal := resolvedFacetOrdinal(facet)

			switch componentType {
			case "terraform":
				for _, tc := range facet.TerraformComponents {
					if tc.GetID() != componentID {
						continue
					}
					when := tc.When
					if when == "" {
						when = facet.When
					}
					if !h.evaluateWhen(when, facet.Path, scope) {
						continue
					}
					strategy := tc.Strategy
					if strategy == "" {
						strategy = "merge"
					}
					effOrd := ordinal
					if tc.Ordinal != nil {
						effOrd = *tc.Ordinal
					}
					results = append(results, ComponentProvenance{
						SourceName:    sourceName,
						FacetName:     facet.Metadata.Name,
						FacetPath:     facet.Path,
						Ordinal:       effOrd,
						Strategy:      strategy,
						RawInputs:     tc.Inputs,
						ComponentLine: findComponentLine(facet.Path, componentType, componentID),
					})
				}
			case "kustomize":
				for _, k := range facet.Kustomizations {
					if k.Name != componentID {
						continue
					}
					when := k.When
					if when == "" {
						when = facet.When
					}
					if !h.evaluateWhen(when, facet.Path, scope) {
						continue
					}
					strategy := k.Strategy
					if strategy == "" {
						strategy = "merge"
					}
					effOrd := ordinal
					if k.Ordinal != nil {
						effOrd = *k.Ordinal
					}
					results = append(results, ComponentProvenance{
						SourceName:    sourceName,
						FacetName:     facet.Metadata.Name,
						FacetPath:     facet.Path,
						Ordinal:       effOrd,
						Strategy:      strategy,
						RawSubs:       k.Substitutions,
						RawComponents: k.Components,
						ComponentLine: findComponentLine(facet.Path, componentType, componentID),
					})
				}
			}
		}
	}
	return results
}

// getConfigBlockSource finds the active facet that defines a config block with the given name
// and returns the facet file path and 1-based line number of the "- name: <name>" declaration.
// Facets whose 'when' conditions evaluate to false are skipped. When multiple facets define
// the same config block name, the last one in ordinal order wins (matching composition semantics).
// Returns ("", 0) if the config block is not found or comes from context values.
func (h *BaseBlueprintHandler) getConfigBlockSource(name string) (string, int) {
	scope := h.composedScope
	if scope == nil {
		scope = h.getConfigValues()
	}
	if scope == nil {
		scope = make(map[string]any)
	}
	var bestPath string
	var bestLine int
	for _, loader := range h.sourceBlueprintLoaders {
		for _, facet := range loader.GetFacets() {
			if !h.evaluateWhen(facet.When, facet.Path, scope) {
				continue
			}
			for _, cb := range facet.Config {
				if cb.Name != name {
					continue
				}
				when := cb.When
				if when == "" {
					when = facet.When
				}
				if !h.evaluateWhen(when, facet.Path, scope) {
					continue
				}
				bestPath = facet.Path
				bestLine = findConfigBlockLine(facet.Path, name)
			}
		}
	}
	return bestPath, bestLine
}

// evaluateWhen evaluates a 'when' condition expression against the given scope using the
// runtime's expression evaluator. Returns true when the expression is empty (unconditional),
// evaluates to true, or when the evaluator is unavailable. Returns false on evaluation error
// or when the condition is false.
func (h *BaseBlueprintHandler) evaluateWhen(when, facetPath string, scope map[string]any) bool {
	if when == "" {
		return true
	}
	if h.runtime == nil || h.runtime.Evaluator == nil {
		return true
	}
	expr := when
	if !evaluator.ContainsExpression(expr) {
		expr = "${" + expr + "}"
	}
	evaluated, err := h.runtime.Evaluator.Evaluate(expr, facetPath, scope, false)
	if err != nil {
		return false
	}
	switch v := evaluated.(type) {
	case bool:
		return v
	case string:
		return v == "true"
	default:
		return false
	}
}

// =============================================================================
// Helpers
// =============================================================================

var scopeRefRe = regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*)+`)

// extractScopeRefs extracts dotted identifier paths (e.g. cluster.endpoint) from an expression
// string. Single-segment identifiers are excluded since they could be local variables or
// function names. Keywords like true/false/nil are filtered out.
func extractScopeRefs(expr string) []string {
	matches := scopeRefRe.FindAllString(expr, -1)
	seen := make(map[string]bool)
	var refs []string
	for _, m := range matches {
		if seen[m] || isExprKeyword(m) {
			continue
		}
		seen[m] = true
		refs = append(refs, m)
	}
	return refs
}

func isExprKeyword(s string) bool {
	switch s {
	case "true", "false", "nil", "null", "len", "not", "and", "or", "in", "matches":
		return true
	}
	return false
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

// formatScopeValue formats a scope value for status detection, using JSON for complex types.
func formatScopeValue(v any) string {
	switch val := v.(type) {
	case string:
		return truncate(val, 80)
	case map[string]any, []any:
		b, err := json.Marshal(val)
		if err != nil {
			return truncate(fmt.Sprintf("%v", val), 80)
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

// findKeyLine returns the 1-based line number of a specific key within a component block in a
// facet YAML file. componentLine is the 1-based line where the component starts (its "- name:"
// line). section is "inputs" or "substitutions". The scan reads from componentLine forward
// until it finds the section header, then the key within that section. Returns 0 if the key
// cannot be located, in which case the caller should fall back to componentLine.
func findKeyLine(facetPath string, componentLine int, section, key string) int {
	if facetPath == "" || componentLine <= 0 {
		return 0
	}
	data, err := os.ReadFile(facetPath)
	if err != nil {
		return 0
	}
	lines := strings.Split(string(data), "\n")
	if componentLine > len(lines) {
		return 0
	}

	compIndent := leadingSpaces(lines[componentLine-1])
	inSection := false
	sectionIndent := -1
	for i := componentLine; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		indent := leadingSpaces(line)
		if indent <= compIndent && i > componentLine-1 {
			break
		}
		if !inSection {
			if trimmed == section+":" {
				inSection = true
				sectionIndent = indent
				continue
			}
		} else {
			if indent <= sectionIndent {
				break
			}
			colonIdx := strings.Index(trimmed, ":")
			if colonIdx > 0 && trimmed[:colonIdx] == key {
				return i + 1
			}
		}
	}
	return 0
}

// findConfigBlockLine scans a facet YAML file for the line number of a config block definition
// matching "- name: <blockName>" under the top-level "config:" section. Returns 0 if not found.
func findConfigBlockLine(facetPath, blockName string) int {
	return findComponentLine(facetPath, "config", blockName)
}

// findComponentLine scans a facet YAML file for the line number of a component definition
// matching "- name: <componentID>" under the given top-level section (terraform or kustomize).
// Returns 0 if the component cannot be located.
func findComponentLine(facetPath, componentType, componentID string) int {
	if facetPath == "" {
		return 0
	}
	data, err := os.ReadFile(facetPath)
	if err != nil {
		return 0
	}
	lines := strings.Split(string(data), "\n")
	inSection := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == componentType+":" {
			inSection = true
			continue
		}
		if inSection {
			if len(trimmed) > 0 && leadingSpaces(line) == 0 && !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "#") {
				inSection = false
				continue
			}
			if trimmed == "- name: "+componentID {
				return i + 1
			}
		}
	}
	return 0
}

// matchesExpressionEntry checks whether a resolved component value could have been produced by
// a raw expression. It extracts single-quoted string literals from the expression and returns
// true if the entry exactly matches a literal or if the entry starts with a literal that is at
// least 4 characters long (handles string concatenation patterns like 'prefix/' + variable).
func matchesExpressionEntry(raw, entry string) bool {
	if strings.Contains(raw, "'"+entry+"'") {
		return true
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\'' {
			continue
		}
		j := strings.Index(raw[i+1:], "'")
		if j < 0 {
			break
		}
		literal := raw[i+1 : i+1+j]
		i = i + 1 + j
		if len(literal) >= 4 && strings.HasPrefix(entry, literal) {
			return true
		}
	}
	return false
}

// findComponentValueLine scans a facet YAML file for a specific component list entry across ALL
// blocks matching "- name: <componentName>" under the given componentType section. For each
// matching block's "components:" list, it checks for literal matches (bare, single-quoted, or
// double-quoted) and expression matches where the value appears as a string literal inside a
// ${...} expression. Returns the 1-based line number or 0 if not found.
func findComponentValueLine(facetPath, componentType, componentName, value string) int {
	if facetPath == "" {
		return 0
	}
	data, err := os.ReadFile(facetPath)
	if err != nil {
		return 0
	}
	lines := strings.Split(string(data), "\n")

	inSection := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == componentType+":" {
			inSection = true
			continue
		}
		if inSection {
			if len(trimmed) > 0 && leadingSpaces(line) == 0 && !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "#") {
				inSection = false
				continue
			}
			if trimmed != "- name: "+componentName {
				continue
			}
			blockIndent := leadingSpaces(line)
			inComponents := false
			componentsIndent := -1
			for j := i + 1; j < len(lines); j++ {
				bline := lines[j]
				btrimmed := strings.TrimSpace(bline)
				if btrimmed == "" || strings.HasPrefix(btrimmed, "#") {
					continue
				}
				bindent := leadingSpaces(bline)
				if bindent <= blockIndent {
					break
				}
				if !inComponents {
					if btrimmed == "components:" {
						inComponents = true
						componentsIndent = bindent
						continue
					}
				} else {
					if bindent <= componentsIndent {
						inComponents = false
						continue
					}
					entry := strings.TrimPrefix(btrimmed, "- ")
					entry = strings.Trim(entry, "\"'")
					if entry == value {
						return j + 1
					}
					if strings.HasPrefix(entry, "${") && matchesExpressionEntry(entry, value) {
						return j + 1
					}
				}
			}
		}
	}
	return 0
}

// leadingSpaces returns the number of leading space characters in s.
func leadingSpaces(s string) int {
	for i, c := range s {
		if c != ' ' {
			return i
		}
	}
	return len(s)
}
