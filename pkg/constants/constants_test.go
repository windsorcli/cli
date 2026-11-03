package constants_test

import (
	"testing"

	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestParseGitopsMode(t *testing.T) {
	cases := map[string]constants.GitopsMode{
		"push":    constants.GitopsModePush,
		"pull":    constants.GitopsModePull,
		"":        constants.GitopsModePull,
		"pushh":   constants.GitopsModePull,
		"unknown": constants.GitopsModePull,
	}
	for in, want := range cases {
		t.Run("Parses"+in, func(t *testing.T) {
			// Given the config value in

			// When ParseGitopsMode is called
			got := constants.ParseGitopsMode(in)

			// Then the expected mode is returned
			if got != want {
				t.Errorf("ParseGitopsMode(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

func TestFluxKustomizationInterval(t *testing.T) {
	t.Run("ReturnsPrimaryIntervalForPrimarySource", func(t *testing.T) {
		// Given a Kustomization of the blueprint repository

		// When FluxKustomizationInterval is called
		got := constants.FluxKustomizationInterval(true)

		// Then the primary interval is returned
		if got != constants.DefaultFluxPrimaryKustomizationInterval {
			t.Errorf("expected %v, got %v", constants.DefaultFluxPrimaryKustomizationInterval, got)
		}
	})

	t.Run("ReturnsVendorIntervalForVendorSource", func(t *testing.T) {
		// Given a Kustomization of a vendor source

		// When FluxKustomizationInterval is called
		got := constants.FluxKustomizationInterval(false)

		// Then the vendor interval is returned
		if got != constants.DefaultFluxKustomizationInterval {
			t.Errorf("expected %v, got %v", constants.DefaultFluxKustomizationInterval, got)
		}
	})
}

func TestFluxSourceInterval(t *testing.T) {
	t.Run("ReturnsPrimaryIntervalForPrimaryRepository", func(t *testing.T) {
		// Given the repository of the blueprint

		// When FluxSourceInterval is called
		got := constants.FluxSourceInterval(true)

		// Then the primary repository interval is returned
		if got != constants.DefaultFluxPrimaryRepositoryInterval {
			t.Errorf("expected %v, got %v", constants.DefaultFluxPrimaryRepositoryInterval, got)
		}
	})

	t.Run("ReturnsVendorIntervalForVendorSource", func(t *testing.T) {
		// Given a vendor source

		// When FluxSourceInterval is called
		got := constants.FluxSourceInterval(false)

		// Then the vendor interval is returned
		if got != constants.DefaultFluxSourceInterval {
			t.Errorf("expected %v, got %v", constants.DefaultFluxSourceInterval, got)
		}
	})
}

func TestGetEffectiveBlueprintURL(t *testing.T) {
	t.Run("ReturnsDefaultWhenNotPinned", func(t *testing.T) {
		// Given no pinned blueprint URL
		setPinnedBlueprintURL(t, "")

		// When GetEffectiveBlueprintURL is called
		got := constants.GetEffectiveBlueprintURL()

		// Then the default blueprint URL is returned
		if got != constants.DefaultOCIBlueprintURL {
			t.Errorf("expected %q, got %q", constants.DefaultOCIBlueprintURL, got)
		}
	})

	t.Run("ReturnsPinnedURLWhenSet", func(t *testing.T) {
		// Given a pinned blueprint URL
		pinned := "oci://ghcr.io/windsorcli/core:v1.2.3"
		setPinnedBlueprintURL(t, pinned)

		// When GetEffectiveBlueprintURL is called
		got := constants.GetEffectiveBlueprintURL()

		// Then the pinned blueprint URL is returned
		if got != pinned {
			t.Errorf("expected %q, got %q", pinned, got)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func setPinnedBlueprintURL(t *testing.T, v string) {
	t.Helper()
	old := constants.PinnedBlueprintURL
	constants.PinnedBlueprintURL = v
	t.Cleanup(func() { constants.PinnedBlueprintURL = old })
}
