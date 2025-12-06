package onepassword

// OnePasswordConfig represents the OnePassword configuration
type OnePasswordConfig struct {
	Vaults map[string]OnePasswordVault `yaml:"vaults,omitempty"`
}

type OnePasswordVault struct {
	ID   string `yaml:"id,omitempty"`
	URL  string `yaml:"url,omitempty"`
	Name string `yaml:"name,omitempty"`
}

// Merge performs a deep merge of the current OnePasswordConfig with another OnePasswordConfig.
func (base *OnePasswordConfig) Merge(overlay *OnePasswordConfig) {
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
			if overlayVault.ID != "" {
				baseVault.ID = overlayVault.ID
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

// DeepCopy creates a deep copy of the OnePasswordConfig object
func (c *OnePasswordConfig) DeepCopy() *OnePasswordConfig {
	if c == nil {
		return nil
	}

	copied := &OnePasswordConfig{
		Vaults: make(map[string]OnePasswordVault),
	}

	for key, vault := range c.Vaults {
		copied.Vaults[key] = vault
	}

	return copied
}
