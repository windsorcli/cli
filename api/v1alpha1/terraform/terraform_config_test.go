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
}

// Helper functions to create pointers for basic types
func ptrBool(b bool) *bool {
	return &b
}

func stringPtr(s string) *string {
	return &s
}
