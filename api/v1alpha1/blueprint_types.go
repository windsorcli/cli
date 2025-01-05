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
	Repository          Repository           `yaml:"repository"`     // The Repository for the blueprint
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
	Url        string    `yaml:"url"`                  // The URL of the repository
	Ref        Reference `yaml:"ref"`                  // The Ref of the repository
	SecretName string    `yaml:"secretName,omitempty"` // The Secret Name of the repository
}

// Source describes a source for a blueprint
type Source struct {
	Name       string    `yaml:"name"`                 // The Name of the source
	Url        string    `yaml:"url"`                  // The URL of the source
	PathPrefix string    `yaml:"pathPrefix,omitempty"` // The Path Prefix of the source
	Ref        Reference `yaml:"ref"`                  // The Ref of the source
	SecretName string    `yaml:"secretName,omitempty"` // The Secret Name of the source
}

type Reference struct {
	Branch string `yaml:"branch,omitempty"` // The branch of the reference
	Tag    string `yaml:"tag,omitempty"`    // The tag of the reference
	SemVer string `yaml:"semver,omitempty"` // The semantic version of the reference
	Name   string `yaml:"name,omitempty"`   // The name of the reference
	Commit string `yaml:"commit,omitempty"` // The commit of the reference
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

// DeepCopy creates a deep copy of the Blueprint object, including all its nested structures.
// This function ensures that changes to the copy do not affect the original Blueprint.
func (b *Blueprint) DeepCopy() *Blueprint {
	if b == nil {
		return nil
	}

	metadataCopy := Metadata{
		Name:        b.Metadata.Name,
		Description: b.Metadata.Description,
		Authors:     append([]string{}, b.Metadata.Authors...),
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

	kustomizationsCopy := make([]Kustomization, len(b.Kustomizations))
	for i, kustomization := range b.Kustomizations {
		kustomizationsCopy[i] = kustomization
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

// Merge integrates another Blueprint into the current one, ensuring that terraform components maintain order.
// This function prioritizes the overlay's values and merges nested structures like Sources and Kustomizations.
func (b *Blueprint) Merge(overlay *Blueprint) {
	if overlay == nil {
		return
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
	if len(overlay.Metadata.Authors) > 0 {
		b.Metadata.Authors = overlay.Metadata.Authors
	}

	if overlay.Repository.Url != "" {
		b.Repository.Url = overlay.Repository.Url
	}

	// Merge the Reference type inline, prioritizing the first non-empty field
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

	componentMap := make(map[string]TerraformComponent)
	for _, component := range b.TerraformComponents {
		key := component.Path
		componentMap[key] = component
	}

	if len(overlay.TerraformComponents) > 0 {
		b.TerraformComponents = make([]TerraformComponent, 0, len(overlay.TerraformComponents))
		for _, overlayComponent := range overlay.TerraformComponents {
			key := overlayComponent.Path
			if existingComponent, exists := componentMap[key]; exists {
				if existingComponent.Source == overlayComponent.Source {
					mergedComponent := existingComponent

					if mergedComponent.Values == nil {
						mergedComponent.Values = make(map[string]interface{})
					}
					for k, v := range overlayComponent.Values {
						mergedComponent.Values[k] = v
					}

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
					b.TerraformComponents = append(b.TerraformComponents, overlayComponent)
				}
			} else {
				b.TerraformComponents = append(b.TerraformComponents, overlayComponent)
			}
		}
	}

	mergedKustomizations := make([]Kustomization, 0, len(b.Kustomizations)+len(overlay.Kustomizations))

	for _, kustomization := range b.Kustomizations {
		mergedKustomizations = append(mergedKustomizations, kustomization)
	}

	for _, overlayKustomization := range overlay.Kustomizations {
		found := false
		for i, existingKustomization := range mergedKustomizations {
			if existingKustomization.Name == overlayKustomization.Name {
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

// mergeUniqueKustomizePatches merges two slices of kustomize.Patch uniquely, ensuring no duplicates.
// It uses a map to track unique patches based on their content and target.
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

// mergeUniqueComponents merges two slices of strings uniquely, ensuring no duplicates.
// It uses a map to track unique components.
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
