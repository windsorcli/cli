// The DefaultBlueprint is the blueprint that Windsor composes when no blueprint exists.
// It provides the kind, API version, and metadata of an empty blueprint.
// It declares no sources, terraform components, or kustomizations.
// Callers deep-copy it before they change it.

package blueprint

import blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"

// =============================================================================
// Constants
// =============================================================================

// DefaultBlueprint is the blueprint that Windsor composes when no blueprint exists.
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
