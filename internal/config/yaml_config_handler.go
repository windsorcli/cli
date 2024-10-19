package config

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

// YamlConfigHandler implements the ConfigHandler interface using goccy/go-yaml
type YamlConfigHandler struct {
	config map[string]interface{}
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

// LoadConfig loads the configuration from the specified path
func (y *YamlConfigHandler) LoadConfig(path string) error {
	if _, err := osStat(path); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist at path: %s", path)
	}

	data, err := osReadFile(path)
	if err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}
	if err := yaml.Unmarshal(data, &y.config); err != nil {
		return fmt.Errorf("error unmarshalling yaml: %w", err)
	}
	return nil
}

// GetConfigValue retrieves the value for the specified key from the configuration
func (y *YamlConfigHandler) GetConfigValue(key string, defaultValue ...string) (string, error) {
	if value, exists := y.config[key]; exists {
		return fmt.Sprintf("%v", value), nil
	}
	if len(defaultValue) > 0 {
		return defaultValue[0], nil
	}
	return "", fmt.Errorf("key %s not found in configuration", key)
}

// SetConfigValue sets the value for the specified key in the configuration
func (y *YamlConfigHandler) SetConfigValue(key string, value interface{}) error {
	y.config[key] = value
	return nil
}

// SaveConfig saves the current configuration to the specified path
func (y *YamlConfigHandler) SaveConfig(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
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

// GetNestedMap retrieves a nested map for the specified key from the configuration
func (y *YamlConfigHandler) GetNestedMap(key string) (map[string]interface{}, error) {
	if value, exists := y.config[key]; exists {
		if nestedMap, ok := value.(map[string]interface{}); ok {
			return nestedMap, nil
		}
		return nil, fmt.Errorf("key %s is not a nested map", key)
	}
	return nil, fmt.Errorf("key %s not found in configuration", key)
}

// ListKeys lists all keys at the specified key level in the configuration
func (y *YamlConfigHandler) ListKeys(key string) ([]string, error) {
	if value, exists := y.config[key]; exists {
		if nestedMap, ok := value.(map[string]interface{}); ok {
			keys := make([]string, 0, len(nestedMap))
			for k := range nestedMap {
				keys = append(keys, k)
			}
			return keys, nil
		}
		return nil, fmt.Errorf("key %s is not a nested map", key)
	}
	return nil, fmt.Errorf("key %s not found in configuration", key)
}

// Ensure YamlConfigHandler implements ConfigHandler
var _ ConfigHandler = (*YamlConfigHandler)(nil)
