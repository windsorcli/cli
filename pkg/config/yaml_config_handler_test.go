package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/aws"
	"github.com/windsorcli/cli/api/v1alpha1/cluster"
	"github.com/windsorcli/cli/pkg/di"
)

// Mock implementation of os.FileInfo
type mockFileInfo struct{}

func TestNewYamlConfigHandler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new dependency injector
		injector := di.NewInjector()

		// When creating a new YamlConfigHandler and initializing it
		handler := NewYamlConfigHandler(injector)
		handler.Initialize()

		// Then the handler should be successfully created and not be nil
		if handler == nil {
			t.Fatal("Expected non-nil YamlConfigHandler")
		}
	})
}

func TestYamlConfigHandler_LoadConfig(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		handler, _ := setup(t)
		// Given a valid config path
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.yaml")

		// When calling LoadConfig
		err := handler.LoadConfig(configPath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("LoadConfig() unexpected error: %v", err)
		}

		// And the path should be set correctly
		if handler.path != configPath {
			t.Errorf("Expected path = %v, got = %v", configPath, handler.path)
		}
	})

	t.Run("CreateEmptyConfigFileIfNotExist", func(t *testing.T) {
		handler, _ := setup(t)

		// When mocking osStat to simulate a non-existent file
		osStat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Create the handler and run LoadConfig
		err := handler.LoadConfig("test_config.yaml")
		if err != nil {
			t.Fatalf("LoadConfig() unexpected error: %v", err)
		}
	})

	t.Run("ReadFileError", func(t *testing.T) {
		handler, _ := setup(t)

		// When mocking osReadFile to return an error
		originalOsReadFile := osReadFile
		defer func() { osReadFile = originalOsReadFile }()
		osReadFile = func(filename string) ([]byte, error) {
			return nil, fmt.Errorf("mocked error reading file")
		}

		// Create the handler and run LoadConfig
		err := handler.LoadConfig("mocked_config.yaml")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("LoadConfig() expected error, got nil")
		}

		// Then check if the error message is as expected
		expectedError := "error reading config file: mocked error reading file"
		if err.Error() != expectedError {
			t.Errorf("LoadConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("UnmarshalError", func(t *testing.T) {
		handler, _ := setup(t)
		// Given a set of safe mocks and a YamlConfigHandler
		// And a mocked yamlUnmarshal function that returns an error
		originalYamlUnmarshal := yamlUnmarshal
		defer func() { yamlUnmarshal = originalYamlUnmarshal }()
		yamlUnmarshal = func(data []byte, v interface{}) error {
			return fmt.Errorf("mocked error unmarshalling yaml")
		}

		// When LoadConfig is called with a mocked path
		err := handler.LoadConfig("mocked_path.yaml")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("LoadConfig() expected error, got nil")
		}

		// And the error message should be as expected
		expectedError := "error unmarshalling yaml: mocked error unmarshalling yaml"
		if err.Error() != expectedError {
			t.Errorf("LoadConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("UnsupportedConfigVersion", func(t *testing.T) {
		handler, _ := setup(t)
		// Given a set of safe mocks and a YamlConfigHandler
		// And a mocked yamlUnmarshal function that sets an unsupported version
		originalYamlUnmarshal := yamlUnmarshal
		defer func() { yamlUnmarshal = originalYamlUnmarshal }()
		yamlUnmarshal = func(data []byte, v interface{}) error {
			if config, ok := v.(*v1alpha1.Config); ok {
				config.Version = "unsupported_version"
			}
			return nil
		}

		// When LoadConfig is called with a mocked path
		err := handler.LoadConfig("mocked_path.yaml")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("LoadConfig() expected error, got nil")
		}

		// And the error message should be as expected
		expectedError := "unsupported config version: unsupported_version"
		if err.Error() != expectedError {
			t.Errorf("LoadConfig() error = %v, expected '%s'", err, expectedError)
		}
	})
}

func TestYamlConfigHandler_Get(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		return handler, mocks
	}

	t.Run("KeyNotUnderContexts", func(t *testing.T) {
		handler, _ := setup(t)
		// When setting the context in y.config
		handler.Set("context", "local")
		// When setting the default context (should not be used)
		defaultContext := v1alpha1.Context{
			AWS: &aws.AWSConfig{
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
		handler, _ := setup(t)
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
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		handler, _ := setup(t)
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
		if _, err := osStat(configPath); os.IsNotExist(err) {
			t.Fatalf("Config file was not created at %s", configPath)
		}
	})

	t.Run("WithEmptyPath", func(t *testing.T) {
		handler, _ := setup(t)
		handler.Set("saveKey", "saveValue")
		// Given an empty config path
		err := handler.SaveConfig("")
		// Then an error should be returned
		if err == nil {
			t.Fatalf("SaveConfig() expected error, got nil")
		}

		// Then check if the error message is as expected
		expectedError := "path cannot be empty"
		if err.Error() != expectedError {
			t.Fatalf("SaveConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("UsePredefinedPath", func(t *testing.T) {
		// Given a YamlConfigHandler with a predefined path
		handler, _ := setup(t)
		handler.path = filepath.Join(t.TempDir(), "config.yaml")

		// When SaveConfig is called with an empty path
		err := handler.SaveConfig("")

		// Then it should use the predefined path and save without error
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}

		// And the config file should exist at the specified path
		if _, err := osStat(handler.path); os.IsNotExist(err) {
			t.Fatalf("Config file was not created at %s", handler.path)
		}
	})

	t.Run("CreateDirectoriesError", func(t *testing.T) {
		handler, _ := setup(t)
		handler.path = filepath.Join(t.TempDir(), "config.yaml")

		// Mock osMkdirAll to simulate a directory creation error
		originalOsMkdirAll := osMkdirAll
		defer func() { osMkdirAll = originalOsMkdirAll }()
		osMkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mocked error creating directories")
		}

		err := handler.SaveConfig(handler.path)
		if err == nil {
			t.Fatalf("SaveConfig() expected error, got nil")
		}

		// Then check if the error message is as expected
		expectedErrorMessage := "error creating directories: mocked error creating directories"
		if err.Error() != expectedErrorMessage {
			t.Errorf("Unexpected error message. Got: %s, Expected: %s", err.Error(), expectedErrorMessage)
		}
	})

	t.Run("MarshallingError", func(t *testing.T) {
		// Mock yamlMarshal to simulate a marshalling error
		originalYamlMarshal := yamlMarshal
		defer func() { yamlMarshal = originalYamlMarshal }()
		yamlMarshal = func(v interface{}) ([]byte, error) {
			return nil, fmt.Errorf("mock marshalling error")
		}

		// Given a YamlConfigHandler with a sample config
		handler, _ := setup(t)
		handler.config = v1alpha1.Config{} // Assuming Config is your struct

		// When calling SaveConfig and expect an error
		err := handler.SaveConfig("test.yaml")
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then check if the error message is as expected
		expectedErrorMessage := "error marshalling yaml: mock marshalling error"
		if err.Error() != expectedErrorMessage {
			t.Errorf("Unexpected error message. Got: %s, Expected: %s", err.Error(), expectedErrorMessage)
		}
	})

	t.Run("WriteFileError", func(t *testing.T) {
		handler, _ := setup(t)
		handler.Set("saveKey", "saveValue")

		// When mocking osWriteFile to return an error
		originalOsWriteFile := osWriteFile
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mocked error writing file")
		}
		defer func() { osWriteFile = originalOsWriteFile }()

		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "save_config.yaml")

		err := handler.SaveConfig(configPath)
		// Then an error should be returned
		if err == nil {
			t.Fatalf("SaveConfig() expected error, got nil")
		}

		// Then check if the error message is as expected
		expectedError := "error writing config file: mocked error writing file"
		if err.Error() != expectedError {
			t.Fatalf("SaveConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("UsesExistingPath", func(t *testing.T) {
		// Given a temporary directory and expected path for the config file
		tempDir := t.TempDir()
		expectedPath := filepath.Join(tempDir, "config.yaml")

		// And a YamlConfigHandler with a path set and a key-value pair
		handler, _ := setup(t)
		handler.path = expectedPath
		handler.Set("key", "value")

		// When calling SaveConfig with an empty path
		err := handler.SaveConfig("")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("SaveConfig() unexpected error: %v", err)
		}
	})

	t.Run("OmitsNullValues", func(t *testing.T) {
		// Given a YamlConfigHandler with some initial configuration
		handler, _ := setup(t)
		handler.context = "local"
		handler.config = v1alpha1.Config{
			Contexts: map[string]*v1alpha1.Context{
				"default": {
					Environment: map[string]string{
						"name":  "John Doe",
						"email": "john.doe@example.com",
					},
					AWS: &aws.AWSConfig{
						AWSEndpointURL: nil,
					},
				},
			},
		}

		// Mock writeFile to capture the data written
		var writtenData []byte
		originalWriteFile := osWriteFile
		defer func() { osWriteFile = originalWriteFile }()
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenData = data
			return nil
		}

		// When calling SaveConfig to write the configuration
		err := handler.SaveConfig("mocked_path.yaml")
		// Then no error should be returned
		if err != nil {
			t.Fatalf("SaveConfig() unexpected error: %v", err)
		}

		// Then check that the YAML data matches the expected content
		expectedContent := "version: v1alpha1\ncontexts:\n  default:\n    environment:\n      email: john.doe@example.com\n      name: John Doe\n    aws: {}\n"
		if string(writtenData) != expectedContent {
			t.Errorf("Config file content = %v, expected %v", string(writtenData), expectedContent)
		}
	})
}

func TestYamlConfigHandler_GetString(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		return handler, mocks
	}

	t.Run("WithNonExistentKey", func(t *testing.T) {
		// Given the existing context in the configuration
		handler, _ := setup(t)
		handler.context = "default"

		// When given a non-existent key in the config
		got := handler.GetString("nonExistentKey")

		// Then an empty string should be returned
		expectedValue := ""
		if got != expectedValue {
			t.Errorf("GetString() = %v, expected %v", got, expectedValue)
		}
	})

	t.Run("GetStringWithDefaultValue", func(t *testing.T) {
		// Given the existing context in the configuration
		handler, _ := setup(t)
		handler.context = "default"

		// When calling GetString with a non-existent key and a default value
		defaultValue := "defaultString"
		value := handler.GetString("non.existent.key", defaultValue)

		// Then the default value should be returned
		if value != defaultValue {
			t.Errorf("Expected value '%v', got '%v'", defaultValue, value)
		}
	})

	t.Run("WithExistingKey", func(t *testing.T) {
		// Given the existing context in the configuration with a key-value pair
		handler, _ := setup(t)
		handler.context = "default"
		handler.config = v1alpha1.Config{
			Contexts: map[string]*v1alpha1.Context{
				"default": {
					Environment: map[string]string{
						"existingKey": "existingValue",
					},
				},
			},
		}

		// When calling GetString with an existing key
		got := handler.GetString("environment.existingKey")

		// Then the value should be returned as a string
		expectedValue := "existingValue"
		if got != expectedValue {
			t.Errorf("GetString() = %v, expected %v", got, expectedValue)
		}
	})
}

func TestYamlConfigHandler_GetInt(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		return handler, mocks
	}

	t.Run("WithExistingNonIntegerKey", func(t *testing.T) {
		// Given the existing context in the configuration with a non-integer value
		handler, _ := setup(t)
		handler.context = "default"
		handler.config = v1alpha1.Config{
			Contexts: map[string]*v1alpha1.Context{
				"default": {
					AWS: &aws.AWSConfig{
						AWSEndpointURL: ptrString("notAnInt"),
					},
				},
			},
		}

		// When calling GetInt with a key that has a non-integer value
		value := handler.GetInt("aws.aws_endpoint_url")

		// Then the default integer value should be returned
		expectedValue := 0
		if value != expectedValue {
			t.Errorf("Expected value %v, got %v", expectedValue, value)
		}
	})

	t.Run("WithNonExistentKey", func(t *testing.T) {
		// Given the existing context in the configuration without the key
		handler, _ := setup(t)
		handler.context = "default"

		// When calling GetInt with a non-existent key
		value := handler.GetInt("nonExistentKey")

		// Then the default integer value should be returned
		expectedValue := 0
		if value != expectedValue {
			t.Errorf("Expected value %v, got %v", expectedValue, value)
		}
	})

	t.Run("WithNonExistentKeyAndDefaultValue", func(t *testing.T) {
		// Given the existing context in the configuration without the key
		handler, _ := setup(t)
		handler.context = "default"

		// When calling GetInt with a non-existent key and a default value
		got := handler.GetInt("nonExistentKey", 99)

		// Then the provided default value should be returned
		expectedValue := 99
		if got != expectedValue {
			t.Errorf("GetInt() = %v, expected %v", got, expectedValue)
		}
	})

	t.Run("WithExistingIntegerKey", func(t *testing.T) {
		// Given the existing context in the configuration with an integer value
		handler, _ := setup(t)
		handler.context = "default"
		handler.config = v1alpha1.Config{
			Contexts: map[string]*v1alpha1.Context{
				"default": {
					Cluster: &cluster.ClusterConfig{
						ControlPlanes: cluster.NodeGroupConfig{
							Count: ptrInt(3),
						},
					},
				},
			},
		}

		// When calling GetInt with an existing integer key
		got := handler.GetInt("cluster.controlplanes.count")

		// Then the integer value should be returned
		expectedValue := 3
		if got != expectedValue {
			t.Errorf("GetInt() = %v, expected %v", got, expectedValue)
		}
	})
}

func TestYamlConfigHandler_GetBool(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		return handler, mocks
	}

	t.Run("WithExistingBooleanKey", func(t *testing.T) {
		// Given the existing context in the configuration with a boolean value
		handler, _ := setup(t)
		handler.context = "default"
		handler.config = v1alpha1.Config{
			Contexts: map[string]*v1alpha1.Context{
				"default": {
					AWS: &aws.AWSConfig{
						Enabled: ptrBool(true),
					},
				},
			},
		}

		// When calling GetBool with an existing boolean key
		got := handler.GetBool("aws.enabled")

		// Then the boolean value should be returned
		expectedValue := true
		if got != expectedValue {
			t.Errorf("GetBool() = %v, expected %v", got, expectedValue)
		}
	})

	t.Run("WithExistingNonBooleanKey", func(t *testing.T) {
		// Given the existing context in the configuration
		handler, _ := setup(t)
		handler.context = "default"

		// When setting a non-boolean value for the key
		handler.Set("contexts.default.aws.aws_endpoint_url", "notABool")

		// When given an existing key with a non-boolean value
		value := handler.GetBool("aws.aws_endpoint_url")
		expectedValue := false

		// Then an error should be returned indicating the value is not a boolean
		if value != expectedValue {
			t.Errorf("Expected value %v, got %v", expectedValue, value)
		}
	})

	t.Run("WithNonExistentKey", func(t *testing.T) {
		// Given the existing context in the configuration
		handler, _ := setup(t)
		handler.context = "default"

		// When given a non-existent key in the config
		value := handler.GetBool("nonExistentKey")
		expectedValue := false
		if value != expectedValue {
			t.Errorf("Expected value %v, got %v", expectedValue, value)
		}
	})

	t.Run("WithNonExistentKeyAndDefaultValue", func(t *testing.T) {
		// Given the existing context in the configuration
		handler, _ := setup(t)
		handler.context = "default"

		// When given a non-existent key in the config and a default value
		got := handler.GetBool("nonExistentKey", false)

		// Then the default value should be returned without error
		expectedValue := false
		if got != expectedValue {
			t.Errorf("GetBool() = %v, expected %v", got, expectedValue)
		}
	})
}

func TestYamlConfigHandler_GetStringSlice(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a handler with a context containing a slice value
		handler, _ := setup(t)
		handler.context = "default"
		handler.config.Contexts = map[string]*v1alpha1.Context{
			"default": {
				Cluster: &cluster.ClusterConfig{
					Workers: cluster.NodeGroupConfig{
						HostPorts: []string{"50000:50002/tcp", "30080:8080/tcp", "30443:8443/tcp"},
					},
				},
			},
		}

		// When retrieving the slice value using GetStringSlice
		value := handler.GetStringSlice("cluster.workers.hostports")

		// Then the returned slice should match the expected slice
		expectedSlice := []string{"50000:50002/tcp", "30080:8080/tcp", "30443:8443/tcp"}
		if !reflect.DeepEqual(value, expectedSlice) {
			t.Errorf("Expected GetStringSlice to return %v, got %v", expectedSlice, value)
		}
	})

	t.Run("WithNonExistentKey", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.context = "default"

		// When retrieving a non-existent key using GetStringSlice
		value := handler.GetStringSlice("nonExistentKey")

		// Then the returned value should be an empty slice
		if len(value) != 0 {
			t.Errorf("Expected GetStringSlice with non-existent key to return an empty slice, got %v", value)
		}
	})

	t.Run("WithNonExistentKeyAndDefaultValue", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.context = "default"
		defaultValue := []string{"default1", "default2"}

		// When retrieving a non-existent key with a default value
		value := handler.GetStringSlice("nonExistentKey", defaultValue)

		// Then the returned value should match the default value
		if !reflect.DeepEqual(value, defaultValue) {
			t.Errorf("Expected GetStringSlice with default to return %v, got %v", defaultValue, value)
		}
	})

	t.Run("TypeMismatch", func(t *testing.T) {
		// Given a handler where the key exists but is not a slice
		handler, _ := setup(t)
		handler.context = "default"
		handler.Set("contexts.default.cluster.workers.hostports", 123) // Set an int instead of slice

		// When retrieving the value using GetStringSlice
		value := handler.GetStringSlice("cluster.workers.hostports")

		// Then the returned slice should be empty
		if len(value) != 0 {
			t.Errorf("Expected empty slice due to type mismatch, got %v", value)
		}
	})
}

func TestYamlConfigHandler_GetStringMap(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.context = "default"
		handler.config.Contexts = map[string]*v1alpha1.Context{
			"default": {
				Environment: map[string]string{
					"KEY1": "value1",
					"KEY2": "value2",
				},
			},
		}

		// When retrieving the map value using GetStringMap
		value := handler.GetStringMap("environment")

		// Then the returned map should match the expected map
		expectedMap := map[string]string{"KEY1": "value1", "KEY2": "value2"}
		if !reflect.DeepEqual(value, expectedMap) {
			t.Errorf("Expected GetStringMap to return %v, got %v", expectedMap, value)
		}
	})

	t.Run("WithNonExistentKey", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.context = "default"

		// When retrieving a non-existent key using GetStringMap
		value := handler.GetStringMap("nonExistentKey")

		// Then the returned value should be an empty map
		if !reflect.DeepEqual(value, map[string]string{}) {
			t.Errorf("Expected GetStringMap with non-existent key to return an empty map, got %v", value)
		}
	})

	t.Run("WithNonExistentKeyAndDefaultValue", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.context = "default"
		defaultValue := map[string]string{"defaultKey1": "defaultValue1", "defaultKey2": "defaultValue2"}

		// When retrieving a non-existent key with a default value
		value := handler.GetStringMap("nonExistentKey", defaultValue)

		// Then the returned value should match the default value
		if !reflect.DeepEqual(value, defaultValue) {
			t.Errorf("Expected GetStringMap with default to return %v, got %v", defaultValue, value)
		}
	})

	t.Run("TypeMismatch", func(t *testing.T) {
		// Given a handler where the key exists but is not a map[string]string
		handler, _ := setup(t)
		handler.context = "default"
		handler.Set("contexts.default.environment", 123) // Set an int instead of map

		// When retrieving the value using GetStringMap
		value := handler.GetStringMap("environment")

		// Then the returned map should be empty
		if len(value) != 0 {
			t.Errorf("Expected empty map due to type mismatch, got %v", value)
		}
	})
}

func TestYamlConfigHandler_GetConfig(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		return handler, mocks
	}

	t.Run("ContextIsSet", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.context = "local"
		handler.config.Contexts = map[string]*v1alpha1.Context{
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

	t.Run("EmptyContextString", func(t *testing.T) {
		// Given a handler with an empty string context and a default config
		handler, _ := setup(t)
		handler.context = "" // Explicitly empty context
		defaultConf := v1alpha1.Context{Environment: map[string]string{"DEFAULT": "yes"}}
		handler.SetDefault(defaultConf)

		// When calling GetConfig
		config := handler.GetConfig()

		// Then the default config should be returned
		if config == nil {
			t.Fatalf("Expected default config, got nil")
		}
		if config.Environment["DEFAULT"] != "yes" {
			t.Errorf("Expected default config environment, got %v", config.Environment)
		}
	})

	t.Run("ContextNotInMap", func(t *testing.T) {
		// Given a handler with a context set, but it's not in the config map
		handler, _ := setup(t)
		handler.context = "missing-context"
		// Config map does *not* contain "missing-context"
		handler.config.Contexts = map[string]*v1alpha1.Context{
			"existing-context": {Environment: map[string]string{"EXISTING": "val"}},
		}
		defaultConf := v1alpha1.Context{Environment: map[string]string{"DEFAULT": "yes"}}
		handler.SetDefault(defaultConf)

		// When calling GetConfig
		config := handler.GetConfig()

		// Then the default config should be returned
		if config == nil {
			t.Fatalf("Expected default config, got nil")
		}
		if config.Environment["DEFAULT"] != "yes" {
			t.Errorf("Expected default config environment, got %v", config.Environment)
		}
		// Ensure it didn't accidentally return the existing context's data
		if _, exists := config.Environment["EXISTING"]; exists {
			t.Errorf("Returned config contains data from an unrelated existing context")
		}
	})
}

func TestYamlConfigHandler_Set(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		handler, _ := setup(t)

		// When setting a value in the configuration
		handler.Set("contexts.default.environment.TEST_VAR", "test_value")

		// Then the value should be correctly set
		expected := "test_value"
		if val := handler.config.Contexts["default"].Environment["TEST_VAR"]; val != expected {
			t.Errorf("Set() did not correctly set value, expected %s, got %s", expected, val)
		}
	})

	t.Run("InvalidPath", func(t *testing.T) {
		handler, _ := setup(t)

		// When attempting to set a value with an empty path
		err := handler.Set("", "test_value")

		// In the current implementation, Set() returns nil for empty path
		// This should be updated in the future to return an error like SetContextValue
		if err != nil {
			t.Errorf("Set() with empty path did not behave as expected, got error: %v", err)
		}
	})
}

func TestYamlConfigHandler_SetContextValue(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		handler, _ := setup(t)
		handler.context = "test" // Set to "test" since GetContext returns "test"

		// Log what GetContext returns
		actualContext := handler.GetContext()
		t.Logf("GetContext() returned: %s", actualContext)

		// Create context with Environment map
		handler.config.Contexts = map[string]*v1alpha1.Context{
			actualContext: {
				Environment: map[string]string{},
			},
		}

		// When setting a value in the context environment
		err := handler.SetContextValue("environment.TEST_VAR", "test_value")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("SetContextValue() unexpected error: %v", err)
		}

		// And the value should be correctly set in the actual context
		expected := "test_value"
		if val := handler.config.Contexts[actualContext].Environment["TEST_VAR"]; val != expected {
			t.Errorf("SetContextValue() did not correctly set value, expected %s, got %s", expected, val)
		}
	})

	t.Run("EmptyPath", func(t *testing.T) {
		handler, _ := setup(t)

		// When attempting to set a value with an empty path
		err := handler.SetContextValue("", "test_value")

		// Then an error should be returned
		if err == nil {
			t.Errorf("SetContextValue() with empty path did not return an error")
		}
		expectedErr := "path cannot be empty"
		if err.Error() != expectedErr {
			t.Errorf("Expected error message '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("SetFails", func(t *testing.T) {
		handler, _ := setup(t)
		handler.context = "test"

		// When attempting to set a value with a path that will cause an error
		err := handler.SetContextValue("invalid..path", "test_value") // Double period will cause empty path component

		// Then an error should be returned
		if err == nil {
			t.Errorf("SetContextValue() with invalid path did not return an error")
		}
	})
}

func TestYamlConfigHandler_SetDefault(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		return handler, mocks
	}

	t.Run("SetDefaultWithExistingContext", func(t *testing.T) {
		handler, _ := setup(t)
		defaultContext := v1alpha1.Context{
			Environment: map[string]string{
				"ENV_VAR": "value",
			},
		}

		handler.Set("context", "local")
		err := handler.SetDefault(defaultContext)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if handler.defaultContextConfig.Environment["ENV_VAR"] != "value" {
			t.Errorf("SetDefault() = %v, expected %v", handler.defaultContextConfig.Environment["ENV_VAR"], "value")
		}
	})

	t.Run("SetDefaultWithNoContext", func(t *testing.T) {
		handler, _ := setup(t)
		handler.context = ""
		defaultContext := v1alpha1.Context{
			Environment: map[string]string{
				"ENV_VAR": "value",
			},
		}

		err := handler.SetDefault(defaultContext)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if handler.defaultContextConfig.Environment["ENV_VAR"] != "value" {
			t.Errorf("SetDefault() = %v, expected %v", handler.defaultContextConfig.Environment["ENV_VAR"], "value")
		}
	})

	t.Run("SetDefaultUsedInSubsequentOperations", func(t *testing.T) {
		handler, _ := setup(t)
		handler.context = "existing-context"
		handler.config.Contexts = map[string]*v1alpha1.Context{
			"existing-context": {ProjectName: ptrString("initial-project")},
		}

		defaultConf := v1alpha1.Context{
			Environment: map[string]string{"DEFAULT_VAR": "default_val"},
		}

		err := handler.SetDefault(defaultConf)
		if err != nil {
			t.Fatalf("SetDefault() unexpected error: %v", err)
		}

		if handler.defaultContextConfig.Environment == nil || handler.defaultContextConfig.Environment["DEFAULT_VAR"] != "default_val" {
			t.Errorf("Expected defaultContextConfig environment to be %v, got %v", defaultConf.Environment, handler.defaultContextConfig.Environment)
		}

		if handler.config.Contexts["existing-context"] == nil ||
			handler.config.Contexts["existing-context"].ProjectName == nil ||
			*handler.config.Contexts["existing-context"].ProjectName != "initial-project" {
			t.Errorf("SetDefault incorrectly overwrote existing context config. Expected ProjectName 'initial-project', got %v", handler.config.Contexts["existing-context"].ProjectName)
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
		config := v1alpha1.Config{
			Contexts: map[string]*v1alpha1.Context{
				"level1": {
					Environment: map[string]string{
						"level2": "value2",
					},
					AWS: &aws.AWSConfig{
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

		// When ensuring that v is addressable
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
		// Given an empty pathKeys slice for value lookup
		var current interface{}
		pathKeys := []string{}

		// When calling getValueByPath with empty pathKeys
		value := getValueByPath(current, pathKeys)

		// Then nil should be returned as the path is invalid
		if value != nil {
			t.Errorf("Expected value to be nil, got %v", value)
		}
	})

	t.Run("InvalidCurrentValue", func(t *testing.T) {
		// Given a nil current value and a valid path key
		var current interface{} = nil
		pathKeys := []string{"key"}

		// When calling getValueByPath with nil current value
		value := getValueByPath(current, pathKeys)

		// Then nil should be returned as the current value is invalid
		if value != nil {
			t.Errorf("Expected value to be nil, got %v", value)
		}
	})

	t.Run("MapKeyTypeMismatch", func(t *testing.T) {
		// Given a map with int keys but attempting to access with a string key
		current := map[int]string{1: "one", 2: "two"}
		pathKeys := []string{"1"}

		// When calling getValueByPath with mismatched key type
		value := getValueByPath(current, pathKeys)

		// Then nil should be returned due to key type mismatch
		if value != nil {
			t.Errorf("Expected value to be nil, got %v", value)
		}
	})

	t.Run("MapSuccess", func(t *testing.T) {
		// Given a map with a string key and corresponding value
		current := map[string]string{"key": "testValue"}
		pathKeys := []string{"key"}

		// When calling getValueByPath with a valid key
		value := getValueByPath(current, pathKeys)

		// Then the corresponding value should be returned successfully
		if value == nil {
			t.Errorf("Expected value to be 'testValue', got nil")
		}
		expectedValue := "testValue"
		if value != expectedValue {
			t.Errorf("Expected value '%s', got '%v'", expectedValue, value)
		}
	})

	t.Run("CannotSetField", func(t *testing.T) {
		// Given a struct with an unexported field that cannot be set
		type TestStruct struct {
			unexportedField string `yaml:"unexportedfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"unexportedfield"}
		value := "testValue"
		fullPath := "unexportedfield"

		// When attempting to set a value on the unexported field
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned indicating the field cannot be set
		expectedErr := "cannot set field"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%v'", expectedErr, err)
		}
	})

	t.Run("RecursiveFailure", func(t *testing.T) {
		// Given a nested map structure without the target field
		level3Map := map[string]interface{}{}
		level2Map := map[string]interface{}{"level3": level3Map}
		level1Map := map[string]interface{}{"level2": level2Map}
		testMap := map[string]interface{}{"level1": level1Map}
		currValue := reflect.ValueOf(testMap)
		pathKeys := []string{"level1", "level2", "nonexistentfield"}
		value := "newValue"
		fullPath := "level1.level2.nonexistentfield"

		// When attempting to set a value at a non-existent nested path
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned indicating the invalid path
		expectedErr := "Invalid path: level1.level2.nonexistentfield"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%v'", expectedErr, err)
		}
	})

	t.Run("AssignValueTypeMismatch", func(t *testing.T) {
		// Given a struct with an int field that cannot accept a string slice
		type TestStruct struct {
			IntField int `yaml:"intfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"intfield"}
		value := []string{"incompatibleType"} // A slice, which is incompatible with int
		fullPath := "intfield"

		// When attempting to assign an incompatible value type
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned indicating the type mismatch
		expectedErr := "cannot assign value of type []string to field of type int"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%v'", expectedErr, err)
		}
	})

	t.Run("AssignPointerValueTypeMismatch", func(t *testing.T) {
		// Given a struct with a pointer field that cannot accept a string slice
		type TestStruct struct {
			IntPtrField *int `yaml:"intptrfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"intptrfield"}
		value := []string{"incompatibleType"} // A slice, which is incompatible with *int
		fullPath := "intptrfield"

		// When attempting to assign an incompatible value type to a pointer field
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned indicating the pointer type mismatch
		expectedErr := "cannot assign value of type []string to field of type *int"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%v'", expectedErr, err)
		}
	})

	t.Run("AssignNonPointerField", func(t *testing.T) {
		// Given a struct with a string field that can be directly assigned
		type TestStruct struct {
			StringField string `yaml:"stringfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"stringfield"}
		value := "testValue" // Directly assignable to string
		fullPath := "stringfield"

		// When assigning a compatible value to the field
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
		// Given a struct with an int field that can accept a convertible float value
		type TestStruct struct {
			IntField int `yaml:"intfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"intfield"}
		value := 42.0 // A float64, which is convertible to int
		fullPath := "intfield"

		// When assigning a value that can be converted to the field's type
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

func TestParsePath(t *testing.T) {
	t.Run("EmptyPath", func(t *testing.T) {
		// Given an empty path string to parse
		path := ""

		// When calling parsePath with the empty string
		pathKeys := parsePath(path)

		// Then an empty slice should be returned
		if len(pathKeys) != 0 {
			t.Errorf("Expected pathKeys to be empty, got %v", pathKeys)
		}
	})

	t.Run("SingleKey", func(t *testing.T) {
		// Given a path with a single key
		path := "key"

		// When calling parsePath with a single key
		pathKeys := parsePath(path)

		// Then a slice with only that key should be returned
		expected := []string{"key"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})

	t.Run("MultipleKeys", func(t *testing.T) {
		// Given a path with multiple keys separated by dots
		path := "key1.key2.key3"

		// When calling parsePath with dot notation
		pathKeys := parsePath(path)

		// Then a slice containing all the keys should be returned
		expected := []string{"key1", "key2", "key3"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})

	t.Run("KeysWithBrackets", func(t *testing.T) {
		// Given a path with keys using bracket notation
		path := "key1[key2][key3]"

		// When calling parsePath with bracket notation
		pathKeys := parsePath(path)

		// Then a slice containing all the keys without brackets should be returned
		expected := []string{"key1", "key2", "key3"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})

	t.Run("MixedDotAndBracketNotation", func(t *testing.T) {
		// Given a path with mixed dot and bracket notation
		path := "key1.key2[key3].key4[key5]"

		// When calling parsePath with mixed notation
		pathKeys := parsePath(path)

		// Then a slice with all keys regardless of notation should be returned
		expected := []string{"key1", "key2", "key3", "key4", "key5"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})

	t.Run("DotInsideBrackets", func(t *testing.T) {
		// Given a path with a dot inside bracket notation
		path := "key1[key2.key3]"

		// When calling parsePath with a dot inside brackets
		pathKeys := parsePath(path)

		// Then the dot inside brackets should be treated as part of the key
		expected := []string{"key1", "key2.key3"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})

	t.Run("PathStartingWithDot", func(t *testing.T) {
		// Given a path starting with a dot
		path := ".key1.key2"

		// When calling parsePath with a leading dot
		pathKeys := parsePath(path)

		// Then the leading dot should be ignored in the result
		expected := []string{"key1", "key2"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys %v, got %v", expected, pathKeys)
		}
	})

	t.Run("PathEndingWithDot", func(t *testing.T) {
		// Given a path ending with a dot
		path := "key1.key2."

		// When calling parsePath with a trailing dot
		pathKeys := parsePath(path)

		// Then the trailing dot should be ignored in the result
		expected := []string{"key1", "key2"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys %v, got %v", expected, pathKeys)
		}
	})

	t.Run("PathStartingWithBracket", func(t *testing.T) {
		// Given a path starting with a bracket
		path := "[key1].key2"

		// When calling parsePath with a leading bracket
		pathKeys := parsePath(path)

		// Then the keys should be correctly extracted without the brackets
		expected := []string{"key1", "key2"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys %v, got %v", expected, pathKeys)
		}
	})

	t.Run("PathEndingWithUnmatchedBracket", func(t *testing.T) {
		// Given a path ending with an unmatched opening bracket
		path := "key1[key2"

		// When calling parsePath with an unmatched bracket
		pathKeys := parsePath(path)

		// Then the parser should handle this gracefully and extract the keys
		expected := []string{"key1", "key2"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys %v, got %v", expected, pathKeys)
		}
	})

	t.Run("MultipleConsecutiveDots", func(t *testing.T) {
		// Given a path with multiple consecutive dots
		path := "key1..key2"

		// When calling parsePath with consecutive dots
		pathKeys := parsePath(path)

		// Then consecutive dots should be treated as a single delimiter
		expected := []string{"key1", "key2"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys %v, got %v", expected, pathKeys)
		}
	})
}

func TestAssignValue(t *testing.T) {
	t.Run("CannotSetField", func(t *testing.T) {
		// Given an unexported field that cannot be set
		var unexportedField struct {
			unexported int
		}
		fieldValue := reflect.ValueOf(&unexportedField).Elem().Field(0)

		// When attempting to assign a value to it
		_, err := assignValue(fieldValue, 10)

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected an error for non-settable field, got nil")
		}
		expectedError := "cannot set field"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("PointerTypeMismatchNonConvertible", func(t *testing.T) {
		// Given a pointer field of type *int
		var field *int
		fieldValue := reflect.ValueOf(&field).Elem()

		// When attempting to assign a string value to it
		value := "not an int"
		_, err := assignValue(fieldValue, value)

		// Then an error should be returned indicating type mismatch
		if err == nil {
			t.Errorf("Expected an error for pointer type mismatch, got nil")
		}
		expectedError := "cannot assign value of type string to field of type *int"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ValueTypeMismatchNonConvertible", func(t *testing.T) {
		// Given a field of type int
		var field int
		fieldValue := reflect.ValueOf(&field).Elem()

		// When attempting to assign a non-convertible string value to it
		value := "not convertible to int"
		_, err := assignValue(fieldValue, value)

		// Then an error should be returned indicating type mismatch
		if err == nil {
			t.Errorf("Expected an error for non-convertible type mismatch, got nil")
		}
		expectedError := "cannot assign value of type string to field of type int"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})
}

func TestYamlConfigHandler_LoadConfigString(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		handler, _ := setup(t)
		handler.SetContext("test")

		yamlContent := `
version: v1alpha1
contexts:
  test:
    environment:
      TEST_VAR: test_value`

		err := handler.LoadConfigString(yamlContent)
		if err != nil {
			t.Fatalf("LoadConfigString() unexpected error: %v", err)
		}

		value := handler.GetString("environment.TEST_VAR")
		if value != "test_value" {
			t.Errorf("Expected TEST_VAR = 'test_value', got '%s'", value)
		}
	})

	t.Run("EmptyContent", func(t *testing.T) {
		handler, _ := setup(t)
		err := handler.LoadConfigString("")
		if err != nil {
			t.Fatalf("LoadConfigString() unexpected error: %v", err)
		}
	})

	t.Run("InvalidYAML", func(t *testing.T) {
		handler, _ := setup(t)
		yamlContent := `invalid: yaml: content: [}`

		err := handler.LoadConfigString(yamlContent)
		if err == nil {
			t.Fatal("LoadConfigString() expected error for invalid YAML")
		}
		if !strings.Contains(err.Error(), "error unmarshalling yaml") {
			t.Errorf("Expected error about invalid YAML, got: %v", err)
		}
	})

	t.Run("UnsupportedVersion", func(t *testing.T) {
		handler, _ := setup(t)
		yamlContent := `
version: v2alpha1
contexts:
  test: {}`

		err := handler.LoadConfigString(yamlContent)
		if err == nil {
			t.Fatal("LoadConfigString() expected error for unsupported version")
		}
		if !strings.Contains(err.Error(), "unsupported config version") {
			t.Errorf("Expected error about unsupported version, got: %v", err)
		}
	})
}
