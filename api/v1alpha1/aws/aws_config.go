package aws

// AWSConfig represents the AWS configuration
type AWSConfig struct {
	// Enabled indicates whether AWS integration is enabled.
	Enabled *bool `yaml:"enabled,omitempty"`

	// EndpointURL specifies the custom endpoint URL for AWS services.
	EndpointURL *string `yaml:"endpoint_url,omitempty"`

	// Profile defines the AWS CLI profile to use for authentication.
	Profile *string `yaml:"profile,omitempty"`

	// S3Hostname sets the custom hostname for the S3 service.
	S3Hostname *string `yaml:"s3_hostname,omitempty"`

	// MWAAEndpoint specifies the endpoint for Managed Workflows for Apache Airflow.
	MWAAEndpoint *string `yaml:"mwaa_endpoint,omitempty"`

	// Localstack contains the configuration for Localstack, a local AWS cloud emulator.
	Localstack *LocalstackConfig `yaml:"localstack,omitempty"`

	// Region specifies the AWS region to use.
	Region *string `yaml:"region,omitempty"`
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
	if overlay.EndpointURL != nil {
		base.EndpointURL = overlay.EndpointURL
	}
	if overlay.Profile != nil {
		base.Profile = overlay.Profile
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
	if overlay.Region != nil {
		base.Region = overlay.Region
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
	if c.EndpointURL != nil {
		copy.EndpointURL = c.EndpointURL
	}
	if c.Profile != nil {
		copy.Profile = c.Profile
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
	if c.Region != nil {
		copy.Region = c.Region
	}
	return copy
}
