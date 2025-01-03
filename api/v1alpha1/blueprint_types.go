package v1alpha1

import (
	"github.com/fluxcd/pkg/apis/kustomize"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// A Blueprint is a collection of metadata that can be used to initialize a project
type Blueprint struct {
	Kind                string               `yaml:"kind"`           // The Kind of the blueprint
	ApiVersion          string               `yaml:"apiVersion"`     // The API Version of the blueprint
	Metadata            Metadata             `yaml:"metadata"`       // The Metadata for the blueprint
	Sources             []Source             `yaml:"sources"`        // The Sources for the blueprint
	TerraformComponents []TerraformComponent `yaml:"terraform"`      // The Terraform components
	Kustomizations      []Kustomization      `yaml:"kustomizations"` // The Kustomizations for the blueprint
}

// PartialBlueprint is a temporary struct for initial unmarshalling
type PartialBlueprint struct {
	Kind                string                   `yaml:"kind"`
	ApiVersion          string                   `yaml:"apiVersion"`
	Metadata            Metadata                 `yaml:"metadata"`
	Sources             []Source                 `yaml:"sources"`
	Repository          Repository               `yaml:"repository"`
	TerraformComponents []TerraformComponent     `yaml:"terraform"`
	Kustomizations      []map[string]interface{} `yaml:"kustomizations"`
}

// Metadata describes the metadata for a blueprint
type Metadata struct {
	Name        string   `yaml:"name"`                  // The Name of the blueprint
	Description string   `yaml:"description,omitempty"` // The Description of the blueprint
	Authors     []string `yaml:"authors,omitempty"`     // The Authors of the blueprint
}

type Repository struct {
	Url string `yaml:"url"` // The URL of the repository
	Ref string `yaml:"ref"` // The Ref of the repository
}

// Source describes a source for a blueprint
type Source struct {
	Name       string `yaml:"name"`                 // The Name of the source
	Url        string `yaml:"url"`                  // The URL of the source
	PathPrefix string `yaml:"pathPrefix,omitempty"` // The Path Prefix of the source
	Ref        string `yaml:"ref"`                  // The Ref of the source
}

// TerraformComponent describes a Terraform component for a blueprint
type TerraformComponent struct {
	Source    string                       `yaml:"source,omitempty"`    // The Source of the module
	Path      string                       `yaml:"path"`                // The Path of the module
	FullPath  string                       `yaml:"-"`                   // The Full Path of the module
	Values    map[string]interface{}       `yaml:"values,omitempty"`    // The Values for the module
	Variables map[string]TerraformVariable `yaml:"variables,omitempty"` // The Variables for the module
}

// TerraformVariable describes a Terraform variable for a Terraform component
type TerraformVariable struct {
	Type        string      `yaml:"type,omitempty"`        // The Type of the variable
	Default     interface{} `yaml:"default,omitempty"`     // The Default value of the variable
	Description string      `yaml:"description,omitempty"` // The Description of the variable
	Sensitive   bool        `yaml:"sensitive,omitempty"`   // Whether to treat the variable as sensitive
}

type Kustomization struct {
	Name          string            `yaml:"name"`
	Path          string            `yaml:"path"`
	Source        string            `yaml:"source,omitempty"`
	DependsOn     []string          `yaml:"dependsOn,omitempty"`
	Interval      *metav1.Duration  `yaml:"interval,omitempty"`
	RetryInterval *metav1.Duration  `yaml:"retryInterval,omitempty"`
	Timeout       *metav1.Duration  `yaml:"timeout,omitempty"`
	Patches       []kustomize.Patch `yaml:"patches,omitempty"`
	Wait          *bool             `yaml:"wait,omitempty"`
	Force         *bool             `yaml:"force,omitempty"`
	Components    []string          `yaml:"components,omitempty"`
}

// Copy creates a deep copy of BlueprintV1Alpha1.
func (b *Blueprint) DeepCopy() *Blueprint {
	if b == nil {
		return nil
	}

	// Copy Metadata
	metadataCopy := Metadata{
		Name:        b.Metadata.Name,
		Description: b.Metadata.Description,
		Authors:     append([]string{}, b.Metadata.Authors...),
	}

	// Copy Sources
	sourcesCopy := make([]Source, len(b.Sources))
	for i, source := range b.Sources {
		sourcesCopy[i] = Source{
			Name:       source.Name,
			Url:        source.Url,
			PathPrefix: source.PathPrefix,
			Ref:        source.Ref,
		}
	}

	// Copy TerraformComponents
	terraformComponentsCopy := make([]TerraformComponent, len(b.TerraformComponents))
	for i, component := range b.TerraformComponents {
		variablesCopy := make(map[string]TerraformVariable, len(component.Variables))
		for key, variable := range component.Variables {
			variablesCopy[key] = TerraformVariable{
				Type:        variable.Type,
				Default:     variable.Default,
				Description: variable.Description,
				Sensitive:   variable.Sensitive,
			}
		}

		valuesCopy := make(map[string]interface{}, len(component.Values))
		for key, value := range component.Values {
			valuesCopy[key] = value
		}

		terraformComponentsCopy[i] = TerraformComponent{
			Source:    component.Source,
			Path:      component.Path,
			FullPath:  component.FullPath,
			Values:    valuesCopy,
			Variables: variablesCopy,
		}
	}

	// Copy Kustomizations using DeepCopy
	kustomizationsCopy := make([]Kustomization, len(b.Kustomizations))
	for i, kustomization := range b.Kustomizations {
		kustomizationsCopy[i] = kustomization
	}

	return &Blueprint{
		Kind:                b.Kind,
		ApiVersion:          b.ApiVersion,
		Metadata:            metadataCopy,
		Sources:             sourcesCopy,
		TerraformComponents: terraformComponentsCopy,
		Kustomizations:      kustomizationsCopy,
	}
}

// Merge integrates another BlueprintV1Alpha1 into the current one, ensuring that terraform components maintain order.
func (b *Blueprint) Merge(overlay *Blueprint) {
	if overlay == nil {
		return
	}

	// Merge top-level fields
	if overlay.Kind != "" {
		b.Kind = overlay.Kind
	}
	if overlay.ApiVersion != "" {
		b.ApiVersion = overlay.ApiVersion
	}

	// Merge Metadata
	if overlay.Metadata.Name != "" {
		b.Metadata.Name = overlay.Metadata.Name
	}
	if overlay.Metadata.Description != "" {
		b.Metadata.Description = overlay.Metadata.Description
	}
	if len(overlay.Metadata.Authors) > 0 {
		b.Metadata.Authors = overlay.Metadata.Authors
	}

	// Merge Sources by "name", preferring overlay values
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

	// Merge TerraformComponents by "path" as primary key and "source" as secondary key, maintaining order
	componentMap := make(map[string]TerraformComponent)
	for _, component := range b.TerraformComponents {
		key := component.Path
		componentMap[key] = component
	}

	// Only merge components that exist in the overlay, preserving order
	if len(overlay.TerraformComponents) > 0 {
		b.TerraformComponents = make([]TerraformComponent, 0, len(overlay.TerraformComponents))
		for _, overlayComponent := range overlay.TerraformComponents {
			key := overlayComponent.Path
			if existingComponent, exists := componentMap[key]; exists {
				// Perform a full merge if both path and source match
				if existingComponent.Source == overlayComponent.Source {
					mergedComponent := existingComponent

					// Merge Values
					if mergedComponent.Values == nil {
						mergedComponent.Values = make(map[string]interface{})
					}
					for k, v := range overlayComponent.Values {
						mergedComponent.Values[k] = v
					}

					// Merge Variables
					if mergedComponent.Variables == nil {
						mergedComponent.Variables = make(map[string]TerraformVariable)
					}
					for k, v := range overlayComponent.Variables {
						mergedComponent.Variables[k] = v
					}

					if overlayComponent.FullPath != "" {
						mergedComponent.FullPath = overlayComponent.FullPath
					}
					b.TerraformComponents = append(b.TerraformComponents, mergedComponent)
				} else {
					// Use the overlay component if the path matches but the source doesn't
					b.TerraformComponents = append(b.TerraformComponents, overlayComponent)
				}
			} else {
				// Add the overlay component if it doesn't exist in the target
				b.TerraformComponents = append(b.TerraformComponents, overlayComponent)
			}
		}
	}

	// Merge Kustomizations, preferring overlay values
	mergedKustomizations := make([]Kustomization, 0, len(b.Kustomizations)+len(overlay.Kustomizations))

	// Add existing kustomizations
	for _, kustomization := range b.Kustomizations {
		mergedKustomizations = append(mergedKustomizations, kustomization)
	}

	// Merge overlay kustomizations
	for _, overlayKustomization := range overlay.Kustomizations {
		found := false
		for i, existingKustomization := range mergedKustomizations {
			if existingKustomization.Name == overlayKustomization.Name {
				// Merge patches and components uniquely
				mergedKustomizations[i].Patches = mergeUniqueKustomizePatches(existingKustomization.Patches, overlayKustomization.Patches)
				mergedKustomizations[i].Components = mergeUniqueComponents(existingKustomization.Components, overlayKustomization.Components)
				found = true
				break
			}
		}
		if !found {
			mergedKustomizations = append(mergedKustomizations, overlayKustomization)
		}
	}

	b.Kustomizations = mergedKustomizations
}

// Helper function to merge patches uniquely
func mergeUniqueKustomizePatches(existing, overlay []kustomize.Patch) []kustomize.Patch {
	patchMap := make(map[string]kustomize.Patch)
	for _, patch := range existing {
		key := patch.Patch
		if patch.Target != nil {
			key += patch.Target.Group + patch.Target.Version + patch.Target.Kind + patch.Target.Namespace + patch.Target.Name
		}
		patchMap[key] = patch
	}
	for _, overlayPatch := range overlay {
		key := overlayPatch.Patch
		if overlayPatch.Target != nil {
			key += overlayPatch.Target.Group + overlayPatch.Target.Version + overlayPatch.Target.Kind + overlayPatch.Target.Namespace + overlayPatch.Target.Name
		}
		patchMap[key] = overlayPatch
	}
	mergedPatches := make([]kustomize.Patch, 0, len(patchMap))
	for _, patch := range patchMap {
		mergedPatches = append(mergedPatches, patch)
	}
	return mergedPatches
}

// Helper function to merge components uniquely
func mergeUniqueComponents(existing, overlay []string) []string {
	componentSet := make(map[string]struct{})
	for _, component := range existing {
		componentSet[component] = struct{}{}
	}
	for _, overlayComponent := range overlay {
		componentSet[overlayComponent] = struct{}{}
	}
	mergedComponents := make([]string, 0, len(componentSet))
	for component := range componentSet {
		mergedComponents = append(mergedComponents, component)
	}
	return mergedComponents
}
