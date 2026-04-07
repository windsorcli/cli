package workstation

// DNSConfig represents the DNS configuration
type DNSConfig struct {
	Enabled *bool    `yaml:"enabled,omitempty"`
	Domain  *string  `yaml:"domain,omitempty"`
	Address *string  `yaml:"address,omitempty"`
	Forward []string `yaml:"forward,omitempty"`
	Records []string `yaml:"records,omitempty"`
}

// Merge performs a deep merge of the current DNSConfig with another DNSConfig.
func (base *DNSConfig) Merge(overlay *DNSConfig) {
	if overlay == nil {
		return
	}
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

// DeepCopy creates a deep copy of the DNSConfig object
func (c *DNSConfig) DeepCopy() *DNSConfig {
	if c == nil {
		return nil
	}

	var forwardCopy []string
	if c.Forward != nil {
		forwardCopy = make([]string, len(c.Forward))
		copy(forwardCopy, c.Forward)
	}

	var recordsCopy []string
	if c.Records != nil {
		recordsCopy = make([]string, len(c.Records))
		copy(recordsCopy, c.Records)
	}

	return &DNSConfig{
		Enabled: c.Enabled,
		Domain:  c.Domain,
		Address: c.Address,
		Forward: forwardCopy,
		Records: recordsCopy,
	}
}

// Helper function to create boolean pointers
func ptrBool(b bool) *bool {
	return &b
}

// Helper function to create string pointers
func ptrString(s string) *string {
	return &s
}
