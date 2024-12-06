package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/goccy/go-yaml"
)

// compareYAML compares two YAML byte slices by unmarshaling them into interface{} and using DeepEqual.
func compareYAML(t *testing.T, actualYAML, expectedYAML []byte) {
	var actualData interface{}
	var expectedData interface{}

	// Unmarshal actual YAML
	err := yaml.Unmarshal(actualYAML, &actualData)
	if err != nil {
		t.Fatalf("Failed to unmarshal actual YAML data: %v", err)
	}

	// Unmarshal expected YAML
	err = yaml.Unmarshal(expectedYAML, &expectedData)
	if err != nil {
		t.Fatalf("Failed to unmarshal expected YAML data: %v", err)
	}

	// Compare the data structures
	if !reflect.DeepEqual(actualData, expectedData) {
		actualFormatted, _ := yaml.Marshal(actualData)
		expectedFormatted, _ := yaml.Marshal(expectedData)
		t.Errorf("YAML mismatch.\nActual:\n%s\nExpected:\n%s", string(actualFormatted), string(expectedFormatted))
	}
}

func TestNewYamlConfigHandler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		handler := NewYamlConfigHandler()
		if handler == nil {
			t.Fatal("Expected non-nil YamlConfigHandler")
		}
	})
}

func TestYamlConfigHandler_LoadConfigUsingMocks(t *testing.T) {

	// Mock functions
	var (
		mockOsStat      = osStat
		mockOsMkdirAll  = osMkdirAll
		mockOsWriteFile = osWriteFile
	)

	// Restore original functions after tests
	defer func() {
		osStat = mockOsStat
		osMkdirAll = mockOsMkdirAll
		osWriteFile = mockOsWriteFile
	}()

	t.Run("ErrorCreatingDirectories", func(t *testing.T) {
		osStat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		osMkdirAll = func(path string, perm os.FileMode) error {
			return errors.New("mock error creating directories")
		}

		handler := &YamlConfigHandler{}
		err := handler.LoadConfig("some/path/config.yaml")

		// Check if the error message matches the expected error
		expectedError := "error creating directories: mock error creating directories"
		if err == nil || err.Error() != expectedError {
			t.Errorf("expected error '%v', got '%v'", expectedError, err)
		}
	})
}

func TestYamlConfigHandler_LoadConfig(t *testing.T) {
	t.Run("WithPath", func(t *testing.T) {
		handler := NewYamlConfigHandler()
		// Given a valid config path
		tempDir := t.TempDir()
		err := handler.LoadConfig(tempDir + "/config.yaml")
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("WithInvalidPath", func(t *testing.T) {
		handler := NewYamlConfigHandler()
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
		handler := NewYamlConfigHandler()
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

		handler := NewYamlConfigHandler()
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
		handler := NewYamlConfigHandler()

		// Mock yamlUnmarshal to return an error
		originalYamlUnmarshal := yamlUnmarshal
		defer func() { yamlUnmarshal = originalYamlUnmarshal }()
		yamlUnmarshal = func(data []byte, v interface{}) error {
			return fmt.Errorf("mocked error unmarshalling yaml")
		}

		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.yaml")

		// Create a dummy config file
		if err := osWriteFile(configPath, []byte("dummy: data"), 0644); err != nil {
			t.Fatalf("Failed to create dummy config file: %v", err)
		}

		err := handler.LoadConfig(configPath)
		if err == nil {
			t.Fatalf("LoadConfig() expected error, got nil")
		}

		expectedError := "error unmarshalling yaml: mocked error unmarshalling yaml"
		if err.Error() != expectedError {
			t.Errorf("LoadConfig() error = %v, expected '%s'", err, expectedError)
		}
	})
}

func TestYamlConfigHandler_Get(t *testing.T) {
	t.Run("KeyNotUnderContexts", func(t *testing.T) {
		// Given a handler with key not under 'contexts'
		handler := NewYamlConfigHandler()
		// Set the context in y.config
		handler.Set("context", "local")
		// Set the default context (should not be used)
		defaultContext := Context{
			AWS: &AWSConfig{
				AWSEndpointURL: ptrString("http://default.aws.endpoint"),
			},
		}
		handler.SetDefault(defaultContext)

		// When getting a key not under 'contexts'
		value := handler.Get("some.other.key")

		// Then default context should not be used, and an error should be returned
		expectedError := "key some.other.key not found in configuration"
		if value != nil {
			t.Errorf("Expected error '%v', got '%v'", expectedError, value)
		}
	})

	t.Run("InvalidPath", func(t *testing.T) {
		// Given a handler
		handler := NewYamlConfigHandler()

		// When calling Get with an empty path
		value := handler.Get("")

		// Then an error should be returned
		expectedErr := "invalid path"
		if value != nil {
			t.Errorf("Expected error '%s', got %v", expectedErr, value)
		}
	})
}

func TestYamlConfigHandler_SaveConfig(t *testing.T) {
	t.Run("WithValidPath", func(t *testing.T) {
		handler := NewYamlConfigHandler()
		handler.Set("saveKey", "saveValue")
		// Given a valid config path
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "save_config.yaml")

		err := handler.SaveConfig(configPath)
		// Then the config should be saved without error
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}

		// And the config file should exist at the specified path
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Fatalf("Config file was not created at %s", configPath)
		}
	})

	t.Run("WithEmptyPath", func(t *testing.T) {
		handler := NewYamlConfigHandler()
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

	t.Run("MarshallingError", func(t *testing.T) {
		// Create a mock for yamlMarshalNonNull
		originalYamlMarshalNonNull := yamlMarshalNonNull
		defer func() { yamlMarshalNonNull = originalYamlMarshalNonNull }()
		yamlMarshalNonNull = func(v interface{}) ([]byte, error) {
			return nil, fmt.Errorf("mock marshalling error")
		}

		// Create a YamlConfigHandler with a sample config
		handler := &YamlConfigHandler{
			config: Config{}, // Assuming Config is your struct
			path:   "test.yaml",
		}

		// Call SaveConfig and expect an error
		err := handler.SaveConfig("test.yaml")
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Check if the error message is as expected
		expectedErrorMessage := "error marshalling yaml: mock marshalling error"
		if err.Error() != expectedErrorMessage {
			t.Errorf("Unexpected error message. Got: %s, Expected: %s", err.Error(), expectedErrorMessage)
		}
	})

	t.Run("WriteFileError", func(t *testing.T) {
		handler := NewYamlConfigHandler()
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
		tempDir := t.TempDir()
		expectedPath := filepath.Join(tempDir, "config.yaml")

		// Create a mock for osWriteFile
		originalOsWriteFile := osWriteFile
		defer func() { osWriteFile = originalOsWriteFile }()
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			if filename != expectedPath {
				return fmt.Errorf("expected filename %s, got %s", expectedPath, filename)
			}
			return nil
		}

		// Create a mock for osReadFile
		originalOsReadFile := osReadFile
		defer func() { osReadFile = originalOsReadFile }()
		osReadFile = func(filename string) ([]byte, error) {
			if filename != expectedPath {
				return nil, fmt.Errorf("expected filename %s, got %s", expectedPath, filename)
			}
			return []byte{}, nil
		}

		handler := NewYamlConfigHandler()
		handler.path = expectedPath // Ensure the config handler has a path set
		handler.Set("key", "value")

		err := handler.SaveConfig("")
		if err != nil {
			t.Fatalf("SaveConfig() unexpected error: %v", err)
		}
	})

	t.Run("OmitsNullValues", func(t *testing.T) {
		// Setup a temporary directory for the test
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.yaml")

		// Create a YamlConfigHandler with some initial configuration
		handler := &YamlConfigHandler{
			config: Config{
				Context: nil,
				Contexts: map[string]*Context{
					"default": {
						Environment: map[string]string{
							"name":  "John Doe",
							"email": "john.doe@example.com",
						},
						AWS: &AWSConfig{
							AWSEndpointURL: nil,
						},
					},
				},
			},
		}

		// Call SaveConfig to write the configuration to a file
		err := handler.SaveConfig(configPath)
		if err != nil {
			t.Fatalf("SaveConfig() unexpected error: %v", err)
		}

		// Read the file to verify its contents
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("Failed to read config file: %v", err)
		}

		// Check that the YAML data does not contain the "age" field
		expectedContent := "contexts:\n  default:\n    environment:\n      email: john.doe@example.com\n      name: John Doe\n"
		if string(data) != expectedContent {
			t.Errorf("Config file content = %v, expected %v", string(data), expectedContent)
		}
	})
}

func TestYamlConfigHandler_GetString(t *testing.T) {
	t.Run("WithNonExistentKey", func(t *testing.T) {
		// Mock the existing context in the configuration
		handler := &YamlConfigHandler{
			config: Config{
				Context: ptrString("default"),
			},
		}

		// Given a non-existent key in the config
		got := handler.GetString("nonExistentKey")

		// Then an error should be returned
		expectedValue := ""
		if got != expectedValue {
			t.Errorf("GetString() = %v, expected %v", got, expectedValue)
		}
	})

	t.Run("GetStringWithDefaultValue", func(t *testing.T) {
		// Mock the existing context in the configuration
		handler := &YamlConfigHandler{
			config: Config{
				Context: ptrString("default"),
			},
		}

		// When calling GetString with a non-existent key and a default value
		defaultValue := "defaultString"
		value := handler.GetString("non.existent.key", defaultValue)

		// Then the default value should be returned without error
		if value != defaultValue {
			t.Errorf("Expected value '%v', got '%v'", defaultValue, value)
		}
	})
}

func TestYamlConfigHandler_GetInt(t *testing.T) {
	t.Run("WithExistingNonIntegerKey", func(t *testing.T) {
		// Mock the existing context in the configuration
		handler := &YamlConfigHandler{
			config: Config{
				Context: ptrString("default"),
				Contexts: map[string]*Context{
					"default": {
						AWS: &AWSConfig{
							AWSEndpointURL: ptrString("notAnInt"),
						},
					},
				},
			},
		}

		// Given an existing key with a non-integer value
		value := handler.GetInt("aws.aws_endpoint_url")

		// Then an error should be returned indicating the value is not an integer
		expectedValue := 0
		if value != expectedValue {
			t.Errorf("Expected value %v, got %v", expectedValue, value)
		}
	})

	t.Run("WithNonExistentKey", func(t *testing.T) {
		// Mock the existing context in the configuration
		handler := &YamlConfigHandler{
			config: Config{
				Context: ptrString("default"),
			},
		}

		// Given a non-existent key in the config
		value := handler.GetInt("nonExistentKey")
		expectedValue := 0
		if value != expectedValue {
			t.Errorf("Expected value %v, got %v", expectedValue, value)
		}

		// Then an error should be returned indicating the key was not found
	})

	t.Run("WithNonExistentKeyAndDefaultValue", func(t *testing.T) {
		// Mock the existing context in the configuration
		handler := &YamlConfigHandler{
			config: Config{
				Context: ptrString("default"),
			},
		}

		// Given a non-existent key in the config and a default value
		got := handler.GetInt("nonExistentKey", 99)

		// Then the default value should be returned without error
		expectedValue := 99
		if got != expectedValue {
			t.Errorf("GetInt() = %v, expected %v", got, expectedValue)
		}
	})
}

func TestYamlConfigHandler_GetBool(t *testing.T) {
	t.Run("WithExistingBooleanKey", func(t *testing.T) {
	})

	t.Run("WithExistingNonBooleanKey", func(t *testing.T) {
		// Mock the existing context in the configuration
		handler := &YamlConfigHandler{
			config: Config{
				Context: ptrString("default"),
			},
		}

		// Set a non-boolean value for the key
		handler.Set("contexts.default.aws.aws_endpoint_url", "notABool")

		// Given an existing key with a non-boolean value
		value := handler.GetBool("aws.aws_endpoint_url")
		expectedValue := false

		// Then an error should be returned indicating the value is not a boolean
		if value != expectedValue {
			t.Errorf("Expected value %v, got %v", expectedValue, value)
		}
	})

	t.Run("WithNonExistentKey", func(t *testing.T) {
		// Mock the existing context in the configuration
		handler := &YamlConfigHandler{
			config: Config{
				Context: ptrString("default"),
			},
		}

		// Given a non-existent key in the config
		value := handler.GetBool("nonExistentKey")
		expectedValue := false
		if value != expectedValue {
			t.Errorf("Expected value %v, got %v", expectedValue, value)
		}
	})

	t.Run("WithNonExistentKeyAndDefaultValue", func(t *testing.T) {
		// Mock the existing context in the configuration
		handler := &YamlConfigHandler{
			config: Config{
				Context: ptrString("default"),
			},
		}

		// Given a non-existent key in the config and a default value
		got := handler.GetBool("nonExistentKey", false)

		// Then the default value should be returned without error
		expectedValue := false
		if got != expectedValue {
			t.Errorf("GetBool() = %v, expected %v", got, expectedValue)
		}
	})
}

func TestYamlConfigHandler_GetConfig(t *testing.T) {
	t.Run("ContextIsSet", func(t *testing.T) {
		// Given a handler with a context set
		handler := NewYamlConfigHandler()
		handler.config.Context = ptrString("local")
		handler.config.Contexts = map[string]*Context{
			"local": {
				Environment: map[string]string{
					"ENV_VAR": "value",
				},
			},
		}

		// When calling GetConfig
		config := handler.GetConfig()

		// Then the context config should be returned without error
		if config == nil || config.Environment["ENV_VAR"] != "value" {
			t.Errorf("Expected context config with ENV_VAR 'value', got %v", config)
		}
	})

	t.Run("ContextIsNotSet", func(t *testing.T) {
		// Given a handler without a context set
		handler := NewYamlConfigHandler()

		// When calling GetConfig
		config := handler.GetConfig()

		// Then the default context config should be returned without error
		if config == nil || len(config.Environment) != 0 {
			t.Errorf("Expected empty config map, got %v", config)
		}
	})

	t.Run("ContextDoesNotExist", func(t *testing.T) {
		// Given a handler with a context set that does not exist in contexts
		handler := NewYamlConfigHandler()
		handler.config.Context = ptrString("nonexistent")

		// When calling GetConfig
		config := handler.GetConfig()

		// Then the config should be an empty map and no error should be returned
		if config == nil || len(config.Environment) != 0 {
			t.Errorf("Expected empty config map, got %v", config)
		}
	})
}

func TestYamlConfigHandler_Set(t *testing.T) {
	t.Run("SetWithInvalidPath", func(t *testing.T) {
		handler := NewYamlConfigHandler()

		// Attempt to set a value with an empty path
		handler.Set("", "someValue")

		// Check if the error is as expected
		if handler.Get("") != nil {
			t.Fatalf("Set() expected error, got nil")
		}
	})
}

func TestYamlConfigHandler_SetContextValue(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a handler with a valid context set
		handler := NewYamlConfigHandler()
		handler.config.Context = ptrString("local")

		// When calling SetContextValue with a valid path and value
		err := handler.SetContextValue("environment.TEST_KEY", "someValue")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("SetContextValue() unexpected error: %v", err)
		}

		// And the value should be set correctly in the context
		value := handler.Get("contexts.local.environment.TEST_KEY")
		if value != "someValue" {
			t.Errorf("Expected value 'someValue', got %v", value)
		}
	})

	t.Run("ContextNotSet", func(t *testing.T) {
		// Given a handler without a context set
		handler := NewYamlConfigHandler()

		// When calling SetContextValue
		err := handler.SetContextValue("some.path", "someValue")

		// Then an error should be returned
		expectedError := "current context is not set"
		if err == nil || err.Error() != expectedError {
			t.Errorf("Expected error '%s', got %v", expectedError, err)
		}
	})

	t.Run("InvalidPath", func(t *testing.T) {
		// Given a handler with a valid context set
		handler := NewYamlConfigHandler()
		handler.config.Context = ptrString("local")

		// When calling SetContextValue with an empty path
		err := handler.SetContextValue("", "someValue")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
	})
}

func TestYamlConfigHandler_SetDefault(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a YamlConfigHandler and a default context
		handler := NewYamlConfigHandler()
		defaultContext := Context{
			Environment: map[string]string{
				"ENV_VAR": "value",
			},
		}

		// Set the context in y.config
		handler.Set("context", "local")

		// When setting the default context
		err := handler.SetDefault(defaultContext)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Then the default context should be set correctly
		if handler.defaultContextConfig.Environment["ENV_VAR"] != "value" {
			t.Errorf("SetDefault() = %v, expected %v", handler.defaultContextConfig.Environment["ENV_VAR"], "value")
		}
	})

	t.Run("NilContext", func(t *testing.T) {
		// Given a handler with a nil context
		handler := NewYamlConfigHandler()
		handler.config.Context = nil

		// When calling SetDefault
		defaultContext := Context{
			Environment: map[string]string{
				"ENV_VAR": "value",
			},
		}
		err := handler.SetDefault(defaultContext)

		// Then the default context should be set correctly
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if handler.defaultContextConfig.Environment["ENV_VAR"] != "value" {
			t.Errorf("SetDefault() = %v, expected %v", handler.defaultContextConfig.Environment["ENV_VAR"], "value")
		}
	})

	t.Run("ContextNotSet", func(t *testing.T) {
		// Given a handler without a context set
		handler := NewYamlConfigHandler()

		// When calling SetDefault
		defaultContext := Context{
			Environment: map[string]string{
				"ENV_VAR": "value",
			},
		}
		err := handler.SetDefault(defaultContext)

		// Then the default context should be set correctly
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if handler.defaultContextConfig.Environment["ENV_VAR"] != "value" {
			t.Errorf("SetDefault() = %v, expected %v", handler.defaultContextConfig.Environment["ENV_VAR"], "value")
		}
	})
}

func TestSetValueByPath(t *testing.T) {
	t.Run("EmptyPathKeys", func(t *testing.T) {
		// Given an empty pathKeys slice
		var currValue reflect.Value
		pathKeys := []string{}
		value := "test"

		// When calling setValueByPath
		fullPath := "some.full.path"
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned
		if err == nil || err.Error() != "pathKeys cannot be empty" {
			t.Errorf("Expected error 'pathKeys cannot be empty', got %v", err)
		}
	})

	t.Run("UnsupportedKind", func(t *testing.T) {
		// Given a currValue of unsupported kind (e.g., int)
		currValue := reflect.ValueOf(42)
		pathKeys := []string{"key"}
		value := "test"
		fullPath := "some.full.path"
		// When calling setValueByPath
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned
		expectedErr := "Invalid path: some.full.path"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got %v", expectedErr, err)
		}
	})

	t.Run("MapKeyTypeMismatch", func(t *testing.T) {
		// Given a map with int keys but providing a string key
		currValue := reflect.ValueOf(make(map[int]string))
		pathKeys := []string{"stringKey"}
		value := "testValue"
		fullPath := "some.full.path"

		// When calling setValueByPath
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned
		expectedErr := "key type mismatch: expected int, got string"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got %v", expectedErr, err)
		}
	})

	t.Run("MapValueTypeMismatch", func(t *testing.T) {
		// Given a map with string values but providing a non-convertible value
		currValue := reflect.ValueOf(make(map[string]int))
		pathKeys := []string{"key"}
		value := "stringValue" // Cannot convert string to int
		fullPath := "some.full.path"

		// When calling setValueByPath
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned
		expectedErr := "value type mismatch for key key: expected int, got string"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got %v", expectedErr, err)
		}
	})

	t.Run("MapSuccess", func(t *testing.T) {
		// Given a map with string keys and interface{} values
		testMap := make(map[string]interface{})
		currValue := reflect.ValueOf(testMap)
		pathKeys := []string{"key"}
		value := "testValue"
		fullPath := "some.full.path"

		// When calling setValueByPath
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then the map should be updated without error
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if testMap["key"] != "testValue" {
			t.Errorf("Expected map['key'] to be 'testValue', got '%v'", testMap["key"])
		}
	})

	t.Run("MapInitializeNilMap", func(t *testing.T) {
		// Given a nil map
		var testMap map[string]interface{}
		currValue := reflect.ValueOf(&testMap).Elem()
		pathKeys := []string{"key"}
		value := "testValue"
		fullPath := "some.full.path"

		// When calling setValueByPath
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then the map should be initialized and updated without error
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if testMap["key"] != "testValue" {
			t.Errorf("Expected map['key'] to be 'testValue', got '%v'", testMap["key"])
		}
	})

	t.Run("MapExistingValue", func(t *testing.T) {
		// Given a Config with an existing nested map
		config := Config{
			Contexts: map[string]*Context{
				"level1": {
					Environment: map[string]string{
						"level2": "value2",
					},
					AWS: &AWSConfig{
						AWSEndpointURL: ptrString("http://aws.test:4566"),
					},
				},
			},
		}
		currValue := reflect.ValueOf(&config).Elem()
		pathKeys := []string{"contexts", "level1", "environment", "level2"}
		value := "testValue"
		fullPath := "contexts.level1.environment.level2"

		// When calling setValueByPath
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then the existing nested map should be updated without error
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		gotValue := config.Contexts["level1"].Environment["level2"]
		if gotValue != "testValue" {
			t.Errorf("Expected value to be 'testValue', got '%v'", gotValue)
		}
	})

	t.Run("MapValueConversion", func(t *testing.T) {
		// Given a map with integer elements
		testMap := map[string]int{}
		currValue := reflect.ValueOf(testMap)
		pathKeys := []string{"key"} // Key in the map
		value := 42.0               // Float value that is convertible to int
		fullPath := "some.full.path"

		// When calling setValueByPath
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then the value should be converted and set without error
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if testMap["key"] != 42 {
			t.Errorf("Expected map['key'] to be 42, got '%v'", testMap["key"])
		}
	})

	t.Run("RecursiveMap", func(t *testing.T) {
		// Given a map with nested maps
		level3Map := map[string]interface{}{}
		level2Map := map[string]interface{}{"level3": level3Map}
		level1Map := map[string]interface{}{"level2": level2Map}
		testMap := map[string]interface{}{"docker": level1Map} // Valid first key
		currValue := reflect.ValueOf(testMap)
		pathKeys := []string{"docker", "level2", "nonexistentfield"}
		value := "newValue"
		fullPath := "docker.level2.nonexistentfield"

		// When calling setValueByPath
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned indicating the field was not found
		expectedErr := "Invalid path: docker.level2.nonexistentfield"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got %v", expectedErr, err)
		}
	})

	t.Run("MakeAddressable_WithAddressableValue", func(t *testing.T) {
		// Given an addressable reflect.Value
		var x int = 42
		v := reflect.ValueOf(&x).Elem()

		// Ensure that v is addressable
		if !v.CanAddr() {
			t.Fatal("Expected v to be addressable")
		}

		// When calling makeAddressable
		result := makeAddressable(v)

		// Then the original value should be returned
		if result.Interface() != v.Interface() {
			t.Errorf("Expected the same value to be returned")
		}
	})
}

// TestGetValueByPath tests the getValueByPath function
func TestGetValueByPath(t *testing.T) {
	t.Run("EmptyPathKeys", func(t *testing.T) {
		// Given an empty pathKeys slice
		var current interface{}
		pathKeys := []string{}

		// When calling getValueByPath
		value := getValueByPath(current, pathKeys)

		// Then an error should be returned
		if value != nil {
			t.Errorf("Expected value to be nil, got %v", value)
		}
	})

	t.Run("InvalidCurrentValue", func(t *testing.T) {
		// Given an invalid current value
		var current interface{} = nil
		pathKeys := []string{"key"}

		// When calling getValueByPath
		value := getValueByPath(current, pathKeys)

		// Then an error should be returned
		if value != nil {
			t.Errorf("Expected value to be nil, got %v", value)
		}
	})

	t.Run("MapKeyTypeMismatch", func(t *testing.T) {
		// Given a map with int keys but providing a string key
		current := map[int]string{1: "one", 2: "two"}
		pathKeys := []string{"1"}

		// When calling getValueByPath
		value := getValueByPath(current, pathKeys)

		// Then an error should be returned
		if value != nil {
			t.Errorf("Expected value to be nil, got %v", value)
		}
	})

	t.Run("MapSuccess", func(t *testing.T) {
		// Given a map with the specified key
		current := map[string]string{"key": "testValue"}
		pathKeys := []string{"key"}

		// When calling getValueByPath
		value := getValueByPath(current, pathKeys)

		// Then the value should be retrieved without error
		if value == nil {
			t.Errorf("Expected value to be 'testValue', got nil")
		}
		expectedValue := "testValue"
		if value != expectedValue {
			t.Errorf("Expected value '%s', got '%v'", expectedValue, value)
		}
	})

	t.Run("CannotSetField", func(t *testing.T) {
		// Given a struct with an unexported field
		type TestStruct struct {
			unexportedField string `yaml:"unexportedfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"unexportedfield"}
		value := "testValue"
		fullPath := "unexportedfield"

		// When calling setValueByPath
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned
		expectedErr := "cannot set field"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%v'", expectedErr, err)
		}
	})

	t.Run("RecursiveFailure", func(t *testing.T) {
		// Given a map with nested maps
		level3Map := map[string]interface{}{}
		level2Map := map[string]interface{}{"level3": level3Map}
		level1Map := map[string]interface{}{"level2": level2Map}
		testMap := map[string]interface{}{"level1": level1Map}
		currValue := reflect.ValueOf(testMap)
		pathKeys := []string{"level1", "level2", "nonexistentfield"}
		value := "newValue"
		fullPath := "level1.level2.nonexistentfield"

		// When calling setValueByPath
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned indicating the field does not exist
		expectedErr := "Invalid path: level1.level2.nonexistentfield"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%v'", expectedErr, err)
		}
	})

	t.Run("AssignValueTypeMismatch", func(t *testing.T) {
		// Given a struct with a field of a specific type
		type TestStruct struct {
			IntField int `yaml:"intfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"intfield"}
		value := []string{"incompatibleType"} // A slice, which is incompatible with int
		fullPath := "intfield"

		// When calling setValueByPath
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned indicating the type mismatch
		expectedErr := "cannot assign value of type []string to field of type int"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%v'", expectedErr, err)
		}
	})

	t.Run("AssignPointerValueTypeMismatch", func(t *testing.T) {
		// Given a struct with a pointer field of a specific type
		type TestStruct struct {
			IntPtrField *int `yaml:"intptrfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"intptrfield"}
		value := []string{"incompatibleType"} // A slice, which is incompatible with *int
		fullPath := "intptrfield"

		// When calling setValueByPath
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned indicating the type mismatch
		expectedErr := "cannot assign value of type []string to field of type *int"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%v'", expectedErr, err)
		}
	})

	t.Run("AssignNonPointerField", func(t *testing.T) {
		// Given a struct with a field of a specific type
		type TestStruct struct {
			StringField string `yaml:"stringfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"stringfield"}
		value := "testValue" // Directly assignable to string
		fullPath := "stringfield"

		// When calling setValueByPath
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then the field should be set without error
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if testStruct.StringField != "testValue" {
			t.Errorf("Expected StringField to be 'testValue', got '%v'", testStruct.StringField)
		}
	})

	t.Run("AssignConvertibleType", func(t *testing.T) {
		// Given a struct with a field of a specific type
		type TestStruct struct {
			IntField int `yaml:"intfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"intfield"}
		value := 42.0 // A float64, which is convertible to int
		fullPath := "intfield"

		// When calling setValueByPath
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then the field should be set without error
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if testStruct.IntField != 42 {
			t.Errorf("Expected IntField to be 42, got '%v'", testStruct.IntField)
		}
	})
}

func TestYamlMarshalNonNull(t *testing.T) {
	// Test case for a struct with all non-nil values
	t.Run("AllNonNilValues", func(t *testing.T) {
		type NestedStruct struct {
			FieldA string `yaml:"field_a"`
			FieldB int    `yaml:"field_b"`
		}

		type TestStruct struct {
			Name    string            `yaml:"name"`
			Age     int               `yaml:"age"`
			Nested  NestedStruct      `yaml:"nested"`
			Numbers []int             `yaml:"numbers"`
			MapData map[string]string `yaml:"map_data"`
		}

		testData := TestStruct{
			Name: "Alice",
			Age:  30,
			Nested: NestedStruct{
				FieldA: "ValueA",
				FieldB: 42,
			},
			Numbers: []int{1, 2, 3},
			MapData: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		}

		expectedYAML := `
name: Alice
age: 30
nested:
  field_a: ValueA
  field_b: 42
numbers:
  - 1
  - 2
  - 3
map_data:
  key1: value1
  key2: value2
`

		data, err := yamlMarshalNonNull(testData)
		if err != nil {
			t.Fatalf("yamlMarshalNonNull() error: %v", err)
		}

		compareYAML(t, data, []byte(expectedYAML))
	})

	// Test case for a struct with nil pointer fields
	t.Run("NilPointerFields", func(t *testing.T) {
		type TestStruct struct {
			Name    *string `yaml:"name"`
			Age     *int    `yaml:"age"`
			Comment *string `yaml:"comment"`
		}

		age := 25
		testData := TestStruct{
			Name:    nil,  // Should be omitted
			Age:     &age, // Should be included
			Comment: nil,  // Should be omitted
		}

		expectedYAML := `age: 25
`

		data, err := yamlMarshalNonNull(testData)
		if err != nil {
			t.Fatalf("yamlMarshalNonNull() error: %v", err)
		}

		if string(data) != expectedYAML {
			t.Errorf("yamlMarshalNonNull() output = %s, expected %s", string(data), expectedYAML)
		}
	})

	// Test case for a struct with zero values
	t.Run("ZeroValues", func(t *testing.T) {
		type TestStruct struct {
			Name    string `yaml:"name"`
			Age     int    `yaml:"age"`
			Active  bool   `yaml:"active"`
			Comment string `yaml:"comment"`
		}

		testData := TestStruct{
			Name:    "",    // Empty string, should be included
			Age:     0,     // Zero value, should be included
			Active:  false, // Zero value for bool, should be included
			Comment: "",    // Empty string, should be included
		}

		expectedYAML := `
name: ""
age: 0
active: false
comment: ""
`

		data, err := yamlMarshalNonNull(testData)
		if err != nil {
			t.Fatalf("yamlMarshalNonNull() error: %v", err)
		}

		compareYAML(t, data, []byte(expectedYAML))
	})

	// Test case for a struct with nil slices and maps
	t.Run("NilSlicesAndMaps", func(t *testing.T) {
		type TestStruct struct {
			Numbers []int          `yaml:"numbers"`
			MapData map[string]int `yaml:"map_data"`
			Nested  *TestStruct    `yaml:"nested"`
		}

		testData := TestStruct{
			Numbers: nil, // Should be omitted
			MapData: nil, // Should be omitted
			Nested:  nil, // Should be omitted
		}

		expectedYAML := `` // Expecting an empty YAML

		data, err := yamlMarshalNonNull(testData)
		if err != nil {
			t.Fatalf("yamlMarshalNonNull() error: %v", err)
		}

		compareYAML(t, data, []byte(expectedYAML))
	})

	// Test case for a struct with empty slices and maps
	t.Run("EmptySlicesAndMaps", func(t *testing.T) {
		type TestStruct struct {
			Numbers []int          `yaml:"numbers"`
			MapData map[string]int `yaml:"map_data"`
		}

		testData := TestStruct{
			Numbers: []int{},          // Should be included as empty slice
			MapData: map[string]int{}, // Should be included as empty map
		}

		expectedYAML := ``

		data, err := yamlMarshalNonNull(testData)
		if err != nil {
			t.Fatalf("yamlMarshalNonNull() error: %v", err)
		}

		compareYAML(t, data, []byte(expectedYAML))
	})

	// Test case for unexported fields
	t.Run("UnexportedFields", func(t *testing.T) {
		type TestStruct struct {
			ExportedField   string `yaml:"exported_field"`
			unexportedField string `yaml:"unexported_field"`
		}

		testData := TestStruct{
			ExportedField:   "Visible",
			unexportedField: "Hidden",
		}

		expectedYAML := `exported_field: Visible
`

		data, err := yamlMarshalNonNull(testData)
		if err != nil {
			t.Fatalf("yamlMarshalNonNull() error: %v", err)
		}
		if string(data) != expectedYAML {
			t.Errorf("yamlMarshalNonNull() output = '%s', expected '%s'", string(data), expectedYAML)
		}
	})

	// Test case with fields tagged to be omitted
	t.Run("OmittedFields", func(t *testing.T) {
		type TestStruct struct {
			Name   string `yaml:"name"`
			Secret string `yaml:"-"` // Should be omitted
		}

		testData := TestStruct{
			Name:   "Bob",
			Secret: "SuperSecret",
		}

		expectedYAML := `
name: Bob
`

		data, err := yamlMarshalNonNull(testData)
		if err != nil {
			t.Fatalf("yamlMarshalNonNull() error: %v", err)
		}

		compareYAML(t, data, []byte(expectedYAML))
	})

	// Test case for nested pointers
	t.Run("NestedPointers", func(t *testing.T) {
		type InnerStruct struct {
			Value *string `yaml:"value"`
		}

		type OuterStruct struct {
			Inner *InnerStruct `yaml:"inner"`
		}

		// Test when Inner is nil
		testData := OuterStruct{
			Inner: nil,
		}
		expectedYAML := `` // Expecting an empty YAML

		data, err := yamlMarshalNonNull(testData)
		if err != nil {
			t.Fatalf("yamlMarshalNonNull() error: %v", err)
		}

		compareYAML(t, data, []byte(expectedYAML))

		// Test when Inner.Value is nil
		testData.Inner = &InnerStruct{
			Value: nil,
		}
		expectedYAML = ``

		data, err = yamlMarshalNonNull(testData)
		if err != nil {
			t.Fatalf("yamlMarshalNonNull() error: %v", err)
		}

		compareYAML(t, data, []byte(expectedYAML))

		// Test when Inner.Value is non-nil
		val := "SomeValue"
		testData.Inner.Value = &val
		expectedYAML = `
inner:
  value: SomeValue
`

		data, err = yamlMarshalNonNull(testData)
		if err != nil {
			t.Fatalf("yamlMarshalNonNull() error: %v", err)
		}

		compareYAML(t, data, []byte(expectedYAML))
	})

	// Test case for slices with nil elements
	t.Run("SliceWithNilElements", func(t *testing.T) {
		type TestStruct struct {
			Items []interface{} `yaml:"items"`
		}

		testData := TestStruct{
			Items: []interface{}{
				"Item1",
				nil, // Should appear as null in YAML
				"Item3",
			},
		}

		expectedYAML := `
items:
  - "Item1"
  - null
  - "Item3"
`

		data, err := yamlMarshalNonNull(testData)
		if err != nil {
			t.Fatalf("yamlMarshalNonNull() error: %v", err)
		}

		compareYAML(t, data, []byte(expectedYAML))
	})

	// Test case for maps with nil values
	t.Run("MapWithNilValues", func(t *testing.T) {
		type TestStruct struct {
			Data map[string]interface{} `yaml:"data"`
		}

		testData := TestStruct{
			Data: map[string]interface{}{
				"key1": "value1",
				"key2": nil, // Should be omitted
			},
		}

		expectedYAML := `
data:
  key1: "value1"
`

		data, err := yamlMarshalNonNull(testData)
		if err != nil {
			t.Fatalf("yamlMarshalNonNull() error: %v", err)
		}

		compareYAML(t, data, []byte(expectedYAML))
	})

	// Test case for interface fields
	t.Run("InterfaceFields", func(t *testing.T) {
		type TestStruct struct {
			Info interface{} `yaml:"info"`
		}

		// When Info is nil
		testData := TestStruct{
			Info: nil,
		}
		expectedYAML := `` // Expecting an empty YAML

		data, err := yamlMarshalNonNull(testData)
		if err != nil {
			t.Fatalf("yamlMarshalNonNull() error when Info is nil: %v", err)
		}

		compareYAML(t, data, []byte(expectedYAML))

		// When Info has a value
		testData.Info = "Some info"
		expectedYAML = `
info: "Some info"
`

		data, err = yamlMarshalNonNull(testData)
		if err != nil {
			t.Fatalf("yamlMarshalNonNull() error when Info has value: %v", err)
		}

		compareYAML(t, data, []byte(expectedYAML))
	})

	// Test case for invalid input
	t.Run("InvalidInput", func(t *testing.T) {
		var invalidInput func() // nil function

		_, err := yamlMarshalNonNull(invalidInput)
		if err == nil {
			t.Errorf("Expected error when marshalling invalid input, got nil")
		}
	})

	t.Run("InvalidReflectValue", func(t *testing.T) {
		// Create a nil interface, which results in a reflect.Invalid kind
		var invalidInput interface{}

		// Test with nil interface
		data, err := yamlMarshalNonNull(invalidInput)
		if err != nil {
			t.Fatalf("yamlMarshalNonNull() error: %v", err)
		}
		if len(data) != 0 {
			t.Errorf("Expected empty output for nil interface, got: '%s'", string(data))
		}
	})

	t.Run("NoYAMLTag", func(t *testing.T) {
		// Define a struct with fields that do not have YAML tags
		type TestStruct struct {
			Name  string
			Age   int
			Email string
		}

		// Create an instance of the struct
		testData := TestStruct{
			Name:  "Alice",
			Age:   30,
			Email: "alice@example.com",
		}

		// Expected YAML output should use field names as keys
		expectedYAML := `
Name: Alice
Age: 30
Email: alice@example.com
`

		// Call yamlMarshalNonNull
		data, err := yamlMarshalNonNull(testData)
		if err != nil {
			t.Fatalf("yamlMarshalNonNull() error: %v", err)
		}

		// Compare the actual YAML output with the expected output
		compareYAML(t, data, []byte(expectedYAML))
	})

	t.Run("EmptyResult", func(t *testing.T) {
		type NestedStruct struct {
			FieldA string `yaml:"field_a"`
			FieldB int    `yaml:"field_b"`
		}

		// Define a struct with all fields set to zero values or nil
		type TestStruct struct {
			Nested  *NestedStruct     `yaml:"nested"`
			Numbers []int             `yaml:"numbers"`
			MapData map[string]string `yaml:"map_data"`
		}

		// Create an instance of the struct with all fields set to zero values
		testData := TestStruct{
			Nested:  nil,
			Numbers: nil,
			MapData: map[string]string{},
		}

		// Call yamlMarshalNonNull
		data, err := yamlMarshalNonNull(testData)
		if err != nil {
			t.Fatalf("yamlMarshalNonNull() error: %v", err)
		}

		// Check that the result is an empty YAML string
		expectedYAML := ""
		if string(data) != expectedYAML {
			t.Errorf("Expected empty YAML string, got: '%s'", string(data))
		}
	})
}
