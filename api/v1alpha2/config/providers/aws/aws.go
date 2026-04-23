package aws

// AWSConfig represents the AWS configuration
type AWSConfig struct {
	// Enabled indicates whether AWS integration is enabled.
	Enabled *bool `yaml:"enabled,omitempty"`

	// AWSEndpointURL specifies the custom endpoint URL for AWS services.
	AWSEndpointURL *string `yaml:"endpoint_url,omitempty"`

	// AWSProfile defines the AWS CLI profile to use for authentication.
	AWSProfile *string `yaml:"profile,omitempty"`

	// AWSRegion is the AWS region used for API calls, exported to downstream tools as AWS_REGION.
	AWSRegion *string `yaml:"region,omitempty"`

	// S3Hostname sets the custom hostname for the S3 service.
	S3Hostname *string `yaml:"s3_hostname,omitempty"`

	// MWAAEndpoint specifies the endpoint for Managed Workflows for Apache Airflow.
	MWAAEndpoint *string `yaml:"mwaa_endpoint,omitempty"`
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
	if overlay.AWSRegion != nil {
		base.AWSRegion = overlay.AWSRegion
	}
	if overlay.S3Hostname != nil {
		base.S3Hostname = overlay.S3Hostname
	}
	if overlay.MWAAEndpoint != nil {
		base.MWAAEndpoint = overlay.MWAAEndpoint
	}
}

// DeepCopy creates a deep copy of the AWSConfig object
func (c *AWSConfig) DeepCopy() *AWSConfig {
	if c == nil {
		return nil
	}
	copied := &AWSConfig{}

	if c.Enabled != nil {
		enabledCopy := *c.Enabled
		copied.Enabled = &enabledCopy
	}
	if c.AWSEndpointURL != nil {
		urlCopy := *c.AWSEndpointURL
		copied.AWSEndpointURL = &urlCopy
	}
	if c.AWSProfile != nil {
		profileCopy := *c.AWSProfile
		copied.AWSProfile = &profileCopy
	}
	if c.AWSRegion != nil {
		regionCopy := *c.AWSRegion
		copied.AWSRegion = &regionCopy
	}
	if c.S3Hostname != nil {
		hostnameCopy := *c.S3Hostname
		copied.S3Hostname = &hostnameCopy
	}
	if c.MWAAEndpoint != nil {
		endpointCopy := *c.MWAAEndpoint
		copied.MWAAEndpoint = &endpointCopy
	}

	return copied
}
