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

	// Inputs contains input values for the terraform module.
	// Values can be expressions using ${} syntax (e.g., "${cluster.workers.count + 2}") or literals (e.g., "us-east-1").
	// Values with ${} are evaluated as expressions, plain values are passed through as literals.
	Inputs map[string]any `yaml:"inputs,omitempty"`
}

// ConditionalKustomization extends Kustomization with conditional logic support.
type ConditionalKustomization struct {
	Kustomization `yaml:",inline"`

	// When is a CEL expression that determines if this kustomization should be applied.
	// If empty, the kustomization is always applied when the parent feature matches.
	When string `yaml:"when,omitempty"`

	// Substitutions contains substitution values for post-build variable replacement.
	// These values are collected and stored in ConfigMaps for use by Flux postBuild substitution.
	// Values can be expressions using ${} syntax (e.g., "${dns.domain}") or literals (e.g., "example.com").
	// Values with ${} are evaluated as expressions, plain values are passed through as literals.
	// All values are converted to strings as required by Flux variable substitution.
	Substitutions map[string]string `yaml:"substitutions,omitempty"`
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
		valuesCopy := make(map[string]any, len(component.Values))
		maps.Copy(valuesCopy, component.Values)

		inputsCopy := make(map[string]any, len(component.Inputs))
		maps.Copy(inputsCopy, component.Inputs)

		dependsOnCopy := append([]string{}, component.DependsOn...)

		terraformComponentsCopy[i] = ConditionalTerraformComponent{
			TerraformComponent: TerraformComponent{
				Source:      component.Source,
				Path:        component.Path,
				FullPath:    component.FullPath,
				DependsOn:   dependsOnCopy,
				Values:      valuesCopy,
				Destroy:     component.Destroy,
				Parallelism: component.Parallelism,
			},
			When:   component.When,
			Inputs: inputsCopy,
		}
	}

	kustomizationsCopy := make([]ConditionalKustomization, len(f.Kustomizations))
	for i, kustomization := range f.Kustomizations {
		substitutionsCopy := make(map[string]string, len(kustomization.Substitutions))
		maps.Copy(substitutionsCopy, kustomization.Substitutions)

		kustomizationsCopy[i] = ConditionalKustomization{
			Kustomization: *kustomization.Kustomization.DeepCopy(),
			When:          kustomization.When,
			Substitutions: substitutionsCopy,
		}
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

	valuesCopy := make(map[string]any, len(c.Values))
	maps.Copy(valuesCopy, c.Values)

	inputsCopy := make(map[string]any, len(c.Inputs))
	maps.Copy(inputsCopy, c.Inputs)

	dependsOnCopy := append([]string{}, c.DependsOn...)

	return &ConditionalTerraformComponent{
		TerraformComponent: TerraformComponent{
			Source:      c.Source,
			Path:        c.Path,
			FullPath:    c.FullPath,
			DependsOn:   dependsOnCopy,
			Values:      valuesCopy,
			Destroy:     c.Destroy,
			Parallelism: c.Parallelism,
		},
		When:   c.When,
		Inputs: inputsCopy,
	}
}

// DeepCopy creates a deep copy of the ConditionalKustomization object.
func (c *ConditionalKustomization) DeepCopy() *ConditionalKustomization {
	if c == nil {
		return nil
	}

	substitutionsCopy := make(map[string]string, len(c.Substitutions))
	maps.Copy(substitutionsCopy, c.Substitutions)

	return &ConditionalKustomization{
		Kustomization: *c.Kustomization.DeepCopy(),
		When:          c.When,
		Substitutions: substitutionsCopy,
	}
}
