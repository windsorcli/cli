// Package v1alpha1 contains types for the v1alpha1 API group
// +groupName=blueprints.windsorcli.dev
package v1alpha1

import (
	"maps"
	"slices"

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

// Metadata describes a blueprint, including name and authors.
type Metadata struct {
	// Name is the blueprint's unique identifier.
	Name string `yaml:"name"`

	// Description is a brief overview of the blueprint.
	Description string `yaml:"description,omitempty"`

	// Authors are the creators or maintainers of the blueprint.
	Authors []string `yaml:"authors,omitempty"`
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

// Kustomization defines a kustomization config in a blueprint.
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
	Patches []kustomize.Patch `yaml:"patches,omitempty"`

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

// Merge integrates another Blueprint into the current one.
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
						mergedComponent.Values = make(map[string]any)
					}
					maps.Copy(mergedComponent.Values, overlayComponent.Values)

					if overlayComponent.FullPath != "" {
						mergedComponent.FullPath = overlayComponent.FullPath
					}

					if overlayComponent.DependsOn != nil {
						mergedComponent.DependsOn = overlayComponent.DependsOn
					}

					if overlayComponent.Destroy != nil {
						mergedComponent.Destroy = overlayComponent.Destroy
					}

					if overlayComponent.Parallelism != nil {
						mergedComponent.Parallelism = overlayComponent.Parallelism
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

	// Always prefer the overlay's entire kustomizations if it's not empty
	if len(overlay.Kustomizations) > 0 {
		b.Kustomizations = overlay.Kustomizations
	}
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
	}
}
