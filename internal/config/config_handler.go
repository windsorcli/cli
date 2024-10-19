package config

// AWSConfig represents the AWS configuration
type AWSConfig struct {
	AWSEndpointURL string `yaml:"aws_endpoint_url"`
	AWSProfile     string `yaml:"aws_profile"`
}

// DockerConfig represents the Docker configuration
type DockerConfig struct {
	Enabled    bool       `yaml:"enabled"`
	Registries []Registry `yaml:"registries"`
}

type Registry struct {
	Name   string `yaml:"name"`
	Remote string `yaml:"remote"`
	Local  string `yaml:"local"`
}

// TerraformConfig represents the Terraform configuration
type TerraformConfig struct {
	Backend string `yaml:"backend"`
}

// VMConfig represents the VM configuration
type VMConfig struct {
	Arch   string `yaml:"arch"`
	CPU    int    `yaml:"cpu"`
	Disk   int    `yaml:"disk"`
	Driver string `yaml:"driver"`
	Memory int    `yaml:"memory"`
}

// Context represents the context configuration
type Context struct {
	AWS       AWSConfig       `yaml:"aws"`
	Docker    DockerConfig    `yaml:"docker"`
	Terraform TerraformConfig `yaml:"terraform"`
	VM        VMConfig        `yaml:"vm"`
}

// Config represents the entire configuration
type Config struct {
	Context  string             `yaml:"context"`
	Contexts map[string]Context `yaml:"contexts"`
}

// DefaultLocalConfig returns the default configuration for the "local" context
var DefaultLocalConfig = Context{
	AWS: AWSConfig{
		AWSEndpointURL: "http://aws.test:4566",
		AWSProfile:     "default",
	},
	Docker: DockerConfig{
		Enabled: true,
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
	Terraform: TerraformConfig{
		Backend: "local",
	},
}

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

	// GetNestedMap retrieves a nested map for the specified key from the configuration
	GetNestedMap(key string) (map[string]interface{}, error)

	// ListKeys lists all keys for the specified key from the configuration
	ListKeys(key string) ([]string, error)

	// SetDefault sets the default configuration for the specified key
	SetDefault(key string, value interface{})
}
