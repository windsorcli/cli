package hetzner

import (
	"testing"
)

func TestHetznerConfig_Merge(t *testing.T) {
	t.Run("OverlayTokenReplacesBase", func(t *testing.T) {
		base := &HetznerConfig{Token: ptrString("base-token")}
		overlay := &HetznerConfig{Token: ptrString("overlay-token")}

		base.Merge(overlay)

		if base.Token == nil || *base.Token != "overlay-token" {
			t.Errorf("Token mismatch: expected 'overlay-token', got %v", base.Token)
		}
	})

	t.Run("NilOverlayTokenLeavesBase", func(t *testing.T) {
		base := &HetznerConfig{Token: ptrString("base-token")}
		overlay := &HetznerConfig{Token: nil}

		base.Merge(overlay)

		if base.Token == nil || *base.Token != "base-token" {
			t.Errorf("Token mismatch: expected 'base-token', got %v", base.Token)
		}
	})

	t.Run("NilOverlayLeavesBaseUnchanged", func(t *testing.T) {
		base := &HetznerConfig{Token: ptrString("base-token")}

		base.Merge(nil)

		if base.Token == nil || *base.Token != "base-token" {
			t.Errorf("Token mismatch: expected 'base-token', got %v", base.Token)
		}
	})
}

func TestHetznerConfig_Copy(t *testing.T) {
	t.Run("CopyWithToken", func(t *testing.T) {
		original := &HetznerConfig{Token: ptrString("test-token")}

		copied := original.Copy()

		if copied == nil {
			t.Fatal("Expected non-nil copy")
		}
		if copied.Token == nil || *copied.Token != "test-token" {
			t.Errorf("Token mismatch: expected 'test-token', got %v", copied.Token)
		}

		copied.Token = ptrString("modified-token")
		if *original.Token != "test-token" {
			t.Errorf("Original Token was modified: got %v", *original.Token)
		}
	})

	t.Run("CopyNil", func(t *testing.T) {
		var original *HetznerConfig = nil

		if copied := original.Copy(); copied != nil {
			t.Errorf("Copy of nil should be nil, got %v", copied)
		}
	})
}

func ptrString(s string) *string {
	return &s
}
