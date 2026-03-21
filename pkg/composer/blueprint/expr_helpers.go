// The expr_helpers file provides shared expression AST utilities for the blueprint package.
// It provides parsing and walking of expr-lang expressions (${...}), dotted path extraction,
// and detection of derived-from-block references used for config merge semantics.
package blueprint

import (
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Helpers
// =============================================================================

// EvaluateWithOrigins evaluates a single input value using per-sub-key origin paths
// stored in origins (dot-separated keys such as "config.db.host"). When a direct origin
// exists for keyPath the entire value is evaluated against that path. When sub-key origins
// exist the value is walked recursively so each leaf resolves against its originating facet.
func EvaluateWithOrigins(eval evaluator.ExpressionEvaluator, keyPath string, value any, origins map[string]string, scope map[string]any, evaluateDeferred bool) (any, error) {
	leafKey := keyPath
	if idx := strings.LastIndex(keyPath, "."); idx >= 0 {
		leafKey = keyPath[idx+1:]
	}

	if origins != nil {
		if origin, ok := origins[keyPath]; ok {
			result, err := eval.EvaluateMap(map[string]any{leafKey: value}, origin, scope, evaluateDeferred)
			if err != nil {
				return nil, err
			}
			return result[leafKey], nil
		}

		if m, ok := value.(map[string]any); ok {
			prefix := keyPath + "."
			for k := range origins {
				if strings.HasPrefix(k, prefix) {
					result := make(map[string]any, len(m))
					for subKey, subValue := range m {
						evaluated, err := EvaluateWithOrigins(eval, keyPath+"."+subKey, subValue, origins, scope, evaluateDeferred)
						if err != nil {
							return nil, err
						}
						result[subKey] = evaluated
					}
					return result, nil
				}
			}
		}
	}

	result, err := eval.EvaluateMap(map[string]any{leafKey: value}, "", scope, evaluateDeferred)
	if err != nil {
		return nil, err
	}
	return result[leafKey], nil
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

// expressionIsDerivedFromBlock returns true when a ${...} expression transforms a reference to
// blockName by passing that reference as a function argument (e.g. string(talos_common.patch)).
// Simple reads (e.g. ${workstation.runtime}) are not treated as derived.
func expressionIsDerivedFromBlock(s, blockName string) bool {
	if blockName == "" {
		return false
	}
	searchStart := 0
	for {
		idx := strings.Index(s[searchStart:], "${")
		if idx == -1 {
			return false
		}
		start := searchStart + idx
		end := findExprEnd(s, start)
		if end == -1 {
			return false
		}
		exprContent := s[start+2 : end]
		tree, err := parser.Parse(exprContent)
		if err == nil && hasDerivedBlockRefInCall(tree.Node, blockName, false) {
			return true
		}
		searchStart = end + 1
	}
}

// hasDerivedBlockRefInCall recursively walks an expr AST and reports whether blockName
// (or a member path starting with blockName.) appears in a "derived" context: as an
// argument to a function or builtin call. inCall is true when the current node is
// inside the arguments of an ast.CallNode or ast.BuiltinNode; only when inCall is
// true does a matching IdentifierNode or MemberNode cause a true return. This
// distinguishes expressions that transform a block value (e.g. string(block_a.x))
// from simple reads (e.g. block_a.runtime) for derived-config handling.
func hasDerivedBlockRefInCall(node ast.Node, blockName string, inCall bool) bool {
	if node == nil {
		return false
	}
	switch n := node.(type) {
	case *ast.MemberNode:
		path := buildMemberPath(n)
		if path != "" && inCall && (path == blockName || strings.HasPrefix(path, blockName+".")) {
			return true
		}
		if path != "" {
			return false
		}
		return hasDerivedBlockRefInCall(n.Node, blockName, inCall) ||
			hasDerivedBlockRefInCall(n.Property, blockName, inCall)
	case *ast.IdentifierNode:
		return inCall && n.Value == blockName
	case *ast.CallNode:
		for _, arg := range n.Arguments {
			if hasDerivedBlockRefInCall(arg, blockName, true) {
				return true
			}
		}
		return false
	case *ast.BuiltinNode:
		for _, arg := range n.Arguments {
			if hasDerivedBlockRefInCall(arg, blockName, true) {
				return true
			}
		}
		return false
	case *ast.ChainNode:
		return hasDerivedBlockRefInCall(n.Node, blockName, inCall)
	case *ast.BinaryNode:
		return hasDerivedBlockRefInCall(n.Left, blockName, inCall) ||
			hasDerivedBlockRefInCall(n.Right, blockName, inCall)
	case *ast.UnaryNode:
		return hasDerivedBlockRefInCall(n.Node, blockName, inCall)
	case *ast.ConditionalNode:
		return hasDerivedBlockRefInCall(n.Cond, blockName, inCall) ||
			hasDerivedBlockRefInCall(n.Exp1, blockName, inCall) ||
			hasDerivedBlockRefInCall(n.Exp2, blockName, inCall)
	case *ast.ArrayNode:
		for _, elem := range n.Nodes {
			if hasDerivedBlockRefInCall(elem, blockName, inCall) {
				return true
			}
		}
		return false
	case *ast.MapNode:
		for _, pair := range n.Pairs {
			p, ok := pair.(*ast.PairNode)
			if !ok {
				continue
			}
			if hasDerivedBlockRefInCall(p.Value, blockName, inCall) {
				return true
			}
		}
		return false
	case *ast.PairNode:
		return hasDerivedBlockRefInCall(n.Value, blockName, inCall)
	case *ast.SliceNode:
		return hasDerivedBlockRefInCall(n.Node, blockName, inCall) ||
			hasDerivedBlockRefInCall(n.From, blockName, inCall) ||
			hasDerivedBlockRefInCall(n.To, blockName, inCall)
	case *ast.PredicateNode:
		return hasDerivedBlockRefInCall(n.Node, blockName, inCall)
	case *ast.SequenceNode:
		for _, child := range n.Nodes {
			if hasDerivedBlockRefInCall(child, blockName, inCall) {
				return true
			}
		}
		return false
	case *ast.VariableDeclaratorNode:
		return hasDerivedBlockRefInCall(n.Value, blockName, inCall) ||
			hasDerivedBlockRefInCall(n.Expr, blockName, inCall)
	default:
		return false
	}
}
