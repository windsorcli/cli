package terraform

import (
	"reflect"
	"testing"
)

func TestTerraformConfig_Merge(t *testing.T) {
	t.Run("MergeWithNonNilValues", func(t *testing.T) {
		base := &TerraformConfig{
			Enabled: nil,
			Driver:  nil,
			Backend: nil,
		}
		overlay := &TerraformConfig{
			Enabled: ptrBool(true),
			Driver:  stringPtr("docker"),
			Backend: &BackendConfig{Type: "s3", Prefix: stringPtr("mock-prefix")},
		}
		base.Merge(overlay)
		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Enabled mismatch: expected %v, got %v", true, *base.Enabled)
		}
		if base.Driver == nil || *base.Driver != "docker" {
			t.Errorf("Driver mismatch: expected %v, got %v", "docker", *base.Driver)
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
			Driver:  stringPtr("local"),
			Backend: &BackendConfig{Type: "s3", Prefix: stringPtr("base-prefix")},
		}
		overlay := &TerraformConfig{
			Enabled: nil,
			Driver:  nil,
			Backend: nil,
		}
		base.Merge(overlay)
		if base.Enabled == nil || *base.Enabled != false {
			t.Errorf("Enabled mismatch: expected %v, got %v", false, *base.Enabled)
		}
		if base.Driver == nil || *base.Driver != "local" {
			t.Errorf("Driver mismatch: expected %v, got %v", "local", *base.Driver)
		}
		if base.Backend == nil || base.Backend.Type != "s3" {
			t.Errorf("Backend mismatch: expected %v, got %v", "s3", base.Backend.Type)
		}
		if base.Backend.Prefix == nil || *base.Backend.Prefix != "base-prefix" {
			t.Errorf("Prefix mismatch: expected %v, got %v", "base-prefix", base.Backend.Prefix)
		}
	})

	t.Run("MergeWithOnlyDriver", func(t *testing.T) {
		base := &TerraformConfig{
			Enabled: ptrBool(false),
			Driver:  stringPtr("local"),
			Backend: &BackendConfig{Type: "s3", Prefix: stringPtr("base-prefix")},
		}
		overlay := &TerraformConfig{
			Driver: stringPtr("docker"),
		}
		base.Merge(overlay)
		if base.Enabled == nil || *base.Enabled != false {
			t.Errorf("Enabled mismatch: expected %v, got %v", false, *base.Enabled)
		}
		if base.Driver == nil || *base.Driver != "docker" {
			t.Errorf("Driver mismatch: expected %v, got %v", "docker", *base.Driver)
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
			Driver:  stringPtr("docker"),
			Backend: &BackendConfig{Type: "s3", Prefix: stringPtr("original-prefix")},
		}

		copy := original.DeepCopy()

		if !reflect.DeepEqual(original, copy) {
			t.Errorf("Copy mismatch: expected %v, got %v", original, copy)
		}

		// Modify the copy and ensure original is unchanged
		copy.Enabled = ptrBool(false)
		if original.Enabled == nil || *original.Enabled == *copy.Enabled {
			t.Errorf("Original Enabled was modified: expected %v, got %v", true, *copy.Enabled)
		}
		copy.Driver = stringPtr("local")
		if original.Driver == nil || *original.Driver == *copy.Driver {
			t.Errorf("Original Driver was modified: expected %v, got %v", "docker", *copy.Driver)
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
			Driver:  nil,
			Backend: nil,
		}

		copy := original.DeepCopy()

		if copy.Enabled != nil || copy.Driver != nil || copy.Backend != nil {
			t.Errorf("Copy mismatch: expected nil values, got Enabled: %v, Driver: %v, Backend: %v", copy.Enabled, copy.Driver, copy.Backend)
		}
	})

	t.Run("CopyNil", func(t *testing.T) {
		var original *TerraformConfig = nil
		mockCopy := original.DeepCopy()
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
