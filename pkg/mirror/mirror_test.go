package mirror

import (
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Helpers
// =============================================================================

func TestMirror_extractBlueprintSeeds(t *testing.T) {
	t.Run("FiltersOCISources", func(t *testing.T) {
		// Given a blueprint with two OCI sources and one non-OCI source
		bp := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "a", Url: "oci://ghcr.io/x/a:v1"},
				{Name: "b", Url: "https://example.com/b.tgz"},
				{Name: "c", Url: "oci://ghcr.io/x/c:v2"},
			},
		}

		// When extracting seeds
		got := extractBlueprintSeeds(bp)

		// Then only the OCI entries are returned in order
		if len(got) != 2 {
			t.Fatalf("expected 2 seeds, got %d: %v", len(got), got)
		}
		if got[0] != "oci://ghcr.io/x/a:v1" || got[1] != "oci://ghcr.io/x/c:v2" {
			t.Errorf("unexpected seeds: %v", got)
		}
	})

	t.Run("NilBlueprintYieldsEmpty", func(t *testing.T) {
		// Given a nil blueprint
		// When extracting
		got := extractBlueprintSeeds(nil)
		// Then result is empty
		if len(got) != 0 {
			t.Errorf("expected empty, got %v", got)
		}
	})
}
