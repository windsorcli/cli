package secrets

import (
	"github.com/windsorcli/cli/api/v1alpha2/config/secrets/onepassword"
)

// SecretsConfig represents the Secrets configuration
type SecretsConfig struct {
	OnePassword *onepassword.OnePasswordConfig `yaml:"onepassword,omitempty"`
}

// Merge performs a deep merge of the current SecretsConfig with another SecretsConfig.
func (base *SecretsConfig) Merge(overlay *SecretsConfig) {
	if overlay == nil {
		return
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
		OnePassword: c.OnePassword.DeepCopy(),
	}
}
