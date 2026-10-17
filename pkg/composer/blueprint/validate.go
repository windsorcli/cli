// The ValidateComposedBlueprint function checks the structure of a composed blueprint.
// It provides the check that the backend field names a declared terraform component.
// A failure wraps ErrBlueprintInvalid, which callers detect with errors.Is.
// A nil blueprint and a blueprint with no backend pass the check.

package blueprint

import (
	"errors"
	"fmt"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Errors
// =============================================================================

// ErrBlueprintInvalid is the sentinel that wraps a structural error in a composed blueprint.
var ErrBlueprintInvalid = errors.New("invalid blueprint")

// =============================================================================
// Helpers
// =============================================================================

// ValidateComposedBlueprint rejects composed blueprints whose Backend field names a
// component that does not exist. Nil and empty-Backend blueprints are accepted.
// Failures wrap ErrBlueprintInvalid.
func ValidateComposedBlueprint(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil || blueprint.Backend == "" {
		return nil
	}

	for i := range blueprint.TerraformComponents {
		if blueprint.TerraformComponents[i].GetID() == blueprint.Backend {
			return nil
		}
	}

	return fmt.Errorf(
		"%w\n\nBlueprint configuration: backend names terraform component %q but no component with that ID is declared. Set backend to the ID of an existing terraform component, or remove the backend field to opt out of the in-blueprint backend.",
		ErrBlueprintInvalid, blueprint.Backend,
	)
}
