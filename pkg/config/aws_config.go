package config

// AWSConfig represents the AWS configuration
type AWSConfig struct {
	Enabled        *bool             `yaml:"enabled,omitempty"`
	AWSEndpointURL *string           `yaml:"aws_endpoint_url,omitempty"`
	AWSProfile     *string           `yaml:"aws_profile,omitempty"`
	S3Hostname     *string           `yaml:"s3_hostname,omitempty"`
	MWAAEndpoint   *string           `yaml:"mwaa_endpoint,omitempty"`
	Localstack     *LocalstackConfig `yaml:"localstack,omitempty"`
}

// LocalstackConfig represents the Localstack configuration
type LocalstackConfig struct {
	Enabled  *bool    `yaml:"enabled,omitempty"`
	Services []string `yaml:"services,omitempty"`
}

// Merge performs a deep merge of the current AWSConfig with another AWSConfig.
func (base *AWSConfig) Merge(overlay *AWSConfig) {
	if overlay.Enabled != nil {
		base.Enabled = overlay.Enabled
	}
	if overlay.AWSEndpointURL != nil {
		base.AWSEndpointURL = overlay.AWSEndpointURL
	}
	if overlay.AWSProfile != nil {
		base.AWSProfile = overlay.AWSProfile
	}
	if overlay.S3Hostname != nil {
		base.S3Hostname = overlay.S3Hostname
	}
	if overlay.MWAAEndpoint != nil {
		base.MWAAEndpoint = overlay.MWAAEndpoint
	}
	if overlay.Localstack != nil {
		if base.Localstack == nil {
			base.Localstack = &LocalstackConfig{}
		}
		if overlay.Localstack.Enabled != nil {
			base.Localstack.Enabled = overlay.Localstack.Enabled
		}
		if overlay.Localstack.Services != nil {
			base.Localstack.Services = overlay.Localstack.Services
		}
	}
}

// Copy creates a deep copy of the AWSConfig object
func (c *AWSConfig) Copy() *AWSConfig {
	if c == nil {
		return nil
	}
	copy := &AWSConfig{}
	if c.Enabled != nil {
		copy.Enabled = c.Enabled
	}
	if c.AWSEndpointURL != nil {
		copy.AWSEndpointURL = c.AWSEndpointURL
	}
	if c.AWSProfile != nil {
		copy.AWSProfile = c.AWSProfile
	}
	if c.S3Hostname != nil {
		copy.S3Hostname = c.S3Hostname
	}
	if c.MWAAEndpoint != nil {
		copy.MWAAEndpoint = c.MWAAEndpoint
	}
	if c.Localstack != nil {
		copy.Localstack = &LocalstackConfig{}
		if c.Localstack.Enabled != nil {
			copy.Localstack.Enabled = c.Localstack.Enabled
		}
		if c.Localstack.Services != nil {
			copy.Localstack.Services = c.Localstack.Services
		}
	}
	return copy
}
