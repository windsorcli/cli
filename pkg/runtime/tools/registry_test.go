package tools

import (
	"strings"
	"testing"
)

// =============================================================================
// Test Helpers
// =============================================================================

// TestMissingToolError pins the formatting contract: registered keys produce errors that
// carry the display name, minimum version, and vendor download URL; an unregistered key
// falls back to a bare error so a programmer typo in a check* function still produces a
// readable message instead of panicking.
func TestMissingToolError(t *testing.T) {
	t.Run("RegisteredKeyIncludesVendorInstallHint", func(t *testing.T) {
		// Given a key that is in the registry
		err := missingToolError("terraform")

		// Then the error includes the display name, minimum version, and vendor download URL
		// — the exact pieces operators need to resolve the failure via the vendor's own
		// install instructions (which encode platform-specific nuance we don't replicate).
		msg := err.Error()
		expected := []string{"Terraform", "1.7.0", "https://developer.hashicorp.com/terraform/install", "not found on PATH"}
		for _, want := range expected {
			if !strings.Contains(msg, want) {
				t.Errorf("expected error to contain %q, got: %s", want, msg)
			}
		}
	})

	t.Run("RegisteredKeyOmitsThirdPartyPackageManagers", func(t *testing.T) {
		// Given a key that is in the registry
		err := missingToolError("terraform")

		// Then the error must NOT mention third-party package managers (aqua, brew, etc.).
		// Operators get the vendor's authoritative install page only — pointing at a
		// distribution wrapper would put us on the hook for that wrapper's signing,
		// version-staleness, and platform-coverage gaps.
		msg := err.Error()
		for _, banned := range []string{"aqua g -i", "brew install", "Or via aqua"} {
			if strings.Contains(msg, banned) {
				t.Errorf("expected error to OMIT %q, got: %s", banned, msg)
			}
		}
	})

	t.Run("UnregisteredKeyFallsBackToBareMessage", func(t *testing.T) {
		// Given a key that is NOT in the registry (simulating a programmer typo in a
		// future check* function)
		err := missingToolError("nonexistent-tool")

		// Then the error still names the key and indicates it is required — no panic,
		// no install-hint scaffolding pretending to exist for a tool we know nothing about.
		msg := err.Error()
		if !strings.Contains(msg, "nonexistent-tool") {
			t.Errorf("expected fallback to mention the key name, got: %s", msg)
		}
		if strings.Contains(msg, "Install:") {
			t.Errorf("expected fallback to OMIT install hints, got: %s", msg)
		}
	})
}

// TestOutdatedToolError pins the format for the version-too-low case, including the
// behavior when the version extractor returned an empty string (rendered as "(unknown)"
// rather than producing a malformed message).
func TestOutdatedToolError(t *testing.T) {
	t.Run("RegisteredKeyShowsFoundAndMinimumVersions", func(t *testing.T) {
		// Given a registered key and a found version below minimum
		err := outdatedToolError("docker", "20.10.0")

		// Then both the found version and the minimum show up alongside the vendor install URL
		msg := err.Error()
		expected := []string{"Docker", "20.10.0", "23.0.0", "below the minimum required version", "https://www.docker.com/products/docker-desktop/"}
		for _, want := range expected {
			if !strings.Contains(msg, want) {
				t.Errorf("expected error to contain %q, got: %s", want, msg)
			}
		}
		for _, banned := range []string{"aqua g -i", "Or via aqua"} {
			if strings.Contains(msg, banned) {
				t.Errorf("expected error to OMIT %q, got: %s", banned, msg)
			}
		}
	})

	t.Run("EmptyFoundVersionRendersAsUnknown", func(t *testing.T) {
		// Given a registered key but extractVersion returned ""
		err := outdatedToolError("sops", "")

		// Then the message stays readable instead of producing "SOPS  is below..."
		msg := err.Error()
		if !strings.Contains(msg, "(unknown)") {
			t.Errorf("expected empty version to render as (unknown), got: %s", msg)
		}
	})

	t.Run("UnregisteredKeyFallsBackToBareMessage", func(t *testing.T) {
		// Given a key that is NOT in the registry
		err := outdatedToolError("ghost-tool", "0.0.1")

		// Then the bare fallback names the key and the found version, no install hints
		msg := err.Error()
		if !strings.Contains(msg, "ghost-tool") || !strings.Contains(msg, "0.0.1") {
			t.Errorf("expected fallback to mention key + version, got: %s", msg)
		}
		if strings.Contains(msg, "Install:") {
			t.Errorf("expected fallback to OMIT install hints, got: %s", msg)
		}
	})
}

// TestToolRegistry_KeysMatchCheckFunctions guards against drift between the toolRegistry
// keys and the strings each check* function passes to execLookPath. If a check* function
// is added that uses an unregistered key, errors silently degrade to the bare-fallback
// format with no install hints — that's the failure mode this test catches at CI time.
func TestToolRegistry_KeysMatchCheckFunctions(t *testing.T) {
	// Each entry is a binary name passed to execLookPath inside a check* function.
	// "tofu" is included alongside "terraform" because GetTerraformCommand can return
	// either depending on the configured driver; both must format errors identically.
	expectedKeys := []string{"docker", "colima", "limactl", "terraform", "tofu", "op", "sops", "kubelogin", "aws"}
	for _, key := range expectedKeys {
		if _, ok := toolRegistry[key]; !ok {
			t.Errorf("toolRegistry is missing %q — a check* function looks up this binary, so a missing entry would degrade its error to the bare fallback (no Download / aqua hints)", key)
		}
	}
}
