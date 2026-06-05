package azure

import (
	"testing"
)

func TestAzureConfig(t *testing.T) {
	t.Run("Merge", func(t *testing.T) {
		tests := []struct {
			name     string
			base     *AzureConfig
			overlay  *AzureConfig
			expected *AzureConfig
		}{
			{
				name: "AllFields",
				base: &AzureConfig{
					SubscriptionID: stringPtr("old-sub"),
					TenantID:       stringPtr("old-tenant"),
					Environment:    stringPtr("old-env"),
					Region:         stringPtr("eastus"),
					KubeloginMode:  stringPtr("azurecli"),
				},
				overlay: &AzureConfig{
					SubscriptionID: stringPtr("new-sub"),
					TenantID:       stringPtr("new-tenant"),
					Environment:    stringPtr("new-env"),
					Region:         stringPtr("eastus2"),
					KubeloginMode:  stringPtr("workloadidentity"),
				},
				expected: &AzureConfig{
					SubscriptionID: stringPtr("new-sub"),
					TenantID:       stringPtr("new-tenant"),
					Environment:    stringPtr("new-env"),
					Region:         stringPtr("eastus2"),
					KubeloginMode:  stringPtr("workloadidentity"),
				},
			},
			{
				name: "PartialOverlay",
				base: &AzureConfig{
					SubscriptionID: stringPtr("old-sub"),
					TenantID:       stringPtr("old-tenant"),
					Environment:    stringPtr("old-env"),
					Region:         stringPtr("eastus"),
				},
				overlay: &AzureConfig{
					KubeloginMode: stringPtr("msi"),
				},
				expected: &AzureConfig{
					SubscriptionID: stringPtr("old-sub"),
					TenantID:       stringPtr("old-tenant"),
					Environment:    stringPtr("old-env"),
					Region:         stringPtr("eastus"),
					KubeloginMode:  stringPtr("msi"),
				},
			},
			{
				name: "RegionOverlayOnly",
				base: &AzureConfig{
					SubscriptionID: stringPtr("old-sub"),
					Region:         stringPtr("eastus"),
				},
				overlay: &AzureConfig{
					Region: stringPtr("westus2"),
				},
				expected: &AzureConfig{
					SubscriptionID: stringPtr("old-sub"),
					Region:         stringPtr("westus2"),
				},
			},
			{
				name: "NilOverlay",
				base: &AzureConfig{
					SubscriptionID: stringPtr("old-sub"),
					TenantID:       stringPtr("old-tenant"),
					Environment:    stringPtr("old-env"),
					Region:         stringPtr("eastus"),
				},
				overlay: nil,
				expected: &AzureConfig{
					SubscriptionID: stringPtr("old-sub"),
					TenantID:       stringPtr("old-tenant"),
					Environment:    stringPtr("old-env"),
					Region:         stringPtr("eastus"),
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tt.base.Merge(tt.overlay)

				assertStringPtrEq(t, "SubscriptionID", tt.base.SubscriptionID, tt.expected.SubscriptionID)
				assertStringPtrEq(t, "TenantID", tt.base.TenantID, tt.expected.TenantID)
				assertStringPtrEq(t, "Environment", tt.base.Environment, tt.expected.Environment)
				assertStringPtrEq(t, "Region", tt.base.Region, tt.expected.Region)
				assertStringPtrEq(t, "KubeloginMode", tt.base.KubeloginMode, tt.expected.KubeloginMode)
			})
		}
	})

	t.Run("Copy", func(t *testing.T) {
		tests := []struct {
			name     string
			original *AzureConfig
		}{
			{
				name: "AllFields",
				original: &AzureConfig{
					SubscriptionID: stringPtr("sub"),
					TenantID:       stringPtr("tenant"),
					Environment:    stringPtr("env"),
					Region:         stringPtr("eastus2"),
					KubeloginMode:  stringPtr("azurecli"),
				},
			},
			{
				name: "SomeFields",
				original: &AzureConfig{
					Region:        stringPtr("westeurope"),
					KubeloginMode: stringPtr("workloadidentity"),
				},
			},
			{
				name:     "Nil",
				original: nil,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				copy := tt.original.Copy()

				if tt.original == nil {
					if copy != nil {
						t.Error("Expected nil copy for nil original")
					}
					return
				}

				if copy == nil {
					t.Fatal("Expected non-nil copy")
				}

				if copy == tt.original {
					t.Error("Expected copy to be a new instance")
				}

				assertStringPtrEq(t, "SubscriptionID", copy.SubscriptionID, tt.original.SubscriptionID)
				assertStringPtrEq(t, "TenantID", copy.TenantID, tt.original.TenantID)
				assertStringPtrEq(t, "Environment", copy.Environment, tt.original.Environment)
				assertStringPtrEq(t, "Region", copy.Region, tt.original.Region)
				assertStringPtrEq(t, "KubeloginMode", copy.KubeloginMode, tt.original.KubeloginMode)
			})
		}
	})
}

func assertStringPtrEq(t *testing.T, name string, got, want *string) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Errorf("%s: got %v, want %v", name, deref(got), deref(want))
		}
		return
	}
	if *got != *want {
		t.Errorf("%s: got %q, want %q", name, *got, *want)
	}
}

func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func stringPtr(s string) *string {
	return &s
}
