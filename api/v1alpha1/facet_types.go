// Package v1alpha1 contains types for the v1alpha1 API group
// +groupName=blueprints.windsorcli.dev
package v1alpha1

import (
	"fmt"
	"maps"
	"slices"

	"github.com/goccy/go-yaml"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Types
// =============================================================================

// ConfigBlock represents a named, optionally conditional configuration block in a facet.
// Only name, when, value, strategy, and ordinal are allowed; value is required and may be a scalar, list, or map.
// The block is exposed at scope root: expressions use <name> for scalar/list values, or
// <name>.<key> when value is a map. References from terraform.inputs and kustomize.substitutions
// use the block name (e.g. talos, platform).
// Strategy "merge" (the default) deep-merges blocks of the same name, "replace" replaces the existing
// block, and "remove" removes the block from scope. A higher ordinal wins on conflict.
// Requires entries apply under the AND of facet.When, block.When, and the entry's own When.
type ConfigBlock struct {
	// Name identifies the block at scope root.
	Name string `yaml:"name"`
	// When is an expression that gates the block. Empty means the block applies while the facet is active.
	When string `yaml:"when,omitempty"`
	// Strategy is "merge" (default), "replace", or "remove".
	Strategy string `yaml:"strategy,omitempty"`
	// Ordinal overrides the facet ordinal for this block's merge precedence.
	Ordinal *int `yaml:"ordinal,omitempty"`
	// Requires lists requirement blocks scoped to this config block.
	Requires []RequirementBlock `yaml:"requires,omitempty"`
	// Body holds the content as {"value": <content>} for merge and evaluation, and is not marshaled.
	Body map[string]any `yaml:"-"`
}

// RequirementBlock declares a rule that the parent facet's inputs must meet. A block with Paths
// requires each path to resolve to a present, non-empty value. A block without Paths rejects a
// combination of values: it fails whenever its When is true, and the error shows its Message.
// When gates the block; if empty, the block applies whenever the parent facet is active.
type RequirementBlock struct {
	// When is an expression gating this block. Empty means always required while the parent facet is active.
	When string `yaml:"when,omitempty"`

	// Paths are dotted scope keys whose values must be present and non-empty.
	Paths []string `yaml:"paths,omitempty"`

	// Message is author-supplied context for the aggregated error. A block without Paths requires it.
	Message string `yaml:"message,omitempty"`
}

// Facet represents a conditional blueprint fragment that can be merged into a base blueprint.
// Facets enable modular composition of blueprints based on user configuration values.
// Facets inherit Repository and Sources from the base blueprint they are merged into.
// When uses expr-lang/expr syntax, for example "platform == 'aws'". A nil Ordinal comes from the file
// basename: config-* 100, platform-base or provider-base 199, platform-* or provider-* 200,
// option-* or options-* 300, and addon-* or addons-* 400.
// Unmet Requires blocks from every active facet are reported in one error.
// Messages render after a successful run, interpolated against composed scope.
type Facet struct {
	// Kind is the facet type, following Kubernetes conventions.
	Kind string `yaml:"kind"`

	// ApiVersion is the API schema version of the facet.
	ApiVersion string `yaml:"apiVersion"`

	// Metadata includes the facet's name and description.
	Metadata Metadata `yaml:"metadata"`

	// Path is the facet's file path, used to resolve relative paths in jsonnet() and file().
	Path string `yaml:"-"`

	// Source names the blueprint source this facet came from during composition, and is not marshaled.
	Source string `yaml:"-"`

	// Ordinal sets the merge precedence of this facet. A higher ordinal wins.
	Ordinal *int `yaml:"ordinal,omitempty"`

	// When is an expression that determines if this facet applies.
	When string `yaml:"when,omitempty"`

	// Backend contributes Blueprint.Backend; the composer merges by ordinal precedence.
	Backend string `yaml:"backend,omitempty"`

	// Config is a list of named configuration blocks exposed at scope root, such as talos.controlplanes.
	Config []ConfigBlock `yaml:"config,omitempty"`

	// Requires lists requirement blocks that must hold while the facet is active.
	Requires []RequirementBlock `yaml:"requires,omitempty"`

	// TerraformComponents are Terraform modules in the facet.
	TerraformComponents []ConditionalTerraformComponent `yaml:"terraform,omitempty"`

	// Kustomizations are kustomization configs in the facet.
	Kustomizations []ConditionalKustomization `yaml:"kustomize,omitempty"`

	// FluxSystems are the flux: entries of this facet, each compiled to install and resources Kustomizations.
	FluxSystems []FluxSystem `yaml:"flux,omitempty"`

	// Crds lists vendored CRD references, such as "cert-manager-1.16.2", that this facet's kustomizations depend on.
	Crds []string `yaml:"crds,omitempty"`

	// Substitutions are key/value pairs, evaluated with facet scope, that go into values-common.
	Substitutions map[string]string `yaml:"substitutions,omitempty"`

	// Messages are operator-facing notes that render at the end of a bootstrap or apply run.
	Messages []Message `yaml:"messages,omitempty"`
}

// Message is an operator-facing note a facet emits at the end of a run. When gates whether it renders;
// Text is the message body, evaluated against composed scope so it can interpolate run values.
type Message struct {
	// When is an optional expression gating whether this message renders. Empty means always.
	When string `yaml:"when,omitempty"`

	// Text is the message body, which may reference config values and run outputs.
	Text string `yaml:"text"`
}

// ConditionalTerraformComponent extends TerraformComponent with conditional logic support.
// Strategy "merge" (the default) merges into the component with the same Path and Source, and "replace"
// replaces it. "remove" removes the values the entry sets from that component, after all merges and
// replaces, and never changes Path or Source.
// A higher ordinal is processed later and wins on conflict.
// Requires entries apply under facet.When && component.When && block.When.
type ConditionalTerraformComponent struct {
	TerraformComponent `yaml:",inline"`

	// When is an expression that gates the component. Empty means it applies while the facet is active.
	When string `yaml:"when,omitempty"`

	// Strategy is "merge" (default), "replace", or "remove".
	Strategy string `yaml:"strategy,omitempty"`

	// Ordinal overrides the facet ordinal for this component's merge precedence.
	Ordinal *int `yaml:"ordinal,omitempty"`

	// Requires lists requirement blocks scoped to this terraform component.
	Requires []RequirementBlock `yaml:"requires,omitempty"`
}

// ConditionalKustomization extends Kustomization with conditional logic support.
// Strategy "merge" (the default) merges into the kustomization with the same Name, and "replace"
// replaces it. "remove" removes the values the entry sets from that kustomization, after all merges
// and replaces, and never changes Name.
// A higher ordinal is processed later and wins on conflict.
// Requires entries apply under facet.When && kustomization.When && block.When.
type ConditionalKustomization struct {
	Kustomization `yaml:",inline"`

	// When is an expression that gates the kustomization. Empty means it applies while the facet is active.
	When string `yaml:"when,omitempty"`

	// Strategy is "merge" (default), "replace", or "remove".
	Strategy string `yaml:"strategy,omitempty"`

	// Ordinal overrides the facet ordinal for this kustomization's merge precedence.
	Ordinal *int `yaml:"ordinal,omitempty"`

	// Requires lists requirement blocks scoped to this kustomization.
	Requires []RequirementBlock `yaml:"requires,omitempty"`
}

// =============================================================================
// Public Methods
// =============================================================================

// UnmarshalYAML implements custom unmarshaling. Only name, when, value, strategy, and ordinal are read; value is required.
// Body is set to {"value": raw["value"]} for merge and evaluation.
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
	if s, ok := raw["strategy"]; ok {
		c.Strategy, _ = s.(string)
	}
	if o, ok := raw["ordinal"]; ok && o != nil {
		switch v := o.(type) {
		case int:
			c.Ordinal = &v
		case int64:
			i := int(v)
			c.Ordinal = &i
		case float64:
			i := int(v)
			c.Ordinal = &i
		}
	}
	if r, ok := raw["requires"]; ok && r != nil {
		b, err := yaml.Marshal(r)
		if err != nil {
			return werror.Wrap(err, "config block %q: marshal requires", c.Name)
		}
		if err := yaml.Unmarshal(b, &c.Requires); err != nil {
			return werror.Wrap(err, "config block %q: parse requires", c.Name)
		}
	}
	if _, hasValue := raw["value"]; !hasValue {
		return fmt.Errorf("config block %q: value is required", c.Name)
	}
	c.Body = map[string]any{"value": raw["value"]}
	return nil
}

// MarshalYAML implements custom marshaling. Only name, when, value, strategy, and ordinal are written.
func (c *ConfigBlock) MarshalYAML() (any, error) {
	out := make(map[string]any)
	if c.Name != "" {
		out["name"] = c.Name
	}
	if c.When != "" {
		out["when"] = c.When
	}
	if c.Strategy != "" {
		out["strategy"] = c.Strategy
	}
	if c.Ordinal != nil {
		out["ordinal"] = *c.Ordinal
	}
	if len(c.Requires) > 0 {
		out["requires"] = c.Requires
	}
	if c.Body != nil {
		if v, ok := c.Body["value"]; ok {
			out["value"] = v
		}
	}
	return out, nil
}

// UnmarshalYAML rejects a block that has neither when nor paths, because it checks nothing. It
// also rejects a block without paths that has no message, because its error would have no text.
func (r *RequirementBlock) UnmarshalYAML(unmarshal func(any) error) error {
	type rawRequirementBlock RequirementBlock
	var raw rawRequirementBlock
	if err := unmarshal(&raw); err != nil {
		return err
	}
	if len(raw.Paths) == 0 && raw.When == "" {
		return fmt.Errorf("requirement block: set when, paths, or both")
	}
	if len(raw.Paths) == 0 && raw.Message == "" {
		return fmt.Errorf("requirement block: message is required on a block without paths")
	}
	*r = RequirementBlock(raw)
	return nil
}

// DeepCopy creates a deep copy of the RequirementBlock object.
func (r *RequirementBlock) DeepCopy() *RequirementBlock {
	if r == nil {
		return nil
	}
	return &RequirementBlock{
		When:    r.When,
		Paths:   slices.Clone(r.Paths),
		Message: r.Message,
	}
}

// DeepCopy creates a deep copy of the ConfigBlock object.
func (c *ConfigBlock) DeepCopy() *ConfigBlock {
	if c == nil {
		return nil
	}
	var ordinalCopy *int
	if c.Ordinal != nil {
		o := *c.Ordinal
		ordinalCopy = &o
	}
	var requiresCopy []RequirementBlock
	if len(c.Requires) > 0 {
		requiresCopy = make([]RequirementBlock, len(c.Requires))
		for i, block := range c.Requires {
			requiresCopy[i] = *block.DeepCopy()
		}
	}
	return &ConfigBlock{
		Name:     c.Name,
		When:     c.When,
		Strategy: c.Strategy,
		Ordinal:  ordinalCopy,
		Requires: requiresCopy,
		Body:     deepCopyMapStringAny(c.Body),
	}
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

	requiresCopy := make([]RequirementBlock, len(f.Requires))
	for i, block := range f.Requires {
		requiresCopy[i] = *block.DeepCopy()
	}

	var ordinalCopy *int
	if f.Ordinal != nil {
		o := *f.Ordinal
		ordinalCopy = &o
	}

	return &Facet{
		Kind:                f.Kind,
		ApiVersion:          f.ApiVersion,
		Metadata:            metadataCopy,
		Path:                f.Path,
		Source:              f.Source,
		Ordinal:             ordinalCopy,
		When:                f.When,
		Backend:             f.Backend,
		Config:              configCopy,
		Requires:            requiresCopy,
		TerraformComponents: terraformComponentsCopy,
		Kustomizations:      kustomizationsCopy,
		Crds:                slices.Clone(f.Crds),
		Substitutions:       maps.Clone(f.Substitutions),
		Messages:            slices.Clone(f.Messages),
	}
}

// DeepCopy creates a deep copy of the ConditionalTerraformComponent object.
func (c *ConditionalTerraformComponent) DeepCopy() *ConditionalTerraformComponent {
	if c == nil {
		return nil
	}

	var ordinalCopy *int
	if c.Ordinal != nil {
		o := *c.Ordinal
		ordinalCopy = &o
	}

	var requiresCopy []RequirementBlock
	if len(c.Requires) > 0 {
		requiresCopy = make([]RequirementBlock, len(c.Requires))
		for i, block := range c.Requires {
			requiresCopy[i] = *block.DeepCopy()
		}
	}

	return &ConditionalTerraformComponent{
		TerraformComponent: *c.TerraformComponent.DeepCopy(),
		When:               c.When,
		Strategy:           c.Strategy,
		Ordinal:            ordinalCopy,
		Requires:           requiresCopy,
	}
}

// DeepCopy creates a deep copy of the ConditionalKustomization object.
func (c *ConditionalKustomization) DeepCopy() *ConditionalKustomization {
	if c == nil {
		return nil
	}

	var ordinalCopy *int
	if c.Ordinal != nil {
		o := *c.Ordinal
		ordinalCopy = &o
	}

	var requiresCopy []RequirementBlock
	if len(c.Requires) > 0 {
		requiresCopy = make([]RequirementBlock, len(c.Requires))
		for i, block := range c.Requires {
			requiresCopy[i] = *block.DeepCopy()
		}
	}

	return &ConditionalKustomization{
		Kustomization: *c.Kustomization.DeepCopy(),
		When:          c.When,
		Strategy:      c.Strategy,
		Ordinal:       ordinalCopy,
		Requires:      requiresCopy,
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
