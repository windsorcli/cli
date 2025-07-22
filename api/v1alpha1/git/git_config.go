package git

// GitConfig represents the Git configuration
type GitConfig struct {
	Livereload *GitLivereloadConfig `yaml:"livereload"`
}

// GitLivereloadConfig represents the Git livereload configuration
type GitLivereloadConfig struct {
	Enabled      *bool   `yaml:"enabled"`
	RsyncInclude *string `yaml:"rsync_include,omitempty"`
	RsyncExclude *string `yaml:"rsync_exclude,omitempty"`
	RsyncProtect *string `yaml:"rsync_protect,omitempty"`
	Username     *string `yaml:"username,omitempty"`
	Password     *string `yaml:"password,omitempty"`
	WebhookUrl   *string `yaml:"webhook_url,omitempty"`
	VerifySsl    *bool   `yaml:"verify_ssl,omitempty"`
	Image        *string `yaml:"image,omitempty"`
}

// Merge performs a deep merge of the current GitConfig with another GitConfig.
func (base *GitConfig) Merge(overlay *GitConfig) {
	if overlay.Livereload != nil {
		if base.Livereload == nil {
			base.Livereload = &GitLivereloadConfig{}
		}
		if overlay.Livereload.Enabled != nil {
			base.Livereload.Enabled = overlay.Livereload.Enabled
		}
		if overlay.Livereload.RsyncInclude != nil {
			base.Livereload.RsyncInclude = overlay.Livereload.RsyncInclude
		}
		if overlay.Livereload.RsyncExclude != nil {
			base.Livereload.RsyncExclude = overlay.Livereload.RsyncExclude
		}
		if overlay.Livereload.RsyncProtect != nil {
			base.Livereload.RsyncProtect = overlay.Livereload.RsyncProtect
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

// Copy creates a deep copy of the GitConfig object
func (c *GitConfig) Copy() *GitConfig {
	if c == nil {
		return nil
	}
	var livereloadCopy *GitLivereloadConfig
	if c.Livereload != nil {
		livereloadCopy = &GitLivereloadConfig{
			Enabled:      c.Livereload.Enabled,
			RsyncInclude: c.Livereload.RsyncInclude,
			RsyncExclude: c.Livereload.RsyncExclude,
			RsyncProtect: c.Livereload.RsyncProtect,
			Username:     c.Livereload.Username,
			Password:     c.Livereload.Password,
			WebhookUrl:   c.Livereload.WebhookUrl,
			VerifySsl:    c.Livereload.VerifySsl,
			Image:        c.Livereload.Image,
		}
	}
	return &GitConfig{
		Livereload: livereloadCopy,
	}
}
