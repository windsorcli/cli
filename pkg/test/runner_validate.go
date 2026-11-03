// The validate functions of the TestRunner check that a composed blueprint is valid.
// They find duplicate terraform components, kustomizations, and kustomization components.
// They also find dependencies that do not exist and circular dependencies.

package test

import (
	"fmt"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
)

// =============================================================================
// Private Methods
// =============================================================================

// validateBlueprint checks the structure of a composed blueprint and returns one message per failure.
// It finds duplicate terraform component IDs, duplicate kustomization names, and duplicate components
// in a kustomization. It also finds circular dependencies and dependencies on components that do not exist.
// An empty slice means the blueprint is valid.
func (r *TestRunner) validateBlueprint(bp *blueprintv1alpha1.Blueprint) []string {
	var errs []string

	errs = append(errs, r.validateDuplicateTerraformComponents(bp)...)
	errs = append(errs, r.validateDuplicateKustomizations(bp)...)
	errs = append(errs, r.validateDuplicateKustomizationComponents(bp)...)
	errs = append(errs, r.validateCircularDependencies(bp)...)
	errs = append(errs, r.validateInvalidDependencies(bp)...)

	return errs
}

// validateDuplicateTerraformComponents checks for duplicate Terraform components in the blueprint by
// comparing component IDs (which are derived from Name or Path via GetID()). It maintains a set of
// seen component IDs and reports an error for each duplicate encountered. Duplicate components indicate
// a composition error where multiple facets or configurations contributed the same component without
// proper merging or replacement. Returns a slice of error messages, one for each duplicate found.
func (r *TestRunner) validateDuplicateTerraformComponents(bp *blueprintv1alpha1.Blueprint) []string {
	var errs []string
	ids := make(map[string]struct{})
	for _, tf := range bp.TerraformComponents {
		id := tf.GetID()
		if _, exists := ids[id]; exists {
			errs = append(errs, fmt.Sprintf("duplicate terraform component ID: %s", id))
		}
		ids[id] = struct{}{}
	}
	return errs
}

// validateDuplicateKustomizations checks for duplicate Kustomizations in the blueprint by comparing
// kustomization names across both plain kustomize: entries and the tiers compiled from flux: systems.
// It maintains a set of seen names and reports an error for each duplicate encountered. Duplicate
// kustomizations indicate a composition error where multiple facets contributed the same kustomization
// without proper merging or replacement. Returns a slice of error messages, one for each duplicate found.
func (r *TestRunner) validateDuplicateKustomizations(bp *blueprintv1alpha1.Blueprint) []string {
	var errs []string
	names := make(map[string]struct{})
	for _, k := range bp.AllKustomizations() {
		if _, exists := names[k.Name]; exists {
			errs = append(errs, fmt.Sprintf("duplicate kustomization name: %s", k.Name))
		}
		names[k.Name] = struct{}{}
	}
	return errs
}

// validateDuplicateKustomizationComponents checks for duplicate component references within each
// Kustomization's Components list, across both plain kustomize: entries and the tiers compiled from
// flux: systems. Empty strings are placeholders (e.g. conditional slot with no component) and may
// appear multiple times; only non-empty duplicate component names are reported. Returns a slice of
// error messages, one for each duplicate found.
func (r *TestRunner) validateDuplicateKustomizationComponents(bp *blueprintv1alpha1.Blueprint) []string {
	var errs []string
	for _, k := range bp.AllKustomizations() {
		components := make(map[string]struct{})
		for _, comp := range k.Components {
			if comp == "" {
				continue
			}
			if _, exists := components[comp]; exists {
				errs = append(errs, fmt.Sprintf("duplicate component %q in kustomization %q", comp, k.Name))
			}
			components[comp] = struct{}{}
		}
	}
	return errs
}

// validateCircularDependencies returns one message per dependency cycle, with the full cycle path.
// It checks terraform components and kustomizations as separate graphs. The kustomization graph
// includes plain kustomize: entries and the tiers compiled from flux: systems.
func (r *TestRunner) validateCircularDependencies(bp *blueprintv1alpha1.Blueprint) []string {
	var errs []string

	tfGraph := make(map[string][]string)
	tfIDs := make(map[string]struct{})
	for _, tf := range bp.TerraformComponents {
		id := tf.GetID()
		tfIDs[id] = struct{}{}
		tfGraph[id] = tf.DependsOn
	}
	errs = append(errs, detectCycles(tfGraph, tfIDs, "terraform component")...)

	kGraph := make(map[string][]string)
	kNames := make(map[string]struct{})
	for _, k := range bp.AllKustomizations() {
		kNames[k.Name] = struct{}{}
		kGraph[k.Name] = k.DependsOn
	}
	errs = append(errs, detectCycles(kGraph, kNames, "kustomization")...)

	return errs
}

// validateInvalidDependencies returns one message per DependsOn entry that names a missing component.
// Each message names the component and the missing dependency. The composer removes invalid
// dependencies, so a message here shows a bug in composition.
func (r *TestRunner) validateInvalidDependencies(bp *blueprintv1alpha1.Blueprint) []string {
	var errs []string

	tfIDs := make(map[string]struct{})
	for _, tf := range bp.TerraformComponents {
		tfIDs[tf.GetID()] = struct{}{}
	}

	allK := bp.AllKustomizations()
	kNames := make(map[string]struct{}, len(allK))
	for _, k := range allK {
		kNames[k.Name] = struct{}{}
	}
	for _, layer := range blueprint.CrdLayers(bp) {
		kNames[blueprintv1alpha1.CrdKustomizationName(layer.Source)] = struct{}{}
	}

	for _, tf := range bp.TerraformComponents {
		for _, dep := range tf.DependsOn {
			if _, exists := tfIDs[dep]; !exists {
				errs = append(errs, fmt.Sprintf("terraform component %q depends on non-existent component %q", tf.GetID(), dep))
			}
		}
	}

	for _, k := range allK {
		for _, dep := range k.DependsOn {
			if _, exists := kNames[dep]; !exists {
				errs = append(errs, fmt.Sprintf("kustomization %q depends on non-existent kustomization %q", k.Name, dep))
			}
		}
	}

	return errs
}

// =============================================================================
// Helpers
// =============================================================================

// detectCycles uses a depth-first search to find the cycles in graph and returns one message per cycle.
// It ignores a reference to a node that is not in validNodes. Each message shows the cycle path and
// uses nodeType to name the graph.
func detectCycles(graph map[string][]string, validNodes map[string]struct{}, nodeType string) []string {
	var errs []string
	visited := make(map[string]bool)
	recursionStack := make(map[string]bool)

	var dfs func(node string, path []string)
	dfs = func(node string, path []string) {
		visited[node] = true
		recursionStack[node] = true
		path = append(path, node)

		for _, neighbor := range graph[node] {
			if _, exists := validNodes[neighbor]; !exists {
				continue
			}
			if recursionStack[neighbor] {
				cyclePath := []string{}
				found := false
				for _, pNode := range path {
					if pNode == neighbor {
						found = true
					}
					if found {
						cyclePath = append(cyclePath, pNode)
					}
				}
				cyclePath = append(cyclePath, neighbor)
				errs = append(errs, fmt.Sprintf("circular dependency detected in %s: %s", nodeType, strings.Join(cyclePath, " -> ")))
				continue
			}
			if !visited[neighbor] {
				dfs(neighbor, path)
			}
		}
		recursionStack[node] = false
	}

	for node := range graph {
		if !visited[node] {
			dfs(node, []string{})
		}
	}
	return errs
}
