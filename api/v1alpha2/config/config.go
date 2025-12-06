package v1alpha2

import (
	"maps"

	"github.com/windsorcli/cli/api/v1alpha2/config/providers"
	"github.com/windsorcli/cli/api/v1alpha2/config/secrets"
	"github.com/windsorcli/cli/api/v1alpha2/config/terraform"
	"github.com/windsorcli/cli/api/v1alpha2/config/workstation"
)

// Config represents the entire configuration
type Config struct {
	Version     string                         `yaml:"version"`
	Workstation *workstation.WorkstationConfig `yaml:"workstation,omitempty"`
	Environment map[string]string              `yaml:"environment,omitempty"`
	Secrets     *secrets.SecretsConfig         `yaml:"secrets,omitempty"`
	Providers   *providers.ProvidersConfig     `yaml:"providers,omitempty"`
	Terraform   *terraform.TerraformConfig     `yaml:"terraform,omitempty"`
}

// Merge performs a deep merge of the current Config with another Config.
func (base *Config) Merge(overlay *Config) {
	if overlay == nil {
		return
	}
	if overlay.Environment != nil {
		if base.Environment == nil {
			base.Environment = make(map[string]string)
		}
		maps.Copy(base.Environment, overlay.Environment)
	}
	if overlay.Secrets != nil {
		if base.Secrets == nil {
			base.Secrets = &secrets.SecretsConfig{}
		}
		base.Secrets.Merge(overlay.Secrets)
	}
	if overlay.Providers != nil {
		if base.Providers == nil {
			base.Providers = &providers.ProvidersConfig{}
		}
		base.Providers.Merge(overlay.Providers)
	}
	if overlay.Terraform != nil {
		if base.Terraform == nil {
			base.Terraform = &terraform.TerraformConfig{}
		}
		base.Terraform.Merge(overlay.Terraform)
	}
	if overlay.Workstation != nil {
		if base.Workstation == nil {
			base.Workstation = &workstation.WorkstationConfig{}
		}
		base.Workstation.Merge(overlay.Workstation)
	}
}

// DeepCopy creates a deep copy of the Config object
func (c *Config) DeepCopy() *Config {
	if c == nil {
		return nil
	}
	var environmentCopy map[string]string
	if c.Environment != nil {
		environmentCopy = make(map[string]string, len(c.Environment))
		maps.Copy(environmentCopy, c.Environment)
	}
	return &Config{
		Version:     c.Version,
		Workstation: c.Workstation.DeepCopy(),
		Environment: environmentCopy,
		Secrets:     c.Secrets.DeepCopy(),
		Providers:   c.Providers.DeepCopy(),
		Terraform:   c.Terraform.DeepCopy(),
	}
}
