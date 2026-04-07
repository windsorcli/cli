package gcp

// GCPConfig represents the GCP configuration
type GCPConfig struct {
	// Enabled indicates whether GCP integration is enabled.
	Enabled *bool `yaml:"enabled,omitempty"`

	// ProjectID is the GCP project identifier
	ProjectID *string `yaml:"project_id,omitempty"`

	// CredentialsPath specifies the path to a service account key file.
	CredentialsPath *string `yaml:"credentials_path,omitempty"`

	// QuotaProject specifies the project to use for quota and billing
	QuotaProject *string `yaml:"quota_project,omitempty"`
}

// Merge performs a deep merge of the current GCPConfig with another GCPConfig.
func (base *GCPConfig) Merge(overlay *GCPConfig) {
	if overlay == nil {
		return
	}
	if overlay.Enabled != nil {
		base.Enabled = overlay.Enabled
	}
	if overlay.ProjectID != nil {
		base.ProjectID = overlay.ProjectID
	}
	if overlay.CredentialsPath != nil {
		base.CredentialsPath = overlay.CredentialsPath
	}
	if overlay.QuotaProject != nil {
		base.QuotaProject = overlay.QuotaProject
	}
}

// Copy creates a deep copy of the GCPConfig object
func (c *GCPConfig) Copy() *GCPConfig {
	if c == nil {
		return nil
	}
	return &GCPConfig{
		Enabled:         c.Enabled,
		ProjectID:       c.ProjectID,
		CredentialsPath: c.CredentialsPath,
		QuotaProject:    c.QuotaProject,
	}
}
