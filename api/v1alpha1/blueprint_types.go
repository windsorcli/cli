// Package v1alpha1 contains types for the v1alpha1 API group
// +groupName=blueprints.windsorcli.dev
package v1alpha1

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/fluxcd/pkg/apis/kustomize"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Blueprint is a configuration blueprint for initializing a project.
type Blueprint struct {
	// Kind is the blueprint type, following Kubernetes conventions.
	Kind string `yaml:"kind"`

	// ApiVersion is the API schema version of the blueprint.
	ApiVersion string `yaml:"apiVersion"`

	// Metadata includes the blueprint's name and description.
	Metadata Metadata `yaml:"metadata"`

	// Repository details the source repository of the blueprint.
	Repository Repository `yaml:"repository"`

	// Sources are external resources referenced by the blueprint.
	Sources []Source `yaml:"sources"`

	// TerraformComponents are Terraform modules in the blueprint.
	TerraformComponents []TerraformComponent `yaml:"terraform"`

	// Kustomizations are kustomization configs in the blueprint.
	Kustomizations []Kustomization `yaml:"kustomize"`
}

type PartialBlueprint struct {
	Kind                string               `yaml:"kind"`
	ApiVersion          string               `yaml:"apiVersion"`
	Metadata            Metadata             `yaml:"metadata"`
	Sources             []Source             `yaml:"sources"`
	Repository          Repository           `yaml:"repository"`
	TerraformComponents []TerraformComponent `yaml:"terraform"`
	Kustomizations      []map[string]any     `yaml:"kustomize"`
}

// Metadata describes a blueprint.
type Metadata struct {
	// Name is the blueprint's unique identifier.
	Name string `yaml:"name"`

	// Description is a brief overview of the blueprint.
	Description string `yaml:"description,omitempty"`
}

// Repository contains source code repository info.
type Repository struct {
	// Url is the repository location.
	Url string `yaml:"url"`

	// Ref details the branch, tag, or commit to use.
	Ref Reference `yaml:"ref"`

	// SecretName is the secret for repository access.
	SecretName string `yaml:"secretName,omitempty"`
}

// Source is an external resource referenced by a blueprint.
type Source struct {
	// Name identifies the source.
	Name string `yaml:"name"`

	// Url is the source location.
	Url string `yaml:"url"`

	// PathPrefix is a prefix to the source path.
	PathPrefix string `yaml:"pathPrefix,omitempty"`

	// Ref details the branch, tag, or commit to use.
	Ref Reference `yaml:"ref,omitempty"`

	// SecretName is the secret for source access.
	SecretName string `yaml:"secretName,omitempty"`
}

// Reference details a specific version or state of a repository or source.
type Reference struct {
	// Branch to use.
	Branch string `yaml:"branch,omitempty"`

	// Tag to use.
	Tag string `yaml:"tag,omitempty"`

	// SemVer to use.
	SemVer string `yaml:"semver,omitempty"`

	// Name of the reference.
	Name string `yaml:"name,omitempty"`

	// Commit hash to use.
	Commit string `yaml:"commit,omitempty"`
}

// TerraformComponent defines a Terraform module in a blueprint.
type TerraformComponent struct {
	// Source of the Terraform module.
	Source string `yaml:"source,omitempty"`

	// Path of the Terraform module.
	Path string `yaml:"path"`

	// FullPath is the complete path, not serialized to YAML.
	FullPath string `yaml:"-"`

	// DependsOn lists dependencies of this terraform component.
	DependsOn []string `yaml:"dependsOn,omitempty"`

	// Values are configuration values for the module.
	Values map[string]any `yaml:"values,omitempty"`

	// Destroy determines if the component should be destroyed during down operations.
	// Defaults to true if not specified.
	Destroy *bool `yaml:"destroy,omitempty"`

	// Parallelism limits the number of concurrent operations as Terraform walks the graph.
	// This corresponds to the -parallelism flag in terraform apply/destroy commands.
	Parallelism *int `yaml:"parallelism,omitempty"`
}

// BlueprintPatch represents a patch in the blueprint format.
// This is converted to kustomize.Patch during processing.
// Supports both blueprint format (Path) and Flux format (Patch + Target).
type BlueprintPatch struct {
	// Path to the patch file relative to the kustomization (blueprint format).
	Path string `yaml:"path,omitempty"`

	// Patch content as YAML string (Flux format).
	Patch string `yaml:"patch,omitempty"`

	// Target selector for the patch (Flux format).
	Target *kustomize.Selector `yaml:"target,omitempty"`
}

// Kustomization represents a kustomization configuration.
type Kustomization struct {
	// Name of the kustomization.
	Name string `yaml:"name"`

	// Path of the kustomization.
	Path string `yaml:"path"`

	// Source of the kustomization.
	Source string `yaml:"source,omitempty"`

	// DependsOn lists dependencies of this kustomization.
	DependsOn []string `yaml:"dependsOn,omitempty"`

	// Interval for applying the kustomization.
	Interval *metav1.Duration `yaml:"interval,omitempty"`

	// RetryInterval before retrying a failed kustomization.
	RetryInterval *metav1.Duration `yaml:"retryInterval,omitempty"`

	// Timeout for the kustomization to complete.
	Timeout *metav1.Duration `yaml:"timeout,omitempty"`

	// Patches to apply to the kustomization.
	Patches []BlueprintPatch `yaml:"patches,omitempty"`

	// Wait for the kustomization to be fully applied.
	Wait *bool `yaml:"wait,omitempty"`

	// Force apply the kustomization.
	Force *bool `yaml:"force,omitempty"`

	// Prune enables garbage collection of resources that are no longer present in the source.
	Prune *bool `yaml:"prune,omitempty"`

	// Components to include in the kustomization.
	Components []string `yaml:"components,omitempty"`

	// Cleanup lists resources to clean up after the kustomization is applied.
	Cleanup []string `yaml:"cleanup,omitempty"`

	// PostBuild is a post-build step to run after the kustomization is applied.
	PostBuild *PostBuild `yaml:"postBuild,omitempty"`

	// Destroy determines if the kustomization should be destroyed during down operations.
	// Defaults to true if not specified.
	Destroy *bool `yaml:"destroy,omitempty"`
}

// PostBuild is a post-build step to run after the kustomization is applied.
type PostBuild struct {
	// Substitute is a map of resources to substitute from.
	Substitute map[string]string `yaml:"substitute,omitempty"`

	// SubstituteFrom is a list of resources to substitute from.
	SubstituteFrom []SubstituteReference `yaml:"substituteFrom,omitempty"`
}

// SubstituteReference is a reference to a resource to substitute from.
type SubstituteReference struct {
	// Kind of the resource to substitute from.
	Kind string `yaml:"kind"`

	// Name of the resource to substitute from.
	Name string `yaml:"name"`

	// Optional indicates if the resource is optional.
	Optional bool `yaml:"optional,omitempty"`
}

// DeepCopy creates a deep copy of the Blueprint object.
func (b *Blueprint) DeepCopy() *Blueprint {
	if b == nil {
		return nil
	}

	metadataCopy := Metadata{
		Name:        b.Metadata.Name,
		Description: b.Metadata.Description,
	}

	repositoryCopy := Repository{
		Url: b.Repository.Url,
		Ref: Reference{
			Commit: b.Repository.Ref.Commit,
			Name:   b.Repository.Ref.Name,
			SemVer: b.Repository.Ref.SemVer,
			Tag:    b.Repository.Ref.Tag,
			Branch: b.Repository.Ref.Branch,
		},
		SecretName: b.Repository.SecretName,
	}

	sourcesCopy := make([]Source, len(b.Sources))
	for i, source := range b.Sources {
		sourcesCopy[i] = Source{
			Name:       source.Name,
			Url:        source.Url,
			PathPrefix: source.PathPrefix,
			Ref: Reference{
				Branch: source.Ref.Branch,
				Tag:    source.Ref.Tag,
				SemVer: source.Ref.SemVer,
				Name:   source.Ref.Name,
				Commit: source.Ref.Commit,
			},
			SecretName: source.SecretName,
		}
	}

	terraformComponentsCopy := make([]TerraformComponent, len(b.TerraformComponents))
	for i, component := range b.TerraformComponents {
		valuesCopy := make(map[string]any, len(component.Values))
		maps.Copy(valuesCopy, component.Values)

		dependsOnCopy := append([]string{}, component.DependsOn...)

		terraformComponentsCopy[i] = TerraformComponent{
			Source:      component.Source,
			Path:        component.Path,
			FullPath:    component.FullPath,
			DependsOn:   dependsOnCopy,
			Values:      valuesCopy,
			Destroy:     component.Destroy,
			Parallelism: component.Parallelism,
		}
	}

	kustomizationsCopy := make([]Kustomization, len(b.Kustomizations))
	for i, kustomization := range b.Kustomizations {
		kustomizationsCopy[i] = *kustomization.DeepCopy()
	}

	return &Blueprint{
		Kind:                b.Kind,
		ApiVersion:          b.ApiVersion,
		Metadata:            metadataCopy,
		Repository:          repositoryCopy,
		Sources:             sourcesCopy,
		TerraformComponents: terraformComponentsCopy,
		Kustomizations:      kustomizationsCopy,
	}
}

// StrategicMerge performs a strategic merge of the provided overlay Blueprint into the receiver Blueprint.
// This method appends to array fields, deep merges map fields, and updates scalar fields if present in the overlay.
// It is designed for feature composition, enabling the combination of multiple features into a single blueprint.
func (b *Blueprint) StrategicMerge(overlay *Blueprint) error {
	if overlay == nil {
		return nil
	}

	if overlay.Kind != "" {
		b.Kind = overlay.Kind
	}
	if overlay.ApiVersion != "" {
		b.ApiVersion = overlay.ApiVersion
	}

	if overlay.Metadata.Name != "" {
		b.Metadata.Name = overlay.Metadata.Name
	}
	if overlay.Metadata.Description != "" {
		b.Metadata.Description = overlay.Metadata.Description
	}

	if overlay.Repository.Url != "" {
		b.Repository.Url = overlay.Repository.Url
	}

	if overlay.Repository.Ref.Commit != "" {
		b.Repository.Ref.Commit = overlay.Repository.Ref.Commit
	} else if overlay.Repository.Ref.Name != "" {
		b.Repository.Ref.Name = overlay.Repository.Ref.Name
	} else if overlay.Repository.Ref.SemVer != "" {
		b.Repository.Ref.SemVer = overlay.Repository.Ref.SemVer
	} else if overlay.Repository.Ref.Tag != "" {
		b.Repository.Ref.Tag = overlay.Repository.Ref.Tag
	} else if overlay.Repository.Ref.Branch != "" {
		b.Repository.Ref.Branch = overlay.Repository.Ref.Branch
	}

	if overlay.Repository.SecretName != "" {
		b.Repository.SecretName = overlay.Repository.SecretName
	}

	sourceMap := make(map[string]Source)
	for _, source := range b.Sources {
		sourceMap[source.Name] = source
	}
	for _, overlaySource := range overlay.Sources {
		if overlaySource.Name != "" {
			sourceMap[overlaySource.Name] = overlaySource
		}
	}
	b.Sources = make([]Source, 0, len(sourceMap))
	for _, source := range sourceMap {
		b.Sources = append(b.Sources, source)
	}

	for _, overlayComponent := range overlay.TerraformComponents {
		if err := b.strategicMergeTerraformComponent(overlayComponent); err != nil {
			return err
		}
	}

	for _, overlayK := range overlay.Kustomizations {
		if err := b.strategicMergeKustomization(overlayK); err != nil {
			return err
		}
	}
	return nil
}

// strategicMergeTerraformComponent performs a strategic merge of the provided TerraformComponent into the Blueprint.
// It merges values, appends unique dependencies, updates fields if provided, and maintains dependency order.
// Returns an error if a dependency cycle is detected during sorting.
func (b *Blueprint) strategicMergeTerraformComponent(component TerraformComponent) error {
	for i, existing := range b.TerraformComponents {
		if existing.Path == component.Path && existing.Source == component.Source {
			if len(component.Values) > 0 {
				if existing.Values == nil {
					existing.Values = make(map[string]any)
				}
				maps.Copy(existing.Values, component.Values)
			}
			for _, dep := range component.DependsOn {
				if !slices.Contains(existing.DependsOn, dep) {
					existing.DependsOn = append(existing.DependsOn, dep)
				}
			}
			if component.Destroy != nil {
				existing.Destroy = component.Destroy
			}
			if component.Parallelism != nil {
				existing.Parallelism = component.Parallelism
			}
			b.TerraformComponents[i] = existing
			return b.sortTerraform()
		}
	}
	b.TerraformComponents = append(b.TerraformComponents, component)
	return b.sortTerraform()
}

// strategicMergeKustomization performs a strategic merge of the provided Kustomization into the Blueprint.
// It merges unique components and dependencies, updates fields if provided, and maintains dependency order.
// Returns an error if a dependency cycle is detected during sorting.
func (b *Blueprint) strategicMergeKustomization(kustomization Kustomization) error {
	for i, existing := range b.Kustomizations {
		if existing.Name == kustomization.Name {
			for _, component := range kustomization.Components {
				if !slices.Contains(existing.Components, component) {
					existing.Components = append(existing.Components, component)
				}
			}
			slices.Sort(existing.Components)
			for _, dep := range kustomization.DependsOn {
				if !slices.Contains(existing.DependsOn, dep) {
					existing.DependsOn = append(existing.DependsOn, dep)
				}
			}
			if kustomization.Path != "" {
				existing.Path = kustomization.Path
			}
			if kustomization.Source != "" {
				existing.Source = kustomization.Source
			}
			if kustomization.Destroy != nil {
				existing.Destroy = kustomization.Destroy
			}
			b.Kustomizations[i] = existing
			return b.sortKustomize()
		}
	}
	b.Kustomizations = append(b.Kustomizations, kustomization)
	return b.sortKustomize()
}

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

// DeepCopy creates a deep copy of the Kustomization object.
func (k *Kustomization) DeepCopy() *Kustomization {
	if k == nil {
		return nil
	}

	var postBuildCopy *PostBuild
	if k.PostBuild != nil {
		substituteCopy := maps.Clone(k.PostBuild.Substitute)
		substituteFromCopy := slices.Clone(k.PostBuild.SubstituteFrom)

		if len(substituteCopy) > 0 || len(substituteFromCopy) > 0 {
			postBuildCopy = &PostBuild{
				Substitute:     substituteCopy,
				SubstituteFrom: substituteFromCopy,
			}
		}
	}

	return &Kustomization{
		Name:          k.Name,
		Path:          k.Path,
		Source:        k.Source,
		DependsOn:     slices.Clone(k.DependsOn),
		Interval:      k.Interval,
		RetryInterval: k.RetryInterval,
		Timeout:       k.Timeout,
		Patches:       slices.Clone(k.Patches),
		Wait:          k.Wait,
		Force:         k.Force,
		Prune:         k.Prune,
		Components:    slices.Clone(k.Components),
		Cleanup:       slices.Clone(k.Cleanup),
		PostBuild:     postBuildCopy,
		Destroy:       k.Destroy,
	}
}

// sortTerraform reorders the Blueprint's TerraformComponents so that dependencies precede dependents.
// It applies a topological sort to ensure dependency order. Components without dependencies come first.
// Returns an error if a dependency cycle is detected.
func (b *Blueprint) sortTerraform() error {
	if len(b.TerraformComponents) <= 1 {
		return nil
	}

	pathToIndex := make(map[string]int)
	for i, component := range b.TerraformComponents {
		pathToIndex[component.Path] = i
	}

	sorted := b.terraformTopologicalSort(pathToIndex)
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

// terraformTopologicalSort computes a topological ordering of terraform components based on dependencies.
// Returns a slice of indices into the TerraformComponents slice, ordered so dependencies precede dependents.
// Returns nil if a cycle is detected in the dependency graph.
func (b *Blueprint) terraformTopologicalSort(pathToIndex map[string]int) []int {
	var sorted []int
	visited := make(map[int]bool)
	visiting := make(map[int]bool)

	var visit func(int) error
	visit = func(componentIndex int) error {
		if visiting[componentIndex] {
			return fmt.Errorf("cycle detected in dependency graph involving terraform component '%s'", b.TerraformComponents[componentIndex].Path)
		}
		if visited[componentIndex] {
			return nil
		}

		visiting[componentIndex] = true
		// Visit dependencies first
		for _, depPath := range b.TerraformComponents[componentIndex].DependsOn {
			if depIndex, exists := pathToIndex[depPath]; exists {
				if err := visit(depIndex); err != nil {
					visiting[componentIndex] = false
					return err
				}
			}
		}
		visiting[componentIndex] = false
		visited[componentIndex] = true
		// Add this component after its dependencies
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
