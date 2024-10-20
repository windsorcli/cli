package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
)

// YamlConfigHandler implements the ConfigHandler interface using goccy/go-yaml
type YamlConfigHandler struct {
	config               map[string]interface{}
	path                 string
	defaultContextConfig Context
}

// NewYamlConfigHandler is a constructor for YamlConfigHandler that accepts a path
func NewYamlConfigHandler(path string) (*YamlConfigHandler, error) {
	handler := &YamlConfigHandler{
		config: make(map[string]interface{}),
	}
	if path != "" {
		if err := handler.LoadConfig(path); err != nil {
			return nil, fmt.Errorf("error loading config: %w", err)
		}
	}
	return handler, nil
}

// osReadFile is a variable to allow mocking os.ReadFile in tests
var osReadFile = os.ReadFile

// osWriteFile is a variable to allow mocking os.WriteFile in tests
var osWriteFile = os.WriteFile

// Override variable for yamlMarshal
var yamlMarshal = yaml.Marshal

// Override variable for yamlUnmarshal
var yamlUnmarshal = yaml.Unmarshal

// osUserHomeDir is a variable to allow mocking os.UserHomeDir in tests
var osUserHomeDir = os.UserHomeDir

// osMkdirAll is a variable to allow mocking os.MkdirAll in tests
var osMkdirAll = os.MkdirAll

// osStat is a variable to allow mocking os.Stat in tests
var osStat = os.Stat

// LoadConfig loads the configuration from the specified path
func (y *YamlConfigHandler) LoadConfig(path string) error {
	if _, err := osStat(path); os.IsNotExist(err) {
		// Create an empty config file if it does not exist
		if err := osWriteFile(path, []byte{}, 0644); err != nil {
			return fmt.Errorf("error creating config file: %w", err)
		}
	}

	data, err := osReadFile(path)
	if err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}
	if err := yamlUnmarshal(data, &y.config); err != nil {
		return fmt.Errorf("error unmarshalling yaml: %w", err)
	}
	y.path = path
	return nil
}

// SaveConfig saves the current configuration to the specified path
func (y *YamlConfigHandler) SaveConfig(path string) error {
	if path == "" {
		if y.path == "" {
			return fmt.Errorf("path cannot be empty")
		}
		path = y.path
	}
	data, err := yamlMarshal(y.config)
	if err != nil {
		return fmt.Errorf("error marshalling yaml: %w", err)
	}
	if err := osWriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}
	return nil
}

// GetString retrieves a string value for the specified key from the configuration
func (y *YamlConfigHandler) GetString(key string, defaultValue ...string) (string, error) {
	if value, exists := y.config[key]; exists {
		return fmt.Sprintf("%v", value), nil
	}
	if len(defaultValue) > 0 {
		return defaultValue[0], nil
	}
	return "", fmt.Errorf("key %s not found in configuration", key)
}

// GetInt retrieves an integer value for the specified key from the configuration
func (y *YamlConfigHandler) GetInt(key string, defaultValue ...int) (int, error) {
	if value, exists := y.config[key]; exists {
		if intValue, ok := value.(int); ok {
			return intValue, nil
		}
		return 0, fmt.Errorf("key %s is not an integer", key)
	}
	if len(defaultValue) > 0 {
		return defaultValue[0], nil
	}
	return 0, fmt.Errorf("key %s not found in configuration", key)
}

// GetBool retrieves a boolean value for the specified key from the configuration
func (y *YamlConfigHandler) GetBool(key string, defaultValue ...bool) (bool, error) {
	if value, exists := y.config[key]; exists {
		if boolValue, ok := value.(bool); ok {
			return boolValue, nil
		}
		return false, fmt.Errorf("key %s is not a boolean", key)
	}
	if len(defaultValue) > 0 {
		return defaultValue[0], nil
	}
	return false, fmt.Errorf("key %s not found in configuration", key)
}

// Set sets the value for the specified key in the configuration
func (y *YamlConfigHandler) Set(key string, value interface{}) error {
	keys := strings.Split(key, ".")
	lastKey := keys[len(keys)-1]
	m := y.config

	for _, k := range keys[:len(keys)-1] {
		if _, exists := m[k]; !exists {
			m[k] = make(map[string]interface{})
		}
		m = m[k].(map[string]interface{})
	}
	m[lastKey] = value
	return nil
}

// Get retrieves a value for the specified key from the configuration
func (y *YamlConfigHandler) Get(key string) (interface{}, error) {
	if value, exists := y.config[key]; exists {
		return value, nil
	}
	return nil, fmt.Errorf("key %s not found in configuration", key)
}

// SetDefault sets the default context configuration
func (y *YamlConfigHandler) SetDefault(context Context) {
	y.defaultContextConfig = context
}

// Ensure YamlConfigHandler implements ConfigHandler
var _ ConfigHandler = (*YamlConfigHandler)(nil)
