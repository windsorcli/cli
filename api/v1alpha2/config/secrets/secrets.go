package secrets

import (
	"github.com/windsorcli/cli/api/v1alpha2/config/secrets/onepassword"
)

// SecretsConfig represents the Secrets configuration
type SecretsConfig struct {
	Sops        *SopsConfig                    `yaml:"sops,omitempty"`
	OnePassword *onepassword.OnePasswordConfig `yaml:"onepassword,omitempty"`
}

// SopsConfig represents SOPS provider configuration.
type SopsConfig struct {
	Enabled *bool `yaml:"enabled,omitempty"`
}

// Merge performs a deep merge of the current SecretsConfig with another SecretsConfig.
func (base *SecretsConfig) Merge(overlay *SecretsConfig) {
	if overlay == nil {
		return
	}
	if overlay.Sops != nil {
		if base.Sops == nil {
			base.Sops = &SopsConfig{}
		}
		if overlay.Sops.Enabled != nil {
			base.Sops.Enabled = overlay.Sops.Enabled
		}
	}
	if overlay.OnePassword != nil {
		if base.OnePassword == nil {
			base.OnePassword = &onepassword.OnePasswordConfig{}
		}
		base.OnePassword.Merge(overlay.OnePassword)
	}
}

// DeepCopy creates a deep copy of the SecretsConfig object
func (c *SecretsConfig) DeepCopy() *SecretsConfig {
	if c == nil {
		return nil
	}
	return &SecretsConfig{
		Sops:        c.Sops.DeepCopy(),
		OnePassword: c.OnePassword.DeepCopy(),
	}
}

// DeepCopy creates a deep copy of the SopsConfig object.
func (c *SopsConfig) DeepCopy() *SopsConfig {
	if c == nil {
		return nil
	}
	var enabledCopy *bool
	if c.Enabled != nil {
		v := *c.Enabled
		enabledCopy = &v
	}
	return &SopsConfig{
		Enabled: enabledCopy,
	}
}
