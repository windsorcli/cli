package blueprint

import (
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestRenderDeferredPlaceholders(t *testing.T) {
	t.Run("ReturnsResourceUnchangedWhenRawMode", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			Substitutions: map[string]string{"key": "${unresolved}"},
		}
		result := RenderDeferredPlaceholders(bp, true, map[string]bool{"substitutions.key": true})
		got := result.(*blueprintv1alpha1.Blueprint)
		if got.Substitutions["key"] != "${unresolved}" {
			t.Errorf("Expected raw value preserved, got '%s'", got.Substitutions["key"])
		}
	})

	t.Run("ReturnsResourceUnchangedWhenNoDeferredPaths", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			Substitutions: map[string]string{"key": "${unresolved}"},
		}
		result := RenderDeferredPlaceholders(bp, false, nil)
		got := result.(*blueprintv1alpha1.Blueprint)
		if got.Substitutions["key"] != "${unresolved}" {
			t.Errorf("Expected value unchanged with empty deferred paths, got '%s'", got.Substitutions["key"])
		}
	})

	t.Run("RewritesDeferredTopLevelSubstitutionToPlaceholder", func(t *testing.T) {
		// Given a blueprint with a deferred top-level substitution
		bp := &blueprintv1alpha1.Blueprint{
			Substitutions: map[string]string{
				"private_dns": "${dns.private}",
				"public_dns":  "8.8.8.8",
			},
		}
		deferredPaths := map[string]bool{"substitutions.private_dns": true}

		// When rendering deferred placeholders
		result := RenderDeferredPlaceholders(bp, false, deferredPaths)

		// Then the deferred key becomes <deferred> and the resolved key is unchanged
		got := result.(*blueprintv1alpha1.Blueprint)
		if got.Substitutions["private_dns"] != deferredPlaceholder {
			t.Errorf("Expected deferred substitution to be '%s', got '%s'", deferredPlaceholder, got.Substitutions["private_dns"])
		}
		if got.Substitutions["public_dns"] != "8.8.8.8" {
			t.Errorf("Expected resolved substitution to be unchanged, got '%s'", got.Substitutions["public_dns"])
		}
	})

	t.Run("DoesNotMutateOriginalBlueprint", func(t *testing.T) {
		// Given a blueprint with a deferred substitution
		bp := &blueprintv1alpha1.Blueprint{
			Substitutions: map[string]string{"private_dns": "${dns.private}"},
		}
		deferredPaths := map[string]bool{"substitutions.private_dns": true}

		// When rendering
		RenderDeferredPlaceholders(bp, false, deferredPaths)

		// Then the original blueprint is not mutated
		if bp.Substitutions["private_dns"] != "${dns.private}" {
			t.Errorf("Original blueprint was mutated: got '%s'", bp.Substitutions["private_dns"])
		}
	})
}
