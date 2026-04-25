package blueprint

import (
	"errors"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestValidateComposedBlueprint(t *testing.T) {
	t.Run("NilBlueprintIsAccepted", func(t *testing.T) {
		// Given a nil blueprint (e.g. LoadBlueprint never ran)
		// When validation runs
		// Then no error is returned — there is nothing to validate yet
		if err := ValidateComposedBlueprint(nil); err != nil {
			t.Errorf("Expected nil error for nil blueprint, got %v", err)
		}
	})

	t.Run("BlueprintWithoutBackendComponentIsAccepted", func(t *testing.T) {
		// Given a blueprint that does not declare a "backend" terraform component —
		// the canonical case for blueprints whose remote state lives out-of-band or
		// that simply use local state.
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "cluster"},
			},
		}

		// When validation runs
		// Then no error is returned
		if err := ValidateComposedBlueprint(bp); err != nil {
			t.Errorf("Expected nil error for blueprint without backend component, got %v", err)
		}
	})

	t.Run("BackendAtIndexZeroIsAccepted", func(t *testing.T) {
		// Given a blueprint with the backend component at index 0
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Path: "vpc"},
				{Path: "cluster"},
			},
		}

		// When validation runs
		// Then no error is returned
		if err := ValidateComposedBlueprint(bp); err != nil {
			t.Errorf("Expected nil error for backend at index 0, got %v", err)
		}
	})

	t.Run("BackendNotAtIndexZeroFails", func(t *testing.T) {
		// Given a blueprint where "backend" is not the first component
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "iam"},
				{Path: "backend"},
			},
		}

		// When validation runs
		err := ValidateComposedBlueprint(bp)

		// Then a wrapped ErrBlueprintInvalid is returned with the offending position
		if err == nil {
			t.Fatal("Expected error for backend not at index 0, got nil")
		}
		if !errors.Is(err, ErrBlueprintInvalid) {
			t.Errorf("Expected error wrapping ErrBlueprintInvalid, got %v", err)
		}
		if !strings.Contains(err.Error(), "position 2") {
			t.Errorf("Expected error to name position 2, got %v", err)
		}
	})

	t.Run("MultipleBackendComponentsFail", func(t *testing.T) {
		// Given a blueprint with two components named "backend" (one via Path, one via
		// Name override)
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "backend"},
				{Name: "backend", Path: "secondary-backend"},
			},
		}

		// When validation runs
		err := ValidateComposedBlueprint(bp)

		// Then a wrapped ErrBlueprintInvalid is returned naming the duplicates
		if err == nil {
			t.Fatal("Expected error for duplicate backend components, got nil")
		}
		if !errors.Is(err, ErrBlueprintInvalid) {
			t.Errorf("Expected error wrapping ErrBlueprintInvalid, got %v", err)
		}
		if !strings.Contains(err.Error(), "appears 2 times") {
			t.Errorf("Expected error to name duplicate count, got %v", err)
		}
	})

	t.Run("BackendIdentifiedByName", func(t *testing.T) {
		// Given a blueprint where the backend component is identified via Name (not
		// Path) — GetID() returns Name when set, so the rule applies symmetrically.
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Name: "backend", Path: "modules/s3-backend"},
			},
		}

		// When validation runs
		err := ValidateComposedBlueprint(bp)

		// Then the position-1 placement is rejected even though Path != "backend"
		if !errors.Is(err, ErrBlueprintInvalid) {
			t.Errorf("Expected error for Name=backend at index 1, got %v", err)
		}
	})
}
