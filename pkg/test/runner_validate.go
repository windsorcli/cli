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

// validateBlueprint performs comprehensive structural validation on a composed blueprint to ensure
// it meets integrity requirements. It checks for duplicate Terraform components (by ID), duplicate
// Kustomizations (by name), duplicate components within Kustomization component lists, circular
// dependencies in both Terraform and Kustomization dependency graphs, and invalid dependencies
// (components that reference non-existent dependencies). These validations catch composition errors
// and blueprint configuration mistakes early. Returns a slice of descriptive error messages for
// each validation failure found, or an empty slice if the blueprint is valid.
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

// validateCircularDependencies checks for circular dependency chains in both Terraform components
// and Kustomizations. It builds dependency graphs for each component type and uses depth-first search
// to detect cycles. Circular dependencies would cause infinite loops or undefined ordering during
// blueprint application, so they must be caught and reported. The Kustomization graph covers both
// plain kustomize: entries and the tiers compiled from flux: systems, since dependsOn edges can
// target or originate from either. The function validates Terraform components separately from
// Kustomizations, as they have independent dependency graphs. Returns a slice of error messages
// describing each circular dependency found, including the full cycle path.
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

// validateInvalidDependencies checks that all components referenced in DependsOn fields actually
// exist in the composed blueprint. It builds sets of valid Terraform component IDs and Kustomization
// names, then validates that every dependency reference points to an existing component. After blueprint
// composition completes, invalid dependencies should have been filtered out by the composer's validation
// logic, so any remaining invalid dependencies indicate a bug in the composition or validation code.
// This check serves as a safety net to catch composition errors. Returns a slice of error messages
// describing each invalid dependency found, identifying both the component with the invalid dependency
// and the non-existent component it references.
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

// detectCycles performs a depth-first search to detect cycles in a dependency graph represented
// as a map from node names to their dependency lists. It uses a recursion stack to track the current
// path and identify back edges that indicate cycles. The validNodes set filters out references to
// non-existent nodes (which are handled by validateInvalidDependencies). When a cycle is detected,
// it constructs a readable cycle path showing the circular dependency chain. The nodeType parameter
// is used in error messages to indicate whether the cycle is in Terraform components or Kustomizations.
// Returns a slice of error messages, one for each cycle found.
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
