package config

import "github.com/windsor-hotel/cli/internal/constants"

// AWSConfig represents the AWS configuration
type AWSConfig struct {
	AWSEndpointURL *string           `yaml:"aws_endpoint_url"`
	AWSProfile     *string           `yaml:"aws_profile"`
	S3Hostname     *string           `yaml:"s3_hostname"`
	MWAAEndpoint   *string           `yaml:"mwaa_endpoint"`
	Localstack     *LocalstackConfig `yaml:"localstack"`
}

// LocalstackConfig represents the Localstack configuration
type LocalstackConfig struct {
	Create   *bool    `yaml:"create"`
	Services []string `yaml:"services"`
}

// DockerConfig represents the Docker configuration
type DockerConfig struct {
	Enabled     *bool      `yaml:"enabled"`
	Registries  []Registry `yaml:"registries"`
	NetworkCIDR *string    `yaml:"network_cidr"`
}

// GitConfig represents the Git configuration
type GitConfig struct {
	Livereload *GitLivereloadConfig `yaml:"livereload"`
}

// GitLivereloadConfig represents the Git livereload configuration
type GitLivereloadConfig struct {
	Create       *bool   `yaml:"create"`
	RsyncExclude *string `yaml:"rsync_exclude"`
	RsyncProtect *string `yaml:"rsync_protect"`
	Username     *string `yaml:"username"`
	Password     *string `yaml:"password"`
	WebhookUrl   *string `yaml:"webhook_url"`
	VerifySsl    *bool   `yaml:"verify_ssl"`
	Image        *string `yaml:"image"`
}

type Registry struct {
	Name   string `yaml:"name"`
	Remote string `yaml:"remote"`
	Local  string `yaml:"local"`
}

// TerraformConfig represents the Terraform configuration
type TerraformConfig struct {
	Backend *string `yaml:"backend"`
}

// VMConfig represents the VM configuration
type VMConfig struct {
	Address *string `yaml:"address"`
	Arch    *string `yaml:"arch"`
	CPU     *int    `yaml:"cpu"`
	Disk    *int    `yaml:"disk"`
	Driver  *string `yaml:"driver"`
	Memory  *int    `yaml:"memory"`
}

// DNSConfig represents the DNS configuration
type DNSConfig struct {
	Create  *bool   `yaml:"create"`
	Name    *string `yaml:"name"`
	Address *string `yaml:"address"`
}

// ClusterConfig represents the cluster configuration
type ClusterConfig struct {
	Driver        *string `yaml:"driver"`
	ControlPlanes struct {
		Count  *int `yaml:"count"`
		CPU    *int `yaml:"cpu"`
		Memory *int `yaml:"memory"`
	} `yaml:"controlplanes"`
	Workers struct {
		Count  *int `yaml:"count"`
		CPU    *int `yaml:"cpu"`
		Memory *int `yaml:"memory"`
	} `yaml:"workers"`
}

// Context represents the context configuration
type Context struct {
	Environment map[string]string `yaml:"environment"`
	AWS         *AWSConfig        `yaml:"aws"`
	Docker      *DockerConfig     `yaml:"docker"`
	Git         *GitConfig        `yaml:"git"`
	Terraform   *TerraformConfig  `yaml:"terraform"`
	VM          *VMConfig         `yaml:"vm"`
	Cluster     *ClusterConfig    `yaml:"cluster"`
	DNS         *DNSConfig        `yaml:"dns"`
}

// Config represents the entire configuration
type Config struct {
	Context  *string             `yaml:"context"`
	Contexts map[string]*Context `yaml:"contexts"`
}

// DefaultConfig returns the default configuration
var DefaultConfig = Context{
	Environment: map[string]string{},
	AWS: &AWSConfig{
		AWSEndpointURL: nil,
		AWSProfile:     nil,
		S3Hostname:     nil,
		MWAAEndpoint:   nil,
		Localstack: &LocalstackConfig{
			Create:   nil,
			Services: nil,
		},
	},
	Docker: &DockerConfig{
		Enabled:     nil,
		Registries:  []Registry{},
		NetworkCIDR: nil,
	},
	Terraform: &TerraformConfig{
		Backend: nil,
	},
	Cluster: nil,
	DNS: &DNSConfig{
		Create:  nil,
		Name:    nil,
		Address: nil,
	},
}

// DefaultLocalConfig returns the default configuration for the "local" context
var DefaultLocalConfig = Context{
	Environment: map[string]string{},
	AWS: &AWSConfig{
		AWSEndpointURL: ptrString("http://aws.test:4566"),
		AWSProfile:     ptrString("default"),
		S3Hostname:     ptrString("http://s3.local.aws.test:4566"),
		MWAAEndpoint:   ptrString("http://mwaa.local.aws.test:4566"),
		Localstack: &LocalstackConfig{
			Create:   ptrBool(true),
			Services: []string{"iam", "sts", "kms", "s3", "dynamodb"},
		},
	},
	Docker: &DockerConfig{
		Enabled: ptrBool(true),
		Registries: []Registry{
			{
				Name: "registry.test",
			},
			// RMV: Temporarily disable to work around a short-term bug, to be fixed in subsequent PR
			// {
			// 	Name:   "registry-1.docker.test",
			// 	Remote: "https://registry-1.docker.io",
			// 	Local:  "https://docker.io",
			// },
			// {
			// 	Name:   "registry.k8s.test",
			// 	Remote: "https://registry.k8s.io",
			// },
			// {
			// 	Name:   "gcr.test",
			// 	Remote: "https://gcr.io",
			// },
			// {
			// 	Name:   "ghcr.test",
			// 	Remote: "https://ghcr.io",
			// },
			// {
			// 	Name:   "quay.test",
			// 	Remote: "https://quay.io",
			// },
		},
		NetworkCIDR: ptrString("10.1.0.0/16"),
	},
	Git: &GitConfig{
		Livereload: &GitLivereloadConfig{
			Create:       ptrBool(true),
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
		Driver: ptrString("talos"),
		ControlPlanes: struct {
			Count  *int `yaml:"count"`
			CPU    *int `yaml:"cpu"`
			Memory *int `yaml:"memory"`
		}{
			Count:  ptrInt(1),
			CPU:    ptrInt(2),
			Memory: ptrInt(2),
		},
		Workers: struct {
			Count  *int `yaml:"count"`
			CPU    *int `yaml:"cpu"`
			Memory *int `yaml:"memory"`
		}{
			Count:  ptrInt(1),
			CPU:    ptrInt(4),
			Memory: ptrInt(4),
		},
	},
	DNS: &DNSConfig{
		Create:  ptrBool(true),
		Name:    ptrString("test"),
		Address: nil,
	},
}
