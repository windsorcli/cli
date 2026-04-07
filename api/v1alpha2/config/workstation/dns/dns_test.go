package workstation

import (
	"reflect"
	"testing"
)

// TestDNSConfig_Merge tests the Merge method of DNSConfig
func TestDNSConfig_Merge(t *testing.T) {
	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &DNSConfig{
			Enabled: ptrBool(true),
			Domain:  ptrString("test.local"),
		}
		original := base.DeepCopy()

		base.Merge(nil)

		if !reflect.DeepEqual(base, original) {
			t.Errorf("Expected no change when merging with nil overlay")
		}
	})

	t.Run("MergeBasicFields", func(t *testing.T) {
		base := &DNSConfig{
			Enabled: ptrBool(false),
			Domain:  ptrString("old.local"),
		}

		overlay := &DNSConfig{
			Enabled: ptrBool(true),
			Domain:  ptrString("new.local"),
			Address: ptrString("10.0.0.1"),
		}

		base.Merge(overlay)

		if !*base.Enabled {
			t.Errorf("Expected Enabled to be true")
		}
		if *base.Domain != "new.local" {
			t.Errorf("Expected Domain to be 'new.local', got %s", *base.Domain)
		}
		if *base.Address != "10.0.0.1" {
			t.Errorf("Expected Address to be '10.0.0.1', got %s", *base.Address)
		}
	})

	t.Run("MergeForwardServers", func(t *testing.T) {
		base := &DNSConfig{
			Forward: []string{"8.8.8.8"},
		}

		overlay := &DNSConfig{
			Forward: []string{"1.1.1.1", "8.8.4.4"},
		}

		base.Merge(overlay)

		expected := []string{"1.1.1.1", "8.8.4.4"}
		if !reflect.DeepEqual(base.Forward, expected) {
			t.Errorf("Expected Forward to be %v, got %v", expected, base.Forward)
		}
	})

	t.Run("MergeRecords", func(t *testing.T) {
		base := &DNSConfig{
			Records: []string{"10.0.0.1 api.local"},
		}

		overlay := &DNSConfig{
			Records: []string{"10.0.0.2 app.local", "10.0.0.3 db.local"},
		}

		base.Merge(overlay)

		expected := []string{"10.0.0.2 app.local", "10.0.0.3 db.local"}
		if !reflect.DeepEqual(base.Records, expected) {
			t.Errorf("Expected Records to be %v, got %v", expected, base.Records)
		}
	})

	t.Run("MergeAllFields", func(t *testing.T) {
		base := &DNSConfig{
			Enabled: ptrBool(false),
			Domain:  ptrString("old.local"),
			Address: ptrString("10.0.0.1"),
			Forward: []string{"8.8.8.8"},
			Records: []string{"10.0.0.1 api.local"},
		}

		overlay := &DNSConfig{
			Enabled: ptrBool(true),
			Domain:  ptrString("new.local"),
			Address: ptrString("10.0.0.2"),
			Forward: []string{"1.1.1.1", "8.8.4.4"},
			Records: []string{"10.0.0.2 app.local", "10.0.0.3 db.local"},
		}

		base.Merge(overlay)

		if !*base.Enabled {
			t.Errorf("Expected Enabled to be true")
		}
		if *base.Domain != "new.local" {
			t.Errorf("Expected Domain to be 'new.local', got %s", *base.Domain)
		}
		if *base.Address != "10.0.0.2" {
			t.Errorf("Expected Address to be '10.0.0.2', got %s", *base.Address)
		}

		expectedForward := []string{"1.1.1.1", "8.8.4.4"}
		if !reflect.DeepEqual(base.Forward, expectedForward) {
			t.Errorf("Expected Forward to be %v, got %v", expectedForward, base.Forward)
		}

		expectedRecords := []string{"10.0.0.2 app.local", "10.0.0.3 db.local"}
		if !reflect.DeepEqual(base.Records, expectedRecords) {
			t.Errorf("Expected Records to be %v, got %v", expectedRecords, base.Records)
		}
	})
}

// TestDNSConfig_Copy tests the Copy method of DNSConfig
func TestDNSConfig_Copy(t *testing.T) {
	t.Run("CopyNilConfig", func(t *testing.T) {
		var config *DNSConfig
		copied := config.DeepCopy()

		if copied != nil {
			t.Errorf("Expected nil when copying nil config")
		}
	})

	t.Run("CopyEmptyConfig", func(t *testing.T) {
		config := &DNSConfig{}
		copied := config.DeepCopy()

		if copied == nil {
			t.Errorf("Expected non-nil copy of empty config")
		}
		if !reflect.DeepEqual(config, copied) {
			t.Errorf("Expected copy to be equal to original")
		}
	})

	t.Run("CopyPopulatedConfig", func(t *testing.T) {
		config := &DNSConfig{
			Enabled: ptrBool(true),
			Domain:  ptrString("test.local"),
			Address: ptrString("10.0.0.1"),
			Forward: []string{"8.8.8.8", "1.1.1.1"},
			Records: []string{"10.0.0.1 api.local", "10.0.0.2 app.local"},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Errorf("Expected non-nil copy")
		}
		if !reflect.DeepEqual(config, copied) {
			t.Errorf("Expected copy to be equal to original")
		}

		// Verify deep copy by modifying original
		*config.Enabled = false
		if *copied.Enabled {
			t.Errorf("Expected copy to be independent of original")
		}

		// Verify deep copy of slices
		config.Forward[0] = "modified"
		if copied.Forward[0] == "modified" {
			t.Errorf("Expected copy slices to be independent of original")
		}

		config.Records[0] = "modified"
		if copied.Records[0] == "modified" {
			t.Errorf("Expected copy records to be independent of original")
		}
	})

	t.Run("CopyWithNilSlices", func(t *testing.T) {
		config := &DNSConfig{
			Enabled: ptrBool(true),
			Domain:  ptrString("test.local"),
			// Forward and Records are nil
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Errorf("Expected non-nil copy")
		}
		if !reflect.DeepEqual(config, copied) {
			t.Errorf("Expected copy to be equal to original")
		}
	})

	t.Run("CopyWithEmptySlices", func(t *testing.T) {
		config := &DNSConfig{
			Enabled: ptrBool(true),
			Domain:  ptrString("test.local"),
			Forward: []string{},
			Records: []string{},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Errorf("Expected non-nil copy")
		}
		if !reflect.DeepEqual(config, copied) {
			t.Errorf("Expected copy to be equal to original")
		}
	})
}
