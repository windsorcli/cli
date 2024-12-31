package blueprint

// A Blueprint is a collection of metadata that can be used to initialize a project
type BlueprintV1Alpha1 struct {
	Kind                string                       `yaml:"kind"`       // The Kind of the blueprint
	ApiVersion          string                       `yaml:"apiVersion"` // The API Version of the blueprint
	Metadata            MetadataV1Alpha1             `yaml:"metadata"`   // The Metadata for the blueprint
	Sources             []SourceV1Alpha1             `yaml:"sources"`    // The Sources for the blueprint
	TerraformComponents []TerraformComponentV1Alpha1 `yaml:"terraform"`  // The Terraform components
}

// Metadata describes the metadata for a blueprint
type MetadataV1Alpha1 struct {
	Name        string   `yaml:"name"`                  // The Name of the blueprint
	Description string   `yaml:"description,omitempty"` // The Description of the blueprint
	Authors     []string `yaml:"authors,omitempty"`     // The Authors of the blueprint
}

// Source describes a source for a blueprint
type SourceV1Alpha1 struct {
	Name       string `yaml:"name"`                 // The Name of the source
	Url        string `yaml:"url"`                  // The URL of the source
	PathPrefix string `yaml:"pathPrefix,omitempty"` // The Path Prefix of the source
	Ref        string `yaml:"ref"`                  // The Ref of the source
}

// TerraformComponent describes a Terraform component for a blueprint
type TerraformComponentV1Alpha1 struct {
	Source    string                               `yaml:"source,omitempty"`    // The Source of the module
	Path      string                               `yaml:"path"`                // The Path of the module
	FullPath  string                               `yaml:"-"`                   // The Full Path of the module
	Values    map[string]interface{}               `yaml:"values,omitempty"`    // The Values for the module
	Variables map[string]TerraformVariableV1Alpha1 `yaml:"variables,omitempty"` // The Variables for the module
}

// TerraformVariable describes a Terraform variable for a Terraform component
type TerraformVariableV1Alpha1 struct {
	Type        string      `yaml:"type,omitempty"`        // The Type of the variable
	Default     interface{} `yaml:"default,omitempty"`     // The Default value of the variable
	Description string      `yaml:"description,omitempty"` // The Description of the variable
	Sensitive   bool        `yaml:"sensitive,omitempty"`   // Whether to treat the variable as sensitive
}

// Merge merges another BlueprintV1Alpha1 into the current one.
func (b *BlueprintV1Alpha1) Merge(overlay *BlueprintV1Alpha1) {
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

	// Merge Sources by "name"
	sourceMap := make(map[string]SourceV1Alpha1)
	for _, source := range b.Sources {
		sourceMap[source.Name] = source
	}
	for _, overlaySource := range overlay.Sources {
		if overlaySource.Name != "" {
			sourceMap[overlaySource.Name] = overlaySource
		}
	}
	b.Sources = make([]SourceV1Alpha1, 0, len(sourceMap))
	for _, source := range sourceMap {
		b.Sources = append(b.Sources, source)
	}

	// Merge TerraformComponents by "path" as primary key and "source" as secondary key
	componentMap := make(map[string]TerraformComponentV1Alpha1)
	for _, component := range b.TerraformComponents {
		key := component.Path
		componentMap[key] = component
	}
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
					mergedComponent.Variables = make(map[string]TerraformVariableV1Alpha1)
				}
				for k, v := range overlayComponent.Variables {
					mergedComponent.Variables[k] = v
				}

				if overlayComponent.FullPath != "" {
					mergedComponent.FullPath = overlayComponent.FullPath
				}
				componentMap[key] = mergedComponent
			} else {
				// Use the overlay component if the path matches but the source doesn't
				componentMap[key] = overlayComponent
			}
		} else {
			// Add the overlay component if it doesn't exist in the target
			componentMap[key] = overlayComponent
		}
	}
	b.TerraformComponents = make([]TerraformComponentV1Alpha1, 0, len(componentMap))
	for _, component := range componentMap {
		b.TerraformComponents = append(b.TerraformComponents, component)
	}
}

// Copy creates a deep copy of the BlueprintV1Alpha1.
func (b *BlueprintV1Alpha1) Copy() *BlueprintV1Alpha1 {
	if b == nil {
		return nil
	}

	// Copy Metadata
	metadataCopy := MetadataV1Alpha1{
		Name:        b.Metadata.Name,
		Description: b.Metadata.Description,
		Authors:     append([]string{}, b.Metadata.Authors...),
	}

	// Copy Sources
	sourcesCopy := make([]SourceV1Alpha1, len(b.Sources))
	for i, source := range b.Sources {
		sourcesCopy[i] = SourceV1Alpha1{
			Name:       source.Name,
			Url:        source.Url,
			PathPrefix: source.PathPrefix,
			Ref:        source.Ref,
		}
	}

	// Copy TerraformComponents
	terraformComponentsCopy := make([]TerraformComponentV1Alpha1, len(b.TerraformComponents))
	for i, component := range b.TerraformComponents {
		variablesCopy := make(map[string]TerraformVariableV1Alpha1, len(component.Variables))
		for key, variable := range component.Variables {
			variablesCopy[key] = TerraformVariableV1Alpha1{
				Type:        variable.Type,
				Default:     variable.Default,
				Description: variable.Description,
			}
		}

		valuesCopy := make(map[string]interface{}, len(component.Values))
		for key, value := range component.Values {
			valuesCopy[key] = value
		}

		terraformComponentsCopy[i] = TerraformComponentV1Alpha1{
			Source:    component.Source,
			Path:      component.Path,
			FullPath:  component.FullPath,
			Values:    valuesCopy,
			Variables: variablesCopy,
		}
	}

	return &BlueprintV1Alpha1{
		Kind:                b.Kind,
		ApiVersion:          b.ApiVersion,
		Metadata:            metadataCopy,
		Sources:             sourcesCopy,
		TerraformComponents: terraformComponentsCopy,
	}
}
