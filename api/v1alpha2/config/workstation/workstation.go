package workstation

import (
	clusterconfig "github.com/windsorcli/cli/api/v1alpha2/config/workstation/cluster"
	dnsconfig "github.com/windsorcli/cli/api/v1alpha2/config/workstation/dns"
	gitconfig "github.com/windsorcli/cli/api/v1alpha2/config/workstation/git"
	localstackconfig "github.com/windsorcli/cli/api/v1alpha2/config/workstation/localstack"
	networkconfig "github.com/windsorcli/cli/api/v1alpha2/config/workstation/network"
	registriesconfig "github.com/windsorcli/cli/api/v1alpha2/config/workstation/registries"
	vmconfig "github.com/windsorcli/cli/api/v1alpha2/config/workstation/vm"
)

// WorkstationConfig represents the local development workstation configuration
type WorkstationConfig struct {
	Registries *registriesconfig.RegistriesConfig `yaml:"registries,omitempty"`
	Git        *gitconfig.GitConfig               `yaml:"git,omitempty"`
	VM         *vmconfig.VMConfig                 `yaml:"vm,omitempty"`
	Cluster    *clusterconfig.ClusterConfig       `yaml:"cluster,omitempty"`
	Network    *networkconfig.NetworkConfig       `yaml:"network,omitempty"`
	DNS        *dnsconfig.DNSConfig               `yaml:"dns,omitempty"`
	Localstack *localstackconfig.LocalstackConfig `yaml:"localstack,omitempty"`
}

// Merge performs a deep merge of the current WorkstationConfig with another WorkstationConfig.
func (base *WorkstationConfig) Merge(overlay *WorkstationConfig) {
	if overlay == nil {
		return
	}
	if overlay.Registries != nil {
		if base.Registries == nil {
			base.Registries = &registriesconfig.RegistriesConfig{}
		}
		base.Registries.Merge(overlay.Registries)
	}
	if overlay.Git != nil {
		if base.Git == nil {
			base.Git = &gitconfig.GitConfig{}
		}
		base.Git.Merge(overlay.Git)
	}
	if overlay.VM != nil {
		if base.VM == nil {
			base.VM = &vmconfig.VMConfig{}
		}
		base.VM.Merge(overlay.VM)
	}
	if overlay.Cluster != nil {
		if base.Cluster == nil {
			base.Cluster = &clusterconfig.ClusterConfig{}
		}
		base.Cluster.Merge(overlay.Cluster)
	}
	if overlay.Network != nil {
		if base.Network == nil {
			base.Network = &networkconfig.NetworkConfig{}
		}
		base.Network.Merge(overlay.Network)
	}
	if overlay.DNS != nil {
		if base.DNS == nil {
			base.DNS = &dnsconfig.DNSConfig{}
		}
		base.DNS.Merge(overlay.DNS)
	}
	if overlay.Localstack != nil {
		if base.Localstack == nil {
			base.Localstack = &localstackconfig.LocalstackConfig{}
		}
		base.Localstack.Merge(overlay.Localstack)
	}
}

// DeepCopy creates a deep copy of the WorkstationConfig object
func (c *WorkstationConfig) DeepCopy() *WorkstationConfig {
	if c == nil {
		return nil
	}
	return &WorkstationConfig{
		Registries: c.Registries.DeepCopy(),
		Git:        c.Git.DeepCopy(),
		VM:         c.VM.DeepCopy(),
		Cluster:    c.Cluster.DeepCopy(),
		Network:    c.Network.DeepCopy(),
		DNS:        c.DNS.DeepCopy(),
		Localstack: c.Localstack.DeepCopy(),
	}
}

// Helper function to create boolean pointers
func ptrBool(b bool) *bool {
	return &b
}
