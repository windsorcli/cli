package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

		// Then the handler should not be nil
		if handler == nil {
			t.Fatal("Expected non-nil YamlConfigHandler")
		}
	})
}

func TestYamlConfigHandler_LoadConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()

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
		mocks := setupSafeMocks()

		// When mocking osStat to simulate a non-existent file
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Create the handler and run LoadConfig
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		err := handler.LoadConfig("test_config.yaml")
		if err != nil {
			t.Fatalf("LoadConfig() unexpected error: %v", err)
		}
	})

	t.Run("ReadFileError", func(t *testing.T) {
		mocks := setupSafeMocks()

		// When mocking osReadFile to return an error
		originalOsReadFile := osReadFile
		defer func() { osReadFile = originalOsReadFile }()
		osReadFile = func(filename string) ([]byte, error) {
			return nil, fmt.Errorf("mocked error reading file")
		}

		// Create the handler and run LoadConfig
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		configPath := "mocked_config.yaml"
		err := handler.LoadConfig(configPath)

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
		// Given a set of safe mocks and a YamlConfigHandler
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()

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
		// Given a set of safe mocks and a YamlConfigHandler
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()

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
	t.Run("KeyNotUnderContexts", func(t *testing.T) {
		// Given a handler with key not under 'contexts'
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
		// Given a handler
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()

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
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
	t.Run("WithNonExistentKey", func(t *testing.T) {
		// Given the existing context in the configuration
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
	t.Run("WithExistingNonIntegerKey", func(t *testing.T) {
		// Given the existing context in the configuration with a non-integer value
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		handler.context = "default"
		handler.config = v1alpha1.Config{
			Contexts: map[string]*v1alpha1.Context{
				"default": {
					Cluster: &cluster.ClusterConfig{
						ControlPlanes: struct {
							Count     *int                          `yaml:"count,omitempty"`
							CPU       *int                          `yaml:"cpu,omitempty"`
							Memory    *int                          `yaml:"memory,omitempty"`
							Nodes     map[string]cluster.NodeConfig `yaml:"nodes,omitempty"`
							HostPorts []string                      `yaml:"hostports,omitempty"`
						}{
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
	t.Run("WithExistingBooleanKey", func(t *testing.T) {
		// Given the existing context in the configuration with a boolean value
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
	t.Run("Success", func(t *testing.T) {
		// Given a handler with a context containing a slice value
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		handler.context = "default"
		handler.config.Contexts = map[string]*v1alpha1.Context{
			"default": {
				Cluster: &cluster.ClusterConfig{
					Workers: struct {
						Count     *int                          `yaml:"count,omitempty"`
						CPU       *int                          `yaml:"cpu,omitempty"`
						Memory    *int                          `yaml:"memory,omitempty"`
						Nodes     map[string]cluster.NodeConfig `yaml:"nodes,omitempty"`
						HostPorts []string                      `yaml:"hostports,omitempty"`
					}{
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		handler.context = "default"
		defaultValue := []string{"default1", "default2"}

		// When retrieving a non-existent key with a default value
		value := handler.GetStringSlice("nonExistentKey", defaultValue)

		// Then the returned value should match the default value
		if !reflect.DeepEqual(value, defaultValue) {
			t.Errorf("Expected GetStringSlice with default to return %v, got %v", defaultValue, value)
		}
	})
}

func TestYamlConfigHandler_GetConfig(t *testing.T) {
	t.Run("ContextIsSet", func(t *testing.T) {
		// Given a handler with a context set
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
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
}

func TestYamlConfigHandler_Set(t *testing.T) {
	t.Run("SetWithInvalidPath", func(t *testing.T) {
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()

		// When attempting to set a value with an empty path
		handler.Set("", "someValue")

		// Then check if the error is as expected
		if handler.Get("") != nil {
			t.Fatalf("Set() expected error, got nil")
		}
	})
}

func TestYamlConfigHandler_SetContextValue(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a handler with a valid context set
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		handler.context = "local"

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

	// t.Run("ContextNotSet", func(t *testing.T) {
	// 	// Given a handler without a context set
	// 	mocks := setupSafeMocks()
	// 	handler := NewYamlConfigHandler(mocks.Injector)
	// 	handler.Initialize()

	// 	// When calling SetContextValue
	// 	err := handler.SetContextValue("some.path", "someValue")

	// 	// Then an error should be returned
	// 	expectedError := "current context is not set"
	// 	if err == nil || err.Error() != expectedError {
	// 		t.Errorf("Expected error '%s', got %v", expectedError, err)
	// 	}
	// })

	t.Run("InvalidPath", func(t *testing.T) {
		// Given a handler with a valid context set
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		handler.context = "local"

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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		defaultContext := v1alpha1.Context{
			Environment: map[string]string{
				"ENV_VAR": "value",
			},
		}

		// When setting the context in y.config
		handler.Set("context", "local")

		// When setting the default context
		err := handler.SetDefault(defaultContext)
		// Then no error should be returned
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()
		handler.context = ""

		// When calling SetDefault
		defaultContext := v1alpha1.Context{
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
		mocks := setupSafeMocks()
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.Initialize()

		// When calling SetDefault
		defaultContext := v1alpha1.Context{
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

func TestParsePath(t *testing.T) {
	t.Run("EmptyPath", func(t *testing.T) {
		// Given an empty path
		path := ""

		// When calling parsePath
		pathKeys := parsePath(path)

		// Then the pathKeys should be an empty slice
		if len(pathKeys) != 0 {
			t.Errorf("Expected pathKeys to be empty, got %v", pathKeys)
		}
	})

	t.Run("SingleKey", func(t *testing.T) {
		// Given a path with a single key
		path := "key"

		// When calling parsePath
		pathKeys := parsePath(path)

		// Then the pathKeys should contain one element
		expected := []string{"key"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})

	t.Run("MultipleKeys", func(t *testing.T) {
		// Given a path with multiple keys
		path := "key1.key2.key3"

		// When calling parsePath
		pathKeys := parsePath(path)

		// Then the pathKeys should contain all keys
		expected := []string{"key1", "key2", "key3"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})

	t.Run("KeysWithBrackets", func(t *testing.T) {
		// Given a path with keys using bracket notation
		path := "key1[key2][key3]"

		// When calling parsePath
		pathKeys := parsePath(path)

		// Then the pathKeys should contain all keys
		expected := []string{"key1", "key2", "key3"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})

	t.Run("MixedDotAndBracketNotation", func(t *testing.T) {
		// Given a path with mixed dot and bracket notation
		path := "key1.key2[key3].key4[key5]"

		// When calling parsePath
		pathKeys := parsePath(path)

		// Then the pathKeys should contain all keys
		expected := []string{"key1", "key2", "key3", "key4", "key5"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})

	t.Run("DotInsideBrackets", func(t *testing.T) {
		// Given a path with a dot inside brackets
		path := "key1[key2.key3]"

		// When calling parsePath
		pathKeys := parsePath(path)

		// Then the pathKeys should treat the dot inside brackets as part of the key
		expected := []string{"key1", "key2.key3"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})
}
