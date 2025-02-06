package dns

// DNSConfig represents the DNS configuration
type DNSConfig struct {
	Enabled *bool    `yaml:"enabled"`
	Domain  *string  `yaml:"domain,omitempty"`
	Address *string  `yaml:"address,omitempty"`
	Forward []string `yaml:"forward,omitempty"`
	Records []string `yaml:"records,omitempty"`
}

// Merge performs a deep merge of the current DNSConfig with another DNSConfig.
func (base *DNSConfig) Merge(overlay *DNSConfig) {
	if overlay.Enabled != nil {
		base.Enabled = overlay.Enabled
	}
	if overlay.Domain != nil {
		base.Domain = overlay.Domain
	}
	if overlay.Address != nil {
		base.Address = overlay.Address
	}
	if overlay.Forward != nil {
		base.Forward = overlay.Forward
	}
	if overlay.Records != nil {
		base.Records = overlay.Records
	}
}

// Copy creates a deep copy of the DNSConfig object
func (c *DNSConfig) Copy() *DNSConfig {
	if c == nil {
		return nil
	}
	return &DNSConfig{
		Enabled: c.Enabled,
		Domain:  c.Domain,
		Address: c.Address,
		Forward: c.Forward,
		Records: c.Records,
	}
}
