// Package v1alpha1 contains types for the v1alpha1 API group
// +groupName=blueprints.windsorcli.dev
package v1alpha1

import (
	"maps"
)

// Feature represents a conditional blueprint fragment that can be merged into a base blueprint.
// Features enable modular composition of blueprints based on user configuration values.
// Features inherit Repository and Sources from the base blueprint they are merged into.
type Feature struct {
	// Kind is the feature type, following Kubernetes conventions.
	Kind string `yaml:"kind"`

	// ApiVersion is the API schema version of the feature.
	ApiVersion string `yaml:"apiVersion"`

	// Metadata includes the feature's name and description.
	Metadata Metadata `yaml:"metadata"`

	// When is a CEL expression that determines if this feature should be applied.
	// The expression is evaluated against user configuration values.
	// Examples: "provider == 'aws'", "observability.enabled == true && observability.backend == 'quickwit'"
	When string `yaml:"when,omitempty"`

	// TerraformComponents are Terraform modules in the feature.
	TerraformComponents []ConditionalTerraformComponent `yaml:"terraform,omitempty"`

	// Kustomizations are kustomization configs in the feature.
	Kustomizations []ConditionalKustomization `yaml:"kustomize,omitempty"`
}

// ConditionalTerraformComponent extends TerraformComponent with conditional logic support.
type ConditionalTerraformComponent struct {
	TerraformComponent `yaml:",inline"`

	// When is a CEL expression that determines if this terraform component should be applied.
	// If empty, the component is always applied when the parent feature matches.
	When string `yaml:"when,omitempty"`
}

// ConditionalKustomization extends Kustomization with conditional logic support.
type ConditionalKustomization struct {
	Kustomization `yaml:",inline"`

	// When is a CEL expression that determines if this kustomization should be applied.
	// If empty, the kustomization is always applied when the parent feature matches.
	When string `yaml:"when,omitempty"`
}

// DeepCopy creates a deep copy of the Feature object.
func (f *Feature) DeepCopy() *Feature {
	if f == nil {
		return nil
	}

	metadataCopy := Metadata{
		Name:        f.Metadata.Name,
		Description: f.Metadata.Description,
	}

	terraformComponentsCopy := make([]ConditionalTerraformComponent, len(f.TerraformComponents))
	for i, component := range f.TerraformComponents {
		terraformComponentsCopy[i] = *component.DeepCopy()
	}

	kustomizationsCopy := make([]ConditionalKustomization, len(f.Kustomizations))
	for i, kustomization := range f.Kustomizations {
		kustomizationsCopy[i] = *kustomization.DeepCopy()
	}

	return &Feature{
		Kind:                f.Kind,
		ApiVersion:          f.ApiVersion,
		Metadata:            metadataCopy,
		When:                f.When,
		TerraformComponents: terraformComponentsCopy,
		Kustomizations:      kustomizationsCopy,
	}
}

// DeepCopy creates a deep copy of the ConditionalTerraformComponent object.
func (c *ConditionalTerraformComponent) DeepCopy() *ConditionalTerraformComponent {
	if c == nil {
		return nil
	}

	return &ConditionalTerraformComponent{
		TerraformComponent: *c.TerraformComponent.DeepCopy(),
		When:               c.When,
	}
}

// DeepCopy creates a deep copy of the ConditionalKustomization object.
func (c *ConditionalKustomization) DeepCopy() *ConditionalKustomization {
	if c == nil {
		return nil
	}

	return &ConditionalKustomization{
		Kustomization: *c.Kustomization.DeepCopy(),
		When:          c.When,
	}
}
