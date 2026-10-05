package secrets

// SecretsConfig represents the Secrets configuration
type SecretsConfig struct {
	OnePasswordConfig `yaml:"onepassword,omitempty"`
}

type OnePasswordConfig struct {
	Vaults map[string]OnePasswordVault `yaml:"vaults,omitempty"`
}

type OnePasswordVault struct {
	ID   string `yaml:"id,omitempty"`
	URL  string `yaml:"url,omitempty"`
	Name string `yaml:"name,omitempty"`
}

// Merge performs a deep merge of the current SecretsConfig with another SecretsConfig.
func (base *SecretsConfig) Merge(overlay *SecretsConfig) {
	if overlay == nil {
		return
	}

	if base.Vaults == nil {
		base.Vaults = make(map[string]OnePasswordVault)
	}

	for key, overlayVault := range overlay.Vaults {
		if baseVault, exists := base.Vaults[key]; exists {
			if overlayVault.URL != "" {
				baseVault.URL = overlayVault.URL
			}
			if overlayVault.Name != "" {
				baseVault.Name = overlayVault.Name
			}
			base.Vaults[key] = baseVault
		} else {
			base.Vaults[key] = overlayVault
		}
	}
}

// Copy creates a deep copy of the SecretsConfig object
func (c *SecretsConfig) Copy() *SecretsConfig {
	if c == nil {
		return nil
	}

	copy := &SecretsConfig{
		OnePasswordConfig: OnePasswordConfig{
			Vaults: make(map[string]OnePasswordVault),
		},
	}

	for key, vault := range c.Vaults {
		copy.Vaults[key] = vault
	}

	return copy
}
