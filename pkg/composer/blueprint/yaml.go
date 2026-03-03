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

// seqEntryMatch is a terminal path segment that searches a sequence for an entry matching
// the given value (literal or expression-derived).
type seqEntryMatch struct {
	value string
}

// =============================================================================
// Helpers
// =============================================================================

// yamlNodeLine parses a facet YAML file into an AST and navigates to the node identified by
// the given path segments. String segments select mapping keys; namedItem segments match
// sequence entries that are mappings with a "name" key equal to the given value; int segments
// select sequence elements by index. Returns the 1-based line number of the target node, or 0
// if the path cannot be resolved.
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
		case int:
			node = astSeqIndex(node, s)
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

// astNamedItem finds a sequence item (mapping) whose "name" key equals the given value.
// Returns the mapping node of the matched item.
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

// astSeqIndex returns the node at a given index in a sequence node.
func astSeqIndex(node ast.Node, idx int) ast.Node {
	seq, ok := node.(*ast.SequenceNode)
	if !ok || idx < 0 || idx >= len(seq.Values) {
		return nil
	}
	return seq.Values[idx]
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
		if strings.HasPrefix(raw, "${") && matchesExpressionEntry(raw, sem.value) {
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
