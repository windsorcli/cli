package hetzner

// HetznerConfig represents the Hetzner Cloud configuration. Hetzner integration activates
// whenever this block is present in a context (or when platform is "hetzner"); there is no
// separate `enabled` flag.
type HetznerConfig struct {
	// Token is the Hetzner Cloud API token, exported to downstream tools as HCLOUD_TOKEN.
	Token *string `yaml:"token,omitempty"`
}

// Merge performs a deep merge of the current HetznerConfig with another HetznerConfig.
func (base *HetznerConfig) Merge(overlay *HetznerConfig) {
	if overlay == nil {
		return
	}
	if overlay.Token != nil {
		base.Token = overlay.Token
	}
}

// Copy creates a deep copy of the HetznerConfig object
func (c *HetznerConfig) Copy() *HetznerConfig {
	if c == nil {
		return nil
	}
	return &HetznerConfig{
		Token: c.Token,
	}
}
