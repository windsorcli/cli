// Package v1alpha1 contains types for the v1alpha1 API group
// +groupName=blueprints.windsorcli.dev
package v1alpha1

// Facet represents a conditional blueprint fragment that can be merged into a base blueprint.
// Facets enable modular composition of blueprints based on user configuration values.
// Facets inherit Repository and Sources from the base blueprint they are merged into.
type Facet struct {
	// Kind is the facet type, following Kubernetes conventions.
	Kind string `yaml:"kind"`

	// ApiVersion is the API schema version of the facet.
	ApiVersion string `yaml:"apiVersion"`

	// Metadata includes the facet's name and description.
	Metadata Metadata `yaml:"metadata"`

	// Path is the file path where this facet was loaded from.
	// This is used for resolving relative paths in jsonnet() and file() functions.
	Path string `yaml:"-"`

	// When is a CEL expression that determines if this facet should be applied.
	// The expression is evaluated against user configuration values.
	// Examples: "provider == 'aws'", "observability.enabled == true && observability.backend == 'quickwit'"
	When string `yaml:"when,omitempty"`

	// TerraformComponents are Terraform modules in the facet.
	TerraformComponents []ConditionalTerraformComponent `yaml:"terraform,omitempty"`

	// Kustomizations are kustomization configs in the facet.
	Kustomizations []ConditionalKustomization `yaml:"kustomize,omitempty"`
}

// ConditionalTerraformComponent extends TerraformComponent with conditional logic support.
type ConditionalTerraformComponent struct {
	TerraformComponent `yaml:",inline"`

	// When is a CEL expression that determines if this terraform component should be applied.
	// If empty, the component is always applied when the parent facet matches.
	When string `yaml:"when,omitempty"`

	// Strategy determines how this component is merged into the blueprint.
	// Valid values are "merge" (default), "replace", and "remove".
	// If empty or "merge", the component is merged with existing components matching the same Path and Source.
	// If "replace", the component completely replaces any existing component with the same Path and Source.
	// If "remove", the component's non-index fields (everything except Path and Source) are removed from the
	// matching existing component. Remove operations are always applied last after all merge/replace operations.
	Strategy string `yaml:"strategy,omitempty"`

	// Priority determines the order in which components are processed when multiple facets target the same component.
	// Higher priority values are processed later and override lower priority components. Default is 0.
	// When priorities are equal, strategy priority is used (remove > replace > merge).
	// When both priority and strategy are equal, components are merged, removals are accumulated, or replace wins.
	// For replace operations with equal priority and strategy, the last processed facet (alphabetically by name) wins.
	// Set different priorities to make ordering explicit and avoid dependency on facet name ordering.
	Priority int `yaml:"priority,omitempty"`
}

// ConditionalKustomization extends Kustomization with conditional logic support.
type ConditionalKustomization struct {
	Kustomization `yaml:",inline"`

	// When is an expression that determines if this kustomization should be applied.
	// If empty, the kustomization is always applied when the parent facet matches.
	When string `yaml:"when,omitempty"`

	// Strategy determines how this kustomization is merged into the blueprint.
	// Valid values are "merge" (default), "replace", and "remove".
	// If empty or "merge", the kustomization is merged with existing kustomizations matching the same Name.
	// If "replace", the kustomization completely replaces any existing kustomization with the same Name.
	// If "remove", the kustomization's non-index fields (everything except Name) are removed from the
	// matching existing kustomization. Remove operations are always applied last after all merge/replace operations.
	Strategy string `yaml:"strategy,omitempty"`

	// Priority determines the order in which kustomizations are processed when multiple facets target the same kustomization.
	// Higher priority values are processed later and override lower priority kustomizations. Default is 0.
	// When priorities are equal, strategy priority is used (remove > replace > merge).
	// When both priority and strategy are equal, kustomizations are merged, removals are accumulated, or replace wins.
	// For replace operations with equal priority and strategy, the last processed facet (alphabetically by name) wins.
	// Set different priorities to make ordering explicit and avoid dependency on facet name ordering.
	Priority int `yaml:"priority,omitempty"`
}

// DeepCopy creates a deep copy of the Facet object.
func (f *Facet) DeepCopy() *Facet {
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

	return &Facet{
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
		Priority:           c.Priority,
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
		Priority:      c.Priority,
	}
}
