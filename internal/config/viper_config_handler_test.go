package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

var tempDir string

func setup() {
	var err error
	tempDir, err = os.MkdirTemp("", "testdata")
	if err != nil {
		panic(err)
	}
	os.WriteFile(tempDir+"/config.yaml", []byte("testKey: testValue\n"), 0644)
	os.WriteFile(tempDir+"/invalid.yaml", []byte("invalid content"), 0644)
}

func teardown() {
	os.RemoveAll(tempDir)
}

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	teardown()
	os.Exit(code)
}

// containsErrorMessage checks if the actual error message contains the expected error message
func containsErrorMessage(actual, expected string) bool {
	return strings.Contains(actual, expected)
}

func TestViperConfigHandler_LoadConfig(t *testing.T) {
	handler := &ViperConfigHandler{}

	// Given a valid config path
	t.Run("ValidConfigPath", func(t *testing.T) {
		// When LoadConfig is called with a valid path
		err := handler.LoadConfig(tempDir + "/config.yaml")
		// Then it should not return an error
		if err != nil {
			t.Errorf("LoadConfig() error = %v, expected nil", err)
		}
	})

	// Given an invalid config path
	t.Run("InvalidConfigPath", func(t *testing.T) {
		// When LoadConfig is called with an invalid path
		err := handler.LoadConfig(tempDir + "/invalid.yaml")
		// Then it should return an error
		if err == nil {
			t.Errorf("LoadConfig() expected error, got nil")
		}
	})

	// Given a non-existent config path
	t.Run("NonExistentConfigPath", func(t *testing.T) {
		// When LoadConfig is called with a non-existent path
		err := handler.LoadConfig(tempDir + "/nonexistent.yaml")

		// Then it should return an error
		if err != nil {
			t.Errorf("LoadConfig() error = %v, expected nil", err)
		}
	})
}

func TestViperConfigHandler_LoadConfig_CreateConfigFile(t *testing.T) {
	handler := &ViperConfigHandler{}

	// Given a non-existent config file path
	t.Run("NonExistentConfigFilePath", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.yaml")

		if _, err := os.Stat(configPath); !os.IsNotExist(err) {
			t.Fatalf("Config file already exists at %s", configPath)
		}

		// When LoadConfig is called with a non-existent file path
		err := handler.LoadConfig(configPath)
		// Then it should not return an error
		if err != nil {
			t.Fatalf("LoadConfig() error = %v, expected nil", err)
		}

		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Fatalf("Config file was not created at %s", configPath)
		}
	})
}

func TestViperConfigHandler_LoadConfig_CreateConfigFileError(t *testing.T) {
	handler := &ViperConfigHandler{}

	// Given a path where parent directories cannot be created
	t.Run("InvalidParentDirectories", func(t *testing.T) {
		tempDir := t.TempDir()
		invalidConfigPath := filepath.Join(tempDir, "invalid", "config.yaml")

		originalOsMkdirAll := osMkdirAll
		defer func() { osMkdirAll = originalOsMkdirAll }()
		osMkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock error creating directories")
		}

		// When LoadConfig is called with a path where parent directories cannot be created
		err := handler.LoadConfig(invalidConfigPath)
		// Then it should return an error
		if err == nil {
			t.Fatalf("LoadConfig() expected error, got nil")
		}

		expectedError := "mock error creating directories"
		if !containsErrorMessage(err.Error(), expectedError) {
			t.Fatalf("LoadConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	// Given a path where creating the config file fails
	t.Run("ConfigFileCreationError", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.yaml")

		originalViperSafeWriteConfigAs := viperSafeWriteConfigAs
		defer func() { viperSafeWriteConfigAs = originalViperSafeWriteConfigAs }()
		viperSafeWriteConfigAs = func(filename string) error {
			return fmt.Errorf("mock error creating config file")
		}

		// When LoadConfig is called with a path where creating the config file fails
		err := handler.LoadConfig(configPath)
		// Then it should return an error
		if err == nil {
			t.Fatalf("LoadConfig() expected error, got nil")
		}

		expectedError := "mock error creating config file"
		if !containsErrorMessage(err.Error(), expectedError) {
			t.Fatalf("LoadConfig() error = %v, expected '%s'", err, expectedError)
		}
	})
}

func TestViperConfigHandler_LoadConfig_EmptyPath(t *testing.T) {
	handler := &ViperConfigHandler{}

	// Given an empty path and a valid home directory
	t.Run("EmptyPathAndValidHomeDir", func(t *testing.T) {
		originalUserHomeDir := osUserHomeDir
		defer func() { osUserHomeDir = originalUserHomeDir }()
		osUserHomeDir = func() (string, error) {
			return tempDir, nil
		}

		originalViperGetString := viperGetString
		defer func() { viperGetString = originalViperGetString }()
		viperGetString = func(key string) string {
			return ""
		}

		expectedPath := filepath.Join(tempDir, ".config", "windsor", "config.yaml")
		os.MkdirAll(filepath.Dir(expectedPath), 0755)
		os.WriteFile(expectedPath, []byte("testKey: testValue\n"), 0644)

		// When LoadConfig is called with an empty path
		err := handler.LoadConfig("")
		// Then it should not return an error and use the expected config file path
		if err != nil {
			t.Fatalf("LoadConfig() error = %v, expected nil", err)
		}

		if viper.ConfigFileUsed() != expectedPath {
			t.Errorf("LoadConfig() used config file = %v, expected %v", viper.ConfigFileUsed(), expectedPath)
		}
	})

	// Given an empty path and an error finding home directory
	t.Run("EmptyPathAndHomeDirError", func(t *testing.T) {
		originalUserHomeDir := osUserHomeDir
		defer func() { osUserHomeDir = originalUserHomeDir }()
		osUserHomeDir = func() (string, error) {
			return "", fmt.Errorf("mock error")
		}

		originalViperGetString := viperGetString
		defer func() { viperGetString = originalViperGetString }()
		viperGetString = func(key string) string {
			return ""
		}

		// When LoadConfig is called with an empty path and an error finding home directory
		err := handler.LoadConfig("")
		// Then it should return an error
		if err == nil || err.Error() != "error finding home directory, mock error" {
			t.Fatalf("LoadConfig() error = %v, expected 'error finding home directory, mock error'", err)
		}
	})
}

func TestViperConfigHandler_GetConfigValue(t *testing.T) {
	handler := &ViperConfigHandler{}
	viper.Set("testKey", "testValue")

	// Given an existing key
	t.Run("ExistingKey", func(t *testing.T) {
		// When GetConfigValue is called with an existing key
		got, err := handler.GetConfigValue("testKey")
		// Then it should return the value and no error
		if err != nil {
			t.Errorf("GetConfigValue() error = %v, expected nil", err)
		}
		if got != "testValue" {
			t.Errorf("GetConfigValue() = %v, want %v", got, "testValue")
		}
	})

	// Given a non-existent key
	t.Run("NonExistentKey", func(t *testing.T) {
		// When GetConfigValue is called with a non-existent key
		_, err := handler.GetConfigValue("nonExistentKey")
		// Then it should return an error
		if err == nil {
			t.Errorf("GetConfigValue() expected error, got nil")
		}
	})
}

func TestViperConfigHandler_SetConfigValue(t *testing.T) {
	handler := &ViperConfigHandler{}

	// Given a new key-value pair
	t.Run("NewKeyValuePair", func(t *testing.T) {
		// When SetConfigValue is called with a new key-value pair
		err := handler.SetConfigValue("newKey", "newValue")
		// Then it should not return an error and the value should be set
		if err != nil {
			t.Errorf("SetConfigValue() error = %v", err)
		}
		if got := viper.GetString("newKey"); got != "newValue" {
			t.Errorf("SetConfigValue() = %v, want %v", got, "newValue")
		}
	})

	// Given an existing key with a new value
	t.Run("ExistingKeyWithNewValue", func(t *testing.T) {
		viper.Set("testKey", "testValue")
		// When SetConfigValue is called with an existing key and a new value
		err := handler.SetConfigValue("testKey", "newTestValue")
		// Then it should not return an error and the value should be updated
		if err != nil {
			t.Errorf("SetConfigValue() error = %v", err)
		}
		if got := viper.GetString("testKey"); got != "newTestValue" {
			t.Errorf("SetConfigValue() = %v, want %v", got, "newTestValue")
		}
	})
}

func TestViperConfigHandler_SaveConfig(t *testing.T) {
	handler := &ViperConfigHandler{}
	viper.Set("saveKey", "saveValue")

	// Given a valid config path
	t.Run("ValidConfigPath", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "save_config.yaml")

		// Ensure the parent directory does not exist before the test
		parentDir := filepath.Dir(configPath)
		os.RemoveAll(parentDir)

		if _, err := os.Stat(parentDir); !os.IsNotExist(err) {
			t.Fatalf("Parent directories already exist at %s", parentDir)
		}

		// When SaveConfig is called with a valid path
		err := handler.SaveConfig(configPath)
		// Then it should not return an error and the config file should be created
		if err != nil {
			t.Fatalf("SaveConfig() error = %v, expected nil", err)
		}

		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Fatalf("Config file was not created at %s", configPath)
		}
	})

	// Given an invalid path where parent directories cannot be created
	t.Run("InvalidParentDirectories", func(t *testing.T) {
		tempDir := t.TempDir()
		invalidConfigPath := filepath.Join(tempDir, "invalid", "save_config.yaml")

		originalOsMkdirAll := osMkdirAll
		defer func() { osMkdirAll = originalOsMkdirAll }()
		osMkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock error creating directories")
		}

		// When SaveConfig is called with an invalid path
		err := handler.SaveConfig(invalidConfigPath)
		// Then it should return an error
		if err == nil {
			t.Fatalf("SaveConfig() expected error, got nil")
		}

		expectedError := "mock error creating directories"
		if !containsErrorMessage(err.Error(), expectedError) {
			t.Fatalf("SaveConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	// Given a valid path but an error writing the config file
	t.Run("WriteError", func(t *testing.T) {
		tempDir := t.TempDir()
		validConfigPath := filepath.Join(tempDir, "save_config.yaml")

		originalViperWriteConfigAs := viperWriteConfigAs
		defer func() { viperWriteConfigAs = originalViperWriteConfigAs }()
		viperWriteConfigAs = func(filename string) error {
			return fmt.Errorf("mock error writing config")
		}

		// When SaveConfig is called with a valid path but an error writing the config file
		err := handler.SaveConfig(validConfigPath)
		// Then it should return an error
		if err == nil {
			t.Fatalf("SaveConfig() expected error, got nil")
		}

		expectedError := "mock error writing config"
		if !containsErrorMessage(err.Error(), expectedError) {
			t.Fatalf("SaveConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	// Given an empty path with a valid config file used
	t.Run("EmptyPathAndValidConfigFile", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.yaml")

		originalViperConfigFileUsed := viperConfigFileUsed
		defer func() { viperConfigFileUsed = originalViperConfigFileUsed }()
		viperConfigFileUsed = func() string {
			return configPath
		}

		if _, err := os.Stat(configPath); !os.IsNotExist(err) {
			t.Fatalf("Config file already exists at %s", configPath)
		}

		// When SaveConfig is called with an empty path
		err := handler.SaveConfig("")
		// Then it should not return an error and the config file should be created
		if err != nil {
			t.Fatalf("SaveConfig() error = %v, expected nil", err)
		}

		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Fatalf("Config file was not created at %s", configPath)
		}
	})

	// Given an empty path with an invalid config file used
	t.Run("EmptyPathAndInvalidConfigFile", func(t *testing.T) {
		originalViperConfigFileUsed := viperConfigFileUsed
		defer func() { viperConfigFileUsed = originalViperConfigFileUsed }()
		viperConfigFileUsed = func() string {
			return ""
		}

		// When SaveConfig is called with an empty path and an invalid config file used
		err := handler.SaveConfig("")
		// Then it should return an error
		if err == nil {
			t.Fatalf("SaveConfig() expected error, got nil")
		}

		expectedError := "path cannot be empty"
		if err.Error() != expectedError {
			t.Fatalf("SaveConfig() error = %v, expected '%s'", err, expectedError)
		}
	})
}

func TestViperConfigHandler_GetNestedMap(t *testing.T) {
	handler := &ViperConfigHandler{}
	viper.Set("contexts.blah.env", map[string]interface{}{
		"some_env":       "value1",
		"some_other_env": "value2",
	})

	// Given an existing nested map key
	t.Run("ExistingNestedMapKey", func(t *testing.T) {
		// When GetNestedMap is called with an existing nested map key
		got, err := handler.GetNestedMap("contexts.blah.env")
		// Then it should return the nested map and no error
		if err != nil {
			t.Errorf("GetNestedMap() error = %v, expected nil", err)
		}
		want := map[string]interface{}{
			"some_env":       "value1",
			"some_other_env": "value2",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("GetNestedMap() = %v, want %v", got, want)
		}
	})

	// Given a non-existent nested map key
	t.Run("NonExistentNestedMapKey", func(t *testing.T) {
		// When GetNestedMap is called with a non-existent nested map key
		_, err := handler.GetNestedMap("contexts.nonexistent.env")
		// Then it should return an error
		if err == nil {
			t.Errorf("GetNestedMap() expected error, got nil")
		}
	})
}

func TestViperConfigHandler_ListKeys(t *testing.T) {
	handler := &ViperConfigHandler{}
	viper.Set("contexts.blah.env", map[string]interface{}{
		"some_env":       "value1",
		"some_other_env": "value2",
	})

	// Given an existing key
	t.Run("ExistingKey", func(t *testing.T) {
		// When ListKeys is called with an existing key
		got, err := handler.ListKeys("contexts.blah.env")
		// Then it should return the list of keys and no error
		if err != nil {
			t.Errorf("ListKeys() error = %v, expected nil", err)
		}
		want := []string{"some_env", "some_other_env"}
		sort.Strings(got)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ListKeys() = %v, want %v", got, want)
		}
	})

	// Given a non-existent key
	t.Run("NonExistentKey", func(t *testing.T) {
		// When ListKeys is called with a non-existent key
		_, err := handler.ListKeys("contexts.nonexistent.env")
		// Then it should return an error
		if err == nil {
			t.Errorf("ListKeys() expected error, got nil")
		}
	})
}

func TestNewViperConfigHandler(t *testing.T) {
	// Given a call to NewViperConfigHandler
	t.Run("NewViperConfigHandler", func(t *testing.T) {
		// When NewViperConfigHandler is called
		handler := NewViperConfigHandler()
		// Then it should return a non-nil handler
		if handler == nil {
			t.Errorf("expected NewViperConfigHandler to return a non-nil instance")
		}
	})
}
