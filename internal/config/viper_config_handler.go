package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// ViperConfigHandler implements the ConfigHandler interface using Viper
type ViperConfigHandler struct{}

// NewViperConfigHandler is a constructor for ViperConfigHandler that accepts a path
func NewViperConfigHandler(path string) (*ViperConfigHandler, error) {
	handler := &ViperConfigHandler{}
	if path != "" {
		if err := handler.LoadConfig(path); err != nil {
			return nil, fmt.Errorf("error loading config: %w", err)
		}
	}
	return handler, nil
}

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

// viperReadInConfig is a variable to allow mocking viper.ReadInConfig in tests
var viperReadInConfig = viper.ReadInConfig

// osStat is a variable to allow mocking os.Stat in tests
var osStat = os.Stat

// createParentDirs ensures that all parent directories for the given path exist
func createParentDirs(path string) error {
	dir := filepath.Dir(path)
	if err := osMkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("error creating directories for path %s, %s", path, err)
	}
	return nil
}

// loadConfig loads the configuration from the specified path
func (v *ViperConfigHandler) LoadConfig(input string) error {
	var path string

	// Check if the input is an environment variable name or a path
	if envPath := os.Getenv(input); envPath != "" {
		path = envPath
	} else {
		path = input
	}

	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")

	// Ensure parent directories exist
	if err := createParentDirs(path); err != nil {
		return err
	}

	// Check if the config file exists, if not create it
	if _, err := osStat(path); os.IsNotExist(err) {
		if err := viperSafeWriteConfigAs(path); err != nil {
			return fmt.Errorf("error creating config file, %s", err)
		}
	} else if err != nil {
		return err
	}

	if err := viperReadInConfig(); err != nil {
		return err
	}

	return nil
}

// GetConfigValue retrieves the value for the specified key from the configuration
func (v *ViperConfigHandler) GetConfigValue(key string, defaultValue ...string) (string, error) {
	if !viper.IsSet(key) {
		if len(defaultValue) > 0 {
			return defaultValue[0], nil
		}
		return "", fmt.Errorf("key %s not found in configuration", key)
	}
	return viper.GetString(key), nil
}

// SetConfigValue sets the value for the specified key in the configuration
func (v *ViperConfigHandler) SetConfigValue(key string, value interface{}) error {
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

// Ensure ViperConfigHandler implements ConfigHandler
var _ ConfigHandler = (*ViperConfigHandler)(nil)
