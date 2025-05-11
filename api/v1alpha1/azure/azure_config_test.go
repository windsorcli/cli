package azure

import (
	"testing"
)

func TestAzureConfig(t *testing.T) {
	t.Run("Merge", func(t *testing.T) {
		base := &AzureConfig{
			Enabled: boolPtr(false),
		}
		overlay := &AzureConfig{
			Enabled: boolPtr(true),
		}

		base.Merge(overlay)

		if *base.Enabled != true {
			t.Errorf("Expected Enabled to be true, got %v", *base.Enabled)
		}
	})

	t.Run("Copy", func(t *testing.T) {
		original := &AzureConfig{
			Enabled: boolPtr(true),
		}

		copy := original.Copy()

		if copy == nil {
			t.Fatal("Expected non-nil copy")
		}

		if copy == original {
			t.Error("Expected copy to be a new instance")
		}

		if *copy.Enabled != *original.Enabled {
			t.Errorf("Expected Enabled to be %v, got %v", *original.Enabled, *copy.Enabled)
		}
	})
}

func boolPtr(b bool) *bool {
	return &b
}
