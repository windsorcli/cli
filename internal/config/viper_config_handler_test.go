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

	t.Run("ValidConfigPath", func(t *testing.T) {
		err := handler.LoadConfig(tempDir + "/config.yaml")
		if err != nil {
			t.Errorf("LoadConfig() error = %v, expected nil", err)
		}
	})

	t.Run("InvalidConfigPath", func(t *testing.T) {
		err := handler.LoadConfig(tempDir + "/invalid.yaml")
		if err == nil {
			t.Errorf("LoadConfig() expected error, got nil")
		}
	})

	t.Run("NonExistentConfigPath", func(t *testing.T) {
		err := handler.LoadConfig(tempDir + "/nonexistent.yaml")
		if err != nil {
			t.Errorf("LoadConfig() error = %v, expected nil", err)
		}
	})

	t.Run("NonExistentConfigFilePath", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.yaml")

		if _, err := os.Stat(configPath); !os.IsNotExist(err) {
			t.Fatalf("Config file already exists at %s", configPath)
		}

		err := handler.LoadConfig(configPath)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v, expected nil", err)
		}

		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Fatalf("Config file was not created at %s", configPath)
		}
	})

	t.Run("InvalidParentDirectories", func(t *testing.T) {
		tempDir := t.TempDir()
		invalidConfigPath := filepath.Join(tempDir, "invalid", "config.yaml")

		originalOsMkdirAll := osMkdirAll
		defer func() { osMkdirAll = originalOsMkdirAll }()
		osMkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock error creating directories")
		}

		err := handler.LoadConfig(invalidConfigPath)
		if err == nil {
			t.Fatalf("LoadConfig() expected error, got nil")
		}

		expectedError := "mock error creating directories"
		if !containsErrorMessage(err.Error(), expectedError) {
			t.Fatalf("LoadConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("ConfigFileCreationError", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.yaml")

		originalViperSafeWriteConfigAs := viperSafeWriteConfigAs
		defer func() { viperSafeWriteConfigAs = originalViperSafeWriteConfigAs }()
		viperSafeWriteConfigAs = func(filename string) error {
			return fmt.Errorf("mock error creating config file")
		}

		err := handler.LoadConfig(configPath)
		if err == nil {
			t.Fatalf("LoadConfig() expected error, got nil")
		}

		expectedError := "mock error creating config file"
		if !containsErrorMessage(err.Error(), expectedError) {
			t.Fatalf("LoadConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("EnvironmentVariablePath", func(t *testing.T) {
		envVar := "CONFIG_PATH"
		expectedPath := tempDir + "/config.yaml"
		os.Setenv(envVar, expectedPath)
		defer os.Unsetenv(envVar)

		err := handler.LoadConfig(envVar)
		if err != nil {
			t.Errorf("LoadConfig() error = %v, expected nil", err)
		}
	})
}

func TestViperConfigHandler_GetConfigValue(t *testing.T) {
	handler := &ViperConfigHandler{}
	viper.Set("testKey", "testValue")

	t.Run("ExistingKey", func(t *testing.T) {
		got, err := handler.GetConfigValue("testKey")
		if err != nil {
			t.Errorf("GetConfigValue() error = %v, expected nil", err)
		}
		if got != "testValue" {
			t.Errorf("GetConfigValue() = %v, want %v", got, "testValue")
		}
	})

	t.Run("NonExistentKey", func(t *testing.T) {
		_, err := handler.GetConfigValue("nonExistentKey")
		if err == nil {
			t.Errorf("GetConfigValue() expected error, got nil")
		}
	})
}

func TestViperConfigHandler_SetConfigValue(t *testing.T) {
	handler := &ViperConfigHandler{}

	t.Run("NewKeyValuePair", func(t *testing.T) {
		err := handler.SetConfigValue("newKey", "newValue")
		if err != nil {
			t.Errorf("SetConfigValue() error = %v", err)
		}
		if got := viper.GetString("newKey"); got != "newValue" {
			t.Errorf("SetConfigValue() = %v, want %v", got, "newValue")
		}
	})

	t.Run("ExistingKeyWithNewValue", func(t *testing.T) {
		viper.Set("testKey", "testValue")
		err := handler.SetConfigValue("testKey", "newTestValue")
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

	t.Run("ValidConfigPath", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "save_config.yaml")

		parentDir := filepath.Dir(configPath)
		os.RemoveAll(parentDir)

		if _, err := os.Stat(parentDir); !os.IsNotExist(err) {
			t.Fatalf("Parent directories already exist at %s", parentDir)
		}

		err := handler.SaveConfig(configPath)
		if err != nil {
			t.Fatalf("SaveConfig() error = %v, expected nil", err)
		}

		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Fatalf("Config file was not created at %s", configPath)
		}
	})

	t.Run("InvalidParentDirectories", func(t *testing.T) {
		tempDir := t.TempDir()
		invalidConfigPath := filepath.Join(tempDir, "invalid", "save_config.yaml")

		originalOsMkdirAll := osMkdirAll
		defer func() { osMkdirAll = originalOsMkdirAll }()
		osMkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock error creating directories")
		}

		err := handler.SaveConfig(invalidConfigPath)
		if err == nil {
			t.Fatalf("SaveConfig() expected error, got nil")
		}

		expectedError := "mock error creating directories"
		if !containsErrorMessage(err.Error(), expectedError) {
			t.Fatalf("SaveConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("WriteError", func(t *testing.T) {
		tempDir := t.TempDir()
		validConfigPath := filepath.Join(tempDir, "save_config.yaml")

		originalViperWriteConfigAs := viperWriteConfigAs
		defer func() { viperWriteConfigAs = originalViperWriteConfigAs }()
		viperWriteConfigAs = func(filename string) error {
			return fmt.Errorf("mock error writing config")
		}

		err := handler.SaveConfig(validConfigPath)
		if err == nil {
			t.Fatalf("SaveConfig() expected error, got nil")
		}

		expectedError := "mock error writing config"
		if !containsErrorMessage(err.Error(), expectedError) {
			t.Fatalf("SaveConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

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

		err := handler.SaveConfig("")
		if err != nil {
			t.Fatalf("SaveConfig() error = %v, expected nil", err)
		}

		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Fatalf("Config file was not created at %s", configPath)
		}
	})

	t.Run("EmptyPathAndInvalidConfigFile", func(t *testing.T) {
		originalViperConfigFileUsed := viperConfigFileUsed
		defer func() { viperConfigFileUsed = originalViperConfigFileUsed }()
		viperConfigFileUsed = func() string {
			return ""
		}

		err := handler.SaveConfig("")
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

	t.Run("ExistingNestedMapKey", func(t *testing.T) {
		got, err := handler.GetNestedMap("contexts.blah.env")
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

	t.Run("NonExistentNestedMapKey", func(t *testing.T) {
		_, err := handler.GetNestedMap("contexts.nonexistent.env")
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

	t.Run("ExistingKey", func(t *testing.T) {
		got, err := handler.ListKeys("contexts.blah.env")
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

	t.Run("NonExistentKey", func(t *testing.T) {
		_, err := handler.ListKeys("contexts.nonexistent.env")
		if err == nil {
			t.Errorf("ListKeys() expected error, got nil")
		}
	})
}

func TestNewViperConfigHandler(t *testing.T) {
	t.Run("NewViperConfigHandler", func(t *testing.T) {
		handler := NewViperConfigHandler(tempDir)
		if handler == nil {
			t.Errorf("expected NewViperConfigHandler to return a non-nil instance")
		}
	})
}
