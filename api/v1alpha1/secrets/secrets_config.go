package secrets

// SecretsConfig represents the Secrets configuration
type SecretsConfig struct {
	Provider string `yaml:"provider,omitempty"`
}

// Merge performs a deep merge of the current SecretsConfig with another SecretsConfig.
func (base *SecretsConfig) Merge(overlay *SecretsConfig) {
	if overlay.Provider != "" {
		base.Provider = overlay.Provider
	}
}

// Copy creates a deep copy of the SecretsConfig object
func (c *SecretsConfig) Copy() *SecretsConfig {
	if c == nil {
		return nil
	}

	return &SecretsConfig{
		Provider: c.Provider,
	}
}
