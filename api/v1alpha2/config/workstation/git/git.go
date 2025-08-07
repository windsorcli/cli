package workstation

// GitConfig represents the Git configuration
type GitConfig struct {
	Livereload *GitLivereloadConfig `yaml:"livereload"`
}

// GitLivereloadConfig represents the Git livereload configuration
type GitLivereloadConfig struct {
	Enabled    *bool   `yaml:"enabled"`
	Include    *string `yaml:"include,omitempty"`
	Exclude    *string `yaml:"exclude,omitempty"`
	Protect    *string `yaml:"protect,omitempty"`
	Username   *string `yaml:"username,omitempty"`
	Password   *string `yaml:"password,omitempty"`
	WebhookUrl *string `yaml:"webhook_url,omitempty"`
	VerifySsl  *bool   `yaml:"verify_ssl,omitempty"`
	Image      *string `yaml:"image,omitempty"`
}

// Merge performs a deep merge of the current GitConfig with another GitConfig.
func (base *GitConfig) Merge(overlay *GitConfig) {
	if overlay == nil {
		return
	}
	if overlay.Livereload != nil {
		if base.Livereload == nil {
			base.Livereload = &GitLivereloadConfig{}
		}
		if overlay.Livereload.Enabled != nil {
			base.Livereload.Enabled = overlay.Livereload.Enabled
		}
		if overlay.Livereload.Include != nil {
			base.Livereload.Include = overlay.Livereload.Include
		}
		if overlay.Livereload.Exclude != nil {
			base.Livereload.Exclude = overlay.Livereload.Exclude
		}
		if overlay.Livereload.Protect != nil {
			base.Livereload.Protect = overlay.Livereload.Protect
		}
		if overlay.Livereload.Username != nil {
			base.Livereload.Username = overlay.Livereload.Username
		}
		if overlay.Livereload.Password != nil {
			base.Livereload.Password = overlay.Livereload.Password
		}
		if overlay.Livereload.WebhookUrl != nil {
			base.Livereload.WebhookUrl = overlay.Livereload.WebhookUrl
		}
		if overlay.Livereload.VerifySsl != nil {
			base.Livereload.VerifySsl = overlay.Livereload.VerifySsl
		}
		if overlay.Livereload.Image != nil {
			base.Livereload.Image = overlay.Livereload.Image
		}
	}
}

// DeepCopy creates a deep copy of the GitConfig object
func (c *GitConfig) DeepCopy() *GitConfig {
	if c == nil {
		return nil
	}
	var livereloadCopy *GitLivereloadConfig
	if c.Livereload != nil {
		livereloadCopy = &GitLivereloadConfig{}

		if c.Livereload.Enabled != nil {
			enabledCopy := *c.Livereload.Enabled
			livereloadCopy.Enabled = &enabledCopy
		}
		if c.Livereload.Include != nil {
			includeCopy := *c.Livereload.Include
			livereloadCopy.Include = &includeCopy
		}
		if c.Livereload.Exclude != nil {
			excludeCopy := *c.Livereload.Exclude
			livereloadCopy.Exclude = &excludeCopy
		}
		if c.Livereload.Protect != nil {
			protectCopy := *c.Livereload.Protect
			livereloadCopy.Protect = &protectCopy
		}
		if c.Livereload.Username != nil {
			usernameCopy := *c.Livereload.Username
			livereloadCopy.Username = &usernameCopy
		}
		if c.Livereload.Password != nil {
			passwordCopy := *c.Livereload.Password
			livereloadCopy.Password = &passwordCopy
		}
		if c.Livereload.WebhookUrl != nil {
			webhookCopy := *c.Livereload.WebhookUrl
			livereloadCopy.WebhookUrl = &webhookCopy
		}
		if c.Livereload.VerifySsl != nil {
			verifyCopy := *c.Livereload.VerifySsl
			livereloadCopy.VerifySsl = &verifyCopy
		}
		if c.Livereload.Image != nil {
			imageCopy := *c.Livereload.Image
			livereloadCopy.Image = &imageCopy
		}
	}
	return &GitConfig{
		Livereload: livereloadCopy,
	}
}
