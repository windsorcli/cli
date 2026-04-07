package providers

import (
	"github.com/windsorcli/cli/api/v1alpha2/config/providers/aws"
	"github.com/windsorcli/cli/api/v1alpha2/config/providers/azure"
	"github.com/windsorcli/cli/api/v1alpha2/config/providers/gcp"
)

// ProvidersConfig represents the configuration for all cloud providers
type ProvidersConfig struct {
	AWS   *aws.AWSConfig     `yaml:"aws,omitempty"`
	Azure *azure.AzureConfig `yaml:"azure,omitempty"`
	GCP   *gcp.GCPConfig     `yaml:"gcp,omitempty"`
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
	if overlay.GCP != nil {
		if base.GCP == nil {
			base.GCP = &gcp.GCPConfig{}
		}
		base.GCP.Merge(overlay.GCP)
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
	if c.GCP != nil {
		copied.GCP = c.GCP.DeepCopy()
	}

	return copied
}
