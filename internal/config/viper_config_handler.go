package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// ViperConfigHandler implements the ConfigHandler interface using Viper
type ViperConfigHandler struct{}

// osUserHomeDir is a variable to allow mocking os.UserHomeDir in tests
var osUserHomeDir = os.UserHomeDir

// viperGetString is a variable to allow mocking viper.GetString in tests
var viperGetString = viper.GetString

// osMkdirAll is a variable to allow mocking os.MkdirAll in tests
var osMkdirAll = os.MkdirAll

// viperWriteConfigAs is a variable to allow mocking viper.WriteConfigAs in tests
var viperWriteConfigAs = viper.WriteConfigAs

// viperSafeWriteConfigAs is a variable to allow mocking viper.SafeWriteConfigAs in tests
var viperSafeWriteConfigAs = viper.SafeWriteConfigAs

// viperConfigFileUsed is a variable to allow mocking viper.ConfigFileUsed in tests
var viperConfigFileUsed = viper.ConfigFileUsed

// createParentDirs ensures that all parent directories for the given path exist
func createParentDirs(path string) error {
	dir := filepath.Dir(path)
	if err := osMkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("error creating directories for path %s, %s", path, err)
	}
	return nil
}

// LoadConfig loads the configuration from the specified path
func (v *ViperConfigHandler) LoadConfig(path string) error {
	if path == "" {
		path = viperGetString("WINDSORCONFIG")
		if path == "" {
			home, err := osUserHomeDir()
			if err != nil {
				return fmt.Errorf("error finding home directory, %s", err)
			}
			path = filepath.Join(home, ".config", "windsor", "config.yaml")
		}
	}

	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")

	// Provide some default configuration values
	viper.SetDefault("context", "default")

	// Ensure parent directories exist
	if err := createParentDirs(path); err != nil {
		return err
	}

	// Check if the config file exists, if not create it
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := viperSafeWriteConfigAs(path); err != nil {
			return fmt.Errorf("error creating config file, %s", err)
		}
	}

	return viper.ReadInConfig()
}

// GetConfigValue retrieves the value for the specified key from the configuration
func (v *ViperConfigHandler) GetConfigValue(key string) (string, error) {
	if !viper.IsSet(key) {
		return "", fmt.Errorf("key %s not found in configuration", key)
	}
	return viper.GetString(key), nil
}

// SetConfigValue sets the value for the specified key in the configuration
func (v *ViperConfigHandler) SetConfigValue(key string, value string) error {
	viper.Set(key, value)
	return nil
}

// SaveConfig saves the current configuration to the specified path
func (v *ViperConfigHandler) SaveConfig(path string) error {
	if path == "" {
		path = viperConfigFileUsed()
		if path == "" {
			return fmt.Errorf("path cannot be empty")
		}
	}

	if err := createParentDirs(path); err != nil {
		fmt.Printf("Error creating parent directories: %v\n", err)
		return err
	}

	if err := viperWriteConfigAs(path); err != nil {
		fmt.Printf("Error writing config: %v\n", err)
		return fmt.Errorf("error writing config to path %s, %w", path, err)
	}

	return nil
}

// GetNestedMap retrieves a nested map for the specified key from the configuration
func (v *ViperConfigHandler) GetNestedMap(key string) (map[string]interface{}, error) {
	if !viper.IsSet(key) {
		return nil, fmt.Errorf("key %s not found in configuration", key)
	}
	return viper.GetStringMap(key), nil
}

// ListKeys lists all keys at the specified key level in the configuration
func (v *ViperConfigHandler) ListKeys(key string) ([]string, error) {
	if !viper.IsSet(key) {
		return nil, fmt.Errorf("key %s not found in configuration", key)
	}
	nestedMap := viper.GetStringMap(key)
	keys := make([]string, 0, len(nestedMap))
	for k := range nestedMap {
		keys = append(keys, k)
	}
	return keys, nil
}

// Ensure ViperConfigHandler implements ConfigHandler
var _ ConfigHandler = (*ViperConfigHandler)(nil)
