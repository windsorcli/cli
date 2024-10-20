package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNewYamlConfigHandler(t *testing.T) {
	t.Run("ErrorLoadingConfig", func(t *testing.T) {
		// Create a temporary directory
		tempDir, err := os.MkdirTemp("", "test")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

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

func TestYamlConfigHandler_LoadConfig(t *testing.T) {
	t.Run("WithPath", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler("")
		// Given a valid config path
		tempDir := t.TempDir()
		err := handler.LoadConfig(tempDir + "/config.yaml")
		// Then no error should be returned
		assertError(t, err, nil)
	})

	t.Run("WithInvalidPath", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler("")
		// Given an invalid config path
		tempDir := t.TempDir()
		invalidPath := tempDir + "/invalid.yaml"

		// Mock osReadFile to return an error
		originalOsReadFile := osReadFile
		osReadFile = func(string) ([]byte, error) {
			return nil, fmt.Errorf("mocked error reading file")
		}
		defer func() { osReadFile = originalOsReadFile }()

		err := handler.LoadConfig(invalidPath)
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

	t.Run("UnmarshalError", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler("")

		// Mock yamlUnmarshal to return an error
		originalYamlUnmarshal := yamlUnmarshal
		yamlUnmarshal = func(data []byte, v interface{}) error {
			return fmt.Errorf("mocked error unmarshalling yaml")
		}
		defer func() { yamlUnmarshal = originalYamlUnmarshal }()

		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.yaml")

		// Create a dummy config file
		osWriteFile(configPath, []byte("dummy: data"), 0644)

		err := handler.LoadConfig(configPath)
		if err == nil {
			t.Fatalf("LoadConfig() expected error, got nil")
		}

		expectedError := "error unmarshalling yaml: mocked error unmarshalling yaml"
		if err.Error() != expectedError {
			t.Fatalf("LoadConfig() error = %v, expected '%s'", err, expectedError)
		}
	})
}

func TestYamlConfigHandler_GetConfigValue(t *testing.T) {
	t.Run("WithExistingKey", func(t *testing.T) {
		tempDir := t.TempDir()
		handler, _ := NewYamlConfigHandler(tempDir + "/config.yaml")

		// Set a value first
		handler.Set("testKey", "testValue")

		// Mock osReadFile to simulate reading a config file with the key "testKey"
		originalOsReadFile := osReadFile
		osReadFile = func(filename string) ([]byte, error) {
			if filename == tempDir+"/config.yaml" {
				return []byte("testKey: testValue"), nil
			}
			return nil, fmt.Errorf("unexpected read operation")
		}
		defer func() { osReadFile = originalOsReadFile }()

		// Given an existing key in the config
		got, err := handler.Get("testKey")
		// Then the value should be retrieved without error
		assertError(t, err, nil)
		assertEqual(t, "testValue", got, "GetConfigValue")
	})

	t.Run("WithNonExistentKey", func(t *testing.T) {
		tempDir := t.TempDir()
		handler, _ := NewYamlConfigHandler(tempDir + "/config.yaml")
		// Given a non-existent key in the config
		_, err := handler.Get("nonExistentKey")
		// Then an error should be returned
		if err == nil {
			t.Errorf("GetConfigValue() expected error, got nil")
		}
	})

	t.Run("WithNonExistentKeyAndDefaultValue", func(t *testing.T) {
		tempDir := t.TempDir()
		handler, _ := NewYamlConfigHandler(tempDir + "/config.yaml")
		// Given a non-existent key in the config and a default value
		got, err := handler.GetString("nonExistentKey", "defaultValue")
		// Then the default value should be returned without error
		assertError(t, err, nil)
		assertEqual(t, "defaultValue", got, "GetConfigValue with default")
	})
}

func TestYamlConfigHandler_SetConfigValue(t *testing.T) {
	t.Run("WithNewKeyValuePair", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler("")
		// Given a new key-value pair
		err := handler.Set("newKey", "newValue")
		// Then the value should be set without error
		assertError(t, err, nil)
		got, _ := handler.Get("newKey")
		assertEqual(t, "newValue", got, "SetConfigValue")
	})

	t.Run("WithExistingKeyAndNewValue", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler("")
		// Given an existing key with a new value
		handler.Set("testKey", "testValue")
		err := handler.Set("testKey", "newTestValue")
		// Then the value should be updated without error
		assertError(t, err, nil)
		got, _ := handler.Get("testKey")
		assertEqual(t, "newTestValue", got, "SetConfigValue")
	})
}

func TestYamlConfigHandler_SaveConfig(t *testing.T) {
	t.Run("WithValidPath", func(t *testing.T) {
		handler, _ := NewYamlConfigHandler("")
		handler.Set("saveKey", "saveValue")
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
		handler.Set("saveKey", "saveValue")
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
		handler.Set("saveKey", "saveValue")

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
		handler.Set("saveKey", "saveValue")

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

	t.Run("UsesExistingPath", func(t *testing.T) {
		// Create a mock for osWriteFile
		expectedPath := "existing/path/to/config.yaml"
		originalOsWriteFile := osWriteFile
		defer func() { osWriteFile = originalOsWriteFile }()
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			if filename != expectedPath {
				return fmt.Errorf("expected filename %s, got %s", expectedPath, filename)
			}
			return nil
		}

		handler := &YamlConfigHandler{
			config: map[string]interface{}{"key": "value"},
			path:   expectedPath,
		}

		err := handler.SaveConfig("")
		if err != nil {
			t.Fatalf("SaveConfig() unexpected error: %v", err)
		}
	})
}

func TestYamlConfigHandler_GetString(t *testing.T) {
	t.Run("WithExistingKey", func(t *testing.T) {
		handler := &YamlConfigHandler{
			config: map[string]interface{}{
				"existingKey": "existingValue",
			},
		}

		// Given an existing key in the config
		got, err := handler.GetString("existingKey")
		if err != nil {
			t.Fatalf("GetString() unexpected error: %v", err)
		}

		// Then the value should be retrieved without error
		expectedValue := "existingValue"
		if got != expectedValue {
			t.Errorf("GetString() = %v, expected %v", got, expectedValue)
		}
	})

	t.Run("WithNonExistentKey", func(t *testing.T) {
		handler := &YamlConfigHandler{
			config: map[string]interface{}{},
		}

		// Given a non-existent key in the config
		_, err := handler.GetString("nonExistentKey")
		if err == nil {
			t.Fatalf("GetString() expected error, got nil")
		}

		// Then an error should be returned
		expectedError := "key nonExistentKey not found in configuration"
		if err.Error() != expectedError {
			t.Errorf("GetString() error = %v, expected '%s'", err, expectedError)
		}
	})
}

func TestYamlConfigHandler_GetInt(t *testing.T) {
	t.Run("WithExistingIntegerKey", func(t *testing.T) {
		handler := &YamlConfigHandler{
			config: map[string]interface{}{
				"integerKey": 42,
			},
		}

		// Given an existing key with an integer value
		got, err := handler.GetInt("integerKey")
		if err != nil {
			t.Fatalf("GetInt() unexpected error: %v", err)
		}

		// Then the integer value should be retrieved without error
		expectedValue := 42
		if got != expectedValue {
			t.Errorf("GetInt() = %v, expected %v", got, expectedValue)
		}
	})

	t.Run("WithExistingNonIntegerKey", func(t *testing.T) {
		handler := &YamlConfigHandler{
			config: map[string]interface{}{
				"nonIntegerKey": "notAnInt",
			},
		}

		// Given an existing key with a non-integer value
		_, err := handler.GetInt("nonIntegerKey")
		if err == nil {
			t.Fatalf("GetInt() expected error, got nil")
		}

		// Then an error should be returned indicating the value is not an integer
		expectedError := "key nonIntegerKey is not an integer"
		if err.Error() != expectedError {
			t.Errorf("GetInt() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("WithNonExistentKey", func(t *testing.T) {
		handler := &YamlConfigHandler{
			config: map[string]interface{}{},
		}

		// Given a non-existent key in the config
		_, err := handler.GetInt("nonExistentKey")
		if err == nil {
			t.Fatalf("GetInt() expected error, got nil")
		}

		// Then an error should be returned indicating the key was not found
		expectedError := "key nonExistentKey not found in configuration"
		if err.Error() != expectedError {
			t.Errorf("GetInt() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("WithNonExistentKeyAndDefaultValue", func(t *testing.T) {
		handler := &YamlConfigHandler{
			config: map[string]interface{}{},
		}

		// Given a non-existent key in the config and a default value
		got, err := handler.GetInt("nonExistentKey", 99)
		if err != nil {
			t.Fatalf("GetInt() unexpected error: %v", err)
		}

		// Then the default value should be returned without error
		expectedValue := 99
		if got != expectedValue {
			t.Errorf("GetInt() = %v, expected %v", got, expectedValue)
		}
	})
}

func TestYamlConfigHandler_GetBool(t *testing.T) {
	t.Run("WithExistingBooleanKey", func(t *testing.T) {
		handler := &YamlConfigHandler{
			config: map[string]interface{}{
				"booleanKey": true,
			},
		}

		// Given an existing key with a boolean value
		got, err := handler.GetBool("booleanKey")
		if err != nil {
			t.Fatalf("GetBool() unexpected error: %v", err)
		}

		// Then the boolean value should be retrieved without error
		expectedValue := true
		if got != expectedValue {
			t.Errorf("GetBool() = %v, expected %v", got, expectedValue)
		}
	})

	t.Run("WithExistingNonBooleanKey", func(t *testing.T) {
		handler := &YamlConfigHandler{
			config: map[string]interface{}{
				"nonBooleanKey": "notABool",
			},
		}

		// Given an existing key with a non-boolean value
		_, err := handler.GetBool("nonBooleanKey")
		if err == nil {
			t.Fatalf("GetBool() expected error, got nil")
		}

		// Then an error should be returned indicating the value is not a boolean
		expectedError := "key nonBooleanKey is not a boolean"
		if err.Error() != expectedError {
			t.Errorf("GetBool() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("WithNonExistentKey", func(t *testing.T) {
		handler := &YamlConfigHandler{
			config: map[string]interface{}{},
		}

		// Given a non-existent key in the config
		_, err := handler.GetBool("nonExistentKey")
		if err == nil {
			t.Fatalf("GetBool() expected error, got nil")
		}

		// Then an error should be returned indicating the key was not found
		expectedError := "key nonExistentKey not found in configuration"
		if err.Error() != expectedError {
			t.Errorf("GetBool() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("WithNonExistentKeyAndDefaultValue", func(t *testing.T) {
		handler := &YamlConfigHandler{
			config: map[string]interface{}{},
		}

		// Given a non-existent key in the config and a default value
		got, err := handler.GetBool("nonExistentKey", false)
		if err != nil {
			t.Fatalf("GetBool() unexpected error: %v", err)
		}

		// Then the default value should be returned without error
		expectedValue := false
		if got != expectedValue {
			t.Errorf("GetBool() = %v, expected %v", got, expectedValue)
		}
	})
}

func TestYamlConfigHandler_Set(t *testing.T) {
	t.Run("WithNestedKeys", func(t *testing.T) {
		handler := &YamlConfigHandler{
			config: make(map[string]interface{}),
		}

		// Given a nested key
		nestedKey := "parent.child"
		value := "nestedValue"

		// When setting the value for the nested key
		err := handler.Set(nestedKey, value)
		if err != nil {
			t.Fatalf("Set() unexpected error: %v", err)
		}

		// Then the nested maps should be created and the value set correctly
		parent, exists := handler.config["parent"]
		if !exists {
			t.Fatalf("Expected 'parent' key to exist")
		}

		childMap, ok := parent.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected 'parent' to be a map, got %T", parent)
		}

		got, exists := childMap["child"]
		if !exists {
			t.Fatalf("Expected 'child' key to exist")
		}

		if got != value {
			t.Errorf("Set() = %v, expected %v", got, value)
		}
	})

	t.Run("WithSingleKey", func(t *testing.T) {
		handler := &YamlConfigHandler{
			config: make(map[string]interface{}),
		}

		// Given a single key
		key := "simpleKey"
		value := "simpleValue"

		// When setting the value for the single key
		err := handler.Set(key, value)
		if err != nil {
			t.Fatalf("Set() unexpected error: %v", err)
		}

		// Then the value should be set correctly
		got, exists := handler.config[key]
		if !exists {
			t.Fatalf("Expected '%s' key to exist", key)
		}

		if got != value {
			t.Errorf("Set() = %v, expected %v", got, value)
		}
	})
}

func TestYamlConfigHandler_SetDefault(t *testing.T) {
	t.Run("SetDefaultContext", func(t *testing.T) {
		// Given a YamlConfigHandler and a default context
		handler := &YamlConfigHandler{
			config: make(map[string]interface{}),
		}
		defaultContext := Context{
			Environment: map[string]string{
				"ENV_VAR": "value",
			},
		}

		// When setting the default context
		handler.SetDefault(defaultContext)

		// Then the default context should be set correctly
		if handler.defaultContextConfig.Environment["ENV_VAR"] != "value" {
			t.Errorf("SetDefault() = %v, expected %v", handler.defaultContextConfig.Environment["ENV_VAR"], "value")
		}
	})
}
