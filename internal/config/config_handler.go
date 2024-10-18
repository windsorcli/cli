package config

// AWSConfig represents the AWS configuration
type AWSConfig struct {
	AWSEndpointURL string `yaml:"aws_endpoint_url"`
	AWSProfile     string `yaml:"aws_profile"`
}

// DockerConfig represents the Docker configuration
type DockerConfig struct {
	Enabled bool `yaml:"enabled"`
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

// LocalContext represents the local context configuration
type LocalContext struct {
	AWS       AWSConfig       `yaml:"aws"`
	Docker    DockerConfig    `yaml:"docker"`
	Terraform TerraformConfig `yaml:"terraform"`
	VM        VMConfig        `yaml:"vm"`
}

// Config represents the entire configuration
type Config struct {
	Contexts map[string]LocalContext `yaml:"contexts"`
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

	// SetValue sets the value for the specified key in the configuration
	SetValue(key string, value interface{}) error

	// SaveConfig saves the current configuration to the specified path
	SaveConfig(path string) error

	// GetNestedMap retrieves a nested map for the specified key from the configuration
	GetNestedMap(key string) (map[string]interface{}, error)
}
