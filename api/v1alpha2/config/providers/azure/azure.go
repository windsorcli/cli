package azure

// AzureConfig represents the Azure configuration. Azure integration activates whenever this
// block is present in a context (or when platform is "azure"); no separate enabled flag.
type AzureConfig struct {
	// SubscriptionID is the Azure subscription identifier
	SubscriptionID *string `yaml:"subscription_id,omitempty"`

	// TenantID is the Azure tenant identifier
	TenantID *string `yaml:"tenant_id,omitempty"`

	// Environment specifies the Azure cloud environment (e.g. "public", "usgovernment")
	Environment *string `yaml:"environment,omitempty"`

	// KubeloginMode overrides the kubelogin login mode for AAD-enabled AKS kubeconfigs.
	// Empty (default) auto-detects from the active credential chain. Set to "msi" on
	// managed-identity runners; other values match kubelogin's own modes.
	KubeloginMode *string `yaml:"kubelogin_mode,omitempty"`
}

// Merge performs a deep merge of the current AzureConfig with another AzureConfig.
func (base *AzureConfig) Merge(overlay *AzureConfig) {
	if overlay == nil {
		return
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
	if overlay.KubeloginMode != nil {
		base.KubeloginMode = overlay.KubeloginMode
	}
}

// DeepCopy creates a deep copy of the AzureConfig object
func (c *AzureConfig) DeepCopy() *AzureConfig {
	if c == nil {
		return nil
	}
	copied := &AzureConfig{}

	if c.SubscriptionID != nil {
		subscriptionCopy := *c.SubscriptionID
		copied.SubscriptionID = &subscriptionCopy
	}
	if c.TenantID != nil {
		tenantCopy := *c.TenantID
		copied.TenantID = &tenantCopy
	}
	if c.Environment != nil {
		environmentCopy := *c.Environment
		copied.Environment = &environmentCopy
	}
	if c.KubeloginMode != nil {
		kubeloginModeCopy := *c.KubeloginMode
		copied.KubeloginMode = &kubeloginModeCopy
	}

	return copied
}
