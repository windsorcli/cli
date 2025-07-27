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
					Enabled:        boolPtr(false),
					SubscriptionID: stringPtr("old-sub"),
					TenantID:       stringPtr("old-tenant"),
					Environment:    stringPtr("old-env"),
				},
				overlay: &AzureConfig{
					Enabled:        boolPtr(true),
					SubscriptionID: stringPtr("new-sub"),
					TenantID:       stringPtr("new-tenant"),
					Environment:    stringPtr("new-env"),
				},
				expected: &AzureConfig{
					Enabled:        boolPtr(true),
					SubscriptionID: stringPtr("new-sub"),
					TenantID:       stringPtr("new-tenant"),
					Environment:    stringPtr("new-env"),
				},
			},
			{
				name: "PartialOverlay",
				base: &AzureConfig{
					Enabled:        boolPtr(false),
					SubscriptionID: stringPtr("old-sub"),
					TenantID:       stringPtr("old-tenant"),
					Environment:    stringPtr("old-env"),
				},
				overlay: &AzureConfig{
					Enabled: boolPtr(true),
				},
				expected: &AzureConfig{
					Enabled:        boolPtr(true),
					SubscriptionID: stringPtr("old-sub"),
					TenantID:       stringPtr("old-tenant"),
					Environment:    stringPtr("old-env"),
				},
			},
			{
				name: "NilOverlay",
				base: &AzureConfig{
					Enabled:        boolPtr(false),
					SubscriptionID: stringPtr("old-sub"),
					TenantID:       stringPtr("old-tenant"),
					Environment:    stringPtr("old-env"),
				},
				overlay: nil,
				expected: &AzureConfig{
					Enabled:        boolPtr(false),
					SubscriptionID: stringPtr("old-sub"),
					TenantID:       stringPtr("old-tenant"),
					Environment:    stringPtr("old-env"),
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tt.base.Merge(tt.overlay)

				if tt.base.Enabled == nil || tt.expected.Enabled == nil {
					if tt.base.Enabled != tt.expected.Enabled {
						t.Errorf("Expected Enabled to be %v, got %v", tt.expected.Enabled, tt.base.Enabled)
					}
				} else if *tt.base.Enabled != *tt.expected.Enabled {
					t.Errorf("Expected Enabled to be %v, got %v", *tt.expected.Enabled, *tt.base.Enabled)
				}

				if tt.base.SubscriptionID == nil || tt.expected.SubscriptionID == nil {
					if tt.base.SubscriptionID != tt.expected.SubscriptionID {
						t.Errorf("Expected SubscriptionID to be %v, got %v", tt.expected.SubscriptionID, tt.base.SubscriptionID)
					}
				} else if *tt.base.SubscriptionID != *tt.expected.SubscriptionID {
					t.Errorf("Expected SubscriptionID to be %v, got %v", *tt.expected.SubscriptionID, *tt.base.SubscriptionID)
				}

				if tt.base.TenantID == nil || tt.expected.TenantID == nil {
					if tt.base.TenantID != tt.expected.TenantID {
						t.Errorf("Expected TenantID to be %v, got %v", tt.expected.TenantID, tt.base.TenantID)
					}
				} else if *tt.base.TenantID != *tt.expected.TenantID {
					t.Errorf("Expected TenantID to be %v, got %v", *tt.expected.TenantID, *tt.base.TenantID)
				}

				if tt.base.Environment == nil || tt.expected.Environment == nil {
					if tt.base.Environment != tt.expected.Environment {
						t.Errorf("Expected Environment to be %v, got %v", tt.expected.Environment, tt.base.Environment)
					}
				} else if *tt.base.Environment != *tt.expected.Environment {
					t.Errorf("Expected Environment to be %v, got %v", *tt.expected.Environment, *tt.base.Environment)
				}
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
					Enabled:        boolPtr(true),
					SubscriptionID: stringPtr("sub"),
					TenantID:       stringPtr("tenant"),
					Environment:    stringPtr("env"),
				},
			},
			{
				name: "SomeFields",
				original: &AzureConfig{
					Enabled: boolPtr(true),
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

				if tt.original.Enabled == nil {
					if copy.Enabled != nil {
						t.Error("Expected Enabled to be nil")
					}
				} else if copy.Enabled == nil || *copy.Enabled != *tt.original.Enabled {
					t.Errorf("Expected Enabled to be %v, got %v", tt.original.Enabled, copy.Enabled)
				}

				if tt.original.SubscriptionID == nil {
					if copy.SubscriptionID != nil {
						t.Error("Expected SubscriptionID to be nil")
					}
				} else if copy.SubscriptionID == nil || *copy.SubscriptionID != *tt.original.SubscriptionID {
					t.Errorf("Expected SubscriptionID to be %v, got %v", tt.original.SubscriptionID, copy.SubscriptionID)
				}

				if tt.original.TenantID == nil {
					if copy.TenantID != nil {
						t.Error("Expected TenantID to be nil")
					}
				} else if copy.TenantID == nil || *copy.TenantID != *tt.original.TenantID {
					t.Errorf("Expected TenantID to be %v, got %v", tt.original.TenantID, copy.TenantID)
				}

				if tt.original.Environment == nil {
					if copy.Environment != nil {
						t.Error("Expected Environment to be nil")
					}
				} else if copy.Environment == nil || *copy.Environment != *tt.original.Environment {
					t.Errorf("Expected Environment to be %v, got %v", tt.original.Environment, copy.Environment)
				}
			})
		}
	})
}

func boolPtr(b bool) *bool {
	return &b
}

func stringPtr(s string) *string {
	return &s
}
