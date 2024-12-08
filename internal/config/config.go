package config

// AWSConfig represents the AWS configuration
type AWSConfig struct {
	Enabled        *bool             `yaml:"enabled"`
	AWSEndpointURL *string           `yaml:"aws_endpoint_url"`
	AWSProfile     *string           `yaml:"aws_profile"`
	S3Hostname     *string           `yaml:"s3_hostname"`
	MWAAEndpoint   *string           `yaml:"mwaa_endpoint"`
	Localstack     *LocalstackConfig `yaml:"localstack"`
}

// LocalstackConfig represents the Localstack configuration
type LocalstackConfig struct {
	Enabled  *bool    `yaml:"enabled"`
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
	Enabled      *bool   `yaml:"enabled"`
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
	Enabled *bool   `yaml:"enabled"`
	Name    *string `yaml:"name"`
	Address *string `yaml:"address"`
}

// ClusterConfig represents the cluster configuration
type ClusterConfig struct {
	Enabled       *bool   `yaml:"enabled"`
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
