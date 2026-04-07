package azure

import (
	"testing"
)

// TestAzureConfig_Merge tests the Merge method of AzureConfig
func TestAzureConfig_Merge(t *testing.T) {
	t.Run("MergeAllFields", func(t *testing.T) {
		base := &AzureConfig{
			Enabled:        boolPtr(false),
			SubscriptionID: stringPtr("old-sub"),
			TenantID:       stringPtr("old-tenant"),
			Environment:    stringPtr("old-env"),
		}
		overlay := &AzureConfig{
			Enabled:        boolPtr(true),
			SubscriptionID: stringPtr("new-sub"),
			TenantID:       stringPtr("new-tenant"),
			Environment:    stringPtr("new-env"),
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Expected Enabled to be true, got %v", base.Enabled)
		}
		if base.SubscriptionID == nil || *base.SubscriptionID != "new-sub" {
			t.Errorf("Expected SubscriptionID to be 'new-sub', got %s", *base.SubscriptionID)
		}
		if base.TenantID == nil || *base.TenantID != "new-tenant" {
			t.Errorf("Expected TenantID to be 'new-tenant', got %s", *base.TenantID)
		}
		if base.Environment == nil || *base.Environment != "new-env" {
			t.Errorf("Expected Environment to be 'new-env', got %s", *base.Environment)
		}
	})

	t.Run("MergePartialOverlay", func(t *testing.T) {
		base := &AzureConfig{
			Enabled:        boolPtr(false),
			SubscriptionID: stringPtr("old-sub"),
			TenantID:       stringPtr("old-tenant"),
			Environment:    stringPtr("old-env"),
		}
		overlay := &AzureConfig{
			Enabled: boolPtr(true),
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Expected Enabled to be true, got %v", base.Enabled)
		}
		if base.SubscriptionID == nil || *base.SubscriptionID != "old-sub" {
			t.Errorf("Expected SubscriptionID to remain 'old-sub', got %s", *base.SubscriptionID)
		}
		if base.TenantID == nil || *base.TenantID != "old-tenant" {
			t.Errorf("Expected TenantID to remain 'old-tenant', got %s", *base.TenantID)
		}
		if base.Environment == nil || *base.Environment != "old-env" {
			t.Errorf("Expected Environment to remain 'old-env', got %s", *base.Environment)
		}
	})

	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &AzureConfig{
			Enabled:        boolPtr(false),
			SubscriptionID: stringPtr("old-sub"),
			TenantID:       stringPtr("old-tenant"),
			Environment:    stringPtr("old-env"),
		}
		original := base.DeepCopy()

		base.Merge(nil)

		if base.Enabled == nil || *base.Enabled != *original.Enabled {
			t.Errorf("Expected Enabled to remain unchanged")
		}
		if base.SubscriptionID == nil || *base.SubscriptionID != *original.SubscriptionID {
			t.Errorf("Expected SubscriptionID to remain unchanged")
		}
		if base.TenantID == nil || *base.TenantID != *original.TenantID {
			t.Errorf("Expected TenantID to remain unchanged")
		}
		if base.Environment == nil || *base.Environment != *original.Environment {
			t.Errorf("Expected Environment to remain unchanged")
		}
	})
}

// TestAzureConfig_Copy tests the Copy method of AzureConfig
func TestAzureConfig_Copy(t *testing.T) {
	t.Run("CopyAllFields", func(t *testing.T) {
		original := &AzureConfig{
			Enabled:        boolPtr(true),
			SubscriptionID: stringPtr("sub"),
			TenantID:       stringPtr("tenant"),
			Environment:    stringPtr("env"),
		}

		copy := original.DeepCopy()

		if copy == nil {
			t.Fatal("Expected non-nil copy")
		}
		if copy == original {
			t.Error("Expected copy to be a new instance")
		}
		if copy.Enabled == nil || *copy.Enabled != *original.Enabled {
			t.Errorf("Expected Enabled to be copied correctly")
		}
		if copy.SubscriptionID == nil || *copy.SubscriptionID != *original.SubscriptionID {
			t.Errorf("Expected SubscriptionID to be copied correctly")
		}
		if copy.TenantID == nil || *copy.TenantID != *original.TenantID {
			t.Errorf("Expected TenantID to be copied correctly")
		}
		if copy.Environment == nil || *copy.Environment != *original.Environment {
			t.Errorf("Expected Environment to be copied correctly")
		}
	})

	t.Run("CopySomeFields", func(t *testing.T) {
		original := &AzureConfig{
			Enabled: boolPtr(true),
		}

		copy := original.DeepCopy()

		if copy == nil {
			t.Fatal("Expected non-nil copy")
		}
		if copy.Enabled == nil || *copy.Enabled != *original.Enabled {
			t.Errorf("Expected Enabled to be copied correctly")
		}
		if copy.SubscriptionID != nil {
			t.Error("Expected SubscriptionID to be nil")
		}
		if copy.TenantID != nil {
			t.Error("Expected TenantID to be nil")
		}
		if copy.Environment != nil {
			t.Error("Expected Environment to be nil")
		}
	})

	t.Run("CopyNilConfig", func(t *testing.T) {
		var original *AzureConfig
		copy := original.DeepCopy()

		if copy != nil {
			t.Error("Expected nil copy for nil original")
		}
	})
}

func boolPtr(b bool) *bool {
	return &b
}

func stringPtr(s string) *string {
	return &s
}
