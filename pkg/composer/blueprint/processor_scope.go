// The processor_scope file holds the scope check of the BlueprintProcessor.
// Each source owns the config blocks that its facets define. Other sources cannot read or write them.
// A source can share a block through the exports of its blueprint, or through its schema.
// The check runs on the facet text before composition, so the result does not depend on config values.

package blueprint

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/expr-lang/expr/parser"
	"github.com/goccy/go-yaml"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Constants
// =============================================================================

const sourceScopeHint = "Each blueprint keeps its config blocks private. Share a value through exports: in blueprint.yaml, or through the schema."

// =============================================================================
// Private Methods
// =============================================================================

// schemaPropertyNames returns the top-level property names of the merged schema. Facets of every
// source can read and write these names. It returns an empty set when no schema is loaded.
func (p *BaseBlueprintProcessor) schemaPropertyNames() map[string]bool {
	names := make(map[string]bool)
	if p.runtime == nil || p.runtime.ConfigHandler == nil {
		return names
	}
	props, _ := p.runtime.ConfigHandler.GetSchema()["properties"].(map[string]any)
	for name := range props {
		names[name] = true
	}
	return names
}

// =============================================================================
// Helpers
// =============================================================================

// checkSourceScopes returns an error that lists each facet that uses a config block of another
// source. A source owns each config block name that its facets define, unless the schema declares
// the name. A facet fails the check when one of these is true:
//   - It reads a block that another source owns and does not export.
//   - It writes a block that another source exports.
//
// Two sources that define the same block name, and neither exports it, also fail. So does an export
// that no facet of its source defines.
func checkSourceScopes(sources []SourceFacetSet, schemaNames map[string]bool) error {
	labels := make(map[string]string, len(sources))
	for _, src := range sources {
		labels[src.Name] = sourceLabel(src)
	}
	definers := make(map[string][]string)
	exporters := make(map[string][]string)
	for _, src := range sources {
		defined := make(map[string]bool)
		for _, facet := range src.Facets {
			for _, block := range facet.Config {
				if block.Name == "" || schemaNames[block.Name] || defined[block.Name] {
					continue
				}
				defined[block.Name] = true
				definers[block.Name] = append(definers[block.Name], src.Name)
			}
		}
	}

	var problems []string
	for _, src := range sources {
		seen := make(map[string]bool)
		for _, name := range src.Exports {
			if schemaNames[name] || seen[name] {
				continue
			}
			seen[name] = true
			if slices.Contains(definers[name], src.Name) {
				exporters[name] = append(exporters[name], src.Name)
				continue
			}
			problems = append(problems, fmt.Sprintf("blueprint %q exports %q, but none of its facets define that config block. Remove %q from exports:, or add the config block.", labels[src.Name], name, name))
		}
	}

	owners := make(map[string]string)
	names := make([]string, 0, len(definers))
	for name := range definers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		defined := exporters[name]
		switch {
		case len(defined) == 1:
			owners[name] = defined[0]
		case len(defined) > 1:
			problems = append(problems, fmt.Sprintf("blueprints %s export the same config block %q. Rename the config block so that only one blueprint exports it.", quoteJoin(defined, labels), name))
		case len(definers[name]) == 1:
			owners[name] = definers[name][0]
		default:
			problems = append(problems, fmt.Sprintf("blueprints %s define the same config block %q. Rename the config block so that only one blueprint defines it, or export it from one of them.", quoteJoin(definers[name], labels), name))
		}
	}

	for _, src := range sources {
		for _, facet := range src.Facets {
			for _, name := range facetWrittenBlocks(facet) {
				owner, ok := owners[name]
				if !ok || owner == src.Name || !slices.Contains(exporters[name], owner) {
					continue
				}
				problems = append(problems, fmt.Sprintf("facet %q in blueprint %q writes config block %q, which blueprint %q owns. Rename the config block, or set a schema value that %q reads.", facet.Metadata.Name, labels[src.Name], name, labels[owner], labels[owner]))
			}
			heads, err := facetReadHeads(facet)
			if err != nil {
				return werror.Wrap(err, "facet %q in blueprint %q", facet.Metadata.Name, labels[src.Name])
			}
			for _, name := range heads {
				owner, ok := owners[name]
				if !ok || owner == src.Name || slices.Contains(exporters[name], owner) || slices.Contains(definers[name], src.Name) {
					continue
				}
				problems = append(problems, fmt.Sprintf("facet %q in blueprint %q reads config block %q, which blueprint %q owns. Add %q to exports: in the blueprint.yaml of %q to share it.", facet.Metadata.Name, labels[src.Name], name, labels[owner], name, labels[owner]))
			}
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return werror.New("facets use config blocks of other blueprints:\n  "+strings.Join(problems, "\n  "), sourceScopeHint, nil)
}

// sourceLabel returns the name that scope errors show for src: the metadata name of its blueprint, or
// the source name when the blueprint has none. The local template source is "template", but its
// blueprint has the name that its author knows.
func sourceLabel(src SourceFacetSet) string {
	if src.Target != nil && src.Target.Metadata.Name != "" {
		return src.Target.Metadata.Name
	}
	return src.Name
}

// facetWrittenBlocks returns the sorted, unique names of the config blocks that facet defines.
func facetWrittenBlocks(facet blueprintv1alpha1.Facet) []string {
	seen := make(map[string]bool)
	for _, block := range facet.Config {
		if block.Name != "" {
			seen[block.Name] = true
		}
	}
	return sortedKeys(seen)
}

// facetReadHeads returns the sorted, unique first segments of the scope references in facet. It reads
// every when: as an expression, every requires paths: entry as a scope path, and every other string
// for ${...} expressions. It returns an error when the facet does not convert to YAML.
func facetReadHeads(facet blueprintv1alpha1.Facet) ([]string, error) {
	heads := make(map[string]bool)
	for _, block := range facet.Config {
		collectScopeHeads(block.When, "when", "", heads)
		for _, req := range block.Requires {
			collectRequirementHeads(req, heads)
		}
		if block.Body != nil {
			collectScopeHeads(block.Body["value"], "value", "", heads)
		}
	}
	rest := facet
	rest.Config = nil
	data, err := yaml.Marshal(rest)
	if err != nil {
		return nil, werror.Wrap(err, "read the scope references")
	}
	var tree any
	if err := yaml.Unmarshal(data, &tree); err != nil {
		return nil, werror.Wrap(err, "read the scope references")
	}
	collectScopeHeads(tree, "", "", heads)
	return sortedKeys(heads), nil
}

// collectRequirementHeads adds the first segments of the scope references of req to heads.
func collectRequirementHeads(req blueprintv1alpha1.RequirementBlock, heads map[string]bool) {
	collectScopeHeads(req.When, "when", "", heads)
	for _, path := range req.Paths {
		collectScopeHeads(path, "paths", "requires", heads)
	}
}

// collectScopeHeads walks value and adds the first segment of each scope reference to heads. key is
// the map key that holds value, and parent is the key of the map that holds key. A string under when
// is an expression. A string under paths in a requires entry is a scope path.
func collectScopeHeads(value any, key, parent string, heads map[string]bool) {
	switch v := value.(type) {
	case string:
		var refs []string
		switch {
		case key == "paths" && parent == "requires":
			refs = []string{v}
		case key == "when" && !evaluator.ContainsExpression(v):
			refs = rawExprRefs(v)
		default:
			refs = extractExprASTRefs(v)
		}
		for _, ref := range refs {
			head, _, _ := strings.Cut(ref, ".")
			if head != "" {
				heads[head] = true
			}
		}
	case map[string]any:
		for k, child := range v {
			collectScopeHeads(child, k, key, heads)
		}
	case []any:
		for _, child := range v {
			collectScopeHeads(child, key, parent, heads)
		}
	}
}

// rawExprRefs returns the scope references of s, an expression without ${...}. It returns nil when s
// does not parse.
func rawExprRefs(s string) []string {
	tree, err := parser.Parse(s)
	if err != nil {
		return nil
	}
	var refs []string
	walkASTForRefs(tree.Node, &refs, make(map[string]bool))
	return refs
}

// quoteJoin returns the label of each source name, quoted and joined with commas, with "and"
// before the last one.
func quoteJoin(names []string, labels map[string]string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = fmt.Sprintf("%q", labels[n])
	}
	if len(quoted) < 2 {
		return strings.Join(quoted, "")
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " and " + quoted[len(quoted)-1]
}
