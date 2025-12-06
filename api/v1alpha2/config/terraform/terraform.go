package terraform

// TerraformConfig represents the Terraform configuration
type TerraformConfig struct {
	Enabled *bool          `yaml:"enabled,omitempty"`
	Driver  *string        `yaml:"driver,omitempty"`
	Backend *BackendConfig `yaml:"backend,omitempty"`
}

type BackendConfig struct {
	Type   string  `yaml:"type"`
	Prefix *string `yaml:"prefix,omitempty"`
}

// Merge performs a simple merge of the current TerraformConfig with another TerraformConfig.
func (base *TerraformConfig) Merge(overlay *TerraformConfig) {
	if overlay.Enabled != nil {
		base.Enabled = overlay.Enabled
	}
	if overlay.Driver != nil {
		base.Driver = overlay.Driver
	}
	if overlay.Backend != nil {
		base.Backend = overlay.Backend
	}
}

// DeepCopy creates a deep copy of the TerraformConfig object
func (c *TerraformConfig) DeepCopy() *TerraformConfig {
	if c == nil {
		return nil
	}
	copied := &TerraformConfig{}

	if c.Enabled != nil {
		enabledCopy := *c.Enabled
		copied.Enabled = &enabledCopy
	}
	if c.Driver != nil {
		driverCopy := *c.Driver
		copied.Driver = &driverCopy
	}
	if c.Backend != nil {
		copied.Backend = c.Backend
	}

	return copied
}
