package v1alpha1

import (
	"github.com/windsorcli/cli/api/v1alpha1/aws"
	"github.com/windsorcli/cli/api/v1alpha1/azure"
	"github.com/windsorcli/cli/api/v1alpha1/cluster"
	"github.com/windsorcli/cli/api/v1alpha1/dns"
	"github.com/windsorcli/cli/api/v1alpha1/docker"
	"github.com/windsorcli/cli/api/v1alpha1/gcp"
	"github.com/windsorcli/cli/api/v1alpha1/git"
	"github.com/windsorcli/cli/api/v1alpha1/network"
	"github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/api/v1alpha1/terraform"
	"github.com/windsorcli/cli/api/v1alpha1/vm"
)

// Config represents the entire configuration
type Config struct {
	Version      string              `yaml:"version"`
	ToolsManager string              `yaml:"toolsManager,omitempty"`
	Contexts     map[string]*Context `yaml:"contexts"`
}

// Context represents the context configuration
type Context struct {
	ID          *string                    `yaml:"id,omitempty"`
	Provider    *string                    `yaml:"provider,omitempty"`
	Environment map[string]string          `yaml:"environment,omitempty"`
	Secrets     *secrets.SecretsConfig     `yaml:"secrets,omitempty"`
	AWS         *aws.AWSConfig             `yaml:"aws,omitempty"`
	Azure       *azure.AzureConfig         `yaml:"azure,omitempty"`
	GCP         *gcp.GCPConfig             `yaml:"gcp,omitempty"`
	Docker      *docker.DockerConfig       `yaml:"docker,omitempty"`
	Git         *git.GitConfig             `yaml:"git,omitempty"`
	Terraform   *terraform.TerraformConfig `yaml:"terraform,omitempty"`
	VM          *vm.VMConfig               `yaml:"vm,omitempty"`
	Cluster     *cluster.ClusterConfig     `yaml:"cluster,omitempty"`
	Network     *network.NetworkConfig     `yaml:"network,omitempty"`
	DNS         *dns.DNSConfig             `yaml:"dns,omitempty"`
}

// Merge performs a deep merge of the current Context with another Context.
func (base *Context) Merge(overlay *Context) {
	if overlay == nil {
		return
	}
	if overlay.ID != nil {
		base.ID = overlay.ID
	}
	if overlay.Provider != nil {
		base.Provider = overlay.Provider
	}
	if overlay.Environment != nil {
		if base.Environment == nil {
			base.Environment = make(map[string]string)
		}
		for key, value := range overlay.Environment {
			base.Environment[key] = value
		}
	}
	if overlay.Secrets != nil {
		if base.Secrets == nil {
			base.Secrets = &secrets.SecretsConfig{}
		}
		base.Secrets.Merge(overlay.Secrets)
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
	if overlay.Docker != nil {
		if base.Docker == nil {
			base.Docker = &docker.DockerConfig{}
		}
		base.Docker.Merge(overlay.Docker)
	}
	if overlay.Git != nil {
		if base.Git == nil {
			base.Git = &git.GitConfig{}
		}
		base.Git.Merge(overlay.Git)
	}
	if overlay.Terraform != nil {
		if base.Terraform == nil {
			base.Terraform = &terraform.TerraformConfig{}
		}
		base.Terraform.Merge(overlay.Terraform)
	}
	if overlay.VM != nil {
		if base.VM == nil {
			base.VM = &vm.VMConfig{}
		}
		base.VM.Merge(overlay.VM)
	}
	if overlay.Cluster != nil {
		if base.Cluster == nil {
			base.Cluster = &cluster.ClusterConfig{}
		}
		base.Cluster.Merge(overlay.Cluster)
	}
	if overlay.Network != nil {
		if base.Network == nil {
			base.Network = &network.NetworkConfig{}
		}
		base.Network.Merge(overlay.Network)
	}
	if overlay.DNS != nil {
		if base.DNS == nil {
			base.DNS = &dns.DNSConfig{}
		}
		base.DNS.Merge(overlay.DNS)
	}
}

// DeepCopy creates a deep copy of the Context object
func (c *Context) DeepCopy() *Context {
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
		ID:          c.ID,
		Provider:    c.Provider,
		Environment: environmentCopy,
		Secrets:     c.Secrets.Copy(),
		AWS:         c.AWS.Copy(),
		Azure:       c.Azure.Copy(),
		GCP:         c.GCP.Copy(),
		Docker:      c.Docker.Copy(),
		Git:         c.Git.Copy(),
		Terraform:   c.Terraform.Copy(),
		VM:          c.VM.Copy(),
		Cluster:     c.Cluster.Copy(),
		Network:     c.Network.Copy(),
		DNS:         c.DNS.Copy(),
	}
}
