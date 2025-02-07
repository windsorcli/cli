package terraform

// TerraformConfig represents the Terraform configuration
type TerraformConfig struct {
	Enabled *bool   `yaml:"enabled,omitempty"`
	Backend *string `yaml:"backend,omitempty"`
}

// Merge performs a deep merge of the current TerraformConfig with another TerraformConfig.
func (base *TerraformConfig) Merge(overlay *TerraformConfig) {
	if overlay.Enabled != nil {
		base.Enabled = overlay.Enabled
	}
	if overlay.Backend != nil {
		base.Backend = overlay.Backend
	}
}

// Copy creates a deep copy of the TerraformConfig object
func (c *TerraformConfig) Copy() *TerraformConfig {
	if c == nil {
		return nil
	}
	return &TerraformConfig{
		Enabled: c.Enabled,
		Backend: c.Backend,
	}
}
