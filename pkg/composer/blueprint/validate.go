package blueprint

import (
	"errors"
	"fmt"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Errors
// =============================================================================

// ErrBlueprintInvalid is the sentinel returned by blueprint validation when the composed
// blueprint violates a structural invariant. Callers use errors.Is to detect this class
// of failure and present the wrapped message to the operator without scary "Error:"
// framing — the run is rejected, but the cause is a blueprint authoring mistake the
// operator can fix, not an internal exception.
var ErrBlueprintInvalid = errors.New("invalid blueprint")

// =============================================================================
// Public Helpers
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
		"%w\n\nBlueprint configuration: backend names terraform component %q but no component with that ID is declared. Set backend to the ID of an existing terraform component, or remove the backend field to opt out of the in-blueprint backend tier.",
		ErrBlueprintInvalid, blueprint.Backend,
	)
}
