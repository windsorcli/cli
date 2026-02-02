// Package v1alpha1 contains types for the v1alpha1 API group
// +groupName=blueprints.windsorcli.dev
package v1alpha1

import "maps"

// =============================================================================
// Types
// =============================================================================

// ConfigBlock represents a named, optionally conditional configuration block in a facet.
// The block has a name (exposed at scope root, e.g. talos.controlplanes), an optional when
// expression, and a body of key-value pairs evaluated in blueprint context. References
// from terraform.inputs and kustomize.substitutions use <name>.<key> (same style as context: cluster.*, network.*).
type ConfigBlock struct {
	// Name identifies the block; exposed at scope root so expressions use name.key (e.g. talos.controlplanes).
	Name string `yaml:"name"`
	// When is an expression that determines if this config block is evaluated; if empty, always evaluated when facet is active.
	When string `yaml:"when,omitempty"`
	// Body is the block content (all keys other than name and when); values may contain expressions.
	Body map[string]any `yaml:"-"`
}

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

	// Config is a list of named configuration blocks evaluated in blueprint context and exposed at scope root.
	// Terraform inputs and kustomize substitutions reference <name>.<key> (e.g. talos.controlplanes), like context (cluster.*, network.*).
	Config []ConfigBlock `yaml:"config,omitempty"`

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

// =============================================================================
// Public Methods
// =============================================================================

// UnmarshalYAML implements custom unmarshaling so Body receives all keys except name and when.
func (c *ConfigBlock) UnmarshalYAML(unmarshal func(any) error) error {
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	if n, ok := raw["name"]; ok {
		c.Name, _ = n.(string)
	}
	if w, ok := raw["when"]; ok {
		c.When, _ = w.(string)
	}
	delete(raw, "name")
	delete(raw, "when")
	c.Body = raw
	return nil
}

// MarshalYAML implements custom marshaling so name, when, and Body keys are written.
// Body is copied first so struct Name and When take precedence over any name/when in Body.
func (c *ConfigBlock) MarshalYAML() (any, error) {
	out := make(map[string]any)
	maps.Copy(out, c.Body)
	if c.Name != "" {
		out["name"] = c.Name
	}
	if c.When != "" {
		out["when"] = c.When
	}
	return out, nil
}

// DeepCopy creates a deep copy of the ConfigBlock object.
func (c *ConfigBlock) DeepCopy() *ConfigBlock {
	if c == nil {
		return nil
	}
	return &ConfigBlock{Name: c.Name, When: c.When, Body: deepCopyMapStringAny(c.Body)}
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

	configCopy := make([]ConfigBlock, len(f.Config))
	for i, block := range f.Config {
		configCopy[i] = *block.DeepCopy()
	}

	return &Facet{
		Kind:                f.Kind,
		ApiVersion:          f.ApiVersion,
		Metadata:            metadataCopy,
		Path:                f.Path,
		When:                f.When,
		Config:              configCopy,
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

// =============================================================================
// Private Methods
// =============================================================================

// deepCopyMapStringAny recursively copies map[string]any so nested maps and slices are independent.
func deepCopyMapStringAny(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopyValue(v)
	}
	return out
}

// deepCopyValue returns a deep copy of v when it is map[string]any or []any; otherwise returns v unchanged.
func deepCopyValue(v any) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case map[string]any:
		return deepCopyMapStringAny(val)
	case []any:
		return deepCopySliceAny(val)
	default:
		return v
	}
}

// deepCopySliceAny recursively copies []any so nested maps and slices are independent.
func deepCopySliceAny(s []any) []any {
	if s == nil {
		return nil
	}
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = deepCopyValue(v)
	}
	return out
}
