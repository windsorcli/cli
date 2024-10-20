package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestYamlConfigHandler_LoadConfig(t *testing.T) {
	t.Run("WithPath", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler("")
		// Given a valid config path
		err := handler.LoadConfig(tempDir + "/config.yaml")
		// Then no error should be returned
		assertError(t, err, nil)
	})

	t.Run("WithInvalidPath", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler("")
		// Given an invalid config path
		err := handler.LoadConfig(tempDir + "/invalid.yaml")
		// Then an error should be returned
		if err == nil {
			t.Errorf("LoadConfig() expected error, got nil")
		}
	})

	t.Run("CreateEmptyConfigFileIfNotExist", func(t *testing.T) {
		// Mock osStat to simulate a non-existent file
		originalOsStat := osStat
		osStat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		defer func() { osStat = originalOsStat }()

		// Mock osWriteFile to simulate file creation
		originalOsWriteFile := osWriteFile
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			if filename == "test_config.yaml" && string(data) == "" {
				// Simulate successful file creation
				return nil
			}
			return fmt.Errorf("unexpected write operation")
		}
		defer func() { osWriteFile = originalOsWriteFile }()

		// Mock osReadFile to simulate reading the created file
		originalOsReadFile := osReadFile
		osReadFile = func(filename string) ([]byte, error) {
			if filename == "test_config.yaml" {
				// Simulate reading an empty file
				return []byte{}, nil
			}
			return nil, fmt.Errorf("unexpected read operation")
		}
		defer func() { osReadFile = originalOsReadFile }()

		// Ensure the file is considered created by the mock
		handler, _ := NewYamlConfigHandler("")
		err := handler.LoadConfig("test_config.yaml")
		if err != nil {
			t.Fatalf("LoadConfig() unexpected error: %v", err)
		}
	})

	t.Run("ErrorCreatingConfigFile", func(t *testing.T) {
		// Mock osStat to simulate a non-existent file
		originalOsStat := osStat
		osStat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		defer func() { osStat = originalOsStat }()

		// Mock osWriteFile to simulate an error during file creation
		originalOsWriteFile := osWriteFile
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mocked error creating file")
		}
		defer func() { osWriteFile = originalOsWriteFile }()

		handler, _ := NewYamlConfigHandler("")
		err := handler.LoadConfig("test_config.yaml")
		if err == nil {
			t.Fatalf("LoadConfig() expected error, got nil")
		}

		expectedError := "error creating config file: mocked error creating file"
		if err.Error() != expectedError {
			t.Fatalf("LoadConfig() error = %v, expected '%s'", err, expectedError)
		}
	})
}

func TestYamlConfigHandler_GetConfigValue(t *testing.T) {
	t.Run("WithExistingKey", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler(tempDir + "/config.yaml")
		// Given an existing key in the config
		got, err := handler.GetConfigValue("testKey")
		// Then the value should be retrieved without error
		assertError(t, err, nil)
		assertEqual(t, "testValue", got, "GetConfigValue")
	})

	t.Run("WithNonExistentKey", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler(tempDir + "/config.yaml")
		// Given a non-existent key in the config
		_, err := handler.GetConfigValue("nonExistentKey")
		// Then an error should be returned
		if err == nil {
			t.Errorf("GetConfigValue() expected error, got nil")
		}
	})

	t.Run("WithNonExistentKeyAndDefaultValue", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler(tempDir + "/config.yaml")
		// Given a non-existent key in the config and a default value
		got, err := handler.GetConfigValue("nonExistentKey", "defaultValue")
		// Then the default value should be returned without error
		assertError(t, err, nil)
		assertEqual(t, "defaultValue", got, "GetConfigValue with default")
	})
}

func TestYamlConfigHandler_SetConfigValue(t *testing.T) {
	t.Run("WithNewKeyValuePair", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler("")
		// Given a new key-value pair
		err := handler.SetConfigValue("newKey", "newValue")
		// Then the value should be set without error
		assertError(t, err, nil)
		got, _ := handler.GetConfigValue("newKey")
		assertEqual(t, "newValue", got, "SetConfigValue")
	})

	t.Run("WithExistingKeyAndNewValue", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler("")
		// Given an existing key with a new value
		handler.SetConfigValue("testKey", "testValue")
		err := handler.SetConfigValue("testKey", "newTestValue")
		// Then the value should be updated without error
		assertError(t, err, nil)
		got, _ := handler.GetConfigValue("testKey")
		assertEqual(t, "newTestValue", got, "SetConfigValue")
	})
}

func TestYamlConfigHandler_SaveConfig(t *testing.T) {
	t.Run("WithValidPath", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler("")
		handler.SetConfigValue("saveKey", "saveValue")
		// Given a valid config path
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "save_config.yaml")

		err := handler.SaveConfig(configPath)
		// Then the config should be saved without error
		assertError(t, err, nil)

		// And the config file should exist at the specified path
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Fatalf("Config file was not created at %s", configPath)
		}
	})

	t.Run("WithEmptyPath", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler("")
		handler.SetConfigValue("saveKey", "saveValue")
		// Given an empty config path
		err := handler.SaveConfig("")
		// Then an error should be returned
		if err == nil {
			t.Fatalf("SaveConfig() expected error, got nil")
		}

		expectedError := "path cannot be empty"
		if err.Error() != expectedError {
			t.Fatalf("SaveConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("MarshalError", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler("")
		handler.SetConfigValue("saveKey", "saveValue")

		// Mock yaml.Marshal to return an error
		originalYamlMarshal := yamlMarshal
		yamlMarshal = func(v interface{}) ([]byte, error) {
			return nil, fmt.Errorf("mocked error marshalling yaml")
		}
		defer func() { yamlMarshal = originalYamlMarshal }()

		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "save_config.yaml")

		err := handler.SaveConfig(configPath)
		if err == nil {
			t.Fatalf("SaveConfig() expected error, got nil")
		}

		expectedError := "error marshalling yaml: mocked error marshalling yaml"
		if err.Error() != expectedError {
			t.Fatalf("SaveConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("WriteFileError", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler("")
		handler.SetConfigValue("saveKey", "saveValue")

		// Mock osWriteFile to return an error
		originalOsWriteFile := osWriteFile
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mocked error writing file")
		}
		defer func() { osWriteFile = originalOsWriteFile }()

		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "save_config.yaml")

		err := handler.SaveConfig(configPath)
		if err == nil {
			t.Fatalf("SaveConfig() expected error, got nil")
		}

		expectedError := "error writing config file: mocked error writing file"
		if err.Error() != expectedError {
			t.Fatalf("SaveConfig() error = %v, expected '%s'", err, expectedError)
		}
	})
}

func TestYamlConfigHandler_GetNestedMap(t *testing.T) {
	t.Run("WithExistingNestedMap", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler("")
		// Given a nested map in the configuration
		nestedMap := map[string]interface{}{
			"some_env":       "value1",
			"some_other_env": "value2",
		}
		handler.SetConfigValue("contexts.blah.env", nestedMap)

		// When retrieving the nested map
		got, err := handler.GetNestedMap("contexts.blah.env")

		// Then the nested map should be retrieved without error
		assertError(t, err, nil)
		assertEqual(t, nestedMap, got, "GetNestedMap")
	})

	t.Run("WithNonExistentKey", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler("")
		handler.SetConfigValue("contexts.blah.env", map[string]interface{}{
			"some_env":       "value1",
			"some_other_env": "value2",
		})
		// Given a non-existent nested map key
		_, err := handler.GetNestedMap("contexts.nonexistent.env")
		// Then an error should be returned
		if err == nil {
			t.Errorf("GetNestedMap() expected error, got nil")
		}
	})

	t.Run("WithNonMapValue", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler("")
		handler.SetConfigValue("nonMapKey", "someValue")
		// Given a key that is not a map
		_, err := handler.GetNestedMap("nonMapKey")
		// Then an error should be returned
		if err == nil {
			t.Errorf("GetNestedMap() expected error, got nil")
		}
		expectedError := "key nonMapKey is not a nested map"
		if err.Error() != expectedError {
			t.Errorf("GetNestedMap() error = %v, expected '%s'", err, expectedError)
		}
	})
}

func TestNewYamlConfigHandler(t *testing.T) {
	t.Run("ErrorLoadingConfig", func(t *testing.T) {
		// Given a non-existent config path
		invalidPath := tempDir + "/nonexistent.yaml"

		// Mock osReadFile to return an error
		originalOsReadFile := osReadFile
		osReadFile = func(string) ([]byte, error) {
			return nil, fmt.Errorf("mocked error reading file")
		}
		defer func() { osReadFile = originalOsReadFile }()

		// When creating a new YamlConfigHandler
		handler, err := NewYamlConfigHandler(invalidPath)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if handler != nil {
			t.Errorf("expected handler to be nil, got %v", handler)
		}
	})
}
