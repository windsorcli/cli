package azure

// AzureConfig represents the Azure configuration
type AzureConfig struct {
	// Enabled indicates whether Azure integration is enabled.
	Enabled *bool `yaml:"enabled,omitempty"`

	// SubscriptionID is the Azure subscription identifier
	SubscriptionID *string `yaml:"subscription_id,omitempty"`

	// TenantID is the Azure tenant identifier
	TenantID *string `yaml:"tenant_id,omitempty"`

	// Environment specifies the Azure cloud environment (e.g. "public", "usgovernment")
	Environment *string `yaml:"environment,omitempty"`
}

// Merge performs a deep merge of the current AzureConfig with another AzureConfig.
func (base *AzureConfig) Merge(overlay *AzureConfig) {
	if overlay == nil {
		return
	}
	if overlay.Enabled != nil {
		base.Enabled = overlay.Enabled
	}
	if overlay.SubscriptionID != nil {
		base.SubscriptionID = overlay.SubscriptionID
	}
	if overlay.TenantID != nil {
		base.TenantID = overlay.TenantID
	}
	if overlay.Environment != nil {
		base.Environment = overlay.Environment
	}
}

// Copy creates a deep copy of the AzureConfig object
func (c *AzureConfig) Copy() *AzureConfig {
	if c == nil {
		return nil
	}
	return &AzureConfig{
		Enabled:        c.Enabled,
		SubscriptionID: c.SubscriptionID,
		TenantID:       c.TenantID,
		Environment:    c.Environment,
	}
}
