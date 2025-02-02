package config

import (
	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/aws"
	"github.com/windsorcli/cli/api/v1alpha1/cluster"
	"github.com/windsorcli/cli/api/v1alpha1/dns"
	"github.com/windsorcli/cli/api/v1alpha1/docker"
	"github.com/windsorcli/cli/api/v1alpha1/git"
	"github.com/windsorcli/cli/api/v1alpha1/network"
	"github.com/windsorcli/cli/api/v1alpha1/terraform"
	"github.com/windsorcli/cli/api/v1alpha1/vm"
	"github.com/windsorcli/cli/pkg/constants"
)

// DefaultConfig returns the default configuration
var DefaultConfig = v1alpha1.Context{
	Environment: map[string]string{},
	AWS: &aws.AWSConfig{
		Enabled:        nil,
		AWSEndpointURL: nil,
		AWSProfile:     nil,
		S3Hostname:     nil,
		MWAAEndpoint:   nil,
		Localstack: &aws.LocalstackConfig{
			Enabled:  nil,
			Services: nil,
		},
	},
	Docker: &docker.DockerConfig{
		Enabled:    nil,
		Registries: map[string]docker.RegistryConfig{},
	},
	Terraform: &terraform.TerraformConfig{
		Enabled: nil,
		Backend: nil,
	},
	Cluster: nil,
	Network: nil,
	DNS: &dns.DNSConfig{
		Enabled: nil,
		Domain:  nil,
		Address: nil,
	},
}

// DefaultLocalConfig returns the default configuration for the "local" context
var DefaultLocalConfig = v1alpha1.Context{
	Environment: map[string]string{},
	Docker: &docker.DockerConfig{
		Enabled: ptrBool(true),
		Registries: map[string]docker.RegistryConfig{
			"registry.test": {},
			"registry-1.docker.io": {
				Remote: "https://registry-1.docker.io",
				Local:  "https://docker.io",
			},
			"registry.k8s.io": {
				Remote: "https://registry.k8s.io",
			},
			"gcr.io": {
				Remote: "https://gcr.io",
			},
			"ghcr.io": {
				Remote: "https://ghcr.io",
			},
			"quay.io": {
				Remote: "https://quay.io",
			},
		},
	},
	Git: &git.GitConfig{
		Livereload: &git.GitLivereloadConfig{
			Enabled:      ptrBool(true),
			RsyncExclude: ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_EXCLUDE),
			RsyncProtect: ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_PROTECT),
			Username:     ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_USERNAME),
			Password:     ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_PASSWORD),
			WebhookUrl:   ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_WEBHOOK_URL),
			Image:        ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_IMAGE),
			VerifySsl:    ptrBool(false),
		},
	},
	Terraform: &terraform.TerraformConfig{
		Enabled: ptrBool(true),
		Backend: ptrString("local"),
	},
	Cluster: &cluster.ClusterConfig{
		Enabled: ptrBool(true),
		Driver:  ptrString("talos"),
		ControlPlanes: struct {
			Count     *int                          `yaml:"count,omitempty"`
			CPU       *int                          `yaml:"cpu,omitempty"`
			Memory    *int                          `yaml:"memory,omitempty"`
			Nodes     map[string]cluster.NodeConfig `yaml:"nodes,omitempty"`
			NodePorts []string                      `yaml:"nodeports,omitempty"`
		}{
			Count:  ptrInt(1),
			CPU:    ptrInt(2),
			Memory: ptrInt(2),
			Nodes:  make(map[string]cluster.NodeConfig),
		},
		Workers: struct {
			Count     *int                          `yaml:"count,omitempty"`
			CPU       *int                          `yaml:"cpu,omitempty"`
			Memory    *int                          `yaml:"memory,omitempty"`
			Nodes     map[string]cluster.NodeConfig `yaml:"nodes,omitempty"`
			NodePorts []string                      `yaml:"nodeports,omitempty"`
		}{
			Count:     ptrInt(1),
			CPU:       ptrInt(4),
			Memory:    ptrInt(4),
			Nodes:     make(map[string]cluster.NodeConfig),
			NodePorts: []string{"8080:30080/tcp", "8443:30443/tcp", "9292:30292/tcp"},
		},
	},
	Network: &network.NetworkConfig{
		CIDRBlock: ptrString("10.5.0.0/16"),
		LoadBalancerIPs: &struct {
			Start *string `yaml:"start,omitempty"`
			End   *string `yaml:"end,omitempty"`
		}{
			Start: ptrString("10.5.1.1"),
			End:   ptrString("10.5.1.10"),
		},
	},
	DNS: &dns.DNSConfig{
		Enabled: ptrBool(true),
		Domain:  ptrString("test"),
		Address: nil,
	},
	VM: &vm.VMConfig{
		Driver: ptrString("docker-desktop"),
	},
}
