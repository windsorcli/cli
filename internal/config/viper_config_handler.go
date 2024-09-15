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

	// Check if the config file exists, if not create it
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := viper.SafeWriteConfigAs(path); err != nil {
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
	viper.SetConfigFile(path)
	return viper.WriteConfig()
}

// Ensure ViperConfigHandler implements ConfigHandler
var _ ConfigHandler = (*ViperConfigHandler)(nil)
