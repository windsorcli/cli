package dns

import "testing"

// Helper functions to create pointers for basic types
func ptrString(s string) *string {
	return &s
}

func ptrBool(b bool) *bool {
	return &b
}

func TestDNSConfig_Merge(t *testing.T) {
	t.Run("MergeWithNonNilValues", func(t *testing.T) {
		base := &DNSConfig{
			Enabled: ptrBool(true),
			Name:    ptrString("base-name"),
			Address: ptrString("base-address"),
		}

		overlay := &DNSConfig{
			Enabled: ptrBool(false),
			Name:    ptrString("overlay-name"),
			Address: ptrString("overlay-address"),
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != false {
			t.Errorf("Enabled mismatch: expected %v, got %v", false, *base.Enabled)
		}
		if base.Name == nil || *base.Name != "overlay-name" {
			t.Errorf("Name mismatch: expected %v, got %v", "overlay-name", *base.Name)
		}
		if base.Address == nil || *base.Address != "overlay-address" {
			t.Errorf("Address mismatch: expected %v, got %v", "overlay-address", *base.Address)
		}
	})

	t.Run("MergeWithNilValues", func(t *testing.T) {
		base := &DNSConfig{
			Enabled: ptrBool(true),
			Name:    ptrString("base-name"),
			Address: ptrString("base-address"),
		}

		overlay := &DNSConfig{
			Enabled: nil,
			Name:    nil,
			Address: nil,
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Enabled mismatch: expected %v, got %v", true, *base.Enabled)
		}
		if base.Name == nil || *base.Name != "base-name" {
			t.Errorf("Name mismatch: expected %v, got %v", "base-name", *base.Name)
		}
		if base.Address == nil || *base.Address != "base-address" {
			t.Errorf("Address mismatch: expected %v, got %v", "base-address", *base.Address)
		}
	})
}

func TestDNSConfig_Copy(t *testing.T) {
	t.Run("CopyWithNonNilValues", func(t *testing.T) {
		original := &DNSConfig{
			Enabled: ptrBool(true),
			Name:    ptrString("original-name"),
			Address: ptrString("original-address"),
		}

		copy := original.Copy()

		if original.Enabled == nil || copy.Enabled == nil || *original.Enabled != *copy.Enabled {
			t.Errorf("Enabled mismatch: expected %v, got %v", *original.Enabled, *copy.Enabled)
		}
		if original.Name == nil || copy.Name == nil || *original.Name != *copy.Name {
			t.Errorf("Name mismatch: expected %v, got %v", *original.Name, *copy.Name)
		}
		if original.Address == nil || copy.Address == nil || *original.Address != *copy.Address {
			t.Errorf("Address mismatch: expected %v, got %v", *original.Address, *copy.Address)
		}

		// Modify the copy and ensure original is unchanged
		copy.Enabled = ptrBool(false)
		if original.Enabled == nil || *original.Enabled == *copy.Enabled {
			t.Errorf("Original Enabled was modified: expected %v, got %v", true, *copy.Enabled)
		}
		copy.Name = ptrString("modified-name")
		if original.Name == nil || *original.Name == *copy.Name {
			t.Errorf("Original Name was modified: expected %v, got %v", "original-name", *copy.Name)
		}
		copy.Address = ptrString("modified-address")
		if original.Address == nil || *original.Address == *copy.Address {
			t.Errorf("Original Address was modified: expected %v, got %v", "original-address", *copy.Address)
		}
	})

	t.Run("CopyWithNilValues", func(t *testing.T) {
		original := &DNSConfig{
			Enabled: nil,
			Name:    nil,
			Address: nil,
		}

		copy := original.Copy()

		if copy.Enabled != nil {
			t.Errorf("Enabled mismatch: expected nil, got %v", copy.Enabled)
		}
		if copy.Name != nil {
			t.Errorf("Name mismatch: expected nil, got %v", copy.Name)
		}
		if copy.Address != nil {
			t.Errorf("Address mismatch: expected nil, got %v", copy.Address)
		}
	})

	t.Run("CopyNil", func(t *testing.T) {
		var original *DNSConfig = nil
		mockCopy := original.Copy()
		if mockCopy != nil {
			t.Errorf("Mock copy should be nil, got %v", mockCopy)
		}
	})
}
