package sops

// SopsConfig represents the SOPS secrets provider configuration.
type SopsConfig struct {
	Enabled *bool `yaml:"enabled,omitempty"`
}

// Merge overlays a set Enabled value onto the current SopsConfig.
func (base *SopsConfig) Merge(overlay *SopsConfig) {
	if overlay == nil {
		return
	}
	if overlay.Enabled != nil {
		enabled := *overlay.Enabled
		base.Enabled = &enabled
	}
}

// DeepCopy creates a deep copy of the SopsConfig object.
func (c *SopsConfig) DeepCopy() *SopsConfig {
	if c == nil {
		return nil
	}
	copied := &SopsConfig{}
	if c.Enabled != nil {
		enabled := *c.Enabled
		copied.Enabled = &enabled
	}
	return copied
}
