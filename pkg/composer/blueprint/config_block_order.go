// The config_block_order file orders config blocks for evaluation by their inter-block
// reference dependencies. The order built up by mergeFacetScopeIntoGlobal reflects facet
// processing order, which does not necessarily match evaluation order: a block that
// references another block must be evaluated after the block it references, regardless
// of which facet wrote which first. Topological sort of the block dependency graph
// produces an order where every block evaluates after the blocks it depends on.
package blueprint

import (
	"fmt"
	"sort"
	"strings"
)

// =============================================================================
// Helpers
// =============================================================================

// collectBlockRefs walks value (string, map, or slice) and returns the set of block names
// referenced via ${...} expressions. blockNames is the set of valid block names; references
// whose first dotted segment does not match a known block (e.g. context values like
// cluster.driver) are ignored. self-references (a block referencing itself) are also
// dropped — same-block iteration handles those.
func collectBlockRefs(value any, blockNames map[string]bool, selfName string) []string {
	seen := make(map[string]bool)
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			for _, ref := range extractExprASTRefs(x) {
				head := ref
				if dot := strings.Index(ref, "."); dot >= 0 {
					head = ref[:dot]
				}
				if head == selfName {
					continue
				}
				if !blockNames[head] {
					continue
				}
				seen[head] = true
			}
		case map[string]any:
			for _, sub := range x {
				walk(sub)
			}
		case []any:
			for _, sub := range x {
				walk(sub)
			}
		}
	}
	walk(value)
	result := make([]string, 0, len(seen))
	for r := range seen {
		result = append(result, r)
	}
	sort.Strings(result)
	return result
}

// topoSortConfigBlocks returns block names sorted so that every block appears after the
// blocks it references. globalScope is the merged config scope (block name -> body). The
// fallback order is consulted only as a stability hint when two blocks have no relative
// dependency: alphabetical tiebreak by block name. Returns an error naming the cycle when
// the dependency graph is not acyclic.
func topoSortConfigBlocks(globalScope map[string]any) ([]string, error) {
	if len(globalScope) == 0 {
		return nil, nil
	}
	blockNames := make(map[string]bool, len(globalScope))
	blocks := make([]string, 0, len(globalScope))
	for name := range globalScope {
		blockNames[name] = true
		blocks = append(blocks, name)
	}
	sort.Strings(blocks)

	// edges[prereq] = list of dependents; inDeg[name] = number of unmet prereqs.
	edges := make(map[string][]string, len(blocks))
	inDeg := make(map[string]int, len(blocks))
	for _, b := range blocks {
		inDeg[b] = 0
	}
	for _, name := range blocks {
		for _, prereq := range collectBlockRefs(globalScope[name], blockNames, name) {
			edges[prereq] = append(edges[prereq], name)
			inDeg[name]++
		}
	}

	// Kahn's algorithm. A sorted ready queue keeps the output deterministic when two
	// blocks are independent (alphabetical tiebreak).
	ready := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if inDeg[b] == 0 {
			ready = append(ready, b)
		}
	}
	sort.Strings(ready)

	result := make([]string, 0, len(blocks))
	for len(ready) > 0 {
		n := ready[0]
		ready = ready[1:]
		result = append(result, n)
		dependents := edges[n]
		sort.Strings(dependents)
		for _, d := range dependents {
			inDeg[d]--
			if inDeg[d] == 0 {
				// Insert into ready in sorted position.
				pos := sort.SearchStrings(ready, d)
				ready = append(ready, "")
				copy(ready[pos+1:], ready[pos:])
				ready[pos] = d
			}
		}
	}

	if len(result) != len(blocks) {
		remaining := make([]string, 0, len(blocks)-len(result))
		for _, b := range blocks {
			if inDeg[b] > 0 {
				remaining = append(remaining, b)
			}
		}
		return nil, fmt.Errorf("circular reference among config blocks: %s", strings.Join(remaining, ", "))
	}

	return result, nil
}
