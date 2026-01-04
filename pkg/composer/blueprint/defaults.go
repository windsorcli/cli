package blueprint

import blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"

// DefaultBlueprint provides the base blueprint structure used when no blueprint exists.
var DefaultBlueprint = blueprintv1alpha1.Blueprint{
	Kind:       "Blueprint",
	ApiVersion: "blueprints.windsorcli.dev/v1alpha1",
	Metadata: blueprintv1alpha1.Metadata{
		Name:        "default",
		Description: "A default blueprint",
	},
	Sources:             []blueprintv1alpha1.Source{},
	TerraformComponents: []blueprintv1alpha1.TerraformComponent{},
	Kustomizations:      []blueprintv1alpha1.Kustomization{},
}
