package blueprint_test

import (
	"errors"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestValidateComposedBlueprint(t *testing.T) {
	t.Run("NilBlueprintIsAccepted", func(t *testing.T) {
		// Given a nil blueprint (e.g. LoadBlueprint never ran)
		// When validation runs
		// Then no error is returned — there is nothing to validate yet
		if err := blueprint.ValidateComposedBlueprint(nil); err != nil {
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

		if err := blueprint.ValidateComposedBlueprint(bp); err != nil {
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

		if err := blueprint.ValidateComposedBlueprint(bp); err != nil {
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

		err := blueprint.ValidateComposedBlueprint(bp)
		if err == nil {
			t.Fatal("Expected error when Backend does not resolve, got nil")
		}
		if !errors.Is(err, blueprint.ErrBlueprintInvalid) {
			t.Errorf("Expected error wrapping ErrBlueprintInvalid, got %v", err)
		}
		if !strings.Contains(err.Error(), `"ghost"`) {
			t.Errorf("Expected error to name the unresolved ID %q, got %v", "ghost", err)
		}
		if werr := werror.Find(err); werr == nil || !strings.Contains(werr.Remediation, "Set backend") {
			t.Errorf("Expected remediation containing %q, got %v", "Set backend", werr)
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

		if err := blueprint.ValidateComposedBlueprint(bp); err != nil {
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

		if err := blueprint.ValidateComposedBlueprint(bp); err != nil {
			t.Errorf("Expected nil error when Backend resolves to a Name override, got %v", err)
		}
	})

	t.Run("NamingConventionIsNotRecognised", func(t *testing.T) {
		// Hard cut: a component named "backend" without Blueprint.Backend set is NOT
		// treated as a backend. The blueprint validates cleanly; the orchestration
		// just won't pivot — the component is a regular non-backend component.
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc"},
				{Path: "backend"},
			},
		}

		if err := blueprint.ValidateComposedBlueprint(bp); err != nil {
			t.Errorf("Expected nil error (naming convention retired), got %v", err)
		}
	})

	t.Run("TLSSecretWithBothKeysIsAccepted", func(t *testing.T) {
		// Given a merged kubernetes.io/tls secret with both data keys
		bp := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{{
				Name:    "gateway",
				Secrets: map[string]blueprintv1alpha1.SecretEntry{"web-tls": {Type: "kubernetes.io/tls", Data: map[string]string{"tls.crt": "${c}", "tls.key": "${k}"}}},
			}},
		}

		// When validation runs
		// Then no error is returned
		if err := blueprint.ValidateComposedBlueprint(bp); err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("TLSSecretWithoutKeyIsRejected", func(t *testing.T) {
		// Given a merged kubernetes.io/tls secret with no tls.key
		bp := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{{
				Name:    "gateway",
				Secrets: map[string]blueprintv1alpha1.SecretEntry{"web-tls": {Type: "kubernetes.io/tls", Data: map[string]string{"tls.crt": "${c}"}}},
			}},
		}

		// When validation runs
		err := blueprint.ValidateComposedBlueprint(bp)

		// Then the error names the missing key and wraps ErrBlueprintInvalid
		if err == nil || !strings.Contains(err.Error(), "no tls.key data key") {
			t.Fatalf("Expected missing tls.key error, got %v", err)
		}
		if !errors.Is(err, blueprint.ErrBlueprintInvalid) {
			t.Errorf("Expected error to wrap ErrBlueprintInvalid, got %v", err)
		}
	})

	t.Run("AcceptsABackendThatNamesAComponent", func(t *testing.T) {
		// Given a blueprint whose backend names a declared component
		bp := &blueprintv1alpha1.Blueprint{Backend: "state", TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Name: "state", Path: "state"}}}

		// When it is validated
		err := blueprint.ValidateComposedBlueprint(bp)

		// Then it is accepted
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("AcceptsANilBlueprintAndAnEmptyBackend", func(t *testing.T) {
		// Given a nil blueprint and a blueprint with no backend
		// When both are validated
		nilErr := blueprint.ValidateComposedBlueprint(nil)
		emptyErr := blueprint.ValidateComposedBlueprint(&blueprintv1alpha1.Blueprint{})

		// Then both are accepted
		if nilErr != nil || emptyErr != nil {
			t.Errorf("expected no errors, got %v and %v", nilErr, emptyErr)
		}
	})

	t.Run("RejectsABackendThatNamesNoComponent", func(t *testing.T) {
		// Given a blueprint whose backend names no declared component
		bp := &blueprintv1alpha1.Blueprint{Backend: "missing"}

		// When it is validated
		err := blueprint.ValidateComposedBlueprint(bp)

		// Then it is rejected as invalid
		if !errors.Is(err, blueprint.ErrBlueprintInvalid) {
			t.Errorf("expected ErrBlueprintInvalid, got %v", err)
		}
	})
}
