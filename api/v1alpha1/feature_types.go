// Package v1alpha1 contains types for the v1alpha1 API group
// +groupName=blueprints.windsorcli.dev
package v1alpha1

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

	// Path is the file path where this feature was loaded from.
	// This is used for resolving relative paths in jsonnet() and file() functions.
	Path string `yaml:"-"`

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

	// Strategy determines how this component is merged into the blueprint.
	// Valid values are "merge" (default, strategic merge) and "replace" (replaces existing component entirely).
	// If empty or "merge", the component is merged with existing components matching the same Path and Source.
	// If "replace", the component completely replaces any existing component with the same Path and Source.
	Strategy string `yaml:"strategy,omitempty"`
}

// ConditionalKustomization extends Kustomization with conditional logic support.
type ConditionalKustomization struct {
	Kustomization `yaml:",inline"`

	// When is an expression that determines if this kustomization should be applied.
	// If empty, the kustomization is always applied when the parent feature matches.
	When string `yaml:"when,omitempty"`

	// Strategy determines how this kustomization is merged into the blueprint.
	// Valid values are "merge" (default, strategic merge) and "replace" (replaces existing kustomization entirely).
	// If empty or "merge", the kustomization is merged with existing kustomizations matching the same Name.
	// If "replace", the kustomization completely replaces any existing kustomization with the same Name.
	Strategy string `yaml:"strategy,omitempty"`
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
		Path:                f.Path,
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
		Strategy:           c.Strategy,
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
		Strategy:      c.Strategy,
	}
}
