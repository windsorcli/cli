// The sort functions of Blueprint order kustomizations and terraform components by their dependencies.
// They return an error when the dependencies contain a cycle.

package v1alpha1

import (
	"fmt"
	"slices"
	"strings"
)

// =============================================================================
// Private Methods
// =============================================================================

// sortKustomize reorders the Blueprint's Kustomizations so that dependencies precede dependents.
// It first applies a topological sort to ensure dependency order, then groups kustomizations with similar name prefixes adjacently.
// Returns an error if a dependency cycle is detected.
func (b *Blueprint) sortKustomize() error {
	if len(b.Kustomizations) <= 1 {
		return nil
	}
	nameToIndex := make(map[string]int)
	for i, k := range b.Kustomizations {
		nameToIndex[k.Name] = i
	}
	sorted := b.basicTopologicalSort(nameToIndex)
	if sorted == nil {
		return fmt.Errorf("dependency cycle detected in kustomizations")
	}
	sorted = b.groupSimilarPrefixes(sorted, nameToIndex)
	newKustomizations := make([]Kustomization, len(b.Kustomizations))
	for i, sortedIndex := range sorted {
		newKustomizations[i] = b.Kustomizations[sortedIndex]
	}
	b.Kustomizations = newKustomizations
	return nil
}

// basicTopologicalSort computes a topological ordering of kustomizations based on dependencies.
// Returns a slice of indices into the Kustomizations slice, ordered so dependencies precede dependents.
// Returns nil if a cycle is detected in the dependency graph.
func (b *Blueprint) basicTopologicalSort(nameToIndex map[string]int) []int {
	var sorted []int
	visited := make(map[int]bool)
	visiting := make(map[int]bool)

	var visit func(int) error
	visit = func(componentIndex int) error {
		if visiting[componentIndex] {
			return fmt.Errorf("cycle detected in dependency graph involving kustomization '%s'", b.Kustomizations[componentIndex].Name)
		}
		if visited[componentIndex] {
			return nil
		}

		visiting[componentIndex] = true
		for _, depName := range b.Kustomizations[componentIndex].DependsOn {
			if depIndex, exists := nameToIndex[depName]; exists {
				if err := visit(depIndex); err != nil {
					visiting[componentIndex] = false
					return err
				}
			}
		}
		visiting[componentIndex] = false
		visited[componentIndex] = true
		sorted = append(sorted, componentIndex)
		return nil
	}

	for i := range b.Kustomizations {
		if !visited[i] {
			if err := visit(i); err != nil {
				fmt.Printf("Error: %v\n", err)
				return nil
			}
		}
	}
	return sorted
}

// groupSimilarPrefixes returns a new ordering of kustomization indices so components with similar
// name prefixes are grouped. It groups kustomizations by the prefix before the first hyphen in
// their name, then processes each group in the order they appear in the input slice. For groups
// with multiple components, it sorts by dependency depth (shallowest first), then by name if
// depths are equal. The resulting slice preserves dependency order and groups related
// kustomizations adjacently.
func (b *Blueprint) groupSimilarPrefixes(sorted []int, nameToIndex map[string]int) []int {
	prefixGroups := make(map[string][]int)
	for _, idx := range sorted {
		prefix := strings.Split(b.Kustomizations[idx].Name, "-")[0]
		prefixGroups[prefix] = append(prefixGroups[prefix], idx)
	}

	var newSorted []int
	processed := make(map[int]bool)

	for _, originalIdx := range sorted {
		if processed[originalIdx] {
			continue
		}

		prefix := strings.Split(b.Kustomizations[originalIdx].Name, "-")[0]
		group := prefixGroups[prefix]

		if len(group) == 1 {
			newSorted = append(newSorted, group[0])
			processed[group[0]] = true
		} else {
			slices.SortFunc(group, func(i, j int) int {
				depthI := b.calculateDependencyDepth(i, nameToIndex)
				depthJ := b.calculateDependencyDepth(j, nameToIndex)
				if depthI != depthJ {
					return depthI - depthJ
				}
				return strings.Compare(b.Kustomizations[i].Name, b.Kustomizations[j].Name)
			})

			for _, idx := range group {
				if !processed[idx] {
					newSorted = append(newSorted, idx)
					processed[idx] = true
				}
			}
		}
	}

	return newSorted
}

// calculateDependencyDepth returns the maximum dependency depth for the specified kustomization index.
// It recursively traverses the dependency graph using the provided name-to-index mapping, computing
// the longest path from the given component to any leaf dependency. A component with no dependencies
// has depth 0. Cycles are not detected and may cause stack overflow.
func (b *Blueprint) calculateDependencyDepth(componentIndex int, nameToIndex map[string]int) int {
	k := b.Kustomizations[componentIndex]
	if len(k.DependsOn) == 0 {
		return 0
	}

	maxDepth := 0
	for _, depName := range k.DependsOn {
		if depIndex, exists := nameToIndex[depName]; exists {
			depth := b.calculateDependencyDepth(depIndex, nameToIndex)
			if depth+1 > maxDepth {
				maxDepth = depth + 1
			}
		}
	}
	return maxDepth
}

// sortTerraform reorders the Blueprint's TerraformComponents so that dependencies precede dependents.
// It applies a topological sort to ensure dependency order. Components without dependencies come first.
// Returns an error if a dependency cycle is detected.
func (b *Blueprint) sortTerraform() error {
	if len(b.TerraformComponents) <= 1 {
		return nil
	}

	idToIndex := make(map[string]int)
	for i, component := range b.TerraformComponents {
		idToIndex[component.GetID()] = i
	}

	sorted := b.terraformTopologicalSort(idToIndex)
	if sorted == nil {
		return fmt.Errorf("dependency cycle detected in terraform components")
	}

	newComponents := make([]TerraformComponent, len(b.TerraformComponents))
	for i, sortedIndex := range sorted {
		newComponents[i] = b.TerraformComponents[sortedIndex]
	}
	b.TerraformComponents = newComponents
	return nil
}

// terraformTopologicalSort returns a topological ordering of Terraform components based on their dependencies.
// It receives a mapping from component IDs to their respective indices within the TerraformComponents slice,
// and produces a slice of indices such that all dependencies come before any component that depends on them.
// If a cycle is detected in the dependency graph, the function returns nil. The returned slice is ordered
// with respect to the topological sort, ensuring that all dependencies precede their dependents.
func (b *Blueprint) terraformTopologicalSort(idToIndex map[string]int) []int {
	var sorted []int
	visited := make(map[int]bool)
	visiting := make(map[int]bool)

	var visit func(int) error
	visit = func(componentIndex int) error {
		if visiting[componentIndex] {
			return fmt.Errorf("cycle detected in dependency graph involving terraform component '%s'", b.TerraformComponents[componentIndex].GetID())
		}
		if visited[componentIndex] {
			return nil
		}

		visiting[componentIndex] = true
		for _, depID := range b.TerraformComponents[componentIndex].DependsOn {
			if depIndex, exists := idToIndex[depID]; exists {
				if err := visit(depIndex); err != nil {
					visiting[componentIndex] = false
					return err
				}
			}
		}
		visiting[componentIndex] = false
		visited[componentIndex] = true
		sorted = append(sorted, componentIndex)
		return nil
	}

	for i := range b.TerraformComponents {
		if !visited[i] {
			if err := visit(i); err != nil {
				fmt.Printf("Error: %v\n", err)
				return nil
			}
		}
	}

	return sorted
}
