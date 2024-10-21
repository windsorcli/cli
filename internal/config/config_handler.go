package config

import "github.com/windsor-hotel/cli/internal/constants"

// AWSConfig represents the AWS configuration
type AWSConfig struct {
	AWSEndpointURL *string `yaml:"aws_endpoint_url"`
	AWSProfile     *string `yaml:"aws_profile"`
}

// DockerConfig represents the Docker configuration
type DockerConfig struct {
	Enabled    *bool      `yaml:"enabled"`
	Registries []Registry `yaml:"registries"`
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
	Arch   *string `yaml:"arch"`
	CPU    *int    `yaml:"cpu"`
	Disk   *int    `yaml:"disk"`
	Driver *string `yaml:"driver"`
	Memory *int    `yaml:"memory"`
}

// Context represents the context configuration
type Context struct {
	Environment map[string]string `yaml:"environment"`
	AWS         *AWSConfig        `yaml:"aws"`
	Docker      *DockerConfig     `yaml:"docker"`
	Git         *GitConfig        `yaml:"git"`
	Terraform   *TerraformConfig  `yaml:"terraform"`
	VM          *VMConfig         `yaml:"vm"`
}

// Config represents the entire configuration
type Config struct {
	Context  *string             `yaml:"context"`
	Contexts map[string]*Context `yaml:"contexts"`
}

// DefaultConfig returns the default configuration for the "local" context
var DefaultConfig = Context{
	Environment: map[string]string{},
	AWS: &AWSConfig{
		AWSEndpointURL: nil,
		AWSProfile:     nil,
	},
	Docker: &DockerConfig{
		Enabled:    nil,
		Registries: []Registry{},
	},
	Terraform: &TerraformConfig{
		Backend: nil,
	},
}

// DefaultLocalConfig returns the default configuration for the "local" context
var DefaultLocalConfig = Context{
	Environment: map[string]string{},
	AWS: &AWSConfig{
		AWSEndpointURL: ptrString("http://aws.test:4566"),
		AWSProfile:     ptrString("default"),
	},
	Docker: &DockerConfig{
		Enabled: ptrBool(true),
		Registries: []Registry{
			{
				Name: "registry.test",
			},
			{
				Name:   "registry-1.docker.test",
				Remote: "https://registry-1.docker.io",
				Local:  "https://docker.io",
			},
			{
				Name:   "registry.k8s.test",
				Remote: "https://registry.k8s.io",
			},
			{
				Name:   "gcr.test",
				Remote: "https://gcr.io",
			},
			{
				Name:   "ghcr.test",
				Remote: "https://ghcr.io",
			},
			{
				Name:   "quay.test",
				Remote: "https://quay.io",
			},
		},
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
}

// Helper functions to create pointers for basic types
func ptrString(s string) *string {
	return &s
}

func ptrBool(b bool) *bool {
	return &b
}

// Disabled until used
// func ptrInt(i int) *int {
// 	return &i
// }

// ConfigHandler defines the interface for handling configuration operations
type ConfigHandler interface {
	// LoadConfig loads the configuration from the specified path
	LoadConfig(path string) error

	// GetString retrieves a string value for the specified key from the configuration
	GetString(key string, defaultValue ...string) (string, error)

	// GetInt retrieves an integer value for the specified key from the configuration
	GetInt(key string, defaultValue ...int) (int, error)

	// GetBool retrieves a boolean value for the specified key from the configuration
	GetBool(key string, defaultValue ...bool) (bool, error)

	// Set sets the value for the specified key in the configuration
	Set(key string, value interface{}) error

	// Get retrieves a value for the specified key from the configuration
	Get(key string) (interface{}, error)

	// SaveConfig saves the current configuration to the specified path
	SaveConfig(path string) error

	// SetDefault sets the default context configuration
	SetDefault(context Context) error
}
