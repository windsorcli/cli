// The YAMLNodeResolver provides YAML AST utilities for resolving line numbers from facet files.
// It navigates goccy/go-yaml parsed trees to locate specific nodes by structural path
// (map key, named sequence item, sequence index, or entry value match), enabling precise
// provenance line number resolution during blueprint composition and explain.
package blueprint

import (
	"strings"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// =============================================================================
// Types
// =============================================================================

// namedItem is a path segment type that matches a sequence item (mapping) whose "name" key
// equals the given string value.
type namedItem string

// mapKeyLine is a terminal path segment that returns the line of the KEY node (not the value)
// for a mapping entry. Use instead of a plain string when the target is a block value whose
// value node starts on the next line.
type mapKeyLine string

// seqEntryMatch is a terminal path segment that searches a sequence for an entry matching
// the given value (literal or expression-derived).
type seqEntryMatch struct {
	value string
}

// =============================================================================
// Helpers
// =============================================================================

// yamlComponentEntryLine returns the 1-based line number of the specific component list entry
// (value) within the "components" sequence. When multiple blocks share the same name (e.g.
// merge strategy), scans all such blocks and returns the line from the block whose components
// list actually contains the value, so the reported line is the list entry, not the block key.
func yamlComponentEntryLine(filePath, componentType, componentName, value string) int {
	if filePath == "" || componentType == "" || componentName == "" || value == "" {
		return 0
	}
	f, err := parser.ParseFile(filePath, 0)
	if err != nil || len(f.Docs) == 0 {
		return 0
	}
	node := f.Docs[0].Body
	node = astMapValue(node, componentType)
	if node == nil {
		return 0
	}
	for _, block := range astAllNamedItems(node, componentName) {
		componentsNode := astMapValue(block, "components")
		if componentsNode == nil {
			continue
		}
		if line := astSeqEntryMatch(componentsNode, seqEntryMatch{value: value}); line > 0 {
			return line
		}
	}
	return 0
}

// yamlNodeLine parses a facet YAML file into an AST and navigates to the node identified by
// the given path segments. String segments select mapping keys; namedItem segments match
// sequence entries that are mappings with a "name" key equal to the given value; mapKeyLine
// returns the line of a mapping key (terminal); seqEntryMatch is terminal. Returns the
// 1-based line number of the target node, or 0 if the path cannot be resolved.
func yamlNodeLine(filePath string, segments ...interface{}) int {
	if filePath == "" || len(segments) == 0 {
		return 0
	}
	f, err := parser.ParseFile(filePath, 0)
	if err != nil || len(f.Docs) == 0 {
		return 0
	}
	node := f.Docs[0].Body
	for _, seg := range segments {
		if node == nil {
			return 0
		}
		switch s := seg.(type) {
		case string:
			node = astMapValue(node, s)
		case namedItem:
			node = astNamedItem(node, string(s))
		case mapKeyLine:
			return astMapKeyLine(node, string(s))
		case seqEntryMatch:
			n := astSeqEntryMatch(node, s)
			if n > 0 {
				return n
			}
			return 0
		default:
			return 0
		}
	}
	if node == nil {
		return 0
	}
	tk := node.GetToken()
	if tk == nil {
		return 0
	}
	return tk.Position.Line
}

// astMapValue extracts the value node for a given key from a mapping node.
func astMapValue(node ast.Node, key string) ast.Node {
	m, ok := node.(*ast.MappingNode)
	if !ok {
		return nil
	}
	for _, mv := range m.Values {
		if mv.Key != nil && mv.Key.String() == key {
			return mv.Value
		}
	}
	return nil
}

// astMapKeyLine returns the line of the KEY node (not the value) for a mapping entry. Used as
// a terminal segment so that block values (whose value node starts on the next line) report
// the key's line instead.
func astMapKeyLine(node ast.Node, key string) int {
	m, ok := node.(*ast.MappingNode)
	if !ok {
		return 0
	}
	for _, mv := range m.Values {
		if mv.Key != nil && mv.Key.String() == key {
			tk := mv.Key.GetToken()
			if tk != nil {
				return tk.Position.Line
			}
		}
	}
	return 0
}

// astNamedItem finds a sequence item (mapping) whose "name" key equals the given value.
// Returns the mapping node of the matched item (first match only).
func astNamedItem(node ast.Node, name string) ast.Node {
	seq, ok := node.(*ast.SequenceNode)
	if !ok {
		return nil
	}
	for _, item := range seq.Values {
		m, ok := item.(*ast.MappingNode)
		if !ok {
			continue
		}
		for _, mv := range m.Values {
			if mv.Key != nil && mv.Key.String() == "name" && mv.Value != nil && mv.Value.String() == name {
				return m
			}
		}
	}
	return nil
}

// astAllNamedItems returns all sequence items (mappings) whose "name" key equals the given value.
// Used when multiple blocks share the same name (e.g. merge strategy) so we can find the
// block that actually contains a specific component list entry.
func astAllNamedItems(node ast.Node, name string) []ast.Node {
	seq, ok := node.(*ast.SequenceNode)
	if !ok {
		return nil
	}
	var out []ast.Node
	for _, item := range seq.Values {
		m, ok := item.(*ast.MappingNode)
		if !ok {
			continue
		}
		for _, mv := range m.Values {
			if mv.Key != nil && mv.Key.String() == "name" && mv.Value != nil && mv.Value.String() == name {
				out = append(out, m)
				break
			}
		}
	}
	return out
}

// astSeqEntryMatch finds the line of a sequence entry whose string value matches a resolved
// component value. Handles literal matches and expression-based matches where the resolved
// value appears as a string literal within a ${...} expression.
func astSeqEntryMatch(node ast.Node, sem seqEntryMatch) int {
	seq, ok := node.(*ast.SequenceNode)
	if !ok {
		return 0
	}
	for _, item := range seq.Values {
		raw := strings.Trim(item.String(), "\"'")
		if raw == sem.value {
			tk := item.GetToken()
			if tk != nil {
				return tk.Position.Line
			}
		}
	}
	for _, item := range seq.Values {
		raw := item.String()
		trimmed := strings.Trim(raw, "\"'")
		if strings.HasPrefix(trimmed, "${") && matchesExpressionEntry(raw, sem.value) {
			tk := item.GetToken()
			if tk != nil {
				return tk.Position.Line
			}
		}
	}
	return 0
}

// matchesExpressionEntry checks whether a resolved component value could have been produced by
// a raw expression. It extracts single-quoted string literals from the expression and returns
// true if the entry exactly matches a literal or if the entry starts with a literal that is a
// path prefix (ends with '/') and at least 4 characters (handles 'prefix/' + variable).
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
		if len(literal) >= 4 && strings.HasSuffix(literal, "/") && strings.HasPrefix(entry, literal) {
			return true
		}
	}
	return false
}
