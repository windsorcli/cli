// The explain functions of the blueprint package parse and format the paths of the windsor explain command.
// It provides the ExplainPath and ExplainTrace types, and ParseExplainPath for terraform, kustomize,
// configMaps, and substitutions paths. The helpers find components and format values for a trace.

package blueprint

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

const (
	ExplainPathKindTerraformInput ExplainPathKind = iota
	ExplainPathKindKustomizeSubstitution
	ExplainPathKindKustomizeComponents
	ExplainPathKindConfigMap
	ExplainPathKindSubstitution
)

// explainPathHint is the remediation for an explain path with an unknown root or too few segments.
const explainPathHint = "Use a path that starts with terraform., kustomize., configMaps., or substitutions., for example terraform.<componentID>.inputs.<key>."

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
	case ExplainPathKindSubstitution:
		return fmt.Sprintf("substitutions.%s", p.Key)
	default:
		return ""
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

// ParseExplainPath parses a path into an ExplainPath. The forms are terraform.<componentID>.inputs.<key>,
// kustomize.<name>.substitutions.<key>, kustomize.<name>.components, configMaps.<name>.<key>, and substitutions.<key>.
// It returns an error if the path is empty. A path with an unknown root or a wrong form
// returns an error with a remediation hint that shows the correct form.
func ParseExplainPath(path string) (ExplainPath, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return ExplainPath{}, errors.New("path is required")
	}
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return ExplainPath{}, werror.New(fmt.Sprintf("invalid path %q", path), explainPathHint, nil)
	}
	switch parts[0] {
	case "terraform":
		if len(parts) < 4 || parts[2] != "inputs" {
			return ExplainPath{}, werror.New(fmt.Sprintf("invalid terraform path %q", path), "Use the form terraform.<componentID>.inputs.<key>.", nil)
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
			return ExplainPath{}, werror.New(fmt.Sprintf("invalid kustomize path %q", path), "Use the form kustomize.<name>.substitutions.<key> or kustomize.<name>.components.", nil)
		}
		return ExplainPath{
			Kind:    ExplainPathKindKustomizeSubstitution,
			Segment: parts[1],
			Key:     strings.Join(parts[3:], "."),
		}, nil
	case "configMaps":
		if len(parts) < 3 {
			return ExplainPath{}, werror.New(fmt.Sprintf("invalid configMaps path %q", path), "Use the form configMaps.<name>.<key>.", nil)
		}
		return ExplainPath{
			Kind:    ExplainPathKindConfigMap,
			Segment: parts[1],
			Key:     strings.Join(parts[2:], "."),
		}, nil
	case "substitutions":
		if len(parts) < 2 {
			return ExplainPath{}, werror.New(fmt.Sprintf("invalid substitutions path %q", path), "Use the form substitutions.<key>.", nil)
		}
		return ExplainPath{
			Kind: ExplainPathKindSubstitution,
			Key:  strings.Join(parts[1:], "."),
		}, nil
	default:
		return ExplainPath{}, werror.New(fmt.Sprintf("invalid path %q", path), explainPathHint, nil)
	}
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
	return current, true
}
