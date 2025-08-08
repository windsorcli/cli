package providers

import (
	"github.com/windsorcli/cli/api/v1alpha2/config/providers/aws"
	"github.com/windsorcli/cli/api/v1alpha2/config/providers/azure"
)

// ProvidersConfig represents the configuration for all cloud providers
type ProvidersConfig struct {
	AWS   *aws.AWSConfig     `yaml:"aws,omitempty"`
	Azure *azure.AzureConfig `yaml:"azure,omitempty"`
}

// Merge performs a deep merge of the current ProvidersConfig with another ProvidersConfig.
func (base *ProvidersConfig) Merge(overlay *ProvidersConfig) {
	if overlay == nil {
		return
	}
	if overlay.AWS != nil {
		if base.AWS == nil {
			base.AWS = &aws.AWSConfig{}
		}
		base.AWS.Merge(overlay.AWS)
	}
	if overlay.Azure != nil {
		if base.Azure == nil {
			base.Azure = &azure.AzureConfig{}
		}
		base.Azure.Merge(overlay.Azure)
	}
}

// DeepCopy creates a deep copy of the ProvidersConfig object
func (c *ProvidersConfig) DeepCopy() *ProvidersConfig {
	if c == nil {
		return nil
	}
	copied := &ProvidersConfig{}

	if c.AWS != nil {
		copied.AWS = c.AWS.DeepCopy()
	}
	if c.Azure != nil {
		copied.Azure = c.Azure.DeepCopy()
	}

	return copied
}
