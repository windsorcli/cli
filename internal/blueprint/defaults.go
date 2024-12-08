package blueprint

var DefaultBlueprint = BlueprintV1Alpha1{
	Kind:       "Blueprint",
	ApiVersion: "blueprints.windsorcli.dev/v1alpha1",
	Metadata: MetadataV1Alpha1{
		Name:        "default",
		Description: "A default blueprint",
		Authors:     []string{},
	},
	Sources:             []SourceV1Alpha1{},
	TerraformComponents: []TerraformComponentV1Alpha1{},
}
