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
			Domain:  ptrString("base-domain"),
			Address: ptrString("base-address"),
			Records: []string{"127.0.0.1 base-domain", "192.168.1.1 base-domain"},
		}

		overlay := &DNSConfig{
			Enabled: ptrBool(false),
			Domain:  ptrString("overlay-domain"),
			Address: ptrString("overlay-address"),
			Records: []string{"127.0.0.2 overlay-domain", "192.168.1.2 overlay-domain"},
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != false {
			t.Errorf("Enabled mismatch: expected %v, got %v", false, *base.Enabled)
		}
		if base.Domain == nil || *base.Domain != "overlay-domain" {
			t.Errorf("Domain mismatch: expected %v, got %v", "overlay-domain", *base.Domain)
		}
		if base.Address == nil || *base.Address != "overlay-address" {
			t.Errorf("Address mismatch: expected %v, got %v", "overlay-address", *base.Address)
		}
		if len(base.Records) != 2 || base.Records[0] != "127.0.0.2 overlay-domain" || base.Records[1] != "192.168.1.2 overlay-domain" {
			t.Errorf("Records mismatch: expected %v, got %v", []string{"127.0.0.2 overlay-domain", "192.168.1.2 overlay-domain"}, base.Records)
		}
	})

	t.Run("MergeWithNilValues", func(t *testing.T) {
		base := &DNSConfig{
			Enabled: ptrBool(true),
			Domain:  ptrString("base-domain"),
			Address: ptrString("base-address"),
			Records: []string{"127.0.0.1 base-domain", "192.168.1.1 base-domain"},
		}

		overlay := &DNSConfig{
			Enabled: nil,
			Domain:  nil,
			Address: nil,
			Records: nil,
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Enabled mismatch: expected %v, got %v", true, *base.Enabled)
		}
		if base.Domain == nil || *base.Domain != "base-domain" {
			t.Errorf("Domain mismatch: expected %v, got %v", "base-domain", *base.Domain)
		}
		if base.Address == nil || *base.Address != "base-address" {
			t.Errorf("Address mismatch: expected %v, got %v", "base-address", *base.Address)
		}
		if len(base.Records) != 2 || base.Records[0] != "127.0.0.1 base-domain" || base.Records[1] != "192.168.1.1 base-domain" {
			t.Errorf("Records mismatch: expected %v, got %v", []string{"127.0.0.1 base-domain", "192.168.1.1 base-domain"}, base.Records)
		}
	})
}

func TestDNSConfig_Copy(t *testing.T) {
	t.Run("CopyWithNonNilValues", func(t *testing.T) {
		original := &DNSConfig{
			Enabled: ptrBool(true),
			Domain:  ptrString("original-domain"),
			Address: ptrString("original-address"),
			Records: []string{"127.0.0.1 original-domain", "192.168.1.1 original-domain"},
		}

		copy := original.Copy()

		if original.Enabled == nil || copy.Enabled == nil || *original.Enabled != *copy.Enabled {
			t.Errorf("Enabled mismatch: expected %v, got %v", *original.Enabled, *copy.Enabled)
		}
		if original.Domain == nil || copy.Domain == nil || *original.Domain != *copy.Domain {
			t.Errorf("Domain mismatch: expected %v, got %v", *original.Domain, *copy.Domain)
		}
		if original.Address == nil || copy.Address == nil || *original.Address != *copy.Address {
			t.Errorf("Address mismatch: expected %v, got %v", *original.Address, *copy.Address)
		}
		if len(original.Records) != len(copy.Records) || original.Records[0] != copy.Records[0] || original.Records[1] != copy.Records[1] {
			t.Errorf("Records mismatch: expected %v, got %v", original.Records, copy.Records)
		}

		// Modify the copy and ensure original is unchanged
		copy.Enabled = ptrBool(false)
		if original.Enabled == nil || *original.Enabled == *copy.Enabled {
			t.Errorf("Original Enabled was modified: expected %v, got %v", true, *copy.Enabled)
		}
		copy.Domain = ptrString("modified-domain")
		if original.Domain == nil || *original.Domain == *copy.Domain {
			t.Errorf("Original Domain was modified: expected %v, got %v", "original-domain", *copy.Domain)
		}
		copy.Address = ptrString("modified-address")
		if original.Address == nil || *original.Address == *copy.Address {
			t.Errorf("Original Address was modified: expected %v, got %v", "original-address", *copy.Address)
		}
		copy.Records = []string{"127.0.0.2 modified-domain", "192.168.1.2 modified-domain"}
		if len(original.Records) != 2 || original.Records[0] == copy.Records[0] || original.Records[1] == copy.Records[1] {
			t.Errorf("Original Records were modified: expected %v, got %v", []string{"127.0.0.1 original-domain", "192.168.1.1 original-domain"}, copy.Records)
		}
	})

	t.Run("CopyWithNilValues", func(t *testing.T) {
		original := &DNSConfig{
			Enabled: nil,
			Domain:  nil,
			Address: nil,
			Records: nil,
		}

		copy := original.Copy()

		if copy.Enabled != nil {
			t.Errorf("Enabled mismatch: expected nil, got %v", copy.Enabled)
		}
		if copy.Domain != nil {
			t.Errorf("Domain mismatch: expected nil, got %v", copy.Domain)
		}
		if copy.Address != nil {
			t.Errorf("Address mismatch: expected nil, got %v", copy.Address)
		}
		if copy.Records != nil {
			t.Errorf("Records mismatch: expected nil, got %v", copy.Records)
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
