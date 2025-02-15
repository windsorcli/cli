package secrets

// SecretsConfig represents the Secrets configuration
type SecretsConfig struct {
	Enabled  *bool  `yaml:"enabled,omitempty"`
	Provider string `yaml:"provider,omitempty"`
}

// Merge performs a deep merge of the current SecretsConfig with another SecretsConfig.
func (base *SecretsConfig) Merge(overlay *SecretsConfig) {
	if overlay.Enabled != nil {
		base.Enabled = overlay.Enabled
	}
	if overlay.Provider != "" {
		base.Provider = overlay.Provider
	}
}

// Copy creates a deep copy of the SecretsConfig object
func (c *SecretsConfig) Copy() *SecretsConfig {
	if c == nil {
		return nil
	}

	var enabledCopy *bool
	if c.Enabled != nil {
		enabledCopy = ptrBool(*c.Enabled)
	}

	return &SecretsConfig{
		Enabled:  enabledCopy,
		Provider: c.Provider,
	}
}

func ptrBool(b bool) *bool {
	return &b
}
