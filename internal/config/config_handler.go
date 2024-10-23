package config

// Helper functions to create pointers for basic types
func ptrString(s string) *string {
	return &s
}

func ptrBool(b bool) *bool {
	return &b
}

func ptrInt(i int) *int {
	return &i
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

	// SetDefault sets the default context configuration
	SetDefault(context Context) error

	// GetConfig returns the context config object
	GetConfig() (*Context, error)
}
