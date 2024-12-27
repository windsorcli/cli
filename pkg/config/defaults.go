package config

import (
	"github.com/windsorcli/cli/pkg/constants"
)

// DefaultConfig is a default configuration that populates values for all
// lists and maps.
var DefaultConfig = Context{
	AWS: &AWSConfig{
		Localstack: &LocalstackConfig{},
	},
	Docker: &DockerConfig{
		Registries: []RegistryConfig{},
	},
	Terraform: &TerraformConfig{},
	Cluster: &ClusterConfig{
		ControlPlanes: struct {
			Count  *int                  `yaml:"count,omitempty"`
			CPU    *int                  `yaml:"cpu,omitempty"`
			Memory *int                  `yaml:"memory,omitempty"`
			Nodes  map[string]NodeConfig `yaml:"nodes,omitempty"`
		}{
			Nodes: make(map[string]NodeConfig),
		},
		Workers: struct {
			Count  *int                  `yaml:"count,omitempty"`
			CPU    *int                  `yaml:"cpu,omitempty"`
			Memory *int                  `yaml:"memory,omitempty"`
			Nodes  map[string]NodeConfig `yaml:"nodes,omitempty"`
		}{
			Nodes: make(map[string]NodeConfig),
		},
	},
	DNS: &DNSConfig{},
}

// DefaultLocalConfig returns the default configuration for the "local" context
var DefaultLocalConfig = Context{
	Environment: map[string]string{},
	Docker: &DockerConfig{
		Enabled: ptrBool(true),
		Registries: []RegistryConfig{
			{
				Name: "registry",
			},
			{
				Name:   "registry-1.docker",
				Remote: "https://registry-1.io",
				Local:  "https://io",
			},
			{
				Name:   "registry.k8s",
				Remote: "https://registry.k8s.io",
			},
			{
				Name:   "gcr",
				Remote: "https://gcr.io",
			},
			{
				Name:   "ghcr",
				Remote: "https://ghcr.io",
			},
			{
				Name:   "quay",
				Remote: "https://quay.io",
			},
		},
		NetworkCIDR: ptrString("10.5.0.0/16"),
	},
	Git: &GitConfig{
		Livereload: &GitLivereloadConfig{
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
	Terraform: &TerraformConfig{
		Backend: ptrString("local"),
	},
	Cluster: &ClusterConfig{
		Enabled: ptrBool(true),
		Driver:  ptrString("talos"),
		ControlPlanes: struct {
			Count  *int                  `yaml:"count,omitempty"`
			CPU    *int                  `yaml:"cpu,omitempty"`
			Memory *int                  `yaml:"memory,omitempty"`
			Nodes  map[string]NodeConfig `yaml:"nodes,omitempty"`
		}{
			Count:  ptrInt(1),
			CPU:    ptrInt(2),
			Memory: ptrInt(2),
			Nodes:  make(map[string]NodeConfig),
		},
		Workers: struct {
			Count  *int                  `yaml:"count,omitempty"`
			CPU    *int                  `yaml:"cpu,omitempty"`
			Memory *int                  `yaml:"memory,omitempty"`
			Nodes  map[string]NodeConfig `yaml:"nodes,omitempty"`
		}{
			Count:  ptrInt(1),
			CPU:    ptrInt(4),
			Memory: ptrInt(4),
			Nodes:  make(map[string]NodeConfig),
		},
	},
	DNS: &DNSConfig{
		Enabled: ptrBool(true),
		Name:    ptrString("test"),
		Address: nil,
	},
}
