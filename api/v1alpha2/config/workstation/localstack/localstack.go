package workstation

// LocalstackConfig represents the Localstack configuration
type LocalstackConfig struct {
	Enabled  *bool    `yaml:"enabled,omitempty"`
	Services []string `yaml:"services,omitempty"`
}

// Merge performs a deep merge of the current LocalstackConfig with another LocalstackConfig.
func (base *LocalstackConfig) Merge(overlay *LocalstackConfig) {
	if overlay == nil {
		return
	}
	if overlay.Enabled != nil {
		base.Enabled = overlay.Enabled
	}
	if overlay.Services != nil {
		base.Services = overlay.Services
	}
}

// DeepCopy creates a deep copy of the LocalstackConfig object
func (c *LocalstackConfig) DeepCopy() *LocalstackConfig {
	if c == nil {
		return nil
	}
	var enabledCopy *bool
	if c.Enabled != nil {
		enabledCopy = new(bool)
		*enabledCopy = *c.Enabled
	}
	var servicesCopy []string
	if c.Services != nil {
		servicesCopy = make([]string, len(c.Services))
		copy(servicesCopy, c.Services)
	}
	return &LocalstackConfig{
		Enabled:  enabledCopy,
		Services: servicesCopy,
	}
}
