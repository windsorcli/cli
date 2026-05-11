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

	t.Run("BlueprintWithoutBackendFieldIsAccepted", func(t *testing.T) {
		// Given a blueprint that omits the Backend field — the "external backend" case,
		// where every component uses the configured terraform.backend.* directly.
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "cluster"},
			},
		}

		if err := ValidateComposedBlueprint(bp); err != nil {
			t.Errorf("Expected nil error for blueprint without Backend, got %v", err)
		}
	})

	t.Run("BackendResolvesToDeclaredComponent", func(t *testing.T) {
		// Given Blueprint.Backend names a component that exists in TerraformComponents
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "cluster",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Name: "cluster", Path: "cluster/eks"},
				{Path: "workloads"},
			},
		}

		if err := ValidateComposedBlueprint(bp); err != nil {
			t.Errorf("Expected nil error when Backend resolves, got %v", err)
		}
	})

	t.Run("BackendNotInTerraformComponentsFails", func(t *testing.T) {
		// Given Blueprint.Backend names a component that does not exist
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "ghost",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "cluster"},
			},
		}

		err := ValidateComposedBlueprint(bp)
		if err == nil {
			t.Fatal("Expected error when Backend does not resolve, got nil")
		}
		if !errors.Is(err, ErrBlueprintInvalid) {
			t.Errorf("Expected error wrapping ErrBlueprintInvalid, got %v", err)
		}
		if !strings.Contains(err.Error(), `"ghost"`) {
			t.Errorf("Expected error to name the unresolved ID %q, got %v", "ghost", err)
		}
	})

	t.Run("BackendResolvesByPathWhenNameUnset", func(t *testing.T) {
		// Given a component whose ID is its Path (no Name override)
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "terraform/backend",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "terraform/backend"},
			},
		}

		if err := ValidateComposedBlueprint(bp); err != nil {
			t.Errorf("Expected nil error when Backend resolves to a path-only component, got %v", err)
		}
	})

	t.Run("BackendResolvesByNameWhenPathDiffers", func(t *testing.T) {
		// Given a Name override where GetID() returns Name
		bp := &blueprintv1alpha1.Blueprint{
			Backend: "state-bucket",
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "state-bucket", Path: "modules/s3-backend"},
				{Path: "vpc"},
			},
		}

		if err := ValidateComposedBlueprint(bp); err != nil {
			t.Errorf("Expected nil error when Backend resolves to a Name override, got %v", err)
		}
	})

	t.Run("NamingConventionIsNotRecognised", func(t *testing.T) {
		// Hard cut: a component named "backend" without Blueprint.Backend set is NOT
		// treated as a backend. The blueprint validates cleanly; the orchestration
		// just won't pivot — the component is a regular non-tier component.
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "backend"},
			},
		}

		if err := ValidateComposedBlueprint(bp); err != nil {
			t.Errorf("Expected nil error (naming convention retired), got %v", err)
		}
	})
}
