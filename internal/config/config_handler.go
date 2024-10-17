package config

// ConfigHandler defines the interface for handling configuration operations
type ConfigHandler interface {
	// LoadConfig loads the configuration from the specified path
	LoadConfig(path string) error

	// GetConfigValue retrieves the value for the specified key from the configuration
	GetConfigValue(key string, defaultValue ...string) (string, error)

	// SetConfigValue sets the value for the specified key in the configuration
	SetConfigValue(key string, value interface{}) error

	// SaveConfig saves the current configuration to the specified path
	SaveConfig(path string) error

	// GetNestedMap retrieves a nested map for the specified key from the configuration
	GetNestedMap(key string) (map[string]interface{}, error)

	// ListKeys lists all keys for the specified key from the configuration
	ListKeys(key string) ([]string, error)
}
