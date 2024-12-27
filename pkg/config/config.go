package config

// Context represents the context configuration
type Context struct {
	Environment map[string]string `yaml:"environment,omitempty"`
	AWS         *AWSConfig        `yaml:"aws,omitempty"`
	Docker      *DockerConfig     `yaml:"docker,omitempty"`
	Git         *GitConfig        `yaml:"git,omitempty"`
	Terraform   *TerraformConfig  `yaml:"terraform,omitempty"`
	VM          *VMConfig         `yaml:"vm,omitempty"`
	Cluster     *ClusterConfig    `yaml:"cluster,omitempty"`
	DNS         *DNSConfig        `yaml:"dns,omitempty"`
}

// Merge performs a deep merge of the current Context with another Context.
func (base *Context) Merge(overlay *Context) {
	if overlay.Environment != nil {
		if base.Environment == nil {
			base.Environment = make(map[string]string)
		}
		for key, value := range overlay.Environment {
			base.Environment[key] = value
		}
	}
	if overlay.AWS != nil {
		if base.AWS == nil {
			base.AWS = &AWSConfig{}
		}
		base.AWS.Merge(overlay.AWS)
	}
	if overlay.Docker != nil {
		if base.Docker == nil {
			base.Docker = &DockerConfig{}
		}
		base.Docker.Merge(overlay.Docker)
	}
	if overlay.Git != nil {
		if base.Git == nil {
			base.Git = &GitConfig{}
		}
		base.Git.Merge(overlay.Git)
	}
	if overlay.Terraform != nil {
		if base.Terraform == nil {
			base.Terraform = &TerraformConfig{}
		}
		base.Terraform.Merge(overlay.Terraform)
	}
	if overlay.VM != nil {
		if base.VM == nil {
			base.VM = &VMConfig{}
		}
		base.VM.Merge(overlay.VM)
	}
	if overlay.Cluster != nil {
		if base.Cluster == nil {
			base.Cluster = &ClusterConfig{}
		}
		base.Cluster.Merge(overlay.Cluster)
	}
	if overlay.DNS != nil {
		if base.DNS == nil {
			base.DNS = &DNSConfig{}
		}
		base.DNS.Merge(overlay.DNS)
	}
}

// Copy creates a deep copy of the Context object
func (c *Context) Copy() *Context {
	if c == nil {
		return nil
	}
	var environmentCopy map[string]string
	if c.Environment != nil {
		environmentCopy = make(map[string]string, len(c.Environment))
		for key, value := range c.Environment {
			environmentCopy[key] = value
		}
	}
	return &Context{
		Environment: environmentCopy,
		AWS:         c.AWS.Copy(),
		Docker:      c.Docker.Copy(),
		Git:         c.Git.Copy(),
		Terraform:   c.Terraform.Copy(),
		VM:          c.VM.Copy(),
		Cluster:     c.Cluster.Copy(),
		DNS:         c.DNS.Copy(),
	}
}

// Config represents the entire configuration
type Config struct {
	Context  *string             `yaml:"context"`
	Contexts map[string]*Context `yaml:"contexts"`
}
