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
	Name        string   `yaml:"name"`        // The Name of the blueprint
	Description string   `yaml:"description"` // The Description of the blueprint
	Authors     []string `yaml:"authors"`     // The Authors of the blueprint
}

// Source describes a source for a blueprint
type SourceV1Alpha1 struct {
	Name    string `yaml:"name"`    // The Name of the source
	Url     string `yaml:"url"`     // The URL of the source
	Version string `yaml:"version"` // The Version of the source
}

// TerraformComponent describes a Terraform component for a blueprint
type TerraformComponentV1Alpha1 struct {
	Name    string                 `yaml:"name"`    // The Name of the module
	Source  string                 `yaml:"source"`  // The Source of the module
	Version string                 `yaml:"version"` // The Version of the module
	Values  map[string]interface{} `yaml:"values"`  // The Values for the module
}

// TerraformVariable describes a Terraform variable for a Terraform component
type TerraformVariableV1Alpha1 struct {
	Name        string      `yaml:"name"`        // The Name of the variable
	Type        string      `yaml:"type"`        // The Type of the variable
	Default     interface{} `yaml:"default"`     // The Default value of the variable
	Description string      `yaml:"description"` // The Description of the variable
}
