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

// ValidateComposedBlueprint enforces structural invariants on a composed blueprint.
// Today the only invariant is the "backend" terraform component placement: when a
// component whose GetID() is "backend" exists, it must be the first item in
// terraformComponents and there must be exactly one such component. Subsequent
// components implicitly depend on the backend bootstrapping the remote state store,
// so misordering or duplication produces a state-storage flow that cannot work.
// Returns nil for blueprints without a backend component (out-of-band buckets are a
// supported workflow). All failures wrap ErrBlueprintInvalid so the cmd layer can
// route them through the calm-output presenter.
func ValidateComposedBlueprint(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return nil
	}

	var backendIndices []int
	for i := range blueprint.TerraformComponents {
		c := blueprint.TerraformComponents[i]
		if c.GetID() == "backend" {
			backendIndices = append(backendIndices, i)
		}
	}

	switch len(backendIndices) {
	case 0:
		return nil
	case 1:
		if backendIndices[0] != 0 {
			// 1-based position in the operator-facing message: YAML authors count their list
			// entries from 1, so "position 1" must refer to the first item. Reporting the raw
			// 0-based index reads as if the offending component is one slot earlier than it
			// actually is.
			return fmt.Errorf(
				"%w\n\nBlueprint configuration: terraform component \"backend\" needs to be the first item in terraformComponents (currently at position %d). The backend component bootstraps the remote state store, so subsequent components depend on it being applied first. Reorder your blueprint so \"backend\" appears first, then re-run.",
				ErrBlueprintInvalid, backendIndices[0]+1,
			)
		}
		return nil
	default:
		oneBasedPositions := make([]int, len(backendIndices))
		for i, idx := range backendIndices {
			oneBasedPositions[i] = idx + 1
		}
		return fmt.Errorf(
			"%w\n\nBlueprint configuration: terraform component \"backend\" appears %d times in terraformComponents (positions %v). Exactly one backend component is allowed; remove the duplicates and re-run.",
			ErrBlueprintInvalid, len(backendIndices), oneBasedPositions,
		)
	}
}
