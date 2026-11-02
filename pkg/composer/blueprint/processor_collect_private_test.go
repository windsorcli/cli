package blueprint

import (
	"slices"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestMergeSecretData(t *testing.T) {
	t.Run("UnionsSecretsAndDataKeysWithOverlayWinning", func(t *testing.T) {
		// Given two facets contributing secrets: a shared secret with an overlapping data key,
		// and a secret unique to each side
		base := map[string]blueprintv1alpha1.SecretEntry{
			"shared":    {Data: map[string]string{"a": "1", "b": "2"}},
			"base-only": {Data: map[string]string{"x": "9"}},
		}
		overlay := map[string]blueprintv1alpha1.SecretEntry{
			"shared":       {Data: map[string]string{"b": "override", "c": "3"}},
			"overlay-only": {Data: map[string]string{"y": "8"}},
		}

		// When merging
		got := mergeSecretData(base, overlay)

		// Then secrets union, data keys union, and overlay wins the data-key conflict
		if got["shared"].Data["a"] != "1" || got["shared"].Data["b"] != "override" || got["shared"].Data["c"] != "3" {
			t.Errorf("Unexpected shared secret merge: %v", got["shared"])
		}
		if got["base-only"].Data["x"] != "9" || got["overlay-only"].Data["y"] != "8" {
			t.Errorf("Expected unique secrets preserved, got %v", got)
		}
	})

	t.Run("UnionsNamespacesForASharedSecret", func(t *testing.T) {
		// Given two facets targeting the same secret at different namespaces
		base := map[string]blueprintv1alpha1.SecretEntry{"acme-dns": {Namespaces: []string{"system-pki"}, Data: map[string]string{"token": "${x.t}"}}}
		overlay := map[string]blueprintv1alpha1.SecretEntry{"acme-dns": {Namespaces: []string{"system-dns", "system-pki"}, Data: map[string]string{"token": "${x.t}"}}}

		// When merging
		got := mergeSecretData(base, overlay)

		// Then the namespaces union without duplicating the shared one
		ns := got["acme-dns"].Namespaces
		if len(ns) != 2 || !slices.Contains(ns, "system-pki") || !slices.Contains(ns, "system-dns") {
			t.Errorf("Expected namespaces to union to [system-pki system-dns], got %v", ns)
		}
	})

	t.Run("OverlayTypeWinsAndEmptyTypeKeepsBase", func(t *testing.T) {
		// Given a typed base secret and an untyped overlay, plus an untyped base and a typed overlay
		base := map[string]blueprintv1alpha1.SecretEntry{
			"typed-base":   {Type: "kubernetes.io/tls", Data: map[string]string{"tls.crt": "${a}"}},
			"untyped-base": {Data: map[string]string{"tls.crt": "${a}"}},
		}
		overlay := map[string]blueprintv1alpha1.SecretEntry{
			"typed-base":   {Data: map[string]string{"tls.key": "${b}"}},
			"untyped-base": {Type: "kubernetes.io/tls", Data: map[string]string{"tls.key": "${b}"}},
		}

		// When merging
		got := mergeSecretData(base, overlay)

		// Then both secrets carry the type
		if got["typed-base"].Type != "kubernetes.io/tls" || got["untyped-base"].Type != "kubernetes.io/tls" {
			t.Errorf("Expected both types kubernetes.io/tls, got %q and %q", got["typed-base"].Type, got["untyped-base"].Type)
		}
	})

	t.Run("ReturnsNilWhenBothEmpty", func(t *testing.T) {
		if got := mergeSecretData(nil, nil); got != nil {
			t.Errorf("Expected nil, got %v", got)
		}
	})

	t.Run("DoesNotMutateInputs", func(t *testing.T) {
		// Given a base with a shared secret
		base := map[string]blueprintv1alpha1.SecretEntry{"shared": {Data: map[string]string{"a": "1"}}}
		overlay := map[string]blueprintv1alpha1.SecretEntry{"shared": {Data: map[string]string{"b": "2"}}}

		// When merging
		_ = mergeSecretData(base, overlay)

		// Then the base's inner map is unchanged
		if len(base["shared"].Data) != 1 || base["shared"].Data["a"] != "1" {
			t.Errorf("Expected base unmutated, got %v", base)
		}
		if _, exists := base["shared"].Data["b"]; exists {
			t.Error("Expected base inner map to not gain overlay key")
		}
	})
}
