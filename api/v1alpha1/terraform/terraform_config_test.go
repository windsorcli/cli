package terraform

import (
	"reflect"
	"testing"
)

func TestTerraformConfig_Merge(t *testing.T) {
	t.Run("MergeWithNonNilValues", func(t *testing.T) {
		base := &TerraformConfig{
			Enabled: nil,
			Backend: nil,
		}
		overlay := &TerraformConfig{
			Enabled: ptrBool(true),
			Backend: &BackendConfig{Type: "s3", Prefix: stringPtr("mock-prefix")},
		}
		base.Merge(overlay)
		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Enabled mismatch: expected %v, got %v", true, *base.Enabled)
		}
		if base.Backend == nil || base.Backend.Type != "s3" {
			t.Errorf("Backend mismatch: expected %v, got %v", "s3", base.Backend.Type)
		}
		if base.Backend.Prefix == nil || *base.Backend.Prefix != "mock-prefix" {
			t.Errorf("Prefix mismatch: expected %v, got %v", "mock-prefix", base.Backend.Prefix)
		}
	})

	t.Run("MergeWithNilValues", func(t *testing.T) {
		base := &TerraformConfig{
			Enabled: ptrBool(false),
			Backend: &BackendConfig{Type: "s3", Prefix: stringPtr("base-prefix")},
		}
		overlay := &TerraformConfig{
			Enabled: nil,
			Backend: nil,
		}
		base.Merge(overlay)
		if base.Enabled == nil || *base.Enabled != false {
			t.Errorf("Enabled mismatch: expected %v, got %v", false, *base.Enabled)
		}
		if base.Backend == nil || base.Backend.Type != "s3" {
			t.Errorf("Backend mismatch: expected %v, got %v", "s3", base.Backend.Type)
		}
		if base.Backend.Prefix == nil || *base.Backend.Prefix != "base-prefix" {
			t.Errorf("Prefix mismatch: expected %v, got %v", "base-prefix", base.Backend.Prefix)
		}
	})

	t.Run("MergePicksUpLockFromOverlay", func(t *testing.T) {
		// Given a base with no lock and an overlay with one
		base := &TerraformConfig{}
		overlay := &TerraformConfig{Lock: &LockConfig{Timeout: stringPtr("30s")}}

		// When merging
		base.Merge(overlay)

		// Then the lock surfaces on base
		if base.Lock == nil || base.Lock.Timeout == nil || *base.Lock.Timeout != "30s" {
			t.Fatalf("expected lock timeout 30s, got %+v", base.Lock)
		}
	})

	t.Run("MergeKeepsBaseLockWhenOverlayLockIsNil", func(t *testing.T) {
		// Given a base with a lock and an overlay without one
		base := &TerraformConfig{Lock: &LockConfig{Timeout: stringPtr("10m")}}
		overlay := &TerraformConfig{}

		// When merging
		base.Merge(overlay)

		// Then base's lock is preserved
		if base.Lock == nil || base.Lock.Timeout == nil || *base.Lock.Timeout != "10m" {
			t.Fatalf("expected lock timeout 10m to survive, got %+v", base.Lock)
		}
	})

	t.Run("MergeKeepsBaseTimeoutWhenOverlayLockHasNilTimeout", func(t *testing.T) {
		// Given a base with a timeout and an overlay carrying lock: {} (non-nil
		// LockConfig with Timeout still nil — the gap case from the review)
		base := &TerraformConfig{Lock: &LockConfig{Timeout: stringPtr("10m")}}
		overlay := &TerraformConfig{Lock: &LockConfig{}}

		// When merging
		base.Merge(overlay)

		// Then base's timeout survives — the field-level merge does not let a
		// nil overlay field blank a real base value
		if base.Lock == nil || base.Lock.Timeout == nil || *base.Lock.Timeout != "10m" {
			t.Fatalf("expected lock timeout 10m to survive, got %+v", base.Lock)
		}
	})

	t.Run("MergeOverlayTimeoutWinsWhenBothPresent", func(t *testing.T) {
		// Given a base and overlay each carrying a timeout
		base := &TerraformConfig{Lock: &LockConfig{Timeout: stringPtr("10m")}}
		overlay := &TerraformConfig{Lock: &LockConfig{Timeout: stringPtr("30s")}}

		// When merging
		base.Merge(overlay)

		// Then the overlay's timeout wins
		if base.Lock == nil || base.Lock.Timeout == nil || *base.Lock.Timeout != "30s" {
			t.Fatalf("expected overlay timeout 30s to win, got %+v", base.Lock)
		}
	})

	t.Run("MergeInitialisesBaseLockWhenOnlyOverlayHasIt", func(t *testing.T) {
		// Given a base with no lock and an overlay carrying a timeout
		base := &TerraformConfig{}
		overlay := &TerraformConfig{Lock: &LockConfig{Timeout: stringPtr("45s")}}

		// When merging
		base.Merge(overlay)

		// Then base.Lock is initialised with the overlay's timeout
		if base.Lock == nil || base.Lock.Timeout == nil || *base.Lock.Timeout != "45s" {
			t.Fatalf("expected base.Lock to be initialised with timeout 45s, got %+v", base.Lock)
		}
	})
}

func TestTerraformConfig_Copy(t *testing.T) {
	t.Run("CopyWithNonNilValues", func(t *testing.T) {
		original := &TerraformConfig{
			Enabled: ptrBool(true),
			Backend: &BackendConfig{Type: "s3", Prefix: stringPtr("original-prefix")},
		}

		copy := original.Copy()

		if !reflect.DeepEqual(original, copy) {
			t.Errorf("Copy mismatch: expected %v, got %v", original, copy)
		}

		// Modify the copy and ensure original is unchanged
		copy.Enabled = ptrBool(false)
		if original.Enabled == nil || *original.Enabled == *copy.Enabled {
			t.Errorf("Original Enabled was modified: expected %v, got %v", true, *copy.Enabled)
		}
		copy.Backend = &BackendConfig{Type: "local", Prefix: stringPtr("copy-prefix")}
		if original.Backend == nil || original.Backend.Type == copy.Backend.Type {
			t.Errorf("Original Backend was modified: expected %v, got %v", "s3", copy.Backend.Type)
		}
		if original.Backend.Prefix == nil || *original.Backend.Prefix == *copy.Backend.Prefix {
			t.Errorf("Original Prefix was modified: expected %v, got %v", "original-prefix", *copy.Backend.Prefix)
		}
	})

	t.Run("CopyWithNilValues", func(t *testing.T) {
		original := &TerraformConfig{
			Enabled: nil,
			Backend: nil,
		}

		copy := original.Copy()

		if copy.Enabled != nil || copy.Backend != nil {
			t.Errorf("Copy mismatch: expected nil values, got Enabled: %v, Backend: %v", copy.Enabled, copy.Backend)
		}
	})

	t.Run("CopyNil", func(t *testing.T) {
		var original *TerraformConfig = nil
		mockCopy := original.Copy()
		if mockCopy != nil {
			t.Errorf("Mock copy should be nil, got %v", mockCopy)
		}
	})

	t.Run("CopyPreservesLock", func(t *testing.T) {
		// Given a config with a lock timeout
		original := &TerraformConfig{Lock: &LockConfig{Timeout: stringPtr("2m")}}

		// When copying
		copied := original.Copy()

		// Then the lock is preserved
		if copied.Lock == nil || copied.Lock.Timeout == nil || *copied.Lock.Timeout != "2m" {
			t.Fatalf("expected lock timeout 2m on copy, got %+v", copied.Lock)
		}
	})

	t.Run("CopyDeepCopiesLockSoMergeOnCopyDoesNotCorruptOriginal", func(t *testing.T) {
		// Given a config that gets copied, then merged with an overlay
		original := &TerraformConfig{Lock: &LockConfig{Timeout: stringPtr("10m")}}
		copied := original.Copy()
		overlay := &TerraformConfig{Lock: &LockConfig{Timeout: stringPtr("30s")}}

		// When the merge runs against the copy
		copied.Merge(overlay)

		// Then the original's timeout is untouched (would be "30s" with a shared pointer)
		if original.Lock == nil || original.Lock.Timeout == nil || *original.Lock.Timeout != "10m" {
			t.Fatalf("original Lock.Timeout was mutated through the shared pointer; got %+v", original.Lock)
		}
		// And the copy carries the merged value
		if copied.Lock == nil || copied.Lock.Timeout == nil || *copied.Lock.Timeout != "30s" {
			t.Fatalf("copy did not pick up overlay timeout; got %+v", copied.Lock)
		}
	})
}

// Helper functions to create pointers for basic types
func ptrBool(b bool) *bool {
	return &b
}

func stringPtr(s string) *string {
	return &s
}
